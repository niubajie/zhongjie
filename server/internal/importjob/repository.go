package importjob

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

func (r *Repository) CreateJob(ctx context.Context, landlordID int64, fileName, fileType string) (int64, string, error) {
	var id int64
	var uuid string
	err := r.pool.QueryRow(ctx, `
		INSERT INTO import_jobs (landlord_id, file_name, file_type, status)
		VALUES ($1, $2, $3, 'processing')
		RETURNING id, uuid::text
	`, landlordID, fileName, fileType).Scan(&id, &uuid)
	if err != nil {
		return 0, "", fmt.Errorf("create import job: %w", err)
	}
	return id, uuid, nil
}

func (r *Repository) AddDraft(ctx context.Context, jobID int64, rowNo int, payload any, status, message, propertyUUID string) error {
	raw, _ := json.Marshal(payload)
	_, err := r.pool.Exec(ctx, `
		INSERT INTO import_drafts (job_id, row_no, payload_json, status, error_message, property_uuid)
		VALUES ($1, $2, $3::jsonb, $4, $5, NULLIF($6, '')::uuid)
	`, jobID, rowNo, string(raw), status, message, propertyUUID)
	if err != nil {
		return fmt.Errorf("add import draft: %w", err)
	}
	return nil
}

func (r *Repository) FinishJob(ctx context.Context, jobID int64, total, success, failed int, errorMessage string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE import_jobs
		SET status = 'completed', total_rows = $2, success_rows = $3, error_rows = $4,
		    error_message = $5, completed_at = now()
		WHERE id = $1
	`, jobID, total, success, failed, errorMessage)
	if err != nil {
		return fmt.Errorf("finish import job: %w", err)
	}
	return nil
}
