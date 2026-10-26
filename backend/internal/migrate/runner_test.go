package migrate_test

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	migrationsrunner "github.com/Max-Christoph/anchor-blueprint/backend/internal/migrate"
	"github.com/Max-Christoph/anchor-blueprint/backend/internal/seed"
)

// testDSN liefert die Basis-DSN; ohne erreichbare Testdatenbank wird uebersprungen.
func testDSN(t *testing.T) string {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL nicht gesetzt - Migrations-Integrationstest uebersprungen")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Skipf("Testdatenbank nicht nutzbar: %v", err)
	}
	defer func() { _ = db.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Skipf("Testdatenbank nicht erreichbar: %v", err)
	}

	return dsn
}

// withDatabase leitet aus der Basis-DSN eine DSN fuer eine andere Datenbank ab.
func withDatabase(t *testing.T, dsn, database string) string {
	t.Helper()

	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("DSN nicht parsebar: %v", err)
	}
	parsed.Path = "/" + database
	return parsed.String()
}

// newTempDatabase legt eine temporaere Datenbank an und raeumt sie am Ende auf.
func newTempDatabase(t *testing.T) string {
	t.Helper()

	baseDSN := testDSN(t)
	database := fmt.Sprintf("anchor_migrate_test_%d", time.Now().UnixNano()%1e9)

	admin, err := sql.Open("pgx", baseDSN)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = admin.Close() })

	adminCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := admin.ExecContext(adminCtx, "CREATE DATABASE "+database); err != nil {
		t.Skipf("Testdatenbank kann nicht angelegt werden (Rechte?): %v", err)
	}
	t.Cleanup(func() {
		if _, dropErr := admin.ExecContext(context.Background(),
			"DROP DATABASE IF EXISTS "+database+" WITH (FORCE)"); dropErr != nil {
			t.Logf("DROP DATABASE %s: %v", database, dropErr)
		}
	})

	return withDatabase(t, baseDSN, database)
}

func tableExists(t *testing.T, dsn, table string) bool {
	t.Helper()

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	var exists bool
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.QueryRowContext(ctx,
		"SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = $1)", table,
	).Scan(&exists); err != nil {
		t.Fatalf("tableExists(%s): %v", table, err)
	}
	return exists
}

// TestRunCreatesSchemaWithoutSeed prueft, dass die Migrationen ausschliesslich
// DDL anwenden: Schema vorhanden, aber keine Beispieldaten.
func TestRunCreatesSchemaWithoutSeed(t *testing.T) {
	dsn := newTempDatabase(t)

	version, err := migrationsrunner.Run(context.Background(), dsn)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if version != 2 {
		t.Fatalf("Schemaversion = %d, erwartet 2", version)
	}

	for _, table := range []string{"example_entries", "audit_logs", migrationsrunner.MigrationsTable} {
		if !tableExists(t, dsn, table) {
			t.Fatalf("Tabelle %s fehlt nach Migration", table)
		}
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var entries int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM example_entries").Scan(&entries); err != nil {
		t.Fatalf("count example_entries: %v", err)
	}
	if entries != 0 {
		t.Fatalf("Seed-Zeilen = %d, erwartet 0 (Migrationen liefern nur DDL)", entries)
	}

	var auditRows int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM audit_logs").Scan(&auditRows); err != nil {
		t.Fatalf("count audit_logs: %v", err)
	}
	if auditRows != 0 {
		t.Fatalf("audit_logs-Zeilen = %d, erwartet 0", auditRows)
	}

	// Zweiter Lauf darf nichts veraendern (ErrNoChange wird intern geschluckt).
	second, err := migrationsrunner.Run(context.Background(), dsn)
	if err != nil {
		t.Fatalf("zweiter Run: %v", err)
	}
	if second != 2 {
		t.Fatalf("Schemaversion nach zweitem Run = %d, erwartet 2", second)
	}
}

// TestSeedIsIdempotent prueft den separaten Seed-Mechanismus.
func TestSeedIsIdempotent(t *testing.T) {
	dsn := newTempDatabase(t)

	if _, err := migrationsrunner.Run(context.Background(), dsn); err != nil {
		t.Fatalf("Run: %v", err)
	}

	first, err := seed.DevData(context.Background(), dsn)
	if err != nil {
		t.Fatalf("erster Seed-Lauf: %v", err)
	}
	if first != 5 {
		t.Fatalf("Zeilen nach erstem Seed = %d, erwartet 5", first)
	}

	second, err := seed.DevData(context.Background(), dsn)
	if err != nil {
		t.Fatalf("zweiter Seed-Lauf: %v", err)
	}
	if second != 5 {
		t.Fatalf("Zeilen nach zweitem Seed = %d, erwartet 5 (idempotent)", second)
	}
}

// TestDownRemovesSchema prueft, dass die Migrationen sauber zurueckrollen.
func TestDownRemovesSchema(t *testing.T) {
	dsn := newTempDatabase(t)

	if _, err := migrationsrunner.Run(context.Background(), dsn); err != nil {
		t.Fatalf("Run: %v", err)
	}

	version, err := migrationsrunner.Down(context.Background(), dsn)
	if err != nil {
		t.Fatalf("Down: %v", err)
	}
	if version != 0 {
		t.Fatalf("Version nach Down = %d, erwartet 0", version)
	}

	for _, table := range []string{"example_entries", "audit_logs"} {
		if tableExists(t, dsn, table) {
			t.Fatalf("Tabelle %s existiert nach Down noch", table)
		}
	}

	// Nach dem Rollback muss ein erneutes Up wieder funktionieren.
	if _, err := migrationsrunner.Run(context.Background(), dsn); err != nil {
		t.Fatalf("Run nach Down: %v", err)
	}
	if !tableExists(t, dsn, "example_entries") {
		t.Fatal("example_entries fehlt nach erneutem Up")
	}
}

func TestRunRejectsEmptyDSN(t *testing.T) {
	if _, err := migrationsrunner.Run(context.Background(), ""); err == nil {
		t.Fatal("leere DSN wurde akzeptiert")
	} else if !strings.Contains(err.Error(), "empty dsn") {
		t.Fatalf("unerwarteter Fehler: %v", err)
	}
}
