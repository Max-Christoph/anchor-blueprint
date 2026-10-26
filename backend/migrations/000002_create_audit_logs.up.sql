-- 000002_create_audit_logs (up)
-- Persistentes Audit-Log fuer sicherheitsrelevante Ereignisse (RBAC-Abweisungen,
-- administrative Aktionen). Nur DDL - Beispieldaten kommen aus dem Seed-Command.

CREATE TABLE IF NOT EXISTS audit_logs (
    id          BIGSERIAL PRIMARY KEY,
    "timestamp" TIMESTAMPTZ NOT NULL DEFAULT now(),
    request_id  TEXT        NOT NULL DEFAULT '',
    user_id     TEXT        NOT NULL DEFAULT '',
    action      TEXT        NOT NULL,
    resource    TEXT        NOT NULL DEFAULT '',
    status_code INTEGER     NOT NULL CHECK (status_code BETWEEN 100 AND 599),
    details     JSONB       NOT NULL DEFAULT '{}'::jsonb
);

CREATE INDEX IF NOT EXISTS idx_audit_logs_timestamp
    ON audit_logs ("timestamp" DESC);

CREATE INDEX IF NOT EXISTS idx_audit_logs_user
    ON audit_logs (user_id, "timestamp" DESC);

CREATE INDEX IF NOT EXISTS idx_audit_logs_action
    ON audit_logs (action, "timestamp" DESC);

COMMENT ON TABLE audit_logs IS 'Audit-Trail: RBAC-Abweisungen und administrative Aktionen.';
