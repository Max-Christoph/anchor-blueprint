package middleware

import (
	"log/slog"
	"math"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/Max-Christoph/anchor-blueprint/backend/internal/httpx"
)

// RateLimitConfig konfiguriert das Failure-Budget pro Client.
type RateLimitConfig struct {
	// Burst ist die Anzahl erlaubter Fehlversuche, bevor 429 greift.
	Burst int
	// PerMinute ist die Nachfuellrate des Budgets.
	PerMinute int
}

const (
	// rateLimitSweepThreshold begrenzt die Map-Groesse (Schutz vor IP-Flut).
	rateLimitSweepThreshold = 2048
	rateLimitMaxIdle        = 10 * time.Minute
)

type rateBucket struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// RateLimit schuetzt vor unautorisiertem Haemmern: nur Antworten mit 401/403
// verbrauchen das Budget eines Clients. Ist es erschoepft, antwortet die
// Middleware sofort mit 429 - ohne Auth-Pruefung, ohne Audit-Schreibvorgang.
func RateLimit(cfg RateLimitConfig, logger *slog.Logger) func(http.Handler) http.Handler {
	burst := cfg.Burst
	if burst <= 0 {
		burst = 10
	}
	perMinute := cfg.PerMinute
	if perMinute <= 0 {
		perMinute = 30
	}
	refill := rate.Limit(float64(perMinute) / 60.0)

	var (
		mu      sync.Mutex
		buckets = make(map[string]*rateBucket)
	)

	bucketFor := func(key string) *rateBucket {
		mu.Lock()
		defer mu.Unlock()

		if bucket, ok := buckets[key]; ok {
			bucket.lastSeen = time.Now()
			return bucket
		}
		if len(buckets) >= rateLimitSweepThreshold {
			sweep(buckets, time.Now().Add(-rateLimitMaxIdle))
		}
		bucket := &rateBucket{limiter: rate.NewLimiter(refill, burst), lastSeen: time.Now()}
		buckets[key] = bucket
		return bucket
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := clientKey(r)
			bucket := bucketFor(key)

			if tokens := bucket.limiter.Tokens(); tokens < 1 {
				retryAfter := int(math.Ceil(float64(1-tokens) / float64(refill)))
				if retryAfter < 1 {
					retryAfter = 1
				}
				w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.Header().Set("Cache-Control", "no-store")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"error":{"code":"rate_limited","message":"too many failed authentication attempts"}}`))

				logger.Warn("rate limit exceeded",
					"client", key,
					"path", r.URL.Path,
					"retry_after_s", retryAfter,
					"request_id", httpx.RequestIDFrom(r.Context()),
				)
				return
			}

			recorder := &statusRecorder{ResponseWriter: w}
			next.ServeHTTP(recorder, r)

			if recorder.status == http.StatusUnauthorized || recorder.status == http.StatusForbidden {
				// Fehlversuch verbraucht genau ein Token.
				bucket.limiter.AllowN(time.Now(), 1)
			}
		})
	}
}

// sweep entfernt veraltete Eintraege; muss unter dem Lock laufen.
func sweep(buckets map[string]*rateBucket, cutoff time.Time) {
	for key, bucket := range buckets {
		if bucket.lastSeen.Before(cutoff) {
			delete(buckets, key)
		}
	}
}

// clientKey bestimmt den Client: X-Real-IP (Nginx), erster X-Forwarded-For-Wert
// oder die Remote-Adresse. Header sind nur hinter einem vertrauenswuerdigen
// Proxy aussagekraeftig.
func clientKey(r *http.Request) string {
	if realIP := r.Header.Get("X-Real-IP"); realIP != "" {
		return realIP
	}
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		for i := 0; i < len(forwarded); i++ {
			if forwarded[i] == ',' {
				return forwarded[:i]
			}
		}
		return forwarded
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
