// Package api enthaelt Router und Handler der REST-API.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"runtime"
	"strconv"
	"time"

	"github.com/Max-Christoph/anchor-blueprint/backend/internal/audit"
	"github.com/Max-Christoph/anchor-blueprint/backend/internal/config"
	"github.com/Max-Christoph/anchor-blueprint/backend/internal/httpx"
	"github.com/Max-Christoph/anchor-blueprint/backend/internal/store"
	"github.com/Max-Christoph/anchor-blueprint/backend/internal/worker"
)

const (
	maxProcessBodyBytes = 1 << 20 // 1 MiB
	maxProcessValues    = 10000
	dbTimeout           = 3 * time.Second
	readinessTimeout    = 4 * time.Second
)

// Handler buendelt alle Abhaengigkeiten der HTTP-Schicht.
type Handler struct {
	cfg     config.Config
	store   *store.Store
	worker  *worker.Client
	logger  *slog.Logger
	started time.Time
}

// NewHandler erzeugt den Handler-Satz.
func NewHandler(cfg config.Config, st *store.Store, wc *worker.Client, logger *slog.Logger) *Handler {
	return &Handler{
		cfg:     cfg,
		store:   st,
		worker:  wc,
		logger:  logger,
		started: time.Now(),
	}
}

type liveResponse struct {
	Status  string `json:"status"`
	Service string `json:"service"`
	Version string `json:"version"`
}

// Livez ist der Liveness-Check: 200, solange der Go-Prozess laeuft.
// Bewusst ohne Abhaengigkeiten (keine DB-Abfrage) - fuer Docker/Kubernetes
// Liveness-Probes, damit ein DB-Ausfall keinen Container-Neustart ausloest.
func (h *Handler) Livez(w http.ResponseWriter, _ *http.Request) {
	respondJSON(h.logger, w, http.StatusOK, liveResponse{
		Status:  "ok",
		Service: "anchor-blueprint-backend",
		Version: h.cfg.Version,
	})
}

type readinessChecks struct {
	Database string `json:"database"`
	Worker   string `json:"worker"`
}

type readyResponse struct {
	Status  string          `json:"status"`
	Checks  readinessChecks `json:"checks"`
	Version string          `json:"version"`
}

// Readyz prueft die Abhaengigkeiten (DB-Pool und Worker) und antwortet mit
// 503, solange eine davon nicht erreichbar ist.
func (h *Handler) Readyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), readinessTimeout)
	defer cancel()

	checks := readinessChecks{Database: "up", Worker: "up"}
	ready := true

	if h.store == nil {
		checks.Database = "down"
		ready = false
	} else if err := h.store.Ping(ctx); err != nil {
		checks.Database = "down"
		ready = false
		h.logger.Warn("readiness: database not reachable", "error", err,
			"request_id", httpx.RequestIDFrom(r.Context()))
	}

	if h.worker == nil {
		checks.Worker = "down"
		ready = false
	} else if err := h.worker.Health(ctx); err != nil {
		checks.Worker = "down"
		ready = false
		h.logger.Warn("readiness: worker not reachable", "error", err,
			"request_id", httpx.RequestIDFrom(r.Context()))
	}

	status := http.StatusOK
	label := "ready"
	if !ready {
		status = http.StatusServiceUnavailable
		label = "not_ready"
	}

	respondJSON(h.logger, w, status, readyResponse{Status: label, Checks: checks, Version: h.cfg.Version})
}

type dbStatus struct {
	Status    string  `json:"status"`
	LatencyMS float64 `json:"latency_ms"`
}

type statusResponse struct {
	Service       string   `json:"service"`
	AuthMode      string   `json:"auth_mode"`
	Version       string   `json:"version"`
	Environment   string   `json:"environment"`
	UptimeSeconds float64  `json:"uptime_seconds"`
	GoVersion     string   `json:"go_version"`
	Goroutines    int      `json:"goroutines"`
	NumCPU        int      `json:"num_cpu"`
	MemoryAllocMB float64  `json:"memory_alloc_mb"`
	Database      dbStatus `json:"database"`
}

