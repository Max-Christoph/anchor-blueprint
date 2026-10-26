-- Initiales Schema. Wird von Postgres ausschliesslich beim ersten Start
-- eines leeren Datenverzeichnisses ausgefuehrt (docker-entrypoint-initdb.d).
-- Spaetere Aenderungen gehoeren in Migrationen (z. B. goose, atlas, golang-migrate).

CREATE TABLE IF NOT EXISTS example_entries (
    id           BIGSERIAL PRIMARY KEY,
    name         TEXT        NOT NULL UNIQUE,
    category     TEXT        NOT NULL,
    amount_cents BIGINT      NOT NULL CHECK (amount_cents >= 0),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_example_entries_category
    ON example_entries (category);

COMMENT ON TABLE example_entries IS 'Beispieltabelle fuer den Template-Datenpfad.';

INSERT INTO example_entries (name, category, amount_cents) VALUES
    ('Onboarding-Automatisierung', 'platform',  125000),
    ('Reporting-Pipeline',         'data',       84500),
    ('Support-Portal',             'product',   210000),
    ('Lizenz-Management',          'platform',   49900),
    ('Audit-Log-Archivierung',     'compliance', 15750)
ON CONFLICT (name) DO NOTHING;
