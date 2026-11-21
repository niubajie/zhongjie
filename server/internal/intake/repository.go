package intake

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) CreateJob(ctx context.Context, adminID int64, rawTextEnc []byte, extracted any) (int64, string, error) {
	raw, _ := json.Marshal(extracted)
	var id int64
	var uuid string
	err := r.pool.QueryRow(ctx, `
		INSERT INTO admin_intake_jobs (admin_id, input_type, raw_text_enc, extracted_json, status)
		VALUES ($1, 'text', $2, $3::jsonb, 'pending')
		RETURNING id, uuid::text
	`, adminID, rawTextEnc, string(raw)).Scan(&id, &uuid)
	if err != nil {
		return 0, "", fmt.Errorf("create intake job: %w", err)
	}
	return id, uuid, nil
}

func (r *Repository) CreateMediaJob(ctx context.Context, adminID int64, inputType string, mediaKeys []string, ocrEnc, transcriptEnc []byte, extracted any, status, errorCode, matchStatus string, landlordID int64, confidence float64) (int64, string, error) {
	raw, _ := json.Marshal(extracted)
	var id int64
	var uuid string
	err := r.pool.QueryRow(ctx, `
		INSERT INTO admin_intake_jobs (
			admin_id, input_type, media_keys, ocr_text_enc, transcript_enc,
			extracted_json, status, error_code, match_status, landlord_id, match_confidence
		)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, NULLIF($8, ''), $9, NULLIF($10, 0), $11)
		RETURNING id, uuid::text
	`, adminID, inputType, mediaKeys, ocrEnc, transcriptEnc, string(raw),
		status, errorCode, matchStatus, landlordID, confidence).Scan(&id, &uuid)
	if err != nil {
		return 0, "", fmt.Errorf("create media intake job: %w", err)
	}
	return id, uuid, nil
}

func (r *Repository) GetJob(ctx context.Context, uuid string) (Job, error) {
	var job Job
	var extracted []byte
	err := r.pool.QueryRow(ctx, `
		SELECT j.id, j.uuid::text, j.input_type, j.status, j.match_status,
		       j.extracted_json, j.media_keys, j.ocr_text_enc, j.transcript_enc,
		       COALESCE(j.landlord_id, 0), COALESCE(u.uuid::text, ''),
		       COALESCE(j.property_uuid::text, ''), j.match_confidence, COALESCE(j.error_message, '')
		FROM admin_intake_jobs j
		LEFT JOIN users u ON u.id = j.landlord_id
		WHERE j.uuid = $1
	`, uuid).Scan(&job.ID, &job.UUID, &job.InputType, &job.Status, &job.MatchStatus,
		&extracted, &job.MediaKeys, &job.OCRText, &job.Transcript,
		&job.LandlordID, &job.LandlordUUID, &job.PropertyUUID, &job.Confidence, &job.ErrorMessage)
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, ErrJobNotFound
	}
	if err != nil {
		return Job{}, fmt.Errorf("get intake job: %w", err)
	}
	_ = json.Unmarshal(extracted, &job.Extracted)
	return job, nil
}

func (r *Repository) UpdateMatch(ctx context.Context, uuid string, landlordID int64, status string, confidence float64) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE admin_intake_jobs
		SET landlord_id = NULLIF($2, 0), match_status = $3, match_confidence = $4, updated_at = now()
		WHERE uuid = $1 AND status IN ('ready', 'failed', 'not_configured')
	`, uuid, landlordID, status, confidence)
	if err != nil {
		return fmt.Errorf("update intake match: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrJobNotEditable
	}
	return nil
}

// UpdateRecognition writes the result of a retried OCR/ASR run.
func (r *Repository) UpdateRecognition(ctx context.Context, uuid string, ocrEnc, transcriptEnc []byte, extracted any, status, errorCode, matchStatus string, landlordID int64, confidence float64) error {
	raw, _ := json.Marshal(extracted)
	tag, err := r.pool.Exec(ctx, `
		UPDATE admin_intake_jobs
		SET ocr_text_enc = $2, transcript_enc = $3, extracted_json = $4::jsonb,
		    status = $5, error_code = NULLIF($6, ''), match_status = $7,
		    landlord_id = NULLIF($8, 0), match_confidence = $9,
		    error_message = NULL, updated_at = now()
		WHERE uuid = $1 AND status IN ('failed', 'not_configured')
	`, uuid, ocrEnc, transcriptEnc, string(raw), status, errorCode, matchStatus, landlordID, confidence)
	if err != nil {
		return fmt.Errorf("update intake recognition: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrJobNotEditable
	}
	return nil
}

// ListJobs returns recent intake drafts for the admin console.
func (r *Repository) ListJobs(ctx context.Context, status string, limit int) ([]JobSummary, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT j.uuid::text, j.input_type, j.status, j.match_status,
		       COALESCE(j.extracted_json->>'communityName', ''),
		       COALESCE(u.uuid::text, ''), COALESCE(j.property_uuid::text, ''),
		       j.match_confidence, COALESCE(j.error_message, ''),
		       COALESCE(NULLIF(j.extracted_json->>'areaSqm', '')::float8, 0),
		       COALESCE(NULLIF(j.extracted_json->>'rentYuan', '')::float8, 0),
		       COALESCE(j.extracted_json->>'title', ''),
		       to_char(j.created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM admin_intake_jobs j
		LEFT JOIN users u ON u.id = j.landlord_id
		WHERE ($1 = '' OR j.status = $1)
		ORDER BY j.created_at DESC
		LIMIT $2
	`, status, limit)
	if err != nil {
		return nil, fmt.Errorf("list intake jobs: %w", err)
	}
	defer rows.Close()
	out := []JobSummary{}
	for rows.Next() {
		var row JobSummary
		if err := rows.Scan(&row.UUID, &row.InputType, &row.Status, &row.MatchStatus,
			&row.CommunityName, &row.LandlordUUID, &row.PropertyUUID,
			&row.Confidence, &row.ErrorMessage, &row.AreaSqm, &row.RentYuan,
			&row.Title, &row.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *Repository) FinishMedia(ctx context.Context, uuid, status, propertyUUID string) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE admin_intake_jobs
		SET status = $2, property_uuid = NULLIF($3, '')::uuid, completed_at = now(), updated_at = now()
		WHERE uuid = $1 AND status = 'ready'
	`, uuid, status, propertyUUID)
	if err != nil {
		return fmt.Errorf("finish media intake: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrJobNotEditable
	}
	return nil
}

func (r *Repository) Audit(ctx context.Context, adminID int64, action, targetUUID string) {
	_, _ = r.pool.Exec(ctx, `
		INSERT INTO audit_logs (actor_type, actor_id, action, target_type, target_id)
		VALUES ('admin', $1, $2, 'intake_job', $3)
	`, adminID, action, targetUUID)
}

func (r *Repository) Finish(ctx context.Context, jobID int64, status, matchStatus string, landlordID int64, propertyUUID, message string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE admin_intake_jobs
		SET status = $2,
		    match_status = $3,
		    landlord_id = NULLIF($4, 0),
		    property_uuid = NULLIF($5, '')::uuid,
		    error_message = $6,
		    completed_at = now()
		WHERE id = $1
	`, jobID, status, matchStatus, landlordID, propertyUUID, message)
	if err != nil {
		return fmt.Errorf("finish intake job: %w", err)
	}
	return nil
}
