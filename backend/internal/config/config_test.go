package config_test

import (
	"testing"

	"github.com/Max-Christoph/anchor-blueprint/backend/internal/auth"
	"github.com/Max-Christoph/anchor-blueprint/backend/internal/config"
)

// setBase setzt die Pflichtwerte, die in jedem Modus gebraucht werden.
func setBase(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://user:pass@db:5432/db?sslmode=disable")
}

func TestLoadDefaultsToJWTAndRequiresOIDC(t *testing.T) {
	setBase(t)
	t.Setenv("AUTH_MODE", "")

	if _, err := config.Load(); err == nil {
		t.Fatal("AUTH_MODE=jwt ohne OIDC_* wurde akzeptiert")
	}
}

func TestLoadJWTModeWithOIDC(t *testing.T) {
	setBase(t)
	t.Setenv("AUTH_MODE", "jwt")
	t.Setenv("OIDC_ISSUER", "https://login.example.com/tenant/v2.0")
	t.Setenv("OIDC_AUDIENCE", "api://anchor-blueprint")
	t.Setenv("OIDC_JWKS_URL", "https://login.example.com/tenant/discovery/v2.0/keys")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if cfg.AuthMode != auth.ModeJWT {
		t.Fatalf("AuthMode = %q, erwartet %q", cfg.AuthMode, auth.ModeJWT)
	}
	if cfg.AuthClockLeeway <= 0 {
		t.Fatalf("AuthClockLeeway = %s, erwartet > 0", cfg.AuthClockLeeway)
	}
}

func TestLoadDevHeaderMode(t *testing.T) {
	setBase(t)
	t.Setenv("AUTH_MODE", "dev_header")
	t.Setenv("AUTH_ALLOWED_EMAIL_DOMAINS", "example.com")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if cfg.AuthMode != auth.ModeDevHeader {
		t.Fatalf("AuthMode = %q, erwartet %q", cfg.AuthMode, auth.ModeDevHeader)
	}
}

func TestLoadDevHeaderRequiresDomains(t *testing.T) {
	setBase(t)
	t.Setenv("AUTH_MODE", "dev_header")
	t.Setenv("AUTH_ALLOWED_EMAIL_DOMAINS", "")

	if _, err := config.Load(); err == nil {
		t.Fatal("dev_header ohne AUTH_ALLOWED_EMAIL_DOMAINS wurde akzeptiert")
	}
}

func TestLoadRejectsUnknownAuthMode(t *testing.T) {
	setBase(t)
	t.Setenv("AUTH_MODE", "basic")

	if _, err := config.Load(); err == nil {
		t.Fatal("unbekannter AUTH_MODE wurde akzeptiert")
	}
}

func TestLoadRequiresDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("AUTH_MODE", "dev_header")
	t.Setenv("AUTH_ALLOWED_EMAIL_DOMAINS", "example.com")

	if _, err := config.Load(); err == nil {
		t.Fatal("fehlende DATABASE_URL wurde akzeptiert")
	}
}
