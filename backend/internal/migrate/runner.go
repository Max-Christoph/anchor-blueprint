// Package migrate fuehrt die eingebetteten golang-migrate-Migrationen beim
// Start des Backends aus. Jede Migration laeuft in einer eigenen Transaktion
// (Postgres-Treiber), der Zustand steht in der Tabelle schema_migrations.
package migrate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/golang-migrate/migrate/v4"
	pgxmigrate "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/jackc/pgx/v5/stdlib" // registriert den "pgx"-Treiber fuer database/sql

	"github.com/Max-Christoph/anchor-blueprint/backend/migrations"
)

// MigrationsTable ist die Verwaltungstabelle von golang-migrate.
const MigrationsTable = "schema_migrations"

// Run spielt alle ausstehenden Migrationen ein und liefert die danach
// gueltige Schemaversion. ErrNoChange ist kein Fehler.
func Run(ctx context.Context, dsn string) (uint, error) {
	if dsn == "" {
		return 0, errors.New("migrate: empty dsn")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return 0, fmt.Errorf("migrate: open database: %w", err)
	}
	defer func() { _ = db.Close() }()

	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		return 0, fmt.Errorf("migrate: ping database: %w", err)
	}

	source, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return 0, fmt.Errorf("migrate: load embedded migrations: %w", err)
	}

	driver, err := pgxmigrate.WithInstance(db, &pgxmigrate.Config{MigrationsTable: MigrationsTable})
	if err != nil {
		return 0, fmt.Errorf("migrate: create driver: %w", err)
	}

	runner, err := migrate.NewWithInstance("iofs", source, "pgx", driver)
	if err != nil {
		return 0, fmt.Errorf("migrate: create runner: %w", err)
	}

	if err := runner.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return 0, fmt.Errorf("migrate: apply migrations: %w", err)
	}

	version, dirty, err := runner.Version()
	if err != nil {
		return 0, fmt.Errorf("migrate: read schema version: %w", err)
	}
	if dirty {
		return version, fmt.Errorf("migrate: schema is dirty at version %d (manual intervention required)", version)
	}

	return version, nil
}
