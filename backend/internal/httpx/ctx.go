// Package httpx enthaelt die gemeinsamen Context-Helfer des Backends:
// Request-ID und authentifizierte Identitaet.
package httpx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"time"
)

type contextKey string

const (
	requestIDKey contextKey = "request_id"
	identityKey  contextKey = "identity"
)

// Identity beschreibt den authentifizierten Aufrufer.
type Identity struct {
	// Subject ist der stabile Benutzer-/Client-Identifier (JWT "sub").
	Subject string
	// Email ist die E-Mail-Adresse aus dem Token (falls vorhanden).
	Email string
	// Roles sind App-Rollen (JWT "roles" oder Dev-Header X-User-Roles).
	Roles []string
	// Groups sind Gruppenzugehoerigkeiten (JWT "groups", z. B. Entra-Objekt-IDs).
	Groups []string
	// Method dokumentiert den Authentifizierungsweg (jwt, dev_header, dev_token).
	Method string
}

// HasRole prueft eine Rolle ohne Beachtung der Gross-/Kleinschreibung.
func (i Identity) HasRole(role string) bool {
	target := strings.TrimSpace(strings.ToLower(role))
	for _, candidate := range i.Roles {
		if strings.ToLower(strings.TrimSpace(candidate)) == target {
			return true
		}
	}
	return false
}

// HasAnyRole prueft, ob mindestens eine der Rollen vorhanden ist.
func (i Identity) HasAnyRole(roles ...string) bool {
	for _, role := range roles {
		if i.HasRole(role) {
			return true
		}
	}
	return false
}

// HasGroup prueft eine Gruppe ohne Beachtung der Gross-/Kleinschreibung
// (Entra-Gruppen sind GUIDs, Schreibweise kann variieren).
func (i Identity) HasGroup(group string) bool {
	target := strings.TrimSpace(strings.ToLower(group))
	for _, candidate := range i.Groups {
		if strings.ToLower(strings.TrimSpace(candidate)) == target {
			return true
		}
	}
	return false
}

// HasAnyGroup prueft, ob mindestens eine der Gruppen vorhanden ist.
func (i Identity) HasAnyGroup(groups ...string) bool {
	for _, group := range groups {
		if i.HasGroup(group) {
			return true
		}
	}
	return false
}

// WithRequestID legt die Request-ID im Context ab.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey, id)
}

// RequestIDFrom liest die Request-ID aus dem Context.
func RequestIDFrom(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey).(string); ok {
		return id
	}
	return ""
}

// NewRequestID erzeugt eine zufaellige Request-ID.
func NewRequestID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "fallback-" + time.Now().UTC().Format("20060102150405.000000000")
	}
	return hex.EncodeToString(buf)
}

// WithIdentity legt die Identitaet im Context ab.
func WithIdentity(ctx context.Context, identity Identity) context.Context {
	return context.WithValue(ctx, identityKey, identity)
}

// IdentityFrom liest die Identitaet aus dem Context.
func IdentityFrom(ctx context.Context) (Identity, bool) {
	identity, ok := ctx.Value(identityKey).(Identity)
	return identity, ok
}
