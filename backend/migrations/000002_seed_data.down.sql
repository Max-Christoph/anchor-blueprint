-- 000002_seed_data (down)

DELETE FROM example_entries WHERE name IN (
    'Onboarding-Automatisierung',
    'Reporting-Pipeline',
    'Support-Portal',
    'Lizenz-Management',
    'Audit-Log-Archivierung'
);
