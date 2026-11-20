package admin

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("admin not found")

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) FindByUsername(ctx context.Context, username string) (Admin, error) {
	var a Admin
	err := r.pool.QueryRow(ctx, `
		SELECT id, uuid::text, username, password_hash, role, status,
		       totp_enabled, totp_secret_enc, last_login_at
		FROM admin_users
		WHERE username = $1 AND deleted_at IS NULL
	`, username).Scan(&a.ID, &a.UUID, &a.Username, &a.PasswordHash, &a.Role, &a.Status,
		&a.TOTPEnabled, &a.TOTPSecretEnc, &a.LastLoginAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Admin{}, ErrNotFound
	}
	if err != nil {
		return Admin{}, fmt.Errorf("find admin: %w", err)
	}
	return a, nil
}

func (r *Repository) FindByID(ctx context.Context, id int64) (Admin, error) {
	var a Admin
	err := r.pool.QueryRow(ctx, `
		SELECT id, uuid::text, username, password_hash, role, status,
		       totp_enabled, totp_secret_enc, last_login_at
		FROM admin_users
		WHERE id = $1 AND deleted_at IS NULL
	`, id).Scan(&a.ID, &a.UUID, &a.Username, &a.PasswordHash, &a.Role, &a.Status,
		&a.TOTPEnabled, &a.TOTPSecretEnc, &a.LastLoginAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Admin{}, ErrNotFound
	}
	if err != nil {
		return Admin{}, fmt.Errorf("find admin by id: %w", err)
	}
	return a, nil
}

func (r *Repository) UpdateLastLogin(ctx context.Context, adminID int64) error {
	if _, err := r.pool.Exec(ctx, `
		UPDATE admin_users SET last_login_at = now() WHERE id = $1
	`, adminID); err != nil {
		return fmt.Errorf("update admin last login: %w", err)
	}
	return nil
}

func (r *Repository) Stats(ctx context.Context) (Stats, error) {
	var s Stats
	err := r.pool.QueryRow(ctx, `
		SELECT
		  (SELECT COUNT(*) FROM properties WHERE status = 'active' AND deleted_at IS NULL),
		  (SELECT COUNT(*) FROM properties WHERE status = 'pending' AND deleted_at IS NULL),
		  (SELECT COUNT(*) FROM users WHERE deleted_at IS NULL),
		  (SELECT COUNT(*) FROM users WHERE role = 'landlord' AND deleted_at IS NULL),
		  (SELECT COUNT(*) FROM contact_requests)
	`).Scan(&s.ActiveProperties, &s.PendingProperties, &s.Users, &s.Landlords, &s.ContactRequests)
	if err != nil {
		return Stats{}, fmt.Errorf("admin stats: %w", err)
	}
	return s, nil
}

func (r *Repository) ListProperties(ctx context.Context, status string, limit, offset int) ([]PropertyRow, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT uuid::text, title_public, status, district_name, street_name,
		       area_sqm::float8, rent_cents,
		       to_char(created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM properties
		WHERE deleted_at IS NULL
		  AND ($1 = '' OR status = $1)
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`, status, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list admin properties: %w", err)
	}
	defer rows.Close()
	out := make([]PropertyRow, 0, limit)
	for rows.Next() {
		var p PropertyRow
		if err := rows.Scan(&p.UUID, &p.Title, &p.Status, &p.DistrictName, &p.StreetName,
			&p.AreaSqm, &p.RentCents, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *Repository) SetTOTPSecret(ctx context.Context, adminID int64, enc []byte) error {
	if _, err := r.pool.Exec(ctx, `
		UPDATE admin_users SET totp_secret_enc = $2, totp_enabled = false WHERE id = $1
	`, adminID, enc); err != nil {
		return fmt.Errorf("set totp secret: %w", err)
	}
	return nil
}

func (r *Repository) SetTOTPEnabled(ctx context.Context, adminID int64, enabled bool) error {
	if _, err := r.pool.Exec(ctx, `
		UPDATE admin_users SET totp_enabled = $2 WHERE id = $1
	`, adminID, enabled); err != nil {
		return fmt.Errorf("set totp enabled: %w", err)
	}
	return nil
}

func (r *Repository) ModerateProperty(ctx context.Context, uuid, action, reason string) (string, error) {
	var status string
	switch action {
	case "approve":
		status = "active"
	case "offline":
		status = "offline"
	default:
		return "", fmt.Errorf("unsupported moderation action %q", action)
	}
	var next string
	err := r.pool.QueryRow(ctx, `
		UPDATE properties
		SET status = $2,
		    pending_reason = CASE WHEN $2 = 'active' THEN NULL ELSE COALESCE($3, '') END,
		    published_at = CASE WHEN $2 = 'active' THEN COALESCE(published_at, now()) ELSE published_at END,
		    last_confirmed_at = CASE WHEN $2 = 'active' THEN now() ELSE last_confirmed_at END
		WHERE uuid = $1 AND deleted_at IS NULL
		RETURNING status
	`, uuid, status, reason).Scan(&next)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("moderate property: %w", err)
	}
	return next, nil
}

func (r *Repository) Audit(ctx context.Context, adminID int64, action, targetType, targetID string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO audit_logs (actor_type, actor_id, action, target_type, target_id)
		VALUES ('admin', $1, $2, $3, $4)
	`, adminID, action, targetType, targetID)
	if err != nil {
		return fmt.Errorf("audit admin action: %w", err)
	}
	return nil
}

func (r *Repository) AuditLogin(ctx context.Context, adminID int64, username string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO audit_logs (actor_type, actor_id, action, target_type, target_id)
		VALUES ('admin', $1, 'admin_login', 'admin_user', $2)
	`, adminID, username)
	if err != nil {
		return fmt.Errorf("audit admin login: %w", err)
	}
	return nil
}