// Status liefert Systemmetriken (oeffentlich, ohne Secrets).
func (h *Handler) Status(w http.ResponseWriter, r *http.Request) {
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)

	ctx, cancel := context.WithTimeout(r.Context(), dbTimeout)
	defer cancel()

	dbState := dbStatus{Status: "up"}
	pingStart := time.Now()
	if err := h.store.Ping(ctx); err != nil {
		dbState.Status = "down"
		h.logger.Warn("status db ping failed", "error", err, "request_id", httpx.RequestIDFrom(r.Context()))
	}
	dbState.LatencyMS = math.Round(float64(time.Since(pingStart).Microseconds())/1000.0*100) / 100

	respondJSON(h.logger, w, http.StatusOK, statusResponse{
		Service:       "anchor-blueprint-backend",
		AuthMode:      h.cfg.AuthMode,
		Version:       h.cfg.Version,
		Environment:   h.cfg.Env,
		UptimeSeconds: math.Round(time.Since(h.started).Seconds()*100) / 100,
		GoVersion:     runtime.Version(),
		Goroutines:    runtime.NumGoroutine(),
		NumCPU:        runtime.NumCPU(),
		MemoryAllocMB: math.Round(float64(memory.Alloc)/(1024*1024)*100) / 100,
		Database:      dbState,
	})
}

type dataResponse struct {
	Items []store.Entry `json:"items"`
	Count int           `json:"count"`
}

// Data liest die Beispiel-Datensaetze (geschuetzt).
func (h *Handler) Data(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), dbTimeout)
	defer cancel()

	entries, err := h.store.ExampleEntries(ctx, 50)
	if err != nil {
		h.logger.Error("load entries failed", "error", err, "request_id", httpx.RequestIDFrom(r.Context()))
		respondError(h.logger, w, r, http.StatusInternalServerError, "database_error", "could not load data")
		return
	}

	respondJSON(h.logger, w, http.StatusOK, dataResponse{Items: entries, Count: len(entries)})
}

type processResponse struct {
	Result      worker.CalcResult `json:"result"`
	RequestedBy string            `json:"requested_by"`
}

// Process leitet den Auftrag an den Python-Worker weiter (geschuetzt).
func (h *Handler) Process(w http.ResponseWriter, r *http.Request) {
	defer func() { _ = r.Body.Close() }()
	r.Body = http.MaxBytesReader(w, r.Body, maxProcessBodyBytes)

	var request worker.CalcRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		respondError(h.logger, w, r, http.StatusBadRequest, "invalid_json", "body must be valid JSON matching the request schema")
		return
	}

	if len(request.Values) == 0 {
		respondError(h.logger, w, r, http.StatusBadRequest, "invalid_values", "values must contain at least one number")
		return
	}
	if len(request.Values) > maxProcessValues {
		respondError(h.logger, w, r, http.StatusRequestEntityTooLarge, "too_many_values", "at most 10000 values are allowed")
		return
	}
	for _, value := range request.Values {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			respondError(h.logger, w, r, http.StatusBadRequest, "invalid_values", "values must be finite numbers")
			return
		}
	}

	result, err := h.worker.Calc(r.Context(), request)
	if err != nil {
		if errors.Is(err, worker.ErrUnavailable) {
			h.logger.Error("worker call failed", "error", err, "request_id", httpx.RequestIDFrom(r.Context()))
			respondError(h.logger, w, r, http.StatusBadGateway, "worker_unavailable", "worker service is not available")
			return
		}
		h.logger.Error("worker call failed", "error", err, "request_id", httpx.RequestIDFrom(r.Context()))
		respondError(h.logger, w, r, http.StatusInternalServerError, "worker_error", "worker call failed")
		return
	}

	subject := "unknown"
	if identity, ok := httpx.IdentityFrom(r.Context()); ok {
		subject = identity.Subject
	}

	respondJSON(h.logger, w, http.StatusOK, processResponse{Result: result, RequestedBy: subject})
}

// Rollen, die das Template verdrahtet.
const (
	roleAdmin    = "Admin"
	roleOperator = "Operator"
)

type meResponse struct {
	Subject    string   `json:"subject"`
	Email      string   `json:"email"`
	Roles      []string `json:"roles"`
	Groups     []string `json:"groups"`
	AuthMethod string   `json:"auth_method"`
}

// Me liefert die eigene Identitaet (jede authentifizierte Identitaet).
func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	identity, ok := httpx.IdentityFrom(r.Context())
	if !ok {
		respondError(h.logger, w, r, http.StatusUnauthorized, "unauthorized", "missing or invalid credentials")
		return
	}

	respondJSON(h.logger, w, http.StatusOK, meResponse{
		Subject:    identity.Subject,
		Email:      identity.Email,
		Roles:      identity.Roles,
		Groups:     identity.Groups,
		AuthMethod: identity.Method,
	})
}

