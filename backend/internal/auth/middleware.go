package auth

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/Max-Christoph/anchor-blueprint/backend/internal/httpx"
)

// MiddlewareConfig konfiguriert die Auth-Middleware.
type MiddlewareConfig struct {
	Verifier Verifier
	// PublicPaths sind exakt diese Pfade und ohne Authentifizierung erreichbar.
	PublicPaths []string
}

// Middleware setzt Deny-by-Default durch: alles ausserhalb von PublicPaths
// erfordert eine gueltige Identitaet. Unbekannte Pfade liefern ohne
// Credentials 401 statt 404 (keine Pfad-Enumeration).
func Middleware(cfg MiddlewareConfig, logger *slog.Logger) func(http.Handler) http.Handler {
	publicPaths := make(map[string]struct{}, len(cfg.PublicPaths))
	for _, path := range cfg.PublicPaths {
		publicPaths[path] = struct{}{}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, ok := publicPaths[r.URL.Path]; ok {
				next.ServeHTTP(w, r)
				return
			}
			if r.Method == http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}

			credentials := Credentials{
				BearerToken: bearerToken(r),
				UserEmail:   r.Header.Get("X-User-Email"),
			}

			identity, err := cfg.Verifier.Verify(r.Context(), credentials)
			if err != nil {
				status := http.StatusUnauthorized
				code := "unauthorized"
				message := "missing or invalid credentials"
				if errors.Is(err, ErrKeyUnavailable) {
					status = http.StatusServiceUnavailable
					code = "auth_backend_unavailable"
					message = "authentication backend unavailable"
				}

				logger.Warn("authentication rejected",
					"auth_mode", cfg.Verifier.Mode(),
					"path", r.URL.Path,
					"reason", err.Error(),
					"remote_addr", r.RemoteAddr,
					"request_id", httpx.RequestIDFrom(r.Context()),
				)

				w.Header().Set("WWW-Authenticate", `Bearer realm="api"`)
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.Header().Set("Cache-Control", "no-store")
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"error":{"code":"` + code + `","message":"` + message + `"}}`))
				return
			}

			next.ServeHTTP(w, r.WithContext(httpx.WithIdentity(r.Context(), identity)))
		})
	}
}

// bearerToken extrahiert den Token aus dem Authorization-Header.
func bearerToken(r *http.Request) string {
	header := r.Header.Get("Authorization")
	if header == "" {
		return ""
	}
	scheme, value, found := strings.Cut(header, " ")
	if !found || !strings.EqualFold(scheme, "Bearer") {
		return ""
	}
	return strings.TrimSpace(value)
}
