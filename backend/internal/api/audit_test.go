package api_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/Max-Christoph/anchor-blueprint/backend/internal/api"
	"github.com/Max-Christoph/anchor-blueprint/backend/internal/auth"
	"github.com/Max-Christoph/anchor-blueprint/backend/internal/config"
	"github.com/Max-Christoph/anchor-blueprint/backend/internal/middleware"
	migrationsrunner "github.com/Max-Christoph/anchor-blueprint/backend/internal/migrate"
	"github.com/Max-Christoph/anchor-blueprint/backend/internal/store"
)

// auditTestDatenbank legt eine migrierte temporaere Datenbank an.
func auditTestDatenbank(t *testing.T) string {
	t.Helper()

	baseDSN := os.Getenv("TEST_DATABASE_URL")
	if baseDSN == "" {
		t.Skip("TEST_DATABASE_URL nicht gesetzt - Audit-Integrationstest uebersprungen")
	}

	admin, err := sql.Open("pgx", baseDSN)
	if err != nil {
		t.Skipf("Testdatenbank nicht nutzbar: %v", err)
	}
	t.Cleanup(func() { _ = admin.Close() })

	pingCtx, cancelPing := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancelPing()
	if err := admin.PingContext(pingCtx); err != nil {
		t.Skipf("Testdatenbank nicht erreichbar: %v", err)
	}

	database := fmt.Sprintf("anchor_audit_test_%d", time.Now().UnixNano()%1e9)
	createCtx, cancelCreate := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelCreate()
	if _, err := admin.ExecContext(createCtx, "CREATE DATABASE "+database); err != nil {
		t.Skipf("Testdatenbank kann nicht angelegt werden (Rechte?): %v", err)
	}
	t.Cleanup(func() {
		if _, dropErr := admin.ExecContext(context.Background(),
			"DROP DATABASE IF EXISTS "+database+" WITH (FORCE)"); dropErr != nil {
			t.Logf("DROP DATABASE %s: %v", database, dropErr)
		}
	})

	parsed, err := url.Parse(baseDSN)
	if err != nil {
		t.Fatalf("DSN nicht parsebar: %v", err)
	}
	parsed.Path = "/" + database
	dsn := parsed.String()

	if _, err := migrationsrunner.Run(context.Background(), dsn); err != nil {
		t.Fatalf("Migrationen: %v", err)
	}

	return dsn
}

// newAuditedChain baut die Kette mit echtem Store (Audit-Recorder).
func newAuditedChain(t *testing.T, dsn string) http.Handler {
	t.Helper()

	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	repository, err := store.New(ctx, dsn, 4)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(repository.Close)

	verifier, err := auth.NewDevHeaderVerifier(auth.DevHeaderConfig{
		AllowedEmailDomains: []string{"example.com"},
		DefaultRole:         "User",
		ModeName:            auth.ModeDevHeader,
	})
	if err != nil {
		t.Fatalf("NewDevHeaderVerifier: %v", err)
	}

	cfg := config.Config{AuthMode: auth.ModeDevHeader, Env: "test", Version: "test"}
	handler := api.NewHandler(cfg, repository, nil, logger)

	// RequestID vor der Auth-Middleware: sorgt fuer die Request-ID im Audit-Log.
	return middleware.RequestID(auth.Middleware(auth.MiddlewareConfig{
		Verifier:    verifier,
		Recorder:    repository,
		PublicPaths: []string{"/livez", "/readyz", "/api/v1/status"},
	}, logger)(api.NewRouter(handler)))
}

// TestForbiddenRequestIsAudited verifiziert den Audit-Eintrag nach einer
// RBAC-Abweisung (403) in audit_logs.
func TestForbiddenRequestIsAudited(t *testing.T) {
	dsn := auditTestDatenbank(t)
	chain := newAuditedChain(t, dsn)

	request := httptest.NewRequestWithContext(context.Background(), http.MethodPost,
		"/api/v1/protected/admin/reset", http.NoBody)
	request.Header.Set("X-User-Email", "max@example.com")
	request.Header.Set("X-User-Roles", "Operator")
	request.Header.Set("X-Request-Id", "audit-test-403")

	recorder := httptest.NewRecorder()
	chain.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("Status = %d, erwartet 403", recorder.Code)
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var (
		action     string
		userID     string
		resource   string
		statusCode int
		details    []byte
	)
	err = db.QueryRowContext(ctx,
		`SELECT action, user_id, resource, status_code, details
		   FROM audit_logs
		  WHERE request_id = $1
		  ORDER BY id DESC
		  LIMIT 1`, "audit-test-403",
	).Scan(&action, &userID, &resource, &statusCode, &details)
	if err != nil {
		t.Fatalf("kein Audit-Eintrag fuer 403 gefunden: %v", err)
	}

	if action != "rbac.deny" {
		t.Fatalf("action = %q, erwartet rbac.deny", action)
	}
	if userID != "max@example.com" {
		t.Fatalf("user_id = %q, erwartet max@example.com", userID)
	}
	if statusCode != http.StatusForbidden {
		t.Fatalf("status_code = %d, erwartet 403", statusCode)
	}
	if resource != "POST /api/v1/protected/admin/reset" {
		t.Fatalf("resource = %q", resource)
	}

	var parsed map[string]any
	if err := json.Unmarshal(details, &parsed); err != nil {
		t.Fatalf("details kein JSON: %v", err)
	}
	if _, ok := parsed["required"]; !ok {
		t.Fatalf("details ohne required: %v", parsed)
	}
}

