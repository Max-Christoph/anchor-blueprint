package api_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Max-Christoph/anchor-blueprint/backend/internal/api"
	"github.com/Max-Christoph/anchor-blueprint/backend/internal/config"
	"github.com/Max-Christoph/anchor-blueprint/backend/internal/store"
	"github.com/Max-Christoph/anchor-blueprint/backend/internal/worker"
)

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestLivezNeedsNoDependencies(t *testing.T) {
	handler := api.NewHandler(config.Config{Version: "test"}, nil, nil, quietLogger())

	recorder := httptest.NewRecorder()
	handler.Livez(recorder, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/livez", http.NoBody))

	if recorder.Code != http.StatusOK {
		t.Fatalf("Status = %d, erwartet 200 (Liveness ohne Abhaengigkeiten)", recorder.Code)
	}

	var payload struct {
		Status  string `json:"status"`
		Version string `json:"version"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Antwort nicht lesbar: %v", err)
	}
	if payload.Status != "ok" || payload.Version != "test" {
		t.Fatalf("Payload = %+v", payload)
	}
}

func TestReadyzWithoutDependencies(t *testing.T) {
	handler := api.NewHandler(config.Config{Version: "test"}, nil, nil, quietLogger())

	recorder := httptest.NewRecorder()
	handler.Readyz(recorder, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/readyz", http.NoBody))

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("Status = %d, erwartet 503 ohne Store/Worker", recorder.Code)
	}
}

// TestReadyzWithDependencies braucht eine erreichbare Testdatenbank.
func TestReadyzWithDependencies(t *testing.T) {
	dsn := testDatabaseDSN(t)
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL nicht gesetzt - Readiness-Integrationstest uebersprungen")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repository, err := store.New(ctx, dsn, 4)
	if err != nil {
		t.Skipf("Testdatenbank nicht nutzbar: %v", err)
	}
	defer repository.Close()

	workerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer workerServer.Close()

	handler := api.NewHandler(config.Config{Version: "test"}, repository,
		worker.New(workerServer.URL, time.Second), quietLogger())

	recorder := httptest.NewRecorder()
	handler.Readyz(recorder, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/readyz", http.NoBody))

	if recorder.Code != http.StatusOK {
		t.Fatalf("Status = %d, erwartet 200 (Body: %s)", recorder.Code, recorder.Body.String())
	}

	var payload struct {
		Status string `json:"status"`
		Checks struct {
			Database string `json:"database"`
			Worker   string `json:"worker"`
		} `json:"checks"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Antwort nicht lesbar: %v", err)
	}
	if payload.Status != "ready" || payload.Checks.Database != "up" || payload.Checks.Worker != "up" {
		t.Fatalf("Payload = %+v", payload)
	}

	// Worker faellt aus -> 503 mit worker=down.
	brokenWorker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer brokenWorker.Close()

	broken := api.NewHandler(config.Config{Version: "test"}, repository,
		worker.New(brokenWorker.URL, time.Second), quietLogger())

	brokenRecorder := httptest.NewRecorder()
	broken.Readyz(brokenRecorder, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/readyz", http.NoBody))
	if brokenRecorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("Status bei Worker-Ausfall = %d, erwartet 503", brokenRecorder.Code)
	}
	if body := brokenRecorder.Body.String(); !strings.Contains(body, `"worker":"down"`) {
		t.Fatalf("Payload ohne worker=down: %s", body)
	}
}

// TestHealthzIsGone stellt sicher, dass der alte Endpunkt entfernt ist.
func TestHealthzIsGone(t *testing.T) {
	handler := api.NewHandler(config.Config{Version: "test"}, nil, nil, quietLogger())
	router := api.NewRouter(handler)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/healthz", http.NoBody))

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("Status = %d, erwartet 404 (/healthz wurde ersetzt)", recorder.Code)
	}
}

// testDatabaseDSN liefert die Basis-DSN oder ueberspringt den Test.
func testDatabaseDSN(t *testing.T) string {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL nicht gesetzt - Integrationstest uebersprungen")
	}
	return dsn
}
