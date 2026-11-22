-- 批次 13：自主获客操作审计日志查询索引
-- 查询始终限定 action LIKE 'prospect.%'，text_pattern_ops 让前缀匹配走上索引。
CREATE INDEX IF NOT EXISTS audit_logs_action_created_idx
    ON audit_logs (action text_pattern_ops, created_at DESC);
