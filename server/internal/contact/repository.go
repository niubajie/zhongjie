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

type Settings struct {
	PhoneMask   string
	WechatEnc   []byte
	SharePhone  bool
	ShareWechat bool
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

func (r *Repository) CreateIfAllowed(ctx context.Context, tenantID int64, t Target, note string) (string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin contact request: %w", err)
	}
	defer tx.Rollback(ctx)

	// Serialize the "check then insert" window for this tenant/property pair.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`,
		fmt.Sprintf("contact:%d:%d", tenantID, t.PropertyID)); err != nil {
		return "", fmt.Errorf("lock contact request: %w", err)
	}
	var recent int
	if err := tx.QueryRow(ctx, `
		SELECT COUNT(*)::int FROM contact_requests
		WHERE tenant_id = $1 AND property_id = $2
		  AND created_at > now() - interval '7 days'
	`, tenantID, t.PropertyID).Scan(&recent); err != nil {
		return "", fmt.Errorf("count recent contacts: %w", err)
	}
	if recent > 0 {
		return "", ErrAlreadyContacted
	}
	var today int
	if err := tx.QueryRow(ctx, `
		SELECT COUNT(*)::int FROM contact_requests
		WHERE tenant_id = $1 AND created_at >= date_trunc('day', now())
	`, tenantID).Scan(&today); err != nil {
		return "", fmt.Errorf("count today's contacts: %w", err)
	}
	if today >= 3 {
		return "", ErrDailyLimit
	}

	consent, _ := json.Marshal(map[string]bool{"contactConsent": true})
	var uuid string
	if err := tx.QueryRow(ctx, `
		INSERT INTO contact_requests (
			tenant_id, landlord_id, property_id, status, note, consent_snapshot
		)
		VALUES ($1, $2, $3, 'pending', $4, $5::jsonb)
		RETURNING uuid::text
	`, tenantID, t.LandlordID, t.PropertyID, note, string(consent)).Scan(&uuid); err != nil {
		return "", fmt.Errorf("create contact request: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("commit contact request: %w", err)
	}
	return uuid, nil
}

func (r *Repository) GetSettings(ctx context.Context, userID int64) (string, Settings, error) {
	var role, phoneMask string
	var s Settings
	err := r.pool.QueryRow(ctx, `
		SELECT u.role, COALESCE(u.phone_mask, ''),
		       lp.wechat_id_enc, COALESCE(lp.share_phone, true), COALESCE(lp.share_wechat, false)
		FROM users u
		LEFT JOIN landlord_profiles lp ON lp.user_id = u.id
		WHERE u.id = $1 AND u.deleted_at IS NULL
	`, userID).Scan(&role, &phoneMask, &s.WechatEnc, &s.SharePhone, &s.ShareWechat)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", Settings{}, ErrNotLandlord
	}
	if err != nil {
		return "", Settings{}, fmt.Errorf("get landlord settings: %w", err)
	}
	s.PhoneMask = phoneMask
	return role, s, nil
}

func (r *Repository) UpdateSettings(ctx context.Context, userID int64, wechatEnc, wechatHash []byte, sharePhone, shareWechat bool) error {
	if _, err := r.pool.Exec(ctx, `
		INSERT INTO landlord_profiles (user_id, wechat_id_enc, wechat_id_hash, share_phone, share_wechat)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (user_id) DO UPDATE
		SET wechat_id_enc = EXCLUDED.wechat_id_enc,
		    wechat_id_hash = EXCLUDED.wechat_id_hash,
		    share_phone = EXCLUDED.share_phone,
		    share_wechat = EXCLUDED.share_wechat
	`, userID, wechatEnc, wechatHash, sharePhone, shareWechat); err != nil {
		return fmt.Errorf("update landlord settings: %w", err)
	}
	return nil
}

func (r *Repository) EnqueueNotification(ctx context.Context, landlordID int64, payload []byte) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO notification_outbox (event_type, recipient_id, payload_enc, status)
		VALUES ('contact_request', $1, $2, 'pending')
	`, landlordID, payload)
	if err != nil {
		return fmt.Errorf("enqueue notification: %w", err)
	}
	return nil
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
