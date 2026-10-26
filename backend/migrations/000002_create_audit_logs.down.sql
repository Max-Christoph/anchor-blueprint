-- 000002_create_audit_logs (down)

DROP INDEX IF EXISTS idx_audit_logs_action;
DROP INDEX IF EXISTS idx_audit_logs_user;
DROP INDEX IF EXISTS idx_audit_logs_timestamp;
DROP TABLE IF EXISTS audit_logs;
