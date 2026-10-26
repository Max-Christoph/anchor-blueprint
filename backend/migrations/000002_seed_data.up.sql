-- 000002_seed_data (up)

INSERT INTO example_entries (name, category, amount_cents) VALUES
    ('Onboarding-Automatisierung', 'platform',  125000),
    ('Reporting-Pipeline',         'data',       84500),
    ('Support-Portal',             'product',   210000),
    ('Lizenz-Management',          'platform',   49900),
    ('Audit-Log-Archivierung',     'compliance', 15750)
ON CONFLICT (name) DO NOTHING;
