-- 0016_notification_outbox_delivery.sql
-- 批次 17：微信模板外发发送记录与重试。
-- 失败记录进入 retrying 并按 5/10/20/40/60 分钟退避，达到上限转 failed；
-- 发送成功记录 sent_at，管理站可查询记录并对 failed / not_configured 手动重试。

ALTER TABLE notification_outbox
    ADD COLUMN IF NOT EXISTS sent_at timestamptz;

ALTER TABLE notification_outbox
    DROP CONSTRAINT IF EXISTS notification_outbox_status_check;
ALTER TABLE notification_outbox
    ADD CONSTRAINT notification_outbox_status_check
    CHECK (status IN ('pending', 'retrying', 'sent', 'failed', 'not_configured'));

CREATE INDEX IF NOT EXISTS notification_outbox_status_updated_idx
    ON notification_outbox (status, updated_at DESC);
