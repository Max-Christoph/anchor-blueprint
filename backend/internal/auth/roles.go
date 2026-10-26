package auth

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/Max-Christoph/anchor-blueprint/backend/internal/audit"
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

			recordDenial(r, identity, kind, values)

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

// recordDenial schreibt die 403-Entscheidung ins Audit-Log. Fehler beim
// Schreiben werden geloggt, aber nicht an den Client weitergegeben.
func recordDenial(r *http.Request, identity httpx.Identity, kind string, required []string) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	err := audit.RecorderFrom(r.Context()).Record(ctx, audit.Entry{
		RequestID:  httpx.RequestIDFrom(r.Context()),
		UserID:     identity.Subject,
		Action:     audit.ActionRBACDeny,
		Resource:   r.Method + " " + r.URL.Path,
		StatusCode: http.StatusForbidden,
		Details: map[string]any{
			"kind":           kind,
			"required":       required,
			"granted_roles":  identity.Roles,
			"granted_groups": identity.Groups,
			"auth_method":    identity.Method,
		},
	})
	if err != nil {
		slog.Warn("audit write failed", "action", audit.ActionRBACDeny, "error", err,
			"request_id", httpx.RequestIDFrom(r.Context()))
	}
}
