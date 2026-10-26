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

// TestRunCreatesSchemaAndSeed spielt die Migrationen in eine temporaere
// Datenbank ein, prueft Schema und Seed und verifiziert Idempotenz.
func TestRunCreatesSchemaAndSeed(t *testing.T) {
	baseDSN := testDSN(t)

	database := fmt.Sprintf("anchor_migrate_test_%d", time.Now().UnixNano()%1e9)
	admin, err := sql.Open("pgx", baseDSN)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = admin.Close() }()

	adminCtx, cancelAdmin := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelAdmin()

	if _, err := admin.ExecContext(adminCtx, "CREATE DATABASE "+database); err != nil {
		t.Skipf("Testdatenbank kann nicht angelegt werden (Rechte?): %v", err)
	}
	t.Cleanup(func() {
		if _, dropErr := admin.ExecContext(context.Background(), "DROP DATABASE IF EXISTS "+database+" WITH (FORCE)"); dropErr != nil {
			t.Logf("DROP DATABASE %s: %v", database, dropErr)
		}
	})

	targetDSN := withDatabase(t, baseDSN, database)

	version, err := migrationsrunner.Run(context.Background(), targetDSN)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if version != 2 {
		t.Fatalf("Schemaversion = %d, erwartet 2", version)
	}

	target, err := sql.Open("pgx", targetDSN)
	if err != nil {
		t.Fatalf("open target: %v", err)
	}
	defer func() { _ = target.Close() }()

	var entries int
	queryCtx, cancelQuery := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelQuery()

	if err := target.QueryRowContext(queryCtx, "SELECT count(*) FROM example_entries").Scan(&entries); err != nil {
		t.Fatalf("count example_entries: %v", err)
	}
	if entries != 5 {
		t.Fatalf("Seed-Zeilen = %d, erwartet 5", entries)
	}

	var appliedVersion int
	var dirty bool
	if err := target.QueryRowContext(queryCtx, "SELECT version, dirty FROM "+migrationsrunner.MigrationsTable).Scan(&appliedVersion, &dirty); err != nil {
		t.Fatalf("schema_migrations: %v", err)
	}
	if appliedVersion != 2 || dirty {
		t.Fatalf("schema_migrations = (version %d, dirty %t), erwartet (2, false)", appliedVersion, dirty)
	}

	// Zweiter Lauf darf nichts veraendern (ErrNoChange wird intern geschluckt).
	second, err := migrationsrunner.Run(context.Background(), targetDSN)
	if err != nil {
		t.Fatalf("zweiter Run: %v", err)
	}
	if second != 2 {
		t.Fatalf("Schemaversion nach zweitem Run = %d, erwartet 2", second)
	}
}

func TestRunRejectsEmptyDSN(t *testing.T) {
	if _, err := migrationsrunner.Run(context.Background(), ""); err == nil {
		t.Fatal("leere DSN wurde akzeptiert")
	} else if !strings.Contains(err.Error(), "empty dsn") {
		t.Fatalf("unerwarteter Fehler: %v", err)
	}
}
