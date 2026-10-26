package middleware

import (
	"context"
	"crypto/subtle"
	"log/slog"
	"net/http"
	"net/mail"
	"strings"
)

// Identity beschreibt den authentifizierten Aufrufer.
type Identity struct {
	Subject string
	Method  string
}

// IdentityFrom liest die Identitaet aus dem Context.
func IdentityFrom(ctx context.Context) (Identity, bool) {
	identity, ok := ctx.Value(identityKey).(Identity)
	return identity, ok
}

// AuthConfig konfiguriert den Auth-Stub.
type AuthConfig struct {
	// BearerToken wird gegen den Authorization-Header geprueft.
	BearerToken string
	// AllowedEmailDomains begrenzt per X-User-Email akzeptierte Domains.
	AllowedEmailDomains []string
	// PublicPaths sind exakt diese Pfade und ohne Auth erreichbar.
	PublicPaths []string
}

// Auth ist ein Auth-STUB fuer lokale Entwicklung: Bearer-Token oder
// X-User-Email-Header. Deny-by-Default: alles, was nicht explizit public ist,
// erfordert eine Identitaet. In Produktion durch echte Token-Validierung
// (OIDC/JWT, mTLS) ersetzen - der Header-Pfad ist nicht faelschungssicher.
func Auth(cfg AuthConfig, logger *slog.Logger) func(http.Handler) http.Handler {
	publicPaths := make(map[string]struct{}, len(cfg.PublicPaths))
	for _, path := range cfg.PublicPaths {
		publicPaths[path] = struct{}{}
	}
	allowedDomains := make(map[string]struct{}, len(cfg.AllowedEmailDomains))
	for _, domain := range cfg.AllowedEmailDomains {
		allowedDomains[strings.ToLower(domain)] = struct{}{}
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

			identity, reason := identify(r, cfg.BearerToken, allowedDomains)
			if reason != "" {
				logger.Warn("authentication rejected",
					"path", r.URL.Path,
					"reason", reason,
					"remote_addr", r.RemoteAddr,
					"request_id", RequestIDFrom(r.Context()),
				)
				w.Header().Set("WWW-Authenticate", `Bearer realm="api"`)
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":{"code":"unauthorized","message":"missing or invalid credentials"}}`))
				return
			}

			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), identityKey, identity)))
		})
	}
}

// identify prueft Bearer-Token und X-User-Email-Header. Rueckgabe: Identitaet
// oder ein Grund (leer = authentifiziert).
func identify(r *http.Request, bearerToken string, allowedDomains map[string]struct{}) (Identity, string) {
	if header := r.Header.Get("Authorization"); header != "" {
		scheme, value, found := strings.Cut(header, " ")
		if !found || !strings.EqualFold(scheme, "Bearer") || strings.TrimSpace(value) == "" {
			return Identity{}, "malformed authorization header"
		}
		if subtle.ConstantTimeCompare([]byte(strings.TrimSpace(value)), []byte(bearerToken)) != 1 {
			return Identity{}, "invalid bearer token"
		}
		return Identity{Subject: "service-account", Method: "bearer"}, ""
	}

	if email := strings.TrimSpace(r.Header.Get("X-User-Email")); email != "" {
		address, err := mail.ParseAddress(email)
		if err != nil || address.Address != email {
			return Identity{}, "malformed user email"
		}
		_, domain, found := strings.Cut(strings.ToLower(address.Address), "@")
		if !found || domain == "" {
			return Identity{}, "user email without domain"
		}
		if _, ok := allowedDomains[domain]; !ok {
			return Identity{}, "email domain not allowed"
		}
		return Identity{Subject: address.Address, Method: "header"}, ""
	}

	return Identity{}, "no credentials"
}