type operatorResponse struct {
	Message       string   `json:"message"`
	Subject       string   `json:"subject"`
	Roles         []string `json:"roles"`
	RequiredRoles []string `json:"required_roles"`
}

// OperatorAction ist ein RBAC-Testendpunkt: Admin ODER Operator.
func (h *Handler) OperatorAction(w http.ResponseWriter, r *http.Request) {
	identity, _ := httpx.IdentityFrom(r.Context())

	h.logger.Info("rbac endpoint reached",
		"endpoint", "operator",
		"user", identity.Subject,
		"roles", identity.Roles,
		"request_id", httpx.RequestIDFrom(r.Context()),
	)

	respondJSON(h.logger, w, http.StatusOK, operatorResponse{
		Message:       "operator action executed",
		Subject:       identity.Subject,
		Roles:         identity.Roles,
		RequiredRoles: []string{roleAdmin, roleOperator},
	})
}

type resetResponse struct {
	Message     string    `json:"message"`
	Reset       bool      `json:"reset"`
	PerformedBy string    `json:"performed_by"`
	PerformedAt time.Time `json:"performed_at"`
}

// AdminReset ist ein RBAC-Testendpunkt: ausschliesslich Admin.
// Die Aktion ist bewusst ein Dry-Run (kein Zustandswechsel).
func (h *Handler) AdminReset(w http.ResponseWriter, r *http.Request) {
	identity, _ := httpx.IdentityFrom(r.Context())

	h.logger.Warn("admin reset invoked",
		"endpoint", "admin_reset",
		"user", identity.Subject,
		"request_id", httpx.RequestIDFrom(r.Context()),
	)

	h.recordAudit(r, audit.ActionAdminReset, "POST /api/v1/protected/admin/reset", http.StatusOK, map[string]any{
		"roles":       identity.Roles,
		"auth_method": identity.Method,
	})

	respondJSON(h.logger, w, http.StatusOK, resetResponse{
		Message:     "admin reset accepted (dry run)",
		Reset:       true,
		PerformedBy: identity.Subject,
		PerformedAt: time.Now().UTC(),
	})
}

type auditListResponse struct {
	Items []audit.Log `json:"items"`
	Count int         `json:"count"`
}

// AuditLogs liefert die neuesten Audit-Eintraege (nur Admin).
func (h *Handler) AuditLogs(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		respondError(h.logger, w, r, http.StatusServiceUnavailable, "store_unavailable", "audit store not configured")
		return
	}

	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 && parsed <= 200 {
			limit = parsed
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), dbTimeout)
	defer cancel()

	entries, err := h.store.ListAuditLogs(ctx, limit)
	if err != nil {
		h.logger.Error("load audit logs failed", "error", err, "request_id", httpx.RequestIDFrom(r.Context()))
		respondError(h.logger, w, r, http.StatusInternalServerError, "audit_error", "could not load audit log")
		return
	}

	// Bewusst NICHT in audit_logs: Lesezugriffe auf das Audit-Log wuerden sich
	// bei Polling selbst fuellen. Nachweis bleibt im strukturierten App-Log.
	identity, _ := httpx.IdentityFrom(r.Context())
	h.logger.Info("audit log read",
		"action", audit.ActionAuditRead,
		"user", identity.Subject,
		"limit", limit,
		"count", len(entries),
		"request_id", httpx.RequestIDFrom(r.Context()),
	)

	respondJSON(h.logger, w, http.StatusOK, auditListResponse{Items: entries, Count: len(entries)})
}

// recordAudit schreibt einen Audit-Eintrag; Fehler werden geloggt, nicht an
// den Client durchgereicht (Audit darf den Request nicht kippen).
func (h *Handler) recordAudit(r *http.Request, action, resource string, status int, details map[string]any) {
	identity, _ := httpx.IdentityFrom(r.Context())

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if err := audit.RecorderFrom(r.Context()).Record(ctx, audit.Entry{
		RequestID:  httpx.RequestIDFrom(r.Context()),
		UserID:     identity.Subject,
		Action:     action,
		Resource:   resource,
		StatusCode: status,
		Details:    details,
	}); err != nil {
		h.logger.Warn("audit write failed", "action", action, "error", err,
			"request_id", httpx.RequestIDFrom(r.Context()))
	}
}

// NotFound ist der Deny-by-Default-Fallback: unbekannte Pfade liefern 404,
// ohne interne Strukturen zu offenbaren.
func (h *Handler) NotFound(w http.ResponseWriter, r *http.Request) {
	respondError(h.logger, w, r, http.StatusNotFound, "not_found", "resource not found")
}
