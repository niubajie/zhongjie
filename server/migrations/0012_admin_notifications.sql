-- 0012_admin_notifications.sql
-- 批次11：跟进提醒的站内通知。按活跃管理员分发，同一提醒同一管理员只保留一条。
-- 微信模板外发仍复用 notification_outbox（由 ADMIN_NOTIFY_USER_ID 可选开启）。

CREATE TABLE IF NOT EXISTS admin_notifications (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    admin_id    bigint NOT NULL REFERENCES admin_users (id) ON DELETE CASCADE,
    event_type  text NOT NULL CHECK (event_type IN ('prospect_followup_due', 'prospect_followup_overdue')),
    level       text NOT NULL CHECK (level IN ('due_3d', 'overdue_7d')),
    title       text NOT NULL,
    body        text NOT NULL,
    lead_id     bigint REFERENCES prospect_leads (id) ON DELETE CASCADE,
    reminder_id bigint REFERENCES prospect_reminders (id) ON DELETE CASCADE,
    status      text NOT NULL DEFAULT 'unread' CHECK (status IN ('unread', 'read')),
    created_at  timestamptz NOT NULL DEFAULT now(),
    read_at     timestamptz
);

CREATE UNIQUE INDEX IF NOT EXISTS admin_notifications_admin_reminder_uq
    ON admin_notifications (admin_id, reminder_id) WHERE reminder_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS admin_notifications_admin_status_idx
    ON admin_notifications (admin_id, status, created_at DESC);
