-- 0013_prospect_collect_schedule.sql
-- 批次 12：采集任务定时调度与失败重试。
-- lead_sources 增加采集间隔与调度状态；next_collect_at 为空表示立即到期。
-- 失败退避由 Ingestor 计算后写回 next_collect_at，consecutive_failures 记录连续失败次数。
-- 每次采集尝试落一条 prospect_collect_runs，供管理站查看运行记录。

ALTER TABLE lead_sources
    ADD COLUMN IF NOT EXISTS collect_interval_minutes integer NOT NULL DEFAULT 60
        CHECK (collect_interval_minutes BETWEEN 5 AND 1440),
    ADD COLUMN IF NOT EXISTS last_collected_at timestamptz,
    ADD COLUMN IF NOT EXISTS next_collect_at timestamptz,
    ADD COLUMN IF NOT EXISTS consecutive_failures integer NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS last_error text,
    ADD COLUMN IF NOT EXISTS last_error_at timestamptz;

CREATE INDEX IF NOT EXISTS lead_sources_next_collect_idx
    ON lead_sources (next_collect_at) WHERE enabled = true AND deleted_at IS NULL;

-- 采集运行记录：每次尝试（成功 / 失败）一行，用于排障与效果核对。
CREATE TABLE IF NOT EXISTS prospect_collect_runs (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    source_id     bigint NOT NULL REFERENCES lead_sources (id) ON DELETE CASCADE,
    status        text NOT NULL CHECK (status IN ('success', 'failed')),
    fetched       integer NOT NULL DEFAULT 0,
    ingested      integer NOT NULL DEFAULT 0,
    duplicate     integer NOT NULL DEFAULT 0,
    opted_out     integer NOT NULL DEFAULT 0,
    invalid       integer NOT NULL DEFAULT 0,
    failed        integer NOT NULL DEFAULT 0,
    error_message text,
    started_at    timestamptz NOT NULL,
    finished_at   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS prospect_collect_runs_source_idx
    ON prospect_collect_runs (source_id, started_at DESC);
CREATE INDEX IF NOT EXISTS prospect_collect_runs_started_idx
    ON prospect_collect_runs (started_at DESC);
