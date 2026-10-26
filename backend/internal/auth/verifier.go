// Package auth enthaelt die Authentifizierung des Backends: JWT-Validierung
// gegen JWKS (RS256), einen Dev-Header-Modus fuer lokale Tests, RBAC-Middleware
// und die Auth-Middleware mit Deny-by-Default.
package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Max-Christoph/anchor-blueprint/backend/internal/httpx"
)

// Fehlerklassen der Authentifizierung. Aufrufer unterscheiden nur diese,
// nicht die internen Details (keine Token-Inhalte in Logs oder Antworten).
var (
	// ErrInvalidToken deckt Format, Signatur, Issuer, Audience, Claims ab.
	ErrInvalidToken = errors.New("invalid token")
	// ErrTokenExpired signalisiert ein abgelaufenes Token.
	ErrTokenExpired = errors.New("token expired")
	// ErrNoCredentials signalisiert fehlende Zugangsdaten.
	ErrNoCredentials = errors.New("no credentials")
	// ErrKeyUnavailable signalisiert ein nicht abrufbares JWKS.
	ErrKeyUnavailable = errors.New("signing key unavailable")
)

// Credentials buendelt die aus dem Request extrahierten Zugangsdaten.
type Credentials struct {
	// BearerToken stammt aus "Authorization: Bearer <token>".
	BearerToken string
	// UserEmail stammt aus dem Dev-Header "X-User-Email".
	UserEmail string
	// UserRoles stammt aus dem Dev-Header "X-User-Roles" (CSV).
	UserRoles string
}

// Verifier prueft Zugangsdaten und liefert die Identitaet.
type Verifier interface {
	Verify(ctx context.Context, credentials Credentials) (httpx.Identity, error)
	// Mode benennt das Verfahren fuer Logs und /api/v1/status.
	Mode() string
}

// Unterstuetzte Betriebsarten.
const (
	ModeJWT       = "jwt"
	ModeDevHeader = "dev_header"
	ModeMock      = "mock"
)

// Mode normalisiert AUTH_MODE; unbekannte Werte werden abgelehnt.
func Mode(raw string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	switch normalized {
	case "":
		return ModeJWT, nil
	case ModeJWT, ModeDevHeader, ModeMock:
		return normalized, nil
	default:
		return "", fmt.Errorf("unsupported AUTH_MODE %q (allowed: %s, %s, %s)", raw, ModeJWT, ModeDevHeader, ModeMock)
	}
}

// SplitList zerlegt CSV-Werte in eine getrimmte, deduplizierte Liste.
func SplitList(raw string) []string {
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		clean := strings.TrimSpace(part)
		if clean == "" {
			continue
		}
		key := strings.ToLower(clean)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		values = append(values, clean)
	}
	return values
}

// toStringList normalisiert Claim-Werte (string, []string, []any) zu []string.
func toStringList(value any) []string {
	switch typed := value.(type) {
	case nil:
		return nil
	case string:
		return SplitList(typed)
	case []string:
		return SplitList(strings.Join(typed, ","))
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok {
				parts = append(parts, text)
			}
		}
		return SplitList(strings.Join(parts, ","))
	default:
		return nil
	}
}
