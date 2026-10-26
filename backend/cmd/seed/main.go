// Command seed schreibt Entwicklungs-Beispieldaten in die Datenbank.
// Aufruf: go run ./cmd/seed  (oder /usr/local/bin/seed im Container).
// Voraussetzung: die Schemamigrationen sind gelaufen (DDL kommt nicht von hier).
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/Max-Christoph/anchor-blueprint/backend/internal/config"
	"github.com/Max-Christoph/anchor-blueprint/backend/internal/seed"
)

func main() {
	if err := run(); err != nil {
		slog.Error("seed failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	count, err := seed.DevData(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}

	slog.Info("dev seed applied", "rows_in_table", count)
	return nil
}
