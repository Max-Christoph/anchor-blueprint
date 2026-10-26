// Package store kapselt den Datenbankzugriff (pgxpool) hinter einem
// schlanken Repository. Handler kennen kein SQL.
package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Max-Christoph/anchor-blueprint/backend/internal/audit"
)

// Entry ist ein Beispiel-Datensatz aus der Tabelle example_entries.
type Entry struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Category    string    `json:"category"`
	AmountCents int64     `json:"amount_cents"`
	CreatedAt   time.Time `json:"created_at"`
}

// Store buendelt alle Datenbankoperationen.
type Store struct {
	pool *pgxpool.Pool
}

// New baut den Connection-Pool auf und prueft die Verbindung mit Retries, damit
// ein langsam startender Postgres-Container nicht sofort zum Abbruch fuehrt.
func New(ctx context.Context, dsn string, maxConns int32) (*Store, error) {
	poolConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	if maxConns > 0 {
		poolConfig.MaxConns = maxConns
	}
	poolConfig.MaxConnLifetime = 30 * time.Minute
	poolConfig.MaxConnIdleTime = 5 * time.Minute
	poolConfig.HealthCheckPeriod = 30 * time.Second

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	if err := pingWithRetry(ctx, pool, 6, 2*time.Second); err != nil {
		pool.Close()
		return nil, err
	}

	return &Store{pool: pool}, nil
}

func pingWithRetry(ctx context.Context, pool *pgxpool.Pool, attempts int, delay time.Duration) error {
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		lastErr = pool.Ping(pingCtx)
		cancel()
		if lastErr == nil {
			return nil
		}
		if ctx.Err() != nil {
			return fmt.Errorf("ping database: %w", ctx.Err())
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("ping database: %w", ctx.Err())
		case <-time.After(delay):
		}
	}
	return fmt.Errorf("ping database after %d attempts: %w", attempts, lastErr)
}

// Ping prueft die DB-Verbindung (Healthcheck).
func (s *Store) Ping(ctx context.Context) error {
	if err := s.pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}
	return nil
}

// ExampleEntries liefert die Beispiel-Datensaetze, limit wird hart begrenzt.
func (s *Store) ExampleEntries(ctx context.Context, limit int) ([]Entry, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	rows, err := s.pool.Query(ctx,
		`SELECT id, name, category, amount_cents, created_at
		   FROM example_entries
		  ORDER BY id
		  LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("query example_entries: %w", err)
	}
	defer rows.Close()

	entries := make([]Entry, 0, limit)
	for rows.Next() {
		var entry Entry
		if err := rows.Scan(&entry.ID, &entry.Name, &entry.Category, &entry.AmountCents, &entry.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan example_entries: %w", err)
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate example_entries: %w", err)
	}
	return entries, nil
}

// Close gibt alle Pool-Verbindungen frei.
func (s *Store) Close() {
	if s == nil || s.pool == nil {
		return
	}
	s.pool.Close()
}

// Record schreibt einen Audit-Eintrag nach audit_logs (Implementierung von
// audit.Recorder). Details landen als JSONB in der Tabelle.
func (s *Store) Record(ctx context.Context, entry audit.Entry) error {
	if s == nil || s.pool == nil {
		return fmt.Errorf("audit: store not initialised")
	}
	if entry.Action == "" {
		return fmt.Errorf("audit: action must not be empty")
	}

	details, err := json.Marshal(entry.Details)
	if err != nil {
		return fmt.Errorf("audit: marshal details: %w", err)
	}

	_, err = s.pool.Exec(ctx,
		`INSERT INTO audit_logs (request_id, user_id, action, resource, status_code, details)
		 VALUES ($1, $2, $3, $4, $5, $6::jsonb)`,
		entry.RequestID, entry.UserID, entry.Action, entry.Resource, entry.StatusCode, string(details),
	)
	if err != nil {
		return fmt.Errorf("audit: insert: %w", err)
	}
	return nil
}

// ListAuditLogs liefert die neuesten Audit-Eintraege (maximal 200).
func (s *Store) ListAuditLogs(ctx context.Context, limit int) ([]audit.Log, error) {
	if s == nil || s.pool == nil {
		return nil, fmt.Errorf("audit: store not initialised")
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	rows, err := s.pool.Query(ctx,
		`SELECT id, "timestamp", request_id, user_id, action, resource, status_code, details
		   FROM audit_logs
		  ORDER BY id DESC
		  LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("audit: query: %w", err)
	}
	defer rows.Close()

	logs := make([]audit.Log, 0, limit)
	for rows.Next() {
		var (
			entry   audit.Log
			details []byte
		)
		if err := rows.Scan(&entry.ID, &entry.Timestamp, &entry.RequestID, &entry.UserID,
			&entry.Action, &entry.Resource, &entry.StatusCode, &details); err != nil {
			return nil, fmt.Errorf("audit: scan: %w", err)
		}
		if len(details) > 0 {
			_ = json.Unmarshal(details, &entry.Details)
		}
		logs = append(logs, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("audit: iterate: %w", err)
	}
	return logs, nil
}
