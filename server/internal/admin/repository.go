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

func (r *Repository) ListImports(ctx context.Context, limit int) ([]ImportJobRow, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT uuid::text, file_name, file_type, status, total_rows, success_rows, error_rows,
		       to_char(created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM import_jobs
		ORDER BY created_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list imports: %w", err)
	}
	defer rows.Close()
	out := []ImportJobRow{}
	for rows.Next() {
		var row ImportJobRow
		if err := rows.Scan(&row.UUID, &row.FileName, &row.FileType, &row.Status,
			&row.TotalRows, &row.SuccessRows, &row.ErrorRows, &row.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *Repository) ListBuildings(ctx context.Context, limit int) ([]BuildingRow, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT uuid::text, name, district_name, street_name, building_type, review_status,
		       lat::float8, lng::float8
		FROM buildings
		WHERE city_code = '440300' AND deleted_at IS NULL
		ORDER BY name
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list buildings: %w", err)
	}
	defer rows.Close()
	out := []BuildingRow{}
	for rows.Next() {
		var row BuildingRow
		if err := rows.Scan(&row.UUID, &row.Name, &row.DistrictName, &row.StreetName,
			&row.BuildingType, &row.ReviewStatus, &row.Lat, &row.Lng); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *Repository) CreateBuilding(ctx context.Context, in CreateBuildingInput) (BuildingRow, error) {
	var row BuildingRow
	err := r.pool.QueryRow(ctx, `
		INSERT INTO buildings (
			city_code, district_code, district_name, street_code, street_name,
			name, building_type, lat, lng, source, review_status
		)
		VALUES ('440300', $1, $2, $3, $4, $5, $6, $7, $8, 'manual', 'approved')
		RETURNING uuid::text, name, district_name, street_name, building_type, review_status,
		          lat::float8, lng::float8
	`, in.DistrictCode, in.DistrictName, in.StreetCode, in.StreetName,
		in.Name, in.BuildingType, in.Lat, in.Lng).Scan(
		&row.UUID, &row.Name, &row.DistrictName, &row.StreetName,
		&row.BuildingType, &row.ReviewStatus, &row.Lat, &row.Lng)
	if err != nil {
		return BuildingRow{}, fmt.Errorf("create building: %w", err)
	}
	return row, nil
}

func (r *Repository) ListContacts(ctx context.Context, limit int) ([]ContactRow, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT cr.uuid::text, COALESCE(u.surname, ''), COALESCE(u.phone_mask, ''),
		       p.title_public, cr.status,
		       to_char(cr.created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM contact_requests cr
		JOIN users u ON u.id = cr.tenant_id
		JOIN properties p ON p.id = cr.property_id
		ORDER BY cr.created_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list contacts: %w", err)
	}
	defer rows.Close()
	out := []ContactRow{}
	for rows.Next() {
		var row ContactRow
		if err := rows.Scan(&row.UUID, &row.TenantSurname, &row.TenantPhoneMask,
			&row.PropertyTitle, &row.Status, &row.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *Repository) GetContact(ctx context.Context, uuid string) (ContactDetail, error) {
	var row ContactDetail
	err := r.pool.QueryRow(ctx, `
		SELECT cr.uuid::text, COALESCE(u.surname, ''), COALESCE(u.phone_mask, ''),
		       p.title_public, cr.status, COALESCE(cr.note, ''),
		       to_char(cr.created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM contact_requests cr
		JOIN users u ON u.id = cr.tenant_id
		JOIN properties p ON p.id = cr.property_id
		WHERE cr.uuid = $1
	`, uuid).Scan(&row.UUID, &row.TenantSurname, &row.TenantPhoneMask,
		&row.PropertyTitle, &row.Status, &row.Note, &row.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ContactDetail{}, ErrNotFound
	}
	if err != nil {
		return ContactDetail{}, fmt.Errorf("get contact: %w", err)
	}
	return row, nil
}

func (r *Repository) ListAuditLogs(ctx context.Context, limit int) ([]AuditLogRow, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, actor_type, action, target_type, COALESCE(target_id, ''),
		       to_char(created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM audit_logs
		ORDER BY created_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list audit logs: %w", err)
	}
	defer rows.Close()
	out := []AuditLogRow{}
	for rows.Next() {
		var row AuditLogRow
		if err := rows.Scan(&row.ID, &row.ActorType, &row.Action, &row.TargetType,
			&row.TargetID, &row.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *Repository) ListFreshness(ctx context.Context, limit int) ([]FreshnessRow, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT fc.id, p.uuid::text, p.title_public, fc.status,
		       to_char(fc.created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM freshness_checks fc
		JOIN properties p ON p.id = fc.property_id
		ORDER BY fc.created_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list freshness: %w", err)
	}
	defer rows.Close()
	out := []FreshnessRow{}
	for rows.Next() {
		var row FreshnessRow
		if err := rows.Scan(&row.ID, &row.PropertyUUID, &row.Title, &row.Status, &row.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
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
