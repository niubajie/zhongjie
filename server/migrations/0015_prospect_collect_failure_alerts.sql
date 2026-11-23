-- 0015_prospect_collect_failure_alerts.sql
-- 批次16：信息源连续采集失败告警。
-- 复用 admin_notifications；collect_run_id 保证同一失败运行记录对每个管理员只通知一次。

ALTER TABLE admin_notifications
    DROP CONSTRAINT IF EXISTS admin_notifications_event_type_check,
    DROP CONSTRAINT IF EXISTS admin_notifications_level_check,
    ADD COLUMN IF NOT EXISTS collect_run_id bigint REFERENCES prospect_collect_runs (id) ON DELETE CASCADE;

ALTER TABLE admin_notifications
    ADD CONSTRAINT admin_notifications_event_type_check
        CHECK (event_type IN ('prospect_followup_due', 'prospect_followup_overdue', 'prospect_collect_failed')),
    ADD CONSTRAINT admin_notifications_level_check
        CHECK (level IN ('due_3d', 'overdue_7d', 'error'));

CREATE UNIQUE INDEX IF NOT EXISTS admin_notifications_admin_collect_run_uq
    ON admin_notifications (admin_id, collect_run_id) WHERE collect_run_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS admin_notifications_collect_run_idx
    ON admin_notifications (collect_run_id) WHERE collect_run_id IS NOT NULL;
