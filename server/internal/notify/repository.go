package notify

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Record struct {
	ID          int64
	RecipientID int64
	PayloadEnc  []byte
}

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) Due(ctx context.Context, limit int) ([]Record, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, recipient_id, payload_enc
		FROM notification_outbox
		WHERE status = 'pending' AND next_retry_at <= now()
		ORDER BY id
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("query notification outbox: %w", err)
	}
	defer rows.Close()
	out := []Record{}
	for rows.Next() {
		var rec Record
		if err := rows.Scan(&rec.ID, &rec.RecipientID, &rec.PayloadEnc); err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

// EnqueueReminder 写入一条获客跟进提醒的外发记录（微信模板由 Process 统一发送）。
func (r *Repository) EnqueueReminder(ctx context.Context, recipientID int64, payloadEnc []byte) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO notification_outbox (event_type, recipient_id, payload_enc, status)
		VALUES ('prospect_followup_reminder', $1, $2, 'pending')
	`, recipientID, payloadEnc)
	if err != nil {
		return fmt.Errorf("enqueue reminder notification: %w", err)
	}
	return nil
}

func (r *Repository) OpenID(ctx context.Context, userID int64) (string, error) {
	var openid string
	err := r.pool.QueryRow(ctx, `
		SELECT COALESCE(openid, '') FROM users WHERE id = $1 AND deleted_at IS NULL
	`, userID).Scan(&openid)
	if err == pgx.ErrNoRows {
		return "", nil
	}
	return openid, err
}

func (r *Repository) Mark(ctx context.Context, id int64, status, message string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE notification_outbox
		SET status = $2, attempts = attempts + 1,
		    last_error = $3, updated_at = now(),
		    next_retry_at = CASE WHEN $2 = 'failed' THEN now() + interval '5 minutes' ELSE next_retry_at END
		WHERE id = $1
	`, id, status, message)
	if err != nil {
		return fmt.Errorf("mark notification: %w", err)
	}
	return nil
}

func (r *Repository) PayloadTitle(rec Record) string {
	var payload struct {
		Title string `json:"title"`
	}
	_ = json.Unmarshal(rec.PayloadEnc, &payload)
	return payload.Title
}
