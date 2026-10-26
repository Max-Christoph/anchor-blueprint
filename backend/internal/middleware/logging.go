// Package middleware enthaelt die HTTP-Middleware des Backends:
// Request-ID, strukturiertes Logging, Panic-Recovery, CORS und Auth-Stub.
package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"time"
)

type contextKey string

const (
	requestIDKey contextKey = "request_id"
	identityKey  contextKey = "identity"
)

// RequestID erzeugt bzw. uebernimmt eine Request-ID und legt sie in den Context
// sowie in den Response-Header.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-Id")
		if len(id) == 0 || len(id) > 64 {
			id = newRequestID()
		}
		w.Header().Set("X-Request-Id", id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey, id)))
	})
}

// RequestIDFrom liest die Request-ID aus dem Context.
func RequestIDFrom(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey).(string); ok {
		return id
	}
	return ""
}

func newRequestID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "fallback-" + time.Now().UTC().Format("20060102150405.000000000")
	}
	return hex.EncodeToString(buf)
}

// statusRecorder merkt sich Statuscode und Bytes fuer das Access-Log.
type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (r *statusRecorder) WriteHeader(status int) {
	if r.status != 0 {
		return
	}
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	written, err := r.ResponseWriter.Write(b)
	r.bytes += written
	return written, err
}

// Unwrap erlaubt http.ResponseController den Zugriff auf das Original.
func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

// Logging schreibt einen strukturierten Access-Log-Eintrag pro Request.
func Logging(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			recorder := &statusRecorder{ResponseWriter: w}

			next.ServeHTTP(recorder, r)

			status := recorder.status
			if status == 0 {
				status = http.StatusOK
			}
			attrs := []any{
				"method", r.Method,
				"path", r.URL.Path,
				"status", status,
				"bytes", recorder.bytes,
				"duration_ms", float64(time.Since(start).Microseconds()) / 1000.0,
				"request_id", RequestIDFrom(r.Context()),
			}
			if identity, ok := IdentityFrom(r.Context()); ok {
				attrs = append(attrs, "user", identity.Subject, "auth_method", identity.Method)
			}
			if status >= http.StatusInternalServerError {
				logger.Error("http request", attrs...)
				return
			}
			logger.Info("http request", attrs...)
		})
	}
}

// Recovery faengt Panics ab und beantwortet den Request mit 500 statt den
// Prozess zu beenden.
func Recovery(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if recovered := recover(); recovered != nil {
					logger.Error("panic recovered",
						"panic", recovered,
						"path", r.URL.Path,
						"request_id", RequestIDFrom(r.Context()),
					)
					w.Header().Set("Content-Type", "application/json; charset=utf-8")
					w.WriteHeader(http.StatusInternalServerError)
					_, _ = w.Write([]byte(`{"error":{"code":"internal_error","message":"internal server error"}}`))
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
