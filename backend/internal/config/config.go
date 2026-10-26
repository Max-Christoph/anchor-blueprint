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

	DatabaseURL string
	DBMaxConns  int32

	WorkerURL     string
	WorkerTimeout time.Duration

	AuthBearerToken     string
	AllowedEmailDomains []string
	CORSAllowedOrigins  []string
}

// Load liest die Konfiguration aus der Umgebung und validiert Pflichtwerte.
func Load() (Config, error) {
	cfg := Config{
		Env:                 getString("APP_ENV", "development"),
		Version:             getString("APP_VERSION", "dev"),
		HTTPAddr:            getString("HTTP_ADDR", ":8080"),
		DatabaseURL:         getString("DATABASE_URL", ""),
		WorkerURL:           strings.TrimRight(getString("WORKER_URL", "http://worker:8000"), "/"),
		AuthBearerToken:     getString("AUTH_BEARER_TOKEN", ""),
		AllowedEmailDomains: getCSV("AUTH_ALLOWED_EMAIL_DOMAINS"),
		CORSAllowedOrigins:  getCSV("CORS_ALLOWED_ORIGINS"),
		LogLevel:            parseLevel(getString("LOG_LEVEL", "info")),
	}

	var err error
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

	maxConns, err := getInt("DB_MAX_CONNS", 10)
	if err != nil {
		return Config{}, err
	}
	if maxConns < 1 || maxConns > 200 {
		return Config{}, fmt.Errorf("DB_MAX_CONNS must be between 1 and 200, got %d", maxConns)
	}
	cfg.DBMaxConns = int32(maxConns)

	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}
	if cfg.AuthBearerToken == "" {
		return Config{}, errors.New("AUTH_BEARER_TOKEN is required (no insecure default)")
	}
	if len(cfg.AllowedEmailDomains) == 0 {
		return Config{}, errors.New("AUTH_ALLOWED_EMAIL_DOMAINS must list at least one domain")
	}

	return cfg, nil
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
