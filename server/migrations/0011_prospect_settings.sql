-- 0011_prospect_settings.sql
-- 自主获客运行时配置（key-value）：当前用于全局停机开关 global_halt。
-- 停机状态持久化，服务重启后保留；默认 false（不停机），主链路零影响。

CREATE TABLE IF NOT EXISTS prospect_settings (
    key         text PRIMARY KEY,
    value       text NOT NULL,
    updated_at  timestamptz NOT NULL DEFAULT now(),
    updated_by  bigint
);

INSERT INTO prospect_settings (key, value)
VALUES ('global_halt', 'false')
ON CONFLICT (key) DO NOTHING;

DROP TRIGGER IF EXISTS prospect_settings_set_updated_at ON prospect_settings;
CREATE TRIGGER prospect_settings_set_updated_at BEFORE UPDATE ON prospect_settings
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