// TestAdminResetIsAudited prueft den Audit-Eintrag fuer die sensitive Aktion.
func TestAdminResetIsAudited(t *testing.T) {
	dsn := auditTestDatenbank(t)
	chain := newAuditedChain(t, dsn)

	request := httptest.NewRequestWithContext(context.Background(), http.MethodPost,
		"/api/v1/protected/admin/reset", http.NoBody)
	request.Header.Set("X-User-Email", "max@example.com")
	request.Header.Set("X-User-Roles", "Admin")
	request.Header.Set("X-Request-Id", "audit-test-reset")

	recorder := httptest.NewRecorder()
	chain.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("Status = %d, erwartet 200", recorder.Code)
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var action string
	var statusCode int
	if err := db.QueryRowContext(ctx,
		`SELECT action, status_code FROM audit_logs WHERE request_id = $1 ORDER BY id DESC LIMIT 1`,
		"audit-test-reset",
	).Scan(&action, &statusCode); err != nil {
		t.Fatalf("kein Audit-Eintrag fuer admin.reset: %v", err)
	}
	if action != "admin.reset" || statusCode != http.StatusOK {
		t.Fatalf("Eintrag = (%q, %d), erwartet (admin.reset, 200)", action, statusCode)
	}
}

// TestAuditLogsEndpoint prueft den Admin-Endpunkt zum Lesen des Audit-Logs.
func TestAuditLogsEndpoint(t *testing.T) {
	dsn := auditTestDatenbank(t)
	chain := newAuditedChain(t, dsn)

	// Ein 403 erzeugen, damit es etwas zu lesen gibt.
	denied := httptest.NewRequestWithContext(context.Background(), http.MethodPost,
		"/api/v1/protected/admin/reset", http.NoBody)
	denied.Header.Set("X-User-Email", "max@example.com")
	denied.Header.Set("X-User-Roles", "Operator")
	chain.ServeHTTP(httptest.NewRecorder(), denied)

	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/protected/admin/audit-logs?limit=10", http.NoBody)
	request.Header.Set("X-User-Email", "max@example.com")
	request.Header.Set("X-User-Roles", "Admin")

	recorder := httptest.NewRecorder()
	chain.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("Status = %d, erwartet 200 (Body: %s)", recorder.Code, recorder.Body.String())
	}

	var payload struct {
		Items []struct {
			Action     string         `json:"action"`
			StatusCode int            `json:"status_code"`
			Details    map[string]any `json:"details"`
		} `json:"items"`
		Count int `json:"count"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Antwort nicht lesbar: %v", err)
	}
	if payload.Count == 0 || len(payload.Items) == 0 {
		t.Fatal("Audit-Log leer, erwartet mindestens einen Eintrag")
	}
	if payload.Items[0].Action == "" {
		t.Fatalf("Eintrag ohne action: %+v", payload.Items[0])
	}

	// Lesezugriffe duerfen die Audit-Tabelle nicht fuellen (Loop-Schutz).
	var auditReadRows int
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	countCtx, cancelCount := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelCount()
	if err := db.QueryRowContext(countCtx,
		"SELECT count(*) FROM audit_logs WHERE action = 'audit.read'").Scan(&auditReadRows); err != nil {
		t.Fatalf("count audit.read: %v", err)
	}
	if auditReadRows != 0 {
		t.Fatalf("audit.read-Zeilen = %d, erwartet 0 (Lesezugriff gehoert ins App-Log)", auditReadRows)
	}

	// Ohne Admin-Rolle ist der Endpunkt gesperrt.
	deniedRead := httptest.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/protected/admin/audit-logs", http.NoBody)
	deniedRead.Header.Set("X-User-Email", "max@example.com")
	deniedRead.Header.Set("X-User-Roles", "Operator")

	deniedRecorder := httptest.NewRecorder()
	chain.ServeHTTP(deniedRecorder, deniedRead)
	if deniedRecorder.Code != http.StatusForbidden {
		t.Fatalf("Audit-Log-Zugriff mit Operator: Status = %d, erwartet 403", deniedRecorder.Code)
	}
}
