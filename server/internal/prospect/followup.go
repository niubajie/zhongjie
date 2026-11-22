package prospect

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// 批次8：后续跟进记录与超期提醒。
// 跟进记录只在 contacted / following_up 阶段追加，写入后自动解除提醒与 stale 标记；
// 提醒由 Worker 周期扫描生成（3 天待跟进、7 天超期），不发送任何外部消息。

const (
	followUpStatusSent     = "sent"
	followUpStatusReceived = "received"
)

// ---- Repository：跟进与提醒 ----

// AddFollowUp 追加一条后续跟进记录，并在同一条语句内：
//   - contacted → following_up（following_up 保持不变）
//   - 清空 stale_at
//   - 把该线索所有未处理提醒标记为已处理
//
// 任何前置状态不满足都不落库。
func (r *Repository) AddFollowUp(ctx context.Context, leadID int64, channel, direction, status string, contentEnc []byte, auditID int64) (FollowUp, error) {
	var f FollowUp
	err := r.pool.QueryRow(ctx, `
		WITH moved AS (
			UPDATE prospect_leads
			SET status = CASE WHEN status = 'contacted' THEN 'following_up' ELSE status END,
			    stale_at = NULL
			WHERE id = $1 AND status IN ('contacted', 'following_up') AND deleted_at IS NULL
			RETURNING id
		), inserted AS (
			INSERT INTO follow_ups (lead_id, channel, direction, content_enc, status, sent_at, audit_id)
			SELECT id, $2, $3, $4, $5, now(), NULLIF($6, 0) FROM moved
			RETURNING id, sent_at, created_at
		), resolved AS (
			UPDATE prospect_reminders SET status = 'resolved', resolved_at = now()
			WHERE lead_id = $1 AND status = 'open'
		)
		SELECT id, `+fmt.Sprintf(tsFmt, "sent_at")+`, `+fmt.Sprintf(tsFmt, "created_at")+`
		FROM inserted
	`, leadID, channel, direction, contentEnc, status, auditID).Scan(&f.ID, &f.SentAt, &f.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return FollowUp{}, ErrInvalidTransition
	}
	if err != nil {
		return FollowUp{}, fmt.Errorf("add follow up: %w", err)
	}
	f.LeadID = leadID
	f.Channel = channel
	f.Direction = direction
	f.Status = status
	f.AuditID = auditID
	return f, nil
}

// ListFollowUpCandidates 扫描 contacted / following_up 线索的最后活动时间。
// 以最近一条跟进记录的创建时间为准，没有跟进记录时退回线索创建时间，
// 避免 stale 标记刷新 updated_at 后把计时重置。
func (r *Repository) ListFollowUpCandidates(ctx context.Context) ([]FollowUpCandidate, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT l.id, l.uuid::text, l.status,
		       COALESCE((SELECT max(f.created_at) FROM follow_ups f WHERE f.lead_id = l.id), l.created_at),
		       l.stale_at IS NOT NULL
		FROM prospect_leads l
		WHERE l.deleted_at IS NULL AND l.status IN ('contacted', 'following_up')
		ORDER BY l.id
	`)
	if err != nil {
		return nil, fmt.Errorf("list follow up candidates: %w", err)
	}
	defer rows.Close()
	out := []FollowUpCandidate{}
	for rows.Next() {
		var c FollowUpCandidate
		if err := rows.Scan(&c.ID, &c.UUID, &c.Status, &c.LastActivity, &c.Stale); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// CreateReminder 幂等生成提醒：同一线索同一级别最多保留一条未处理记录。
func (r *Repository) CreateReminder(ctx context.Context, leadID int64, level string) (bool, error) {
	tag, err := r.pool.Exec(ctx, `
		INSERT INTO prospect_reminders (lead_id, level)
		VALUES ($1, $2)
		ON CONFLICT (lead_id, level) WHERE status = 'open' DO NOTHING
	`, leadID, level)
	if err != nil {
		return false, fmt.Errorf("create reminder: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// MarkLeadStale 标记超期：只写 stale_at，不改状态。
func (r *Repository) MarkLeadStale(ctx context.Context, leadID int64) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE prospect_leads SET stale_at = COALESCE(stale_at, now()) WHERE id = $1
	`, leadID)
	if err != nil {
		return fmt.Errorf("mark lead stale: %w", err)
	}
	return nil
}

// ResolveRemindersByLeadUUID 线索离开跟进阶段后关闭其未处理提醒。
func (r *Repository) ResolveRemindersByLeadUUID(ctx context.Context, uuid string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE prospect_reminders SET status = 'resolved', resolved_at = now()
		WHERE status = 'open'
		  AND lead_id = (SELECT id FROM prospect_leads WHERE uuid = $1)
	`, uuid)
	if err != nil {
		return fmt.Errorf("resolve reminders by lead: %w", err)
	}
	return nil
}

// ResolveOutOfScopeReminders 兜底清理：线索已删除或已不在 contacted/following_up 时关闭提醒。
func (r *Repository) ResolveOutOfScopeReminders(ctx context.Context) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE prospect_reminders r SET status = 'resolved', resolved_at = now()
		FROM prospect_leads l
		WHERE r.lead_id = l.id AND r.status = 'open'
		  AND (l.deleted_at IS NOT NULL OR l.status NOT IN ('contacted', 'following_up'))
	`)
	if err != nil {
		return fmt.Errorf("resolve out of scope reminders: %w", err)
	}
	return nil
}

