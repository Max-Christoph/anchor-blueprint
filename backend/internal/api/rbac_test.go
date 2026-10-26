package api_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Max-Christoph/anchor-blueprint/backend/internal/api"
	"github.com/Max-Christoph/anchor-blueprint/backend/internal/auth"
	"github.com/Max-Christoph/anchor-blueprint/backend/internal/config"
)

// newTestChain baut die komplette Kette RequestID -> Auth -> Router mit
// Dev-Header-Verifier (Store und Worker bleiben nil, DB-Endpunkte werden hier
// nicht aufgerufen).
func newTestChain(t *testing.T) http.Handler {
	t.Helper()

	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))

	verifier, err := auth.NewDevHeaderVerifier(auth.DevHeaderConfig{
		AllowedEmailDomains: []string{"example.com"},
		DefaultRole:         "User",
		ModeName:            auth.ModeDevHeader,
	})
	if err != nil {
		t.Fatalf("NewDevHeaderVerifier: %v", err)
	}

	cfg := config.Config{AuthMode: auth.ModeDevHeader, Env: "test", Version: "test"}
	handler := api.NewHandler(cfg, nil, nil, logger)

	return auth.Middleware(auth.MiddlewareConfig{
		Verifier:    verifier,
		PublicPaths: []string{"/healthz", "/api/v1/status"},
	}, logger)(api.NewRouter(handler))
}

func doRequest(chain http.Handler, method, path, roles string) *httptest.ResponseRecorder {
	request := httptest.NewRequestWithContext(context.Background(), method, path, http.NoBody)
	if roles != "-" {
		request.Header.Set("X-User-Email", "max@example.com")
	}
	if roles != "" && roles != "-" {
		request.Header.Set("X-User-Roles", roles)
	}

	recorder := httptest.NewRecorder()
	chain.ServeHTTP(recorder, request)
	return recorder
}

func TestRBACStatuscodes(t *testing.T) {
	chain := newTestChain(t)

	tests := []struct {
		name   string
		method string
		path   string
		roles  string
		want   int
	}{
		{name: "Operator ohne Credentials", method: http.MethodGet, path: "/api/v1/protected/operator", roles: "-", want: http.StatusUnauthorized},
		{name: "Operator mit Default-Rolle User", method: http.MethodGet, path: "/api/v1/protected/operator", roles: "", want: http.StatusForbidden},
		{name: "Operator mit fremder Rolle", method: http.MethodGet, path: "/api/v1/protected/operator", roles: "Viewer", want: http.StatusForbidden},
		{name: "Operator mit Rolle Operator", method: http.MethodGet, path: "/api/v1/protected/operator", roles: "Operator", want: http.StatusOK},
		{name: "Operator mit Rolle Admin", method: http.MethodGet, path: "/api/v1/protected/operator", roles: "Admin", want: http.StatusOK},
		{name: "Operator mit mehreren Rollen", method: http.MethodGet, path: "/api/v1/protected/operator", roles: "Viewer,Operator", want: http.StatusOK},
		{name: "Admin-Reset ohne Credentials", method: http.MethodPost, path: "/api/v1/protected/admin/reset", roles: "-", want: http.StatusUnauthorized},
		{name: "Admin-Reset mit Operator", method: http.MethodPost, path: "/api/v1/protected/admin/reset", roles: "Operator", want: http.StatusForbidden},
		{name: "Admin-Reset mit Admin", method: http.MethodPost, path: "/api/v1/protected/admin/reset", roles: "Admin", want: http.StatusOK},
		{name: "Admin-Reset mit Admin unter mehreren", method: http.MethodPost, path: "/api/v1/protected/admin/reset", roles: "Viewer,Admin", want: http.StatusOK},
		{name: "Unbekannter Pfad mit Credentials", method: http.MethodGet, path: "/api/v1/does-not-exist", roles: "Admin", want: http.StatusNotFound},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			recorder := doRequest(chain, testCase.method, testCase.path, testCase.roles)
			if recorder.Code != testCase.want {
				t.Fatalf("Status = %d, erwartet %d (Body: %s)", recorder.Code, testCase.want, recorder.Body.String())
			}
		})
	}
}

func TestRBACForbiddenBody(t *testing.T) {
	chain := newTestChain(t)
	recorder := doRequest(chain, http.MethodPost, "/api/v1/protected/admin/reset", "Operator")

	var payload struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Fehlerkoerper nicht lesbar: %v (%s)", err, recorder.Body.String())
	}
	if payload.Error.Code != "forbidden" {
		t.Fatalf("Code = %q, erwartet forbidden", payload.Error.Code)
	}
}

func TestAdminResetResponse(t *testing.T) {
	chain := newTestChain(t)
	recorder := doRequest(chain, http.MethodPost, "/api/v1/protected/admin/reset", "Admin")

	var payload struct {
		Reset       bool   `json:"reset"`
		PerformedBy string `json:"performed_by"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Antwort nicht lesbar: %v", err)
	}
	if !payload.Reset {
		t.Fatal("reset = false, erwartet true")
	}
	if payload.PerformedBy != "max@example.com" {
		t.Fatalf("performed_by = %q, erwartet max@example.com", payload.PerformedBy)
	}
}

func TestMeEndpoint(t *testing.T) {
	chain := newTestChain(t)

	unauthorized := doRequest(chain, http.MethodGet, "/api/v1/me", "-")
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("ohne Credentials: Status = %d, erwartet 401", unauthorized.Code)
	}

	recorder := doRequest(chain, http.MethodGet, "/api/v1/me", "Operator,Admin")
	if recorder.Code != http.StatusOK {
		t.Fatalf("Status = %d, erwartet 200", recorder.Code)
	}

	var payload struct {
		Subject    string   `json:"subject"`
		Roles      []string `json:"roles"`
		AuthMethod string   `json:"auth_method"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Antwort nicht lesbar: %v", err)
	}
	if payload.Subject != "max@example.com" {
		t.Fatalf("subject = %q", payload.Subject)
	}
	if len(payload.Roles) != 2 || payload.Roles[0] != "Operator" {
		t.Fatalf("roles = %v, erwartet [Operator Admin]", payload.Roles)
	}
	if payload.AuthMethod != auth.ModeDevHeader {
		t.Fatalf("auth_method = %q", payload.AuthMethod)
	}
}
