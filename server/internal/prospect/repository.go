package prospect

import (
	"context"
	"errors"
	"fmt"
	"time"

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

// sourceColumns 是 lead_sources 的统一查询列（顺序与 scanSource 一致）。
var sourceColumns = `id, uuid::text, name, kind, COALESCE(base_url, ''), robots_ok, terms_ok,
	rate_limit_ms, enabled, COALESCE(review_note, ''), collect_interval_minutes,
	COALESCE(` + fmt.Sprintf(tsFmt, "last_collected_at") + `, ''),
	COALESCE(` + fmt.Sprintf(tsFmt, "next_collect_at") + `, ''),
	consecutive_failures, COALESCE(last_error, ''),
	COALESCE(` + fmt.Sprintf(tsFmt, "last_error_at") + `, ''),
	` + fmt.Sprintf(tsFmt, "created_at") + `, ` + fmt.Sprintf(tsFmt, "updated_at")

type rowScanner interface {
	Scan(dest ...any) error
}

func scanSource(row rowScanner) (LeadSource, error) {
	var s LeadSource
	err := row.Scan(&s.ID, &s.UUID, &s.Name, &s.Kind, &s.BaseURL, &s.RobotsOK, &s.TermsOK,
		&s.RateLimitMs, &s.Enabled, &s.ReviewNote, &s.CollectIntervalMinutes,
		&s.LastCollectedAt, &s.NextCollectAt, &s.ConsecutiveFailures,
		&s.LastError, &s.LastErrorAt, &s.CreatedAt, &s.UpdatedAt)
	return s, err
}

// ---- 信息源 ----

func (r *Repository) CreateSource(ctx context.Context, in CreateSourceInput) (int64, string, error) {
	var id int64
	var uuid string
	rate := in.RateLimitMs
	if rate <= 0 {
		rate = 3000
	}
	interval := in.CollectIntervalMinutes
	if interval <= 0 {
		interval = DefaultCollectIntervalMinutes
	}
	err := r.pool.QueryRow(ctx, `
		INSERT INTO lead_sources (name, kind, base_url, rate_limit_ms, collect_interval_minutes, review_note)
		VALUES ($1, $2, NULLIF($3, ''), $4, $5, NULLIF($6, ''))
		RETURNING id, uuid::text
	`, in.Name, in.Kind, in.BaseURL, rate, interval, in.ReviewNote).Scan(&id, &uuid)
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
	s, err := scanSource(r.pool.QueryRow(ctx, `
		SELECT `+sourceColumns+`
		FROM lead_sources
		WHERE uuid = $1 AND deleted_at IS NULL
	`, uuid))
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
		SELECT `+sourceColumns+`
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
		s, err := scanSource(rows)
		if err != nil {
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
		       l.external_post_id, l.raw_text_enc, l.contact_hash, l.contact_channel,
		       COALESCE(l.city_code, ''),
		       COALESCE(l.district_code, ''), COALESCE(l.budget_cents, 0),
		       COALESCE(l.area_sqm, 0), COALESCE(l.layout, ''), COALESCE(l.intent_score, 0),
		       l.intent_status, l.status, COALESCE(l.matched_user_id, 0),
		       COALESCE(l.matched_property_id::text, ''),
		       COALESCE(`+fmt.Sprintf(tsFmt, "l.stale_at")+`, ''),
		       `+fmt.Sprintf(tsFmt, "l.created_at")+`, `+fmt.Sprintf(tsFmt, "l.updated_at")+`
		FROM prospect_leads l
		LEFT JOIN lead_sources s ON s.id = l.source_id
		WHERE l.uuid = $1 AND l.deleted_at IS NULL
	`, uuid).Scan(&l.ID, &l.UUID, &l.SourceID, &l.SourceName, &l.LeadType, &l.Platform,
		&l.ExternalPostID, &l.RawTextEnc, &l.ContactHash, &l.ContactChannel, &l.CityCode,
		&l.DistrictCode, &l.BudgetCents, &l.AreaSqm, &l.Layout, &l.IntentScore,
		&l.IntentStatus, &l.Status, &l.MatchedUserID, &l.MatchedPropertyID,
		&l.StaleAt, &l.CreatedAt, &l.UpdatedAt)
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
		       COALESCE(`+fmt.Sprintf(tsFmt, "l.stale_at")+`, ''),
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
			&l.MatchedPropertyID, &l.StaleAt, &l.CreatedAt, &l.UpdatedAt); err != nil {
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

// ListOptOuts 拒收名单（批次 6），不返回 contact_hash。
func (r *Repository) ListOptOuts(ctx context.Context, leadType string, limit int) ([]OptOut, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, lead_type, COALESCE(reason, ''), `+fmt.Sprintf(tsFmt, "opted_out_at")+`
		FROM opt_outs
		WHERE ($1 = '' OR lead_type = $1)
		ORDER BY opted_out_at DESC
		LIMIT $2
	`, leadType, limit)
	if err != nil {
		return nil, fmt.Errorf("list opt outs: %w", err)
	}
	defer rows.Close()
	out := []OptOut{}
	for rows.Next() {
		var o OptOut
		if err := rows.Scan(&o.ID, &o.LeadType, &o.Reason, &o.OptedOutAt); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// ---- 跟进 ----

func (r *Repository) CreateFollowUp(ctx context.Context, leadID int64, channel, direction string, contentEnc []byte, auditID int64) (int64, error) {
	var id int64
	err := r.pool.QueryRow(ctx, `
		INSERT INTO follow_ups (lead_id, channel, direction, content_enc, status, sent_at, audit_id)
		VALUES ($1, $2, $3, $4, 'sent', now(), NULLIF($5, 0))
		RETURNING id
	`, leadID, channel, direction, contentEnc, auditID).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("create follow up: %w", err)
	}
	return id, nil
}

func (r *Repository) ListFollowUps(ctx context.Context, leadID int64) ([]FollowUp, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, lead_id, channel, direction, status, content_enc,
		       COALESCE(`+fmt.Sprintf(tsFmt, "sent_at")+`, ''), COALESCE(audit_id, 0),
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
		if err := rows.Scan(&f.ID, &f.LeadID, &f.Channel, &f.Direction, &f.Status, &f.ContentEnc,
			&f.SentAt, &f.AuditID, &f.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// ---- 审计 ----

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
	interval := in.CollectIntervalMinutes
	if interval <= 0 {
		interval = DefaultCollectIntervalMinutes
	}
	tag, err := r.pool.Exec(ctx, `
		UPDATE lead_sources
		SET name = $2, kind = $3, base_url = NULLIF($4, ''), rate_limit_ms = $5,
		    collect_interval_minutes = $6, robots_ok = $7, terms_ok = $8,
		    review_note = NULLIF($9, ''), updated_at = now()
		WHERE uuid = $1 AND deleted_at IS NULL
	`, uuid, in.Name, in.Kind, in.BaseURL, rate, interval, in.RobotsOK, in.TermsOK, in.ReviewNote)
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

// ---- 批次 3：采集 ----

// ListEnabledSources 返回已启用且未删除的信息源，供采集器逐源拉取。
func (r *Repository) ListEnabledSources(ctx context.Context) ([]LeadSource, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+sourceColumns+`
		FROM lead_sources
		WHERE enabled = true AND deleted_at IS NULL
		ORDER BY id
	`)
	if err != nil {
		return nil, fmt.Errorf("list enabled lead sources: %w", err)
	}
	defer rows.Close()
	out := []LeadSource{}
	for rows.Next() {
		s, err := scanSource(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// ListDueSources 返回已到采集时间的信息源：next_collect_at 为空视为立即到期。
func (r *Repository) ListDueSources(ctx context.Context, now time.Time) ([]LeadSource, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+sourceColumns+`
		FROM lead_sources
		WHERE enabled = true AND deleted_at IS NULL
		  AND (next_collect_at IS NULL OR next_collect_at <= $1)
		ORDER BY id
	`, now)
	if err != nil {
		return nil, fmt.Errorf("list due lead sources: %w", err)
	}
	defer rows.Close()
	out := []LeadSource{}
	for rows.Next() {
		s, err := scanSource(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// RecordCollectResult 原子更新信息源调度状态并写入一条采集运行记录。
func (r *Repository) RecordCollectResult(ctx context.Context, sourceID int64, success bool, startedAt, nextAt time.Time, st CollectRunStats, errMsg string) error {
	status := CollectStatusSuccess
	if !success {
		status = CollectStatusFailed
	}
	_, err := r.pool.Exec(ctx, `
		WITH upd AS (
			UPDATE lead_sources
			SET last_collected_at = $2,
			    next_collect_at = $3,
			    consecutive_failures = CASE WHEN $4::boolean THEN 0 ELSE consecutive_failures + 1 END,
			    last_error = CASE WHEN $4::boolean THEN NULL ELSE NULLIF($5, '') END,
			    last_error_at = CASE WHEN $4::boolean THEN NULL ELSE now() END,
			    updated_at = now()
			WHERE id = $1
			RETURNING id
		)
		INSERT INTO prospect_collect_runs (
			source_id, status, fetched, ingested, duplicate, opted_out, invalid, failed,
			error_message, started_at, finished_at
		)
		SELECT id, $6, $7, $8, $9, $10, $11, $12, NULLIF($5, ''), $2, now() FROM upd
	`, sourceID, startedAt, nextAt, success, errMsg, status,
		st.Fetched, st.Ingested, st.Duplicate, st.OptedOut, st.Invalid, st.Failed)
	if err != nil {
		return fmt.Errorf("record collect result: %w", err)
	}
	return nil
}

// ListCollectRuns 采集运行记录列表，可按信息源 UUID 过滤。
func (r *Repository) ListCollectRuns(ctx context.Context, sourceUUID string, limit int) ([]CollectRun, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := r.pool.Query(ctx, `
		SELECT cr.id, s.uuid::text, s.name, cr.status, cr.fetched, cr.ingested, cr.duplicate,
		       cr.opted_out, cr.invalid, cr.failed, COALESCE(cr.error_message, ''),
		       `+fmt.Sprintf(tsFmt, "cr.started_at")+`, `+fmt.Sprintf(tsFmt, "cr.finished_at")+`
		FROM prospect_collect_runs cr
		JOIN lead_sources s ON s.id = cr.source_id
		WHERE ($1 = '' OR s.uuid::text = $1)
		ORDER BY cr.started_at DESC, cr.id DESC
		LIMIT $2
	`, sourceUUID, limit)
	if err != nil {
		return nil, fmt.Errorf("list collect runs: %w", err)
	}
	defer rows.Close()
	out := []CollectRun{}
	for rows.Next() {
		var c CollectRun
		if err := rows.Scan(&c.ID, &c.SourceUUID, &c.SourceName, &c.Status,
			&c.Stats.Fetched, &c.Stats.Ingested, &c.Stats.Duplicate, &c.Stats.OptedOut,
			&c.Stats.Invalid, &c.Stats.Failed, &c.ErrorMessage, &c.StartedAt, &c.FinishedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ---- 批次 13：操作审计日志查询 ----

// auditLogWhere 只查 prospect.* 动作，避免把其他模块日志暴露到获客后台。
const auditLogWhere = `l.action LIKE 'prospect.%'
	AND ($1 = '' OR l.action = $1)
	AND ($2 = '' OR l.target_type = $2)
	AND ($3 = '' OR l.target_id = $3)
	AND ($4 = 0 OR l.actor_id = $4)
	AND ($5::timestamptz IS NULL OR l.created_at >= $5)
	AND ($6::timestamptz IS NULL OR l.created_at <= $6)`

// ListAuditLogs 分页查询获客审计日志，返回当前页与符合条件总数。
func (r *Repository) ListAuditLogs(ctx context.Context, f AuditLogFilter) ([]AuditLog, int64, error) {
	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	offset := f.Offset
	if offset < 0 {
		offset = 0
	}
	var fromArg, toArg *time.Time
	if !f.From.IsZero() {
		fromArg = &f.From
	}
	if !f.To.IsZero() {
		toArg = &f.To
	}

	var total int64
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs l WHERE `+auditLogWhere,
		f.Action, f.TargetType, f.TargetID, f.ActorID, fromArg, toArg).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count audit logs: %w", err)
	}

	rows, err := r.pool.Query(ctx, `
		SELECT l.id, l.actor_type, COALESCE(l.actor_id, 0), COALESCE(a.username, ''),
		       l.action, l.target_type, COALESCE(l.target_id, ''),
		       `+fmt.Sprintf(tsFmt, "l.created_at")+`
		FROM audit_logs l
		LEFT JOIN admin_users a ON a.id = l.actor_id AND l.actor_type = 'admin'
		WHERE `+auditLogWhere+`
		ORDER BY l.created_at DESC, l.id DESC
		LIMIT $7 OFFSET $8
	`, f.Action, f.TargetType, f.TargetID, f.ActorID, fromArg, toArg, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list audit logs: %w", err)
	}
	defer rows.Close()
	out := []AuditLog{}
	for rows.Next() {
		var l AuditLog
		if err := rows.Scan(&l.ID, &l.ActorType, &l.ActorID, &l.ActorName,
			&l.Action, &l.TargetType, &l.TargetID, &l.CreatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, l)
	}
	return out, total, rows.Err()
}

// LeadExistsByContactHash 判断同一联系方式（同 lead_type）是否已入库，用于跨平台去重。
func (r *Repository) LeadExistsByContactHash(ctx context.Context, contactHash []byte, leadType string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM prospect_leads
			WHERE contact_hash = $1 AND lead_type = $2 AND deleted_at IS NULL
		)
	`, contactHash, leadType).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check lead contact hash: %w", err)
	}
	return exists, nil
}

// ListRecentSelfOwned 读取自有留资（租客在站内对房源发起的联系），作为零合规风险的采集源。
// external_post_id 复用 contact_requests.uuid，重复扫描由唯一索引幂等去重。
func (r *Repository) ListRecentSelfOwned(ctx context.Context, limit int) ([]RawPost, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := r.pool.Query(ctx, `
		SELECT cr.uuid::text, u.phone_hash, p.city_code, p.district_code,
		       COALESCE(p.rent_cents, 0), COALESCE(p.area_sqm, 0), COALESCE(p.layout, ''),
		       p.title_public
		FROM contact_requests cr
		JOIN users u ON u.id = cr.tenant_id
		JOIN properties p ON p.id = cr.property_id
		WHERE u.deleted_at IS NULL
		ORDER BY cr.id DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list self-owned contact requests: %w", err)
	}
	defer rows.Close()
	out := []RawPost{}
	for rows.Next() {
		var post RawPost
		var phoneHash []byte
		var title string
		if err := rows.Scan(&post.ExternalPostID, &phoneHash, &post.CityCode, &post.DistrictCode,
			&post.BudgetCents, &post.AreaSqm, &post.Layout, &title); err != nil {
			return nil, err
		}
		post.Platform = SourceKindSelfOwned
		post.LeadType = LeadTypeTenant
		post.ContactHash = phoneHash
		post.ContactChannel = ChannelUnknown
		if len(phoneHash) > 0 {
			post.ContactChannel = ChannelPhone
		}
		post.RawText = "站内咨询房源：" + title
		out = append(out, post)
	}
	return out, rows.Err()
}

// ---- 批次 4：意向评分 ----

// ListUnscoredLeads 取出待评分的线索（status=new 且 intent_status=unknown）。
func (r *Repository) ListUnscoredLeads(ctx context.Context, limit int) ([]UnscoredLead, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.pool.Query(ctx, `
		SELECT uuid::text, lead_type, raw_text_enc, contact_channel,
		       COALESCE(city_code, ''), COALESCE(district_code, ''),
		       COALESCE(budget_cents, 0), COALESCE(area_sqm, 0), COALESCE(layout, ''),
		       `+fmt.Sprintf(tsFmt, "created_at")+`
		FROM prospect_leads
		WHERE status = 'new' AND intent_status = 'unknown' AND deleted_at IS NULL
		ORDER BY id
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list unscored leads: %w", err)
	}
	defer rows.Close()
	out := []UnscoredLead{}
	for rows.Next() {
		var l UnscoredLead
		if err := rows.Scan(&l.UUID, &l.LeadType, &l.RawTextEnc, &l.ContactChannel,
			&l.CityCode, &l.DistrictCode, &l.BudgetCents, &l.AreaSqm, &l.Layout, &l.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// UpdateLeadIntent 写入 LLM 分类与评分结果，并按分流结论更新 status。
func (r *Repository) UpdateLeadIntent(ctx context.Context, uuid, intentStatus, status string, score int) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE prospect_leads
		SET intent_status = $2, intent_score = $3, status = $4, updated_at = now()
		WHERE uuid = $1 AND deleted_at IS NULL
	`, uuid, intentStatus, score, status)
	if err != nil {
		return fmt.Errorf("update lead intent: %w", err)
	}
	return nil
}

// ---- 批次 5：人工复核与首条联系 ----

// ListReviewQueue 待复核队列：status=new 且已评分达标（有效意向）的线索，按分数降序。
func (r *Repository) ListReviewQueue(ctx context.Context, leadType string, minScore, limit int) ([]ProspectLead, error) {
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
		       COALESCE(`+fmt.Sprintf(tsFmt, "l.stale_at")+`, ''),
		       `+fmt.Sprintf(tsFmt, "l.created_at")+`, `+fmt.Sprintf(tsFmt, "l.updated_at")+`
		FROM prospect_leads l
		LEFT JOIN lead_sources s ON s.id = l.source_id
		WHERE l.deleted_at IS NULL
		  AND l.status = 'new'
		  AND l.intent_status <> 'unknown'
		  AND COALESCE(l.intent_score, 0) >= $1
		  AND ($2 = '' OR l.lead_type = $2)
		ORDER BY COALESCE(l.intent_score, 0) DESC, l.created_at ASC
		LIMIT $3
	`, minScore, leadType, limit)
	if err != nil {
		return nil, fmt.Errorf("list review queue: %w", err)
	}
	defer rows.Close()
	out := []ProspectLead{}
	for rows.Next() {
		var l ProspectLead
		if err := rows.Scan(&l.ID, &l.UUID, &l.SourceID, &l.SourceName, &l.LeadType, &l.Platform,
			&l.ExternalPostID, &l.ContactChannel, &l.CityCode, &l.DistrictCode, &l.BudgetCents,
			&l.AreaSqm, &l.Layout, &l.IntentScore, &l.IntentStatus, &l.Status, &l.MatchedUserID,
			&l.MatchedPropertyID, &l.StaleAt, &l.CreatedAt, &l.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// TransitionLeadStatus 带前置状态校验的状态更新，防止并发下的越权流转。
func (r *Repository) TransitionLeadStatus(ctx context.Context, uuid, from, to string) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE prospect_leads SET status = $3, updated_at = now()
		WHERE uuid = $1 AND status = $2 AND deleted_at IS NULL
	`, uuid, from, to)
	if err != nil {
		return fmt.Errorf("transition lead status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrInvalidTransition
	}
	return nil
}

// ContactLead 原子完成：reviewed → contacted，并写入一条已发送的 outbound 跟进记录。
// 任何一步不满足前置条件都不落库，避免出现「状态改了但没记录」。
func (r *Repository) ContactLead(ctx context.Context, leadID int64, channel string, contentEnc []byte, auditID int64) (FollowUp, error) {
	var f FollowUp
	err := r.pool.QueryRow(ctx, `
		WITH moved AS (
			UPDATE prospect_leads SET status = 'contacted', updated_at = now()
			WHERE id = $1 AND status = 'reviewed' AND deleted_at IS NULL
			RETURNING id
		)
		INSERT INTO follow_ups (lead_id, channel, direction, content_enc, status, sent_at, audit_id)
		SELECT id, $2, 'outbound', $3, 'sent', now(), NULLIF($4, 0) FROM moved
		RETURNING id, `+fmt.Sprintf(tsFmt, "sent_at")+`, `+fmt.Sprintf(tsFmt, "created_at")+`
	`, leadID, channel, contentEnc, auditID).Scan(&f.ID, &f.SentAt, &f.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return FollowUp{}, ErrInvalidTransition
	}
	if err != nil {
		return FollowUp{}, fmt.Errorf("contact lead: %w", err)
	}
	f.LeadID = leadID
	f.Channel = channel
	f.Direction = DirectionOutbound
	f.Status = "sent"
	f.AuditID = auditID
	return f, nil
}
