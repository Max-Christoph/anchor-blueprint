// Package auth enthaelt die Authentifizierung des Backends: JWT-Validierung
// gegen JWKS (RS256), einen Dev-Header-Modus fuer lokale Tests und die
// HTTP-Middleware mit Deny-by-Default.
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
}

// Verifier prueft Zugangsdaten und liefert die Identitaet.
type Verifier interface {
	Verify(ctx context.Context, credentials Credentials) (httpx.Identity, error)
	// Mode benennt das Verfahren fuer Logs und /api/v1/status.
	Mode() string
}

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

// Unterstuetzte Betriebsarten.
const (
	ModeJWT       = "jwt"
	ModeDevHeader = "dev_header"
	ModeMock      = "mock"
)
