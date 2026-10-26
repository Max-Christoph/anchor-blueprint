// Package httpx enthaelt die gemeinsamen Context-Helfer des Backends:
// Request-ID und authentifizierte Identitaet.
package httpx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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
	// Roles und Groups stammen aus den gleichnamigen Claims.
	Roles  []string
	Groups []string
	// Method dokumentiert den Authentifizierungsweg (jwt, dev_header, dev_token).
	Method string
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
