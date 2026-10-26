package middleware_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Max-Christoph/anchor-blueprint/backend/internal/middleware"
)

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

// guardedHandler antwortet 200 bei gueltiger Kennung, sonst 401.
func guardedHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer ok" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
	})
}

func do(handler http.Handler, ip, auth string) int {
	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/data", http.NoBody)
	request.Header.Set("X-Real-IP", ip)
	if auth != "" {
		request.Header.Set("Authorization", auth)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder.Code
}

func TestRateLimitBlocksAfterBurstOfDenials(t *testing.T) {
	handler := middleware.RateLimit(middleware.RateLimitConfig{Burst: 3, PerMinute: 60}, quietLogger())(guardedHandler())

	for i := 0; i < 3; i++ {
		if code := do(handler, "198.51.100.1", ""); code != http.StatusUnauthorized {
			t.Fatalf("Fehlversuch %d: Status = %d, erwartet 401", i+1, code)
		}
	}

	if code := do(handler, "198.51.100.1", ""); code != http.StatusTooManyRequests {
		t.Fatalf("Status nach erschoepftem Budget = %d, erwartet 429", code)
	}

	// Auch mit gueltigen Credentials ist der Client jetzt blockiert.
	if code := do(handler, "198.51.100.1", "Bearer ok"); code != http.StatusTooManyRequests {
		t.Fatalf("Status mit Credentials = %d, erwartet 429 (Budget erschoepft)", code)
	}
}

func TestRateLimitIsolatesClients(t *testing.T) {
	handler := middleware.RateLimit(middleware.RateLimitConfig{Burst: 2, PerMinute: 60}, quietLogger())(guardedHandler())

	for i := 0; i < 2; i++ {
		do(handler, "198.51.100.2", "")
	}
	if code := do(handler, "198.51.100.2", ""); code != http.StatusTooManyRequests {
		t.Fatalf("Client A: Status = %d, erwartet 429", code)
	}
	if code := do(handler, "198.51.100.3", ""); code != http.StatusUnauthorized {
		t.Fatalf("Client B: Status = %d, erwartet 401 (eigenes Budget)", code)
	}
}

func TestRateLimitIgnoresSuccessfulRequests(t *testing.T) {
	handler := middleware.RateLimit(middleware.RateLimitConfig{Burst: 2, PerMinute: 60}, quietLogger())(guardedHandler())

	for i := 0; i < 20; i++ {
		if code := do(handler, "198.51.100.4", "Bearer ok"); code != http.StatusOK {
			t.Fatalf("erfolgreicher Request %d: Status = %d, erwartet 200", i+1, code)
		}
	}
}

func TestRateLimitClientKeyFromForwardedHeader(t *testing.T) {
	handler := middleware.RateLimit(middleware.RateLimitConfig{Burst: 1, PerMinute: 60}, quietLogger())(guardedHandler())

	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/data", http.NoBody)
	request.Header.Set("X-Forwarded-For", "192.0.2.10, 192.0.2.11")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("Status = %d, erwartet 401", recorder.Code)
	}

	// Der zweite Aufruf mit derselben Forwarded-IP muss blockiert sein.
	second := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/data", http.NoBody)
	second.Header.Set("X-Forwarded-For", "192.0.2.10, 192.0.2.11")
	secondRecorder := httptest.NewRecorder()
	handler.ServeHTTP(secondRecorder, second)
	if secondRecorder.Code != http.StatusTooManyRequests {
		t.Fatalf("Status = %d, erwartet 429", secondRecorder.Code)
	}

	// Andere Forwarded-IP bleibt unbeeinflusst.
	other := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/v1/data", http.NoBody)
	other.Header.Set("X-Forwarded-For", "192.0.2.99")
	otherRecorder := httptest.NewRecorder()
	handler.ServeHTTP(otherRecorder, other)
	if otherRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("andere IP: Status = %d, erwartet 401", otherRecorder.Code)
	}
}
