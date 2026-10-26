// Package config laedt die Laufzeitkonfiguration ausschliesslich aus
// Umgebungsvariablen. Fehlkonfiguration fuehrt zu einem Startfehler statt zu
// einem stillen Fallback (fail fast, no insecure defaults).
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Max-Christoph/anchor-blueprint/backend/internal/auth"
)

// Config buendelt alle Laufzeiteinstellungen des Backends.
type Config struct {
	Env             string
	Version         string
	HTTPAddr        string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	ShutdownTimeout time.Duration
	LogLevel        slog.Level

	DatabaseURL   string
	DBMaxConns    int32
	DBAutoMigrate bool
	// DBSeedDevData schreibt beim Start Entwicklungs-Beispieldaten (Seed).
	DBSeedDevData bool

	WorkerURL     string
	WorkerTimeout time.Duration

	// AuthMode ist jwt (Standard), dev_header oder mock.
	AuthMode string
	// AuthRolesClaim / AuthGroupsClaim benennen die Claims fuer RBAC.
	AuthRolesClaim  string
	AuthGroupsClaim string
	// AuthGroupRoleMap mappt Gruppen-IDs auf zusaetzliche Rollen.
	AuthGroupRoleMap map[string][]string
	// AuthDevDefaultRole greift, wenn im Dev-Modus X-User-Roles fehlt.
	AuthDevDefaultRole string
	// OIDC-Parameter (nur AUTH_MODE=jwt).
	OIDCIssuer       string
	OIDCAudience     string
	OIDCJWKSURL      string
	OIDCJWKSCacheTTL time.Duration
	OIDCHTTPTimeout  time.Duration
	AuthClockLeeway  time.Duration
	// Dev-Modus-Parameter (AUTH_MODE=dev_header|mock).
	AuthBearerToken     string
	AllowedEmailDomains []string

	CORSAllowedOrigins []string

	// RateLimit schuetzt vor unautorisiertem Haemmern (401/403-Fehlversuche).
	RateLimitEnabled   bool
	RateLimitBurst     int
	RateLimitPerMinute int
}

// Load liest die Konfiguration aus der Umgebung und validiert Pflichtwerte.
func Load() (Config, error) {
	mode, err := auth.Mode(getString("AUTH_MODE", auth.ModeJWT))
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		Env:                 getString("APP_ENV", "development"),
		Version:             getString("APP_VERSION", "dev"),
		HTTPAddr:            getString("HTTP_ADDR", ":8080"),
		DatabaseURL:         getString("DATABASE_URL", ""),
		WorkerURL:           strings.TrimRight(getString("WORKER_URL", "http://worker:8000"), "/"),
		AuthMode:            mode,
		AuthRolesClaim:      getString("AUTH_ROLES_CLAIM", "roles"),
		AuthGroupsClaim:     getString("AUTH_GROUPS_CLAIM", "groups"),
		AuthGroupRoleMap:    getGroupRoleMap("OIDC_GROUP_ROLE_MAP"),
		AuthDevDefaultRole:  getString("AUTH_DEV_DEFAULT_ROLE", "User"),
		DBAutoMigrate:       getBool("DB_AUTO_MIGRATE", true),
		DBSeedDevData:       getBool("DB_SEED_DEV_DATA", false),
		OIDCIssuer:          getString("OIDC_ISSUER", ""),
		OIDCAudience:        getString("OIDC_AUDIENCE", ""),
		OIDCJWKSURL:         getString("OIDC_JWKS_URL", ""),
		AuthBearerToken:     getString("AUTH_BEARER_TOKEN", ""),
		AllowedEmailDomains: getCSV("AUTH_ALLOWED_EMAIL_DOMAINS"),
		CORSAllowedOrigins:  getCSV("CORS_ALLOWED_ORIGINS"),
		RateLimitEnabled:    getBool("RATE_LIMIT_ENABLED", true),
		LogLevel:            parseLevel(getString("LOG_LEVEL", "info")),
	}

	if cfg.ReadTimeout, err = getDuration("HTTP_READ_TIMEOUT", 15*time.Second); err != nil {
		return Config{}, err
	}
	if cfg.WriteTimeout, err = getDuration("HTTP_WRITE_TIMEOUT", 20*time.Second); err != nil {
		return Config{}, err
	}
	if cfg.IdleTimeout, err = getDuration("HTTP_IDLE_TIMEOUT", 60*time.Second); err != nil {
		return Config{}, err
	}
	if cfg.ShutdownTimeout, err = getDuration("SHUTDOWN_TIMEOUT", 10*time.Second); err != nil {
		return Config{}, err
	}
	if cfg.WorkerTimeout, err = getDuration("WORKER_TIMEOUT", 10*time.Second); err != nil {
		return Config{}, err
	}
	if cfg.OIDCJWKSCacheTTL, err = getDuration("OIDC_JWKS_CACHE_TTL", 15*time.Minute); err != nil {
		return Config{}, err
	}
	if cfg.OIDCHTTPTimeout, err = getDuration("OIDC_HTTP_TIMEOUT", 5*time.Second); err != nil {
		return Config{}, err
	}
	if cfg.AuthClockLeeway, err = getDuration("AUTH_CLOCK_LEEWAY", 30*time.Second); err != nil {
		return Config{}, err
	}

	maxConns, err := getInt("DB_MAX_CONNS", 10)
	if err != nil {
		return Config{}, err
	}
	if maxConns < 1 || maxConns > 200 {
		return Config{}, fmt.Errorf("DB_MAX_CONNS must be between 1 and 200, got %d", maxConns)
	}
	cfg.DBMaxConns = int32(maxConns)

	burst, err := getInt("RATE_LIMIT_BURST", 10)
	if err != nil {
		return Config{}, err
	}
	if burst < 1 || burst > 1000 {
		return Config{}, fmt.Errorf("RATE_LIMIT_BURST must be between 1 and 1000, got %d", burst)
	}
	cfg.RateLimitBurst = burst

	perMinute, err := getInt("RATE_LIMIT_PER_MINUTE", 30)
	if err != nil {
		return Config{}, err
	}
	if perMinute < 1 || perMinute > 6000 {
		return Config{}, fmt.Errorf("RATE_LIMIT_PER_MINUTE must be between 1 and 6000, got %d", perMinute)
	}
	cfg.RateLimitPerMinute = perMinute

	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}
	if err := validateAuth(cfg); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

