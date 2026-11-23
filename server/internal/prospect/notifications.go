package prospect

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// 批次11：跟进提醒通知外发。
// 站内通知按活跃管理员 fan-out 写入 admin_notifications，(admin_id, reminder_id)
// 唯一索引天然实现重复提醒抑制；微信外发为可选通道，写入 notification_outbox
// 后由 notify.Service 统一发送，业务代码不直接调厂商接口。

// 站内通知事件类型（与 admin_notifications.event_type 的 CHECK 约束一致）。
const (
	AdminEventFollowUpDue     = "prospect_followup_due"
	AdminEventFollowUpOverdue = "prospect_followup_overdue"
	AdminEventCollectFailed   = "prospect_collect_failed"

	adminNotificationUnread = "unread"
)

// ReminderNotifier 外发通道抽象（由 notify.Repository 实现）。
type ReminderNotifier interface {
	EnqueueReminder(ctx context.Context, recipientID int64, payloadEnc []byte) error
}

// SetReminderNotifier 注入外发通道；adminUserID <= 0 表示不开启微信外发。
func (s *Service) SetReminderNotifier(n ReminderNotifier, adminUserID int64) {
	s.notifier = n
	s.notifyUserID = adminUserID
}

// reminderNotification 把提醒级别映射为站内通知的事件类型与文案（纯函数）。
func reminderNotification(level string) (eventType, title, body string) {
	if level == ReminderLevelOverdue7d {
		return AdminEventFollowUpOverdue, "获客跟进已超期", "有线索超过 7 天未跟进，请尽快处理"
	}
	return AdminEventFollowUpDue, "获客跟进待处理", "有线索超过 3 天未跟进，请及时跟进"
}

// notifyReminder 发送一条跟进提醒：先写站内通知（去重），再按配置决定是否外发微信。
func (s *Service) notifyReminder(ctx context.Context, reminderID int64, c FollowUpCandidate, level string) error {
	eventType, title, body := reminderNotification(level)
	if _, err := s.repo.CreateAdminNotifications(ctx, reminderID, c.ID, eventType, level, title, body); err != nil {
		return err
	}
	if s.notifier == nil || s.notifyUserID <= 0 || s.pii == nil {
		return nil
	}
	raw, err := json.Marshal(map[string]string{
		"reminder": title,
		"title":    body,
		"district": "",
		"street":   "",
	})
	if err != nil {
		return err
	}
	payloadEnc, err := s.pii.EncryptString(string(raw))
	if err != nil {
		return err
	}
	return s.notifier.EnqueueReminder(ctx, s.notifyUserID, payloadEnc)
}

// ---- Repository：站内通知 ----

// CreateAdminNotifications 向可访问获客模块的活跃管理员 fan-out 站内通知。
// 同一管理员的同一条提醒只会生成一条通知，重复扫描不会新增。
func (r *Repository) CreateAdminNotifications(ctx context.Context, reminderID, leadID int64, eventType, level, title, body string) (int64, error) {
	tag, err := r.pool.Exec(ctx, `
		INSERT INTO admin_notifications (admin_id, event_type, level, title, body, lead_id, reminder_id)
		SELECT a.id, $1, $2, $3, $4, $5, $6
		FROM admin_users a
		WHERE a.deleted_at IS NULL AND a.status = 'active'
		  AND a.role IN ('super_admin', 'operations')
		ON CONFLICT (admin_id, reminder_id) WHERE reminder_id IS NOT NULL DO NOTHING
	`, eventType, level, title, body, leadID, reminderID)
	if err != nil {
		return 0, fmt.Errorf("create admin notifications: %w", err)
	}
	return tag.RowsAffected(), nil
}

// CreateCollectFailureNotifications 向活跃管理员分发采集失败告警。
// 同一采集运行记录对每个管理员最多一条，重复调用安全。
func (r *Repository) CreateCollectFailureNotifications(ctx context.Context, collectRunID int64, title, body string) (int64, error) {
	tag, err := r.pool.Exec(ctx, `
		INSERT INTO admin_notifications (admin_id, event_type, level, title, body, collect_run_id)
		SELECT a.id, $1, 'error', $2, $3, $4
		FROM admin_users a
		WHERE a.deleted_at IS NULL AND a.status = 'active'
		  AND a.role IN ('super_admin', 'operations')
		ON CONFLICT (admin_id, collect_run_id) WHERE collect_run_id IS NOT NULL DO NOTHING
	`, AdminEventCollectFailed, title, body, collectRunID)
	if err != nil {
		return 0, fmt.Errorf("create collect failure notifications: %w", err)
	}
	return tag.RowsAffected(), nil
}

