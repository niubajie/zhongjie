package prospect

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository 只执行 SQL、组装实体；业务判断与事务在 service 层。
type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

const tsFmt = `to_char(%s AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')`

// ---- 信息源 ----

func (r *Repository) CreateSource(ctx context.Context, in CreateSourceInput) (int64, string, error) {
	var id int64
	var uuid string
	rate := in.RateLimitMs
	if rate <= 0 {
		rate = 3000
	}
	err := r.pool.QueryRow(ctx, `
		INSERT INTO lead_sources (name, kind, base_url, rate_limit_ms, review_note)
		VALUES ($1, $2, NULLIF($3, ''), $4, NULLIF($5, ''))
		RETURNING id, uuid::text
	`, in.Name, in.Kind, in.BaseURL, rate, in.ReviewNote).Scan(&id, &uuid)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return 0, "", ErrDuplicateSource
		}
		return 0, "", fmt.Errorf("create lead source: %w", err)
	}
	return id, uuid, nil
}

func (r *Repository) GetSource(ctx context.Context, uuid string) (LeadSource, error) {
	var s LeadSource
	err := r.pool.QueryRow(ctx, `
		SELECT id, uuid::text, name, kind, COALESCE(base_url, ''), robots_ok, terms_ok,
		       rate_limit_ms, enabled, COALESCE(review_note, ''),
		       `+fmt.Sprintf(tsFmt, "created_at")+`, `+fmt.Sprintf(tsFmt, "updated_at")+`
		FROM lead_sources
		WHERE uuid = $1 AND deleted_at IS NULL
	`, uuid).Scan(&s.ID, &s.UUID, &s.Name, &s.Kind, &s.BaseURL, &s.RobotsOK, &s.TermsOK,
		&s.RateLimitMs, &s.Enabled, &s.ReviewNote, &s.CreatedAt, &s.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return LeadSource{}, ErrSourceNotFound
	}
	if err != nil {
		return LeadSource{}, fmt.Errorf("get lead source: %w", err)
	}
	return s, nil
}

