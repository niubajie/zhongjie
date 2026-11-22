-- 0011_prospect_followup.sql
-- 批次8：跟进提醒与超期标记。只新增，不修改历史迁移。
-- stale_at 只是超期标记，线索状态仍保持 following_up；跟进记录后清空。
-- prospect_reminders 由 Worker 幂等生成（同一线索同一级别最多一条未处理）。

ALTER TABLE prospect_leads ADD COLUMN IF NOT EXISTS stale_at timestamptz;

CREATE TABLE IF NOT EXISTS prospect_reminders (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    lead_id     bigint NOT NULL REFERENCES prospect_leads (id) ON DELETE CASCADE,
    level       text NOT NULL CHECK (level IN ('due_3d', 'overdue_7d')),
    status      text NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'resolved')),
    created_at  timestamptz NOT NULL DEFAULT now(),
    resolved_at timestamptz
);

CREATE UNIQUE INDEX IF NOT EXISTS prospect_reminders_open_uq
    ON prospect_reminders (lead_id, level) WHERE status = 'open';
CREATE INDEX IF NOT EXISTS prospect_reminders_open_idx
    ON prospect_reminders (status, created_at DESC) WHERE status = 'open';

CREATE INDEX IF NOT EXISTS prospect_leads_followup_scan_idx
    ON prospect_leads (status, created_at)
    WHERE deleted_at IS NULL AND status IN ('contacted', 'following_up');
