-- 000001_init_schema (up)
-- Grundschema der Beispiel-Domaene. Migrationen laufen je Datei in einer
-- Transaktion (Postgres-Treiber von golang-migrate).

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
