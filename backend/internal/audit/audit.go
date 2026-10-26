// Package audit definiert den Audit-Trail: Eintraege fuer sicherheitsrelevante
// Ereignisse (RBAC-Abweisungen, administrative Aktionen) und die Ablage-Schnittstelle.
// Die Implementierung liegt in internal/store (Tabelle audit_logs).
package audit

import (
	"context"
	"time"
)

// Entry ist ein Audit-Eintrag.
type Entry struct {
	RequestID  string
	UserID     string
	Action     string
	Resource   string
	StatusCode int
	Details    map[string]any
}

// Log ist ein gelesener Audit-Eintrag.
type Log struct {
	ID         int64          `json:"id"`
	Timestamp  time.Time      `json:"timestamp"`
	RequestID  string         `json:"request_id"`
	UserID     string         `json:"user_id"`
	Action     string         `json:"action"`
	Resource   string         `json:"resource"`
	StatusCode int            `json:"status_code"`
	Details    map[string]any `json:"details"`
}

// Recorder schreibt Audit-Eintraege. Implementierungen muessen fehlertolerant
// sein: ein fehlgeschlagener Audit-Write darf den Request nicht kippen.
type Recorder interface {
	Record(ctx context.Context, entry Entry) error
}

// Aktionsnamen (stabil, fuer Auswertungen).
const (
	ActionRBACDeny   = "rbac.deny"
	ActionAdminReset = "admin.reset"
	ActionAuditRead  = "audit.read"
)

type contextKey string

const recorderKey contextKey = "audit_recorder"

// WithRecorder legt den Recorder im Request-Context ab.
func WithRecorder(ctx context.Context, recorder Recorder) context.Context {
	if recorder == nil {
		return ctx
	}
	return context.WithValue(ctx, recorderKey, recorder)
}

// RecorderFrom liest den Recorder; ohne konfigurierten Recorder ein Noop.
func RecorderFrom(ctx context.Context) Recorder {
	if recorder, ok := ctx.Value(recorderKey).(Recorder); ok && recorder != nil {
		return recorder
	}
	return noop{}
}

type noop struct{}

func (noop) Record(context.Context, Entry) error { return nil }