// validateAuth prueft die modusabhaengigen Pflichtwerte.
func validateAuth(cfg Config) error {
	if cfg.AuthMode == auth.ModeJWT {
		missing := make([]string, 0, 3)
		if cfg.OIDCIssuer == "" {
			missing = append(missing, "OIDC_ISSUER")
		}
		if cfg.OIDCAudience == "" {
			missing = append(missing, "OIDC_AUDIENCE")
		}
		if cfg.OIDCJWKSURL == "" {
			missing = append(missing, "OIDC_JWKS_URL")
		}
		if len(missing) > 0 {
			return fmt.Errorf("AUTH_MODE=jwt requires %s", strings.Join(missing, ", "))
		}
		return nil
	}

	if len(cfg.AllowedEmailDomains) == 0 {
		return fmt.Errorf("AUTH_MODE=%s requires AUTH_ALLOWED_EMAIL_DOMAINS", cfg.AuthMode)
	}
	return nil
}

func getBool(key string, fallback bool) bool {
	raw := strings.ToLower(getString(key, ""))
	switch raw {
	case "":
		return fallback
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// getGroupRoleMap parst "groupA=Admin;groupB=Operator,Viewer".
func getGroupRoleMap(key string) map[string][]string {
	raw := getString(key, "")
	if raw == "" {
		return nil
	}
	mapping := make(map[string][]string)
	for _, pair := range strings.Split(raw, ";") {
		group, roles, found := strings.Cut(pair, "=")
		group = strings.TrimSpace(group)
		if !found || group == "" {
			continue
		}
		parsed := auth.SplitList(roles)
		if len(parsed) == 0 {
			continue
		}
		mapping[group] = parsed
	}
	if len(mapping) == 0 {
		return nil
	}
	return mapping
}

func getString(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return fallback
}

func getCSV(key string) []string {
	raw := getString(key, "")
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		clean := strings.ToLower(strings.TrimSpace(part))
		if clean != "" {
			values = append(values, clean)
		}
	}
	return values
}

func getDuration(key string, fallback time.Duration) (time.Duration, error) {
	raw := getString(key, "")
	if raw == "" {
		return fallback, nil
	}
	value, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", key, err)
	}
	if value <= 0 {
		return 0, fmt.Errorf("%s must be positive, got %s", key, raw)
	}
	return value, nil
}

func getInt(key string, fallback int) (int, error) {
	raw := getString(key, "")
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", key, err)
	}
	return value, nil
}

func parseLevel(raw string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
