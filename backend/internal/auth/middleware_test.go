package auth_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Max-Christoph/anchor-blueprint/backend/internal/auth"
	"github.com/Max-Christoph/anchor-blueprint/backend/internal/httpx"
)

const devEmail = "demo.user@example.com"

// testLogger unterdrueckt Log-Ausgaben in Tests.
func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

// protectedHandler liefert die Identitaet aus dem Context ueber Header zurueck.
func protectedHandler(t *testing.T) http.Handler {
	t.Helper()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		identity, ok := httpx.IdentityFrom(r.Context())
		if !ok {
			t.Error("Handler ohne Identitaet im Context aufgerufen")
		}
		w.Header().Set("X-Identity", identity.Subject)
		w.Header().Set("X-Method", identity.Method)
		w.WriteHeader(http.StatusOK)
	})
}

// okHandler antwortet ohne Identitaets-Assertion (public/Preflight).
func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func newDevVerifier(t *testing.T, mode string) *auth.DevHeaderVerifier {
	t.Helper()

	verifier, err := auth.NewDevHeaderVerifier(auth.DevHeaderConfig{
		AllowedEmailDomains: []string{"example.com"},
		StaticToken:         "dev-token",
		ModeName:            mode,
	})
	if err != nil {
		t.Fatalf("NewDevHeaderVerifier: %v", err)
	}
	return verifier
}

func TestMiddlewarePublicPathWithoutCredentials(t *testing.T) {
	signingKey, _ := testKeys(t)
	server := newJWKSServer(t)
	server.addKey(testKID, &signingKey.PublicKey)
	verifier := newVerifier(t, server, time.Minute)

	handler := auth.Middleware(auth.MiddlewareConfig{
		Verifier:    verifier,
		PublicPaths: []string{"/healthz"},
	}, testLogger())(okHandler())

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/healthz", http.NoBody))

	if recorder.Code != http.StatusOK {
		t.Fatalf("Status = %d, erwartet 200", recorder.Code)
	}
}

func TestMiddlewareRejectsMissingCredentials(t *testing.T) {
	signingKey, _ := testKeys(t)
	server := newJWKSServer(t)
	server.addKey(testKID, &signingKey.PublicKey)
	verifier := newVerifier(t, server, time.Minute)

	handler := auth.Middleware(auth.MiddlewareConfig{
		Verifier:    verifier,
		PublicPaths: []string{"/healthz"},
	}, testLogger())(protectedHandler(t))

	for _, path := range []string{"/api/v1/data", "/does-not-exist"} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequestWithContext(context.Background(), http.MethodGet, path, http.NoBody))

		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("%s: Status = %d, erwartet 401", path, recorder.Code)
		}
		if recorder.Header().Get("WWW-Authenticate") == "" {
			t.Errorf("%s: WWW-Authenticate fehlt", path)
		}
	}
}

func TestMiddlewareAcceptsValidJWT(t *testing.T) {
	signingKey, _ := testKeys(t)
	server := newJWKSServer(t)
	server.addKey(testKID, &signingKey.PublicKey)
	verifier := newVerifier(t, server, time.Minute)

	handler := auth.Middleware(auth.MiddlewareConfig{Verifier: verifier}, testLogger())(protectedHandler(t))

	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/data", http.NoBody)
	request.Header.Set("Authorization", "Bearer "+signToken(t, tokenOptions{kid: testKID, key: signingKey}))

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("Status = %d, erwartet 200", recorder.Code)
	}
	if subject := recorder.Header().Get("X-Identity"); subject != testSub {
		t.Fatalf("Identity = %q, erwartet %q", subject, testSub)
	}
	if method := recorder.Header().Get("X-Method"); method != auth.ModeJWT {
		t.Fatalf("Method = %q, erwartet %q", method, auth.ModeJWT)
	}
}

func TestMiddlewareOptionsBypass(t *testing.T) {
	signingKey, _ := testKeys(t)
	server := newJWKSServer(t)
	server.addKey(testKID, &signingKey.PublicKey)
	verifier := newVerifier(t, server, time.Minute)

	handler := auth.Middleware(auth.MiddlewareConfig{Verifier: verifier}, testLogger())(okHandler())

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequestWithContext(context.Background(), http.MethodOptions, "/api/v1/data", http.NoBody))

	if recorder.Code != http.StatusOK {
		t.Fatalf("Status = %d, erwartet 200 (Preflight)", recorder.Code)
	}
}

func TestDevHeaderMode(t *testing.T) {
	handler := auth.Middleware(auth.MiddlewareConfig{Verifier: newDevVerifier(t, auth.ModeDevHeader)},
		testLogger())(protectedHandler(t))

	tests := []struct {
		name       string
		header     string
		email      string
		wantStatus int
	}{
		{name: "erlaubte Domain", email: devEmail, wantStatus: http.StatusOK},
		{name: "fremde Domain", email: "attacker@evil.example", wantStatus: http.StatusUnauthorized},
		{name: "keine Angaben", wantStatus: http.StatusUnauthorized},
		{name: "statischer Bearer", header: "Bearer dev-token", wantStatus: http.StatusOK},
		{name: "falscher Bearer", header: "Bearer nope", wantStatus: http.StatusUnauthorized},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/data", http.NoBody)
			if testCase.header != "" {
				request.Header.Set("Authorization", testCase.header)
			}
			if testCase.email != "" {
				request.Header.Set("X-User-Email", testCase.email)
			}

			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)

			if recorder.Code != testCase.wantStatus {
				t.Fatalf("Status = %d, erwartet %d", recorder.Code, testCase.wantStatus)
			}
		})
	}
}

func TestJWTModeIgnoresDevHeader(t *testing.T) {
	signingKey, _ := testKeys(t)
	server := newJWKSServer(t)
	server.addKey(testKID, &signingKey.PublicKey)
	verifier := newVerifier(t, server, time.Minute)

	handler := auth.Middleware(auth.MiddlewareConfig{Verifier: verifier}, testLogger())(protectedHandler(t))

	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/data", http.NoBody)
	request.Header.Set("X-User-Email", devEmail)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("Status = %d, erwartet 401 (X-User-Email darf im JWT-Modus nicht greifen)", recorder.Code)
	}
}

func TestDevHeaderVerifierRequiresDomains(t *testing.T) {
	if _, err := auth.NewDevHeaderVerifier(auth.DevHeaderConfig{ModeName: auth.ModeDevHeader}); err == nil {
		t.Fatal("Konfiguration ohne Domains wurde akzeptiert")
	}
}
