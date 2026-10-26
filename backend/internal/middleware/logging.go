// Package middleware enthaelt die transportnahe HTTP-Middleware des Backends:
// Request-ID, strukturiertes Logging, Panic-Recovery und CORS.
// Authentifizierung liegt in internal/auth.
package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/Max-Christoph/anchor-blueprint/backend/internal/httpx"
)

// RequestID erzeugt bzw. uebernimmt eine Request-ID und legt sie in den Context
// sowie in den Response-Header.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-Id")
		if id == "" || len(id) > 64 {
			id = httpx.NewRequestID()
		}
		w.Header().Set("X-Request-Id", id)
		next.ServeHTTP(w, r.WithContext(httpx.WithRequestID(r.Context(), id)))
	})
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
				"request_id", httpx.RequestIDFrom(r.Context()),
			}
			if identity, ok := httpx.IdentityFrom(r.Context()); ok {
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
						"request_id", httpx.RequestIDFrom(r.Context()),
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