// ListAdminNotifications 当前管理员的站内通知，按创建时间倒序。
func (r *Repository) ListAdminNotifications(ctx context.Context, adminID int64, unreadOnly bool, limit int) ([]AdminNotification, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := r.pool.Query(ctx, `
		SELECT n.id, n.event_type, n.level, n.title, n.body,
		       COALESCE(l.uuid::text, ''), n.status,
		       `+fmt.Sprintf(tsFmt, "n.created_at")+`
		FROM admin_notifications n
		LEFT JOIN prospect_leads l ON l.id = n.lead_id
		WHERE n.admin_id = $1 AND ($2 = false OR n.status = 'unread')
		ORDER BY n.created_at DESC
		LIMIT $3
	`, adminID, unreadOnly, limit)
	if err != nil {
		return nil, fmt.Errorf("list admin notifications: %w", err)
	}
	defer rows.Close()
	out := []AdminNotification{}
	for rows.Next() {
		var n AdminNotification
		if err := rows.Scan(&n.ID, &n.EventType, &n.Level, &n.Title, &n.Body, &n.LeadUUID, &n.Status, &n.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// CountUnreadAdminNotifications 当前管理员的未读通知数。
func (r *Repository) CountUnreadAdminNotifications(ctx context.Context, adminID int64) (int64, error) {
	var n int64
	err := r.pool.QueryRow(ctx, `
		SELECT count(*) FROM admin_notifications
		WHERE admin_id = $1 AND status = 'unread'
	`, adminID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count unread admin notifications: %w", err)
	}
	return n, nil
}

// MarkAdminNotificationRead 标记已读；只能操作自己的通知，重复标记幂等。
func (r *Repository) MarkAdminNotificationRead(ctx context.Context, adminID, id int64) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE admin_notifications
		SET status = 'read', read_at = now()
		WHERE id = $1 AND admin_id = $2 AND status = 'unread'
	`, id, adminID)
	if err != nil {
		return fmt.Errorf("mark admin notification read: %w", err)
	}
	if tag.RowsAffected() > 0 {
		return nil
	}
	var exists bool
	err = r.pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM admin_notifications WHERE id = $1 AND admin_id = $2)
	`, id, adminID).Scan(&exists)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotificationNotFound
		}
		return fmt.Errorf("check admin notification: %w", err)
	}
	if !exists {
		return ErrNotificationNotFound
	}
	return nil
}

// ---- Service：站内通知 ----

// ListNotifications 当前管理员的站内通知列表与未读数。
func (s *Service) ListNotifications(ctx context.Context, adminID int64, unreadOnly bool, limit int) (AdminNotificationListVO, error) {
	rows, err := s.repo.ListAdminNotifications(ctx, adminID, unreadOnly, limit)
	if err != nil {
		return AdminNotificationListVO{}, err
	}
	unread, err := s.repo.CountUnreadAdminNotifications(ctx, adminID)
	if err != nil {
		return AdminNotificationListVO{}, err
	}
	out := AdminNotificationListVO{
		Items:  make([]AdminNotificationVO, 0, len(rows)),
		Unread: unread,
	}
	for _, n := range rows {
		out.Items = append(out.Items, AdminNotificationVO{
			ID:        n.ID,
			EventType: n.EventType,
			Level:     n.Level,
			Title:     n.Title,
			Body:      n.Body,
			LeadUUID:  n.LeadUUID,
			Status:    n.Status,
			CreatedAt: n.CreatedAt,
		})
	}
	return out, nil
}

// MarkNotificationRead 标记通知已读（仅限当前管理员自己的通知）。
func (s *Service) MarkNotificationRead(ctx context.Context, adminID, id int64) error {
	return s.repo.MarkAdminNotificationRead(ctx, adminID, id)
}