func (r *Repository) ListSources(ctx context.Context) ([]LeadSource, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, uuid::text, name, kind, COALESCE(base_url, ''), robots_ok, terms_ok,
		       rate_limit_ms, enabled, COALESCE(review_note, ''),
		       `+fmt.Sprintf(tsFmt, "created_at")+`, `+fmt.Sprintf(tsFmt, "updated_at")+`
		FROM lead_sources
		WHERE deleted_at IS NULL
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("list lead sources: %w", err)
	}
	defer rows.Close()
	out := []LeadSource{}
	for rows.Next() {
		var s LeadSource
		if err := rows.Scan(&s.ID, &s.UUID, &s.Name, &s.Kind, &s.BaseURL, &s.RobotsOK, &s.TermsOK,
			&s.RateLimitMs, &s.Enabled, &s.ReviewNote, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *Repository) SetSourceEnabled(ctx context.Context, uuid string, enabled bool) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE lead_sources SET enabled = $2, updated_at = now()
		WHERE uuid = $1 AND deleted_at IS NULL
	`, uuid, enabled)
	if err != nil {
		return fmt.Errorf("set lead source enabled: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrSourceNotFound
	}
	return nil
}

// ---- 线索 ----

func (r *Repository) CreateLead(ctx context.Context, lead ProspectLead, rawTextEnc, contactHash []byte) (int64, string, error) {
	var id int64
	var uuid string
	err := r.pool.QueryRow(ctx, `
		INSERT INTO prospect_leads (
			source_id, lead_type, platform, external_post_id, raw_text_enc, contact_hash,
			contact_channel, city_code, district_code, budget_cents, area_sqm, layout,
			intent_score, intent_status, status
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NULLIF($8, ''), NULLIF($9, ''),
		        NULLIF($10, 0), NULLIF($11, 0), NULLIF($12, ''),
		        NULLIF($13, 0), $14, $15)
		RETURNING id, uuid::text
	`, lead.SourceID, lead.LeadType, lead.Platform, lead.ExternalPostID, rawTextEnc, contactHash,
		lead.ContactChannel, lead.CityCode, lead.DistrictCode, lead.BudgetCents, lead.AreaSqm,
		lead.Layout, lead.IntentScore, lead.IntentStatus, lead.Status).Scan(&id, &uuid)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return 0, "", ErrDuplicateLead
		}
		return 0, "", fmt.Errorf("create prospect lead: %w", err)
	}
	return id, uuid, nil
}

func (r *Repository) GetLead(ctx context.Context, uuid string) (ProspectLead, error) {
	var l ProspectLead
	err := r.pool.QueryRow(ctx, `
		SELECT l.id, l.uuid::text, l.source_id, COALESCE(s.name, ''), l.lead_type, l.platform,
		       l.external_post_id, l.contact_channel, COALESCE(l.city_code, ''),
		       COALESCE(l.district_code, ''), COALESCE(l.budget_cents, 0),
		       COALESCE(l.area_sqm, 0), COALESCE(l.layout, ''), COALESCE(l.intent_score, 0),
		       l.intent_status, l.status, COALESCE(l.matched_user_id, 0),
		       COALESCE(l.matched_property_id::text, ''),
		       `+fmt.Sprintf(tsFmt, "l.created_at")+`, `+fmt.Sprintf(tsFmt, "l.updated_at")+`
		FROM prospect_leads l
		LEFT JOIN lead_sources s ON s.id = l.source_id
		WHERE l.uuid = $1 AND l.deleted_at IS NULL
	`, uuid).Scan(&l.ID, &l.UUID, &l.SourceID, &l.SourceName, &l.LeadType, &l.Platform,
		&l.ExternalPostID, &l.ContactChannel, &l.CityCode, &l.DistrictCode, &l.BudgetCents,
		&l.AreaSqm, &l.Layout, &l.IntentScore, &l.IntentStatus, &l.Status, &l.MatchedUserID,
		&l.MatchedPropertyID, &l.CreatedAt, &l.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ProspectLead{}, ErrLeadNotFound
	}
	if err != nil {
		return ProspectLead{}, fmt.Errorf("get prospect lead: %w", err)
	}
	return l, nil
}

func (r *Repository) ListLeads(ctx context.Context, leadType, status string, limit int) ([]ProspectLead, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := r.pool.Query(ctx, `
		SELECT l.id, l.uuid::text, l.source_id, COALESCE(s.name, ''), l.lead_type, l.platform,
		       l.external_post_id, l.contact_channel, COALESCE(l.city_code, ''),
		       COALESCE(l.district_code, ''), COALESCE(l.budget_cents, 0),
		       COALESCE(l.area_sqm, 0), COALESCE(l.layout, ''), COALESCE(l.intent_score, 0),
		       l.intent_status, l.status, COALESCE(l.matched_user_id, 0),
		       COALESCE(l.matched_property_id::text, ''),
		       `+fmt.Sprintf(tsFmt, "l.created_at")+`, `+fmt.Sprintf(tsFmt, "l.updated_at")+`
		FROM prospect_leads l
		LEFT JOIN lead_sources s ON s.id = l.source_id
		WHERE l.deleted_at IS NULL
		  AND ($1 = '' OR l.lead_type = $1)
		  AND ($2 = '' OR l.status = $2)
		ORDER BY l.created_at DESC
		LIMIT $3
	`, leadType, status, limit)
	if err != nil {
		return nil, fmt.Errorf("list prospect leads: %w", err)
	}
	defer rows.Close()
	out := []ProspectLead{}
	for rows.Next() {
		var l ProspectLead
		if err := rows.Scan(&l.ID, &l.UUID, &l.SourceID, &l.SourceName, &l.LeadType, &l.Platform,
			&l.ExternalPostID, &l.ContactChannel, &l.CityCode, &l.DistrictCode, &l.BudgetCents,
			&l.AreaSqm, &l.Layout, &l.IntentScore, &l.IntentStatus, &l.Status, &l.MatchedUserID,
			&l.MatchedPropertyID, &l.CreatedAt, &l.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (r *Repository) UpdateLeadStatus(ctx context.Context, uuid, status string) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE prospect_leads SET status = $2, updated_at = now()
		WHERE uuid = $1 AND deleted_at IS NULL
	`, uuid, status)
	if err != nil {
		return fmt.Errorf("update prospect lead status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrLeadNotFound
	}
	return nil
}

// ---- 拒收 ----

