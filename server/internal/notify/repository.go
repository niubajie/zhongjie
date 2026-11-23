package notify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrOutboxNotFound     = errors.New("外发记录不存在")
	ErrOutboxNotRetryable = errors.New("只有失败或未配置的记录可以重试")
	ErrInvalidOutboxQuery = errors.New("外发记录查询参数不合法")
)

// 外发记录状态（与 notification_outbox.status 的 CHECK 约束一致）。
const (
	StatusPending       = "pending"
	StatusRetrying      = "retrying"
	StatusSent          = "sent"
	StatusFailed        = "failed"
	StatusNotConfigured = "not_configured"
)

var validOutboxStatuses = map[string]bool{
	StatusPending:       true,
	StatusRetrying:      true,
	StatusSent:          true,
	StatusFailed:        true,
	StatusNotConfigured: true,
}

type Record struct {
	ID          int64
	EventType   string
	RecipientID int64
	PayloadEnc  []byte
}

// OutboxRecord 是管理站可见的外发记录视图（不含密文载荷）。
type OutboxRecord struct {
	ID          int64
	EventType   string
	RecipientID int64
	Status      string
	Attempts    int
	LastError   string
	NextRetryAt string
	SentAt      string
	CreatedAt   string
	UpdatedAt   string
}

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

const outboxTS = `to_char(%s AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')`

func (r *Repository) Due(ctx context.Context, limit int) ([]Record, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, event_type, recipient_id, payload_enc
		FROM notification_outbox
		WHERE status IN ('pending', 'retrying') AND next_retry_at <= now()
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
		if err := rows.Scan(&rec.ID, &rec.EventType, &rec.RecipientID, &rec.PayloadEnc); err != nil {
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

// MarkSent 记录发送成功：状态置 sent 并写入发送时间。
func (r *Repository) MarkSent(ctx context.Context, id int64) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE notification_outbox
		SET status = 'sent', attempts = attempts + 1,
		    last_error = NULL, sent_at = now(), updated_at = now()
		WHERE id = $1
	`, id)
	if err != nil {
		return fmt.Errorf("mark notification sent: %w", err)
	}
	return nil
}

// MarkRetry 记录一次失败：未达上限进入 retrying 并按 5/10/20/40/60 分钟退避，
// 达到上限转 failed 停止自动重试。返回更新后的状态。
func (r *Repository) MarkRetry(ctx context.Context, id int64, message string, maxAttempts int) (string, error) {
	if maxAttempts <= 0 {
		maxAttempts = 5
	}
	var status string
	err := r.pool.QueryRow(ctx, `
		UPDATE notification_outbox
		SET attempts = attempts + 1,
		    status = CASE WHEN attempts + 1 >= $2 THEN 'failed' ELSE 'retrying' END,
		    last_error = $3,
		    next_retry_at = CASE
		        WHEN attempts + 1 >= $2 THEN next_retry_at
		        ELSE now() + make_interval(mins => LEAST(60, (5 * power(2, GREATEST(attempts, 0)))::int))
		    END,
		    updated_at = now()
		WHERE id = $1
		RETURNING status
	`, id, maxAttempts, message).Scan(&status)
	if err != nil {
		return "", fmt.Errorf("mark notification retry: %w", err)
	}
	return status, nil
}

// MarkTerminal 记录不可重试的终态（未配置、载荷损坏、收件人缺失）。
func (r *Repository) MarkTerminal(ctx context.Context, id int64, status, message string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE notification_outbox
		SET status = $2, attempts = attempts + 1, last_error = $3, updated_at = now()
		WHERE id = $1
	`, id, status, message)
	if err != nil {
		return fmt.Errorf("mark notification terminal: %w", err)
	}
	return nil
}

// List 管理站查询外发记录，按更新时间倒序；status 为空表示不过滤。
func (r *Repository) List(ctx context.Context, status string, limit int) ([]OutboxRecord, error) {
	if status != "" && !validOutboxStatuses[status] {
		return nil, ErrInvalidOutboxQuery
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, event_type, recipient_id, status, attempts, COALESCE(last_error, ''),
		       COALESCE(`+fmt.Sprintf(outboxTS, "next_retry_at")+`, ''),
		       COALESCE(`+fmt.Sprintf(outboxTS, "sent_at")+`, ''),
		       `+fmt.Sprintf(outboxTS, "created_at")+`, `+fmt.Sprintf(outboxTS, "updated_at")+`
		FROM notification_outbox
		WHERE ($1 = '' OR status = $1)
		ORDER BY updated_at DESC, id DESC
		LIMIT $2
	`, status, limit)
	if err != nil {
		return nil, fmt.Errorf("list notification outbox: %w", err)
	}
	defer rows.Close()
	out := []OutboxRecord{}
	for rows.Next() {
		var rec OutboxRecord
		if err := rows.Scan(&rec.ID, &rec.EventType, &rec.RecipientID, &rec.Status, &rec.Attempts,
			&rec.LastError, &rec.NextRetryAt, &rec.SentAt, &rec.CreatedAt, &rec.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

// Retry 将失败或未配置的记录重置为待发送，立即进入下一轮处理。
func (r *Repository) Retry(ctx context.Context, id int64) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE notification_outbox
		SET status = 'pending', attempts = 0, next_retry_at = now(),
		    last_error = NULL, updated_at = now()
		WHERE id = $1 AND status IN ('failed', 'not_configured')
	`, id)
	if err != nil {
		return fmt.Errorf("retry notification: %w", err)
	}
	if tag.RowsAffected() > 0 {
		return nil
	}
	var exists bool
	if err := r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM notification_outbox WHERE id = $1)`, id).Scan(&exists); err != nil {
		return fmt.Errorf("check notification outbox: %w", err)
	}
	if !exists {
		return ErrOutboxNotFound
	}
	return ErrOutboxNotRetryable
}

// Audit 写一条管理站审计记录，返回审计 ID（失败返回 0，不阻断主流程）。
func (r *Repository) Audit(ctx context.Context, adminID int64, action, targetType, targetID string) int64 {
	var id int64
	err := r.pool.QueryRow(ctx, `
		INSERT INTO audit_logs (actor_type, actor_id, action, target_type, target_id)
		VALUES ('admin', $1, $2, $3, $4)
		RETURNING id
	`, adminID, action, targetType, targetID).Scan(&id)
	if err != nil {
		return 0
	}
	return id
}

func (r *Repository) PayloadTitle(rec Record) string {
	var payload struct {
		Title string `json:"title"`
	}
	_ = json.Unmarshal(rec.PayloadEnc, &payload)
	return payload.Title
}
