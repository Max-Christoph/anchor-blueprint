package api

import (
	"net/http"

	"github.com/Max-Christoph/anchor-blueprint/backend/internal/auth"
)

// NewRouter registriert alle Routen. Go 1.22+ ServeMux-Pattern mit Methoden.
// Auth (Deny-by-Default) liegt als Middleware-Kette in cmd/server/main.go davor;
// RBAC wird pro Route ueber auth.RequireRole / auth.RequireGroup ergaenzt.
func NewRouter(h *Handler) *http.ServeMux {
	mux := http.NewServeMux()

	// Oeffentlich: Liveness/Readiness (Kubernetes-/Docker-Standard) + Status
	mux.HandleFunc("GET /livez", h.Livez)
	mux.HandleFunc("GET /readyz", h.Readyz)
	mux.HandleFunc("GET /api/v1/status", h.Status)

	// Authentifiziert, keine Rollenpruefung
	mux.HandleFunc("GET /api/v1/me", h.Me)
	mux.HandleFunc("GET /api/v1/data", h.Data)
	mux.HandleFunc("POST /api/v1/process", h.Process)

	// RBAC
	mux.Handle("GET /api/v1/protected/operator",
		auth.RequireRole(roleAdmin, roleOperator)(http.HandlerFunc(h.OperatorAction)))
	mux.Handle("POST /api/v1/protected/admin/reset",
		auth.RequireRole(roleAdmin)(http.HandlerFunc(h.AdminReset)))
	mux.Handle("GET /api/v1/protected/admin/audit-logs",
		auth.RequireRole(roleAdmin)(http.HandlerFunc(h.AuditLogs)))

	// Fallback fuer alles andere.
	mux.HandleFunc("/", h.NotFound)

	return mux
}
