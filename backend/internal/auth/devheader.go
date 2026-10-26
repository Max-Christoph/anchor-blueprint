package auth

import (
	"context"
	"crypto/subtle"
	"fmt"
	"net/mail"
	"strings"

	"github.com/Max-Christoph/anchor-blueprint/backend/internal/httpx"
)

// DevHeaderConfig konfiguriert den lokalen Auth-Modus (ohne OIDC).
type DevHeaderConfig struct {
	// AllowedEmailDomains begrenzt per X-User-Email akzeptierte Domains.
	AllowedEmailDomains []string
	// StaticToken ist ein optionaler fester Bearer-Token fuer Service-Aufrufe.
	StaticToken string
	// DefaultRole wird gesetzt, wenn X-User-Roles fehlt (Default "User").
	DefaultRole string
	// ModeName landet in der Identitaet (dev_header oder mock).
	ModeName string
}

// DevHeaderVerifier ist der Entwicklungs-Verifier: X-User-Email mit
// Domain-Allowlist, Rollen via X-User-Roles oder ein statischer Bearer-Token.
// Nicht fuer Produktion.
type DevHeaderVerifier struct {
	allowedDomains map[string]struct{}
	staticToken    string
	defaultRole    string
	modeName       string
}

// NewDevHeaderVerifier erzeugt den Entwicklungs-Verifier.
func NewDevHeaderVerifier(cfg DevHeaderConfig) (*DevHeaderVerifier, error) {
	if len(cfg.AllowedEmailDomains) == 0 {
		return nil, fmt.Errorf("AUTH_ALLOWED_EMAIL_DOMAINS is required for AUTH_MODE=%s", cfg.ModeName)
	}
	domains := make(map[string]struct{}, len(cfg.AllowedEmailDomains))
	for _, domain := range cfg.AllowedEmailDomains {
		domains[strings.ToLower(strings.TrimSpace(domain))] = struct{}{}
	}
	modeName := cfg.ModeName
	if modeName == "" {
		modeName = ModeDevHeader
	}
	defaultRole := strings.TrimSpace(cfg.DefaultRole)
	if defaultRole == "" {
		defaultRole = "User"
	}
	return &DevHeaderVerifier{allowedDomains: domains, staticToken: cfg.StaticToken, defaultRole: defaultRole, modeName: modeName}, nil
}

// Mode implementiert Verifier.
func (v *DevHeaderVerifier) Mode() string { return v.modeName }

// Verify prueft statischen Token oder Dev-Header.
func (v *DevHeaderVerifier) Verify(_ context.Context, credentials Credentials) (httpx.Identity, error) {
	if token := strings.TrimSpace(credentials.BearerToken); token != "" {
		if v.staticToken == "" {
			return httpx.Identity{}, ErrInvalidToken
		}
		if subtle.ConstantTimeCompare([]byte(token), []byte(v.staticToken)) != 1 {
			return httpx.Identity{}, ErrInvalidToken
		}
		return httpx.Identity{
			Subject: "service-account",
			Roles:   []string{v.defaultRole},
			Method:  "dev_token",
		}, nil
	}

	email := strings.TrimSpace(credentials.UserEmail)
	if email == "" {
		return httpx.Identity{}, ErrNoCredentials
	}
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email {
		return httpx.Identity{}, fmt.Errorf("%w: malformed user email", ErrInvalidToken)
	}
	_, domain, found := strings.Cut(strings.ToLower(address.Address), "@")
	if !found || domain == "" {
		return httpx.Identity{}, fmt.Errorf("%w: user email without domain", ErrInvalidToken)
	}
	if _, ok := v.allowedDomains[domain]; !ok {
		return httpx.Identity{}, fmt.Errorf("%w: email domain not allowed", ErrInvalidToken)
	}

	roles := SplitList(credentials.UserRoles)
	if len(roles) == 0 {
		roles = []string{v.defaultRole}
	}

	return httpx.Identity{
		Subject: address.Address,
		Email:   address.Address,
		Roles:   roles,
		Method:  v.modeName,
	}, nil
}
