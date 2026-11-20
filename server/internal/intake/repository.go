package intake

import (
	"context"
	"encoding/json"
	"fmt"

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