func (r *Repository) OptOutExists(ctx context.Context, contactHash []byte, leadType string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM opt_outs WHERE contact_hash = $1 AND lead_type = $2)
	`, contactHash, leadType).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check opt out: %w", err)
	}
	return exists, nil
}

func (r *Repository) CreateOptOut(ctx context.Context, contactHash []byte, leadType, reason string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO opt_outs (contact_hash, lead_type, reason)
		VALUES ($1, $2, NULLIF($3, ''))
		ON CONFLICT (contact_hash, lead_type) DO NOTHING
	`, contactHash, leadType, reason)
	if err != nil {
		return fmt.Errorf("create opt out: %w", err)
	}
	return nil
}

// ---- 跟进 ----

func (r *Repository) CreateFollowUp(ctx context.Context, leadID int64, channel, direction string, contentEnc []byte, auditID int64) (int64, error) {
	var id int64
	err := r.pool.QueryRow(ctx, `
		INSERT INTO follow_ups (lead_id, channel, direction, content_enc, status, audit_id)
		VALUES ($1, $2, $3, $4, 'sent', NULLIF($5, 0))
		RETURNING id
	`, leadID, channel, direction, contentEnc, auditID).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("create follow up: %w", err)
	}
	return id, nil
}

func (r *Repository) ListFollowUps(ctx context.Context, leadID int64) ([]FollowUp, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, lead_id, channel, direction, status,
		       `+fmt.Sprintf(tsFmt, "sent_at")+`, COALESCE(audit_id, 0),
		       `+fmt.Sprintf(tsFmt, "created_at")+`
		FROM follow_ups
		WHERE lead_id = $1
		ORDER BY created_at ASC
	`, leadID)
	if err != nil {
		return nil, fmt.Errorf("list follow ups: %w", err)
	}
	defer rows.Close()
	out := []FollowUp{}
	for rows.Next() {
		var f FollowUp
		if err := rows.Scan(&f.ID, &f.LeadID, &f.Channel, &f.Direction, &f.Status,
			&f.SentAt, &f.AuditID, &f.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// ---- 审计 ----

func (r *Repository) Audit(ctx context.Context, adminID int64, action, targetType, targetID string) {
	_, _ = r.pool.Exec(ctx, `
		INSERT INTO audit_logs (actor_type, actor_id, action, target_type, target_id)
		VALUES ('admin', $1, $2, $3, $4)
	`, adminID, action, targetType, targetID)
}

// ---- 批次 2：运行时配置 ----

func (r *Repository) GetSetting(ctx context.Context, key string) (string, error) {
	var value string
	err := r.pool.QueryRow(ctx, `SELECT value FROM prospect_settings WHERE key = $1`, key).Scan(&value)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get prospect setting: %w", err)
	}
	return value, nil
}

func (r *Repository) UpsertSetting(ctx context.Context, key, value string, adminID int64) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO prospect_settings (key, value, updated_by)
		VALUES ($1, $2, NULLIF($3, 0))
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_by = EXCLUDED.updated_by, updated_at = now()
	`, key, value, adminID)
	if err != nil {
		return fmt.Errorf("upsert prospect setting: %w", err)
	}
	return nil
}

// ---- 批次 2：信息源更新与软删除 ----

func (r *Repository) UpdateSource(ctx context.Context, uuid string, in UpdateSourceInput) error {
	rate := in.RateLimitMs
	if rate <= 0 {
		rate = 3000
	}
	tag, err := r.pool.Exec(ctx, `
		UPDATE lead_sources
		SET name = $2, kind = $3, base_url = NULLIF($4, ''), rate_limit_ms = $5,
		    robots_ok = $6, terms_ok = $7, review_note = NULLIF($8, ''), updated_at = now()
		WHERE uuid = $1 AND deleted_at IS NULL
	`, uuid, in.Name, in.Kind, in.BaseURL, rate, in.RobotsOK, in.TermsOK, in.ReviewNote)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ErrDuplicateSource
		}
		return fmt.Errorf("update lead source: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrSourceNotFound
	}
	return nil
}

func (r *Repository) SoftDeleteSource(ctx context.Context, uuid string) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE lead_sources SET deleted_at = now(), enabled = false, updated_at = now()
		WHERE uuid = $1 AND deleted_at IS NULL
	`, uuid)
	if err != nil {
		return fmt.Errorf("delete lead source: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrSourceNotFound
	}
	return nil
}
