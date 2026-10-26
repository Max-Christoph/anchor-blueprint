package auth

import (
	"log/slog"
	"net/http"

	"github.com/Max-Christoph/anchor-blueprint/backend/internal/httpx"
)

// RequireRole laesst den Request nur durch, wenn die Identitaet mindestens eine
// der Rollen besitzt. Ohne Identitaet: 401 (Auth-Middleware fehlt), sonst 403.
func RequireRole(roles ...string) func(http.Handler) http.Handler {
	return require(roles, func(identity httpx.Identity) bool {
		return identity.HasAnyRole(roles...)
	}, "role")
}

// RequireGroup verlangt analog mindestens eine der Gruppen.
func RequireGroup(groups ...string) func(http.Handler) http.Handler {
	return require(groups, func(identity httpx.Identity) bool {
		return identity.HasAnyGroup(groups...)
	}, "group")
}

// require baut die gemeinsame RBAC-Middleware.
func require(values []string, allowed func(httpx.Identity) bool, kind string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			identity, ok := httpx.IdentityFrom(r.Context())
			if !ok {
				writeError(w, http.StatusUnauthorized, "unauthorized", "missing or invalid credentials")
				return
			}
			if allowed(identity) {
				next.ServeHTTP(w, r)
				return
			}

			slog.Warn("rbac rejected",
				"kind", kind,
				"required", values,
				"granted_roles", identity.Roles,
				"path", r.URL.Path,
				"user", identity.Subject,
				"request_id", httpx.RequestIDFrom(r.Context()),
			)
			writeError(w, http.StatusForbidden, "forbidden", "insufficient permissions")
		})
	}
}