// ListReminders 未处理提醒列表，按生成时间倒序。
func (r *Repository) ListReminders(ctx context.Context, limit int) ([]Reminder, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := r.pool.Query(ctx, `
		SELECT r.id, l.uuid::text, l.lead_type, l.status, r.level,
		       `+fmt.Sprintf(tsFmt, "r.created_at")+`
		FROM prospect_reminders r
		JOIN prospect_leads l ON l.id = r.lead_id
		WHERE r.status = 'open' AND l.deleted_at IS NULL
		ORDER BY r.created_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list reminders: %w", err)
	}
	defer rows.Close()
	out := []Reminder{}
	for rows.Next() {
		var m Reminder
		if err := rows.Scan(&m.ID, &m.LeadUUID, &m.LeadType, &m.LeadStatus, &m.Level, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ---- Service ----

// AddFollowUp 记录后续跟进：仅 contacted / following_up 可用，内容加密并写审计。
// 记录成功后线索进入 following_up，并解除 stale 与未处理提醒。
func (s *Service) AddFollowUp(ctx context.Context, adminID int64, uuid string, in AddFollowUpInput) (FollowUpVO, error) {
	channel, direction, status, err := validateFollowUpInput(in)
	if err != nil {
		return FollowUpVO{}, err
	}
	lead, err := s.repo.GetLead(ctx, uuid)
	if err != nil {
		return FollowUpVO{}, err
	}
	if lead.Status != StatusContacted && lead.Status != StatusFollowingUp {
		return FollowUpVO{}, fmt.Errorf("%w: 当前状态 %s 不能记录跟进", ErrInvalidTransition, lead.Status)
	}
	if s.pii == nil {
		return FollowUpVO{}, errors.New("跟进内容加密未初始化")
	}
	content := strings.TrimSpace(in.Content)
	contentEnc, err := s.pii.EncryptString(content)
	if err != nil {
		return FollowUpVO{}, err
	}
	auditID := s.repo.Audit(ctx, adminID, "prospect.lead.followup", "prospect_lead", uuid)
	followUp, err := s.repo.AddFollowUp(ctx, lead.ID, channel, direction, status, contentEnc, auditID)
	if err != nil {
		return FollowUpVO{}, err
	}
	return toFollowUpVO(followUp, content), nil
}

// ListReminders 未处理提醒列表。
func (s *Service) ListReminders(ctx context.Context, limit int) ([]ReminderVO, error) {
	rows, err := s.repo.ListReminders(ctx, limit)
	if err != nil {
		return nil, err
	}
	out := make([]ReminderVO, 0, len(rows))
	for _, m := range rows {
		out = append(out, ReminderVO{
			ID:         m.ID,
			LeadUUID:   m.LeadUUID,
			LeadType:   m.LeadType,
			LeadStatus: m.LeadStatus,
			Level:      m.Level,
			CreatedAt:  m.CreatedAt,
		})
	}
	return out, nil
}

// ScanFollowUpReminders 扫描一次跟进提醒：3 天生成待跟进提醒，7 天标记超期。
// 幂等：重复扫描不会产生重复提醒，已标记超期的线索不会重复计数。
func (s *Service) ScanFollowUpReminders(ctx context.Context, now time.Time) (ReminderStats, error) {
	if err := s.repo.ResolveOutOfScopeReminders(ctx); err != nil {
		return ReminderStats{}, err
	}
	candidates, err := s.repo.ListFollowUpCandidates(ctx)
	if err != nil {
		return ReminderStats{}, err
	}
	stats := ReminderStats{Scanned: len(candidates)}
	for _, c := range candidates {
		level, markStale := planFollowUpReminder(c.LastActivity, now)
		if level == "" {
			continue
		}
		inserted, err := s.repo.CreateReminder(ctx, c.ID, level)
		if err != nil {
			return stats, err
		}
		if inserted {
			if level == ReminderLevelOverdue7d {
				stats.Overdue++
			} else {
				stats.Due++
			}
		}
		if markStale && !c.Stale {
			if err := s.repo.MarkLeadStale(ctx, c.ID); err != nil {
				return stats, err
			}
			stats.Stale++
		}
	}
	return stats, nil
}

// ---- 纯函数（便于单测）----

// planFollowUpReminder 根据最后活动时间决定提醒级别与是否标记超期。
func planFollowUpReminder(lastActivity, now time.Time) (level string, markStale bool) {
	if lastActivity.IsZero() {
		return "", false
	}
	age := now.Sub(lastActivity)
	switch {
	case age >= FollowUpOverdueAfter:
		return ReminderLevelOverdue7d, true
	case age >= FollowUpDueAfter:
		return ReminderLevelDue3d, false
	default:
		return "", false
	}
}

// validateFollowUpInput 校验跟进参数并补齐默认方向。
func validateFollowUpInput(in AddFollowUpInput) (channel, direction, status string, err error) {
	switch in.Channel {
	case ChannelInApp, ChannelSMS, ChannelWeChat, ChannelPhone:
		channel = in.Channel
	default:
		return "", "", "", fmt.Errorf("%w: 跟进渠道不合法", ErrInvalidFollowUp)
	}
	direction = strings.TrimSpace(in.Direction)
	if direction == "" {
		direction = DirectionOutbound
	}
	switch direction {
	case DirectionOutbound:
		status = followUpStatusSent
	case DirectionInbound:
		status = followUpStatusReceived
	default:
		return "", "", "", fmt.Errorf("%w: 跟进方向不合法", ErrInvalidFollowUp)
	}
	if strings.TrimSpace(in.Content) == "" {
		return "", "", "", fmt.Errorf("%w: 跟进内容不能为空", ErrInvalidFollowUp)
	}
	return channel, direction, status, nil
}
