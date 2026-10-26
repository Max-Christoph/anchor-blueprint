// Package seed haelt Entwicklungs-Beispieldaten. Seed-Daten sind bewusst nicht
// Teil der Schemamigrationen (dort nur DDL); sie laufen entweder ueber das
// Command cmd/seed oder beim Start, wenn DB_SEED_DEV_DATA=true gesetzt ist.
package seed

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // registriert den "pgx"-Treiber fuer database/sql
)

// devEntries sind die Beispieldatensaetze fuer lokale Umgebungen.
var devEntries = []struct {
	Name        string
	Category    string
	AmountCents int64
}{
	{"Onboarding-Automatisierung", "platform", 125000},
	{"Reporting-Pipeline", "data", 84500},
	{"Support-Portal", "product", 210000},
	{"Lizenz-Management", "platform", 49900},
	{"Audit-Log-Archivierung", "compliance", 15750},
}

// DevData schreibt die Beispieldaten idempotent (ON CONFLICT DO NOTHING) und
// liefert die Anzahl der Zeilen in der Tabelle nach dem Lauf.
func DevData(ctx context.Context, dsn string) (int, error) {
	if dsn == "" {
		return 0, fmt.Errorf("seed: empty dsn")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return 0, fmt.Errorf("seed: open database: %w", err)
	}
	defer func() { _ = db.Close() }()

	execCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	for _, entry := range devEntries {
		if _, err := db.ExecContext(execCtx,
			`INSERT INTO example_entries (name, category, amount_cents)
			 VALUES ($1, $2, $3)
			 ON CONFLICT (name) DO NOTHING`,
			entry.Name, entry.Category, entry.AmountCents,
		); err != nil {
			return 0, fmt.Errorf("seed: insert %q: %w", entry.Name, err)
		}
	}

	var count int
	if err := db.QueryRowContext(execCtx, "SELECT count(*) FROM example_entries").Scan(&count); err != nil {
		return 0, fmt.Errorf("seed: count example_entries: %w", err)
	}
	return count, nil
}
