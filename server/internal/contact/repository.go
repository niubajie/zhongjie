package contact

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Target struct {
	PropertyID   int64
	LandlordID   int64
	Title        string
	DistrictName string
	StreetName   string
}

type LandlordContact struct {
	PhoneEnc    []byte
	PhoneMask   string
	SharePhone  bool
	WechatEnc   []byte
	ShareWechat bool
}

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) FindTarget(ctx context.Context, propertyUUID string) (Target, error) {
	var t Target
	err := r.pool.QueryRow(ctx, `
		SELECT id, landlord_id, title_public, district_name, street_name
		FROM properties
		WHERE uuid = $1 AND status = 'active' AND deleted_at IS NULL
	`, propertyUUID).Scan(&t.PropertyID, &t.LandlordID, &t.Title, &t.DistrictName, &t.StreetName)
	if errors.Is(err, pgx.ErrNoRows) {
		return Target{}, ErrPropertyNotFound
	}
	if err != nil {
		return Target{}, fmt.Errorf("find contact target: %w", err)
	}
	return t, nil
}

func (r *Repository) FindLandlord(ctx context.Context, landlordID int64) (LandlordContact, error) {
	var c LandlordContact
	err := r.pool.QueryRow(ctx, `
		SELECT u.phone_enc, COALESCE(u.phone_mask, ''),
		       COALESCE(lp.share_phone, true),
		       lp.wechat_id_enc, COALESCE(lp.share_wechat, false)
		FROM users u
		LEFT JOIN landlord_profiles lp ON lp.user_id = u.id
		WHERE u.id = $1 AND u.deleted_at IS NULL
	`, landlordID).Scan(&c.PhoneEnc, &c.PhoneMask, &c.SharePhone, &c.WechatEnc, &c.ShareWechat)
	if errors.Is(err, pgx.ErrNoRows) {
		return LandlordContact{}, errors.New("landlord not found")
	}
	if err != nil {
		return LandlordContact{}, fmt.Errorf("find landlord contact: %w", err)
	}
	return c, nil
}

func (r *Repository) TenantHasPhone(ctx context.Context, tenantID int64) (bool, error) {
	var ok bool
	err := r.pool.QueryRow(ctx, `
		SELECT phone_hash IS NOT NULL AND phone_mask IS NOT NULL
		FROM users WHERE id = $1 AND deleted_at IS NULL
	`, tenantID).Scan(&ok)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrPhoneRequired
	}
	if err != nil {
		return false, fmt.Errorf("check tenant phone: %w", err)
	}
	return ok, nil
}

func (r *Repository) CountRecent(ctx context.Context, tenantID, propertyID int64) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*)::int FROM contact_requests
		WHERE tenant_id = $1 AND property_id = $2
		  AND created_at > now() - interval '7 days'
	`, tenantID, propertyID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count recent contacts: %w", err)
	}
	return n, nil
}

func (r *Repository) CountToday(ctx context.Context, tenantID int64) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*)::int FROM contact_requests
		WHERE tenant_id = $1 AND created_at >= date_trunc('day', now())
	`, tenantID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count today's contacts: %w", err)
	}
	return n, nil
}

func (r *Repository) Create(ctx context.Context, tenantID int64, t Target, note string) (string, error) {
	consent, _ := json.Marshal(map[string]bool{"contactConsent": true})
	var uuid string
	err := r.pool.QueryRow(ctx, `
		INSERT INTO contact_requests (
			tenant_id, landlord_id, property_id, status, note, consent_snapshot
		)
		VALUES ($1, $2, $3, 'pending', $4, $5::jsonb)
		RETURNING uuid::text
	`, tenantID, t.LandlordID, t.PropertyID, note, string(consent)).Scan(&uuid)
	if err != nil {
		return "", fmt.Errorf("create contact request: %w", err)
	}
	return uuid, nil
}

func (r *Repository) Audit(ctx context.Context, tenantID int64, targetUUID string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO audit_logs (actor_type, actor_id, action, target_type, target_id)
		VALUES ('user', $1, 'contact_reveal', 'property', $2)
	`, tenantID, targetUUID)
	if err != nil {
		return fmt.Errorf("write contact audit: %w", err)
	}
	return nil
}
