package auth_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/Max-Christoph/anchor-blueprint/backend/internal/auth"
)

const (
	testIssuer   = "https://login.example.com/tenant-id/v2.0"
	testAudience = "api://anchor-blueprint"
	testKID      = "test-key-1"
	testSub      = "user-42"
	testEmail    = "max@example.com"
)

var (
	keyOnce     sync.Once
	generatedA  *rsa.PrivateKey
	generatedB  *rsa.PrivateKey
	generatedOK bool
)

// testKeys liefert zwei unabhaengige RSA-Schluessel (Signatur + Fremdschluessel).
func testKeys(t *testing.T) (signing, foreign *rsa.PrivateKey) {
	t.Helper()

	keyOnce.Do(func() {
		a, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			return
		}
		b, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			return
		}
		generatedA, generatedB, generatedOK = a, b, true
	})
	if !generatedOK {
		t.Fatal("RSA-Schluessel konnten nicht erzeugt werden")
	}
	return generatedA, generatedB
}

// jwksEntry ist ein Schluessel im JWKS-Testserver.
type jwksEntry struct {
	KID string
	Pub *rsa.PublicKey
}

// jwksServer ist ein gemockter OIDC-JWKS-Endpunkt.
type jwksServer struct {
	mu      sync.Mutex
	entries []jwksEntry
	fail    bool
	hits    atomic.Int32
	server  *httptest.Server
}

func newJWKSServer(t *testing.T) *jwksServer {
	t.Helper()

	s := &jwksServer{}
	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		s.hits.Add(1)
		s.mu.Lock()
		fail, entries := s.fail, append([]jwksEntry(nil), s.entries...)
		s.mu.Unlock()

		if fail {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		keys := make([]map[string]string, 0, len(entries))
		for _, entry := range entries {
			keys = append(keys, map[string]string{
				"kid": entry.KID,
				"kty": "RSA",
				"alg": "RS256",
				"use": "sig",
				"n":   base64.RawURLEncoding.EncodeToString(entry.Pub.N.Bytes()),
				"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(entry.Pub.E)).Bytes()),
			})
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": keys})
	}))
	t.Cleanup(s.server.Close)

	return s
}

func (s *jwksServer) addKey(kid string, pub *rsa.PublicKey) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = append(s.entries, jwksEntry{KID: kid, Pub: pub})
}

func (s *jwksServer) setFail(fail bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fail = fail
}

func (s *jwksServer) url() string { return s.server.URL }

// tokenOptions beschreibt ein zu signierendes Test-Token.
type tokenOptions struct {
	issuer            string
	audience          string
	subject           string
	email             string
	preferredUsername string
	roles             []string
	groups            []string
	kid               string
	expiresIn         time.Duration
	method            jwt.SigningMethod
	key               *rsa.PrivateKey
}

func signToken(t *testing.T, options tokenOptions) string {
	t.Helper()

	if options.method == nil {
		options.method = jwt.SigningMethodRS256
	}
	if options.issuer == "" {
		options.issuer = testIssuer
	}
	if options.audience == "" {
		options.audience = testAudience
	}
	switch options.subject {
	case "-":
		options.subject = ""
	case "":
		options.subject = testSub
	}
	if options.expiresIn == 0 {
		options.expiresIn = 5 * time.Minute
	}

	if options.key == nil {
		options.key, _ = testKeys(t)
	}
	now := time.Now()
	claims := auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    options.issuer,
			Audience:  jwt.ClaimStrings{options.audience},
			Subject:   options.subject,
			ExpiresAt: jwt.NewNumericDate(now.Add(options.expiresIn)),
			IssuedAt:  jwt.NewNumericDate(now.Add(-time.Minute)),
			ID:        "test-jti",
		},
		Email:             options.email,
		PreferredUsername: options.preferredUsername,
		Roles:             options.roles,
		Groups:            options.groups,
	}

	token := jwt.NewWithClaims(options.method, claims)
	if options.kid != "" {
		token.Header["kid"] = options.kid
	}

	var signingKey any = options.key
	if _, isHMAC := options.method.(*jwt.SigningMethodHMAC); isHMAC {
		signingKey = []byte("shared-hmac-secret")
	}

	signed, err := token.SignedString(signingKey)
	if err != nil {
		t.Fatalf("Token konnte nicht signiert werden: %v", err)
	}
	return signed
}

// newVerifier baut einen JWT-Verifier gegen den Test-JWKS-Server.
func newVerifier(t *testing.T, server *jwksServer, cacheTTL time.Duration) *auth.JWTVerifier {
	t.Helper()

	verifier, err := auth.NewJWTVerifier(auth.JWKSConfig{
		Issuer:      testIssuer,
		Audience:    testAudience,
		JWKSURL:     server.url(),
		CacheTTL:    cacheTTL,
		HTTPTimeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewJWTVerifier: %v", err)
	}
	return verifier
}

func TestJWTVerifierValidToken(t *testing.T) {
	signingKey, _ := testKeys(t)
	server := newJWKSServer(t)
	server.addKey(testKID, &signingKey.PublicKey)
	verifier := newVerifier(t, server, time.Minute)

	token := signToken(t, tokenOptions{
		email:  testEmail,
		roles:  []string{"reader", "operator"},
		groups: []string{"group-platform"},
		kid:    testKID,
		key:    signingKey,
	})

	identity, err := verifier.Verify(context.Background(), auth.Credentials{BearerToken: token})
	if err != nil {
		t.Fatalf("gueltiges Token wurde abgelehnt: %v", err)
	}
	if identity.Subject != testSub {
		t.Errorf("Subject = %q, erwartet %q", identity.Subject, testSub)
	}
	if identity.Email != testEmail {
		t.Errorf("Email = %q, erwartet %q", identity.Email, testEmail)
	}
	if identity.Method != auth.ModeJWT {
		t.Errorf("Method = %q, erwartet %q", identity.Method, auth.ModeJWT)
	}
	if len(identity.Roles) != 2 || identity.Roles[0] != "reader" {
		t.Errorf("Roles = %v, erwartet [reader operator]", identity.Roles)
	}
	if len(identity.Groups) != 1 || identity.Groups[0] != "group-platform" {
		t.Errorf("Groups = %v, erwartet [group-platform]", identity.Groups)
	}
}

func TestJWTVerifierEmailFallsBackToPreferredUsername(t *testing.T) {
	signingKey, _ := testKeys(t)
	server := newJWKSServer(t)
	server.addKey(testKID, &signingKey.PublicKey)
	verifier := newVerifier(t, server, time.Minute)

	token := signToken(t, tokenOptions{
		preferredUsername: "alias@example.com",
		kid:               testKID,
		key:               signingKey,
	})

	identity, err := verifier.Verify(context.Background(), auth.Credentials{BearerToken: token})
	if err != nil {
		t.Fatalf("Token wurde abgelehnt: %v", err)
	}
	if identity.Email != "alias@example.com" {
		t.Errorf("Email = %q, erwartet alias@example.com", identity.Email)
	}
}

func TestJWTVerifierRejects(t *testing.T) {
	signingKey, foreignKey := testKeys(t)

	tests := []struct {
		name     string
		token    func(t *testing.T, server *jwksServer) string
		expected error
	}{
		{
			name: "abgelaufenes Token",
			token: func(t *testing.T, _ *jwksServer) string {
				return signToken(t, tokenOptions{kid: testKID, key: signingKey, expiresIn: -5 * time.Minute})
			},
			expected: auth.ErrTokenExpired,
		},
		{
			name: "falscher Issuer",
			token: func(t *testing.T, _ *jwksServer) string {
				return signToken(t, tokenOptions{issuer: "https://evil.example.com", kid: testKID, key: signingKey})
			},
			expected: auth.ErrInvalidToken,
		},
		{
			name: "falsche Audience",
			token: func(t *testing.T, _ *jwksServer) string {
				return signToken(t, tokenOptions{audience: "api://other-service", kid: testKID, key: signingKey})
			},
			expected: auth.ErrInvalidToken,
		},
		{
			name: "manipulierte Signatur",
			token: func(t *testing.T, _ *jwksServer) string {
				return signToken(t, tokenOptions{kid: testKID, key: foreignKey})
			},
			expected: auth.ErrInvalidToken,
		},
		{
			name: "unbekanntes kid",
			token: func(t *testing.T, _ *jwksServer) string {
				return signToken(t, tokenOptions{kid: "unknown-kid", key: signingKey})
			},
			expected: auth.ErrInvalidToken,
		},
		{
			name: "Algorithmus-Verwechslung (HS256)",
			token: func(t *testing.T, _ *jwksServer) string {
				return signToken(t, tokenOptions{kid: testKID, method: jwt.SigningMethodHS256, key: signingKey})
			},
			expected: auth.ErrInvalidToken,
		},
		{
			name: "fehlendes sub-Claim",
			token: func(t *testing.T, _ *jwksServer) string {
				return signToken(t, tokenOptions{subject: "-", kid: testKID, key: signingKey})
			},
			expected: auth.ErrInvalidToken,
		},
		{
			name: "manipulierter Token-String",
			token: func(t *testing.T, _ *jwksServer) string {
				return signToken(t, tokenOptions{kid: testKID, key: signingKey}) + "x"
			},
			expected: auth.ErrInvalidToken,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			server := newJWKSServer(t)
			server.addKey(testKID, &signingKey.PublicKey)
			verifier := newVerifier(t, server, time.Minute)

			_, err := verifier.Verify(context.Background(), auth.Credentials{BearerToken: testCase.token(t, server)})
			if err == nil {
				t.Fatalf("Token wurde akzeptiert, erwartet wurde ein Fehler")
			}
			if !errors.Is(err, testCase.expected) {
				t.Fatalf("Fehler = %v, erwartet %v", err, testCase.expected)
			}
		})
	}
}

func TestJWTVerifierRequiresBearerToken(t *testing.T) {
	signingKey, _ := testKeys(t)
	server := newJWKSServer(t)
	server.addKey(testKID, &signingKey.PublicKey)
	verifier := newVerifier(t, server, time.Minute)

	_, err := verifier.Verify(context.Background(), auth.Credentials{UserEmail: testEmail})
	if !errors.Is(err, auth.ErrNoCredentials) {
		t.Fatalf("Fehler = %v, erwartet ErrNoCredentials (Dev-Header darf im JWT-Modus nicht greifen)", err)
	}
}

func TestJWTVerifierCachesJWKS(t *testing.T) {
	signingKey, _ := testKeys(t)
	server := newJWKSServer(t)
	server.addKey(testKID, &signingKey.PublicKey)
	verifier := newVerifier(t, server, time.Hour)

	token := signToken(t, tokenOptions{kid: testKID, key: signingKey})
	for i := 0; i < 3; i++ {
		if _, err := verifier.Verify(context.Background(), auth.Credentials{BearerToken: token}); err != nil {
			t.Fatalf("Verify #%d: %v", i, err)
		}
	}

	if hits := server.hits.Load(); hits != 1 {
		t.Fatalf("JWKS-Abrufe = %d, erwartet 1 (Cache)", hits)
	}
}

func TestJWTVerifierReloadsOnKeyRotation(t *testing.T) {
	signingKey, rotatedKey := testKeys(t)
	server := newJWKSServer(t)
	server.addKey(testKID, &signingKey.PublicKey)
	verifier := newVerifier(t, server, time.Hour)

	if _, err := verifier.Verify(context.Background(), auth.Credentials{
		BearerToken: signToken(t, tokenOptions{kid: testKID, key: signingKey}),
	}); err != nil {
		t.Fatalf("erster Verify: %v", err)
	}

	// Neue Schluessel-ID im JWKS (Rotation): unbekanntes kid loest Refresh aus.
	server.addKey("rotated-key-2", &rotatedKey.PublicKey)
	if _, err := verifier.Verify(context.Background(), auth.Credentials{
		BearerToken: signToken(t, tokenOptions{kid: "rotated-key-2", key: rotatedKey}),
	}); err != nil {
		t.Fatalf("Verify nach Rotation: %v", err)
	}

	if hits := server.hits.Load(); hits < 2 {
		t.Fatalf("JWKS-Abrufe = %d, erwartet >= 2 nach Rotation", hits)
	}
}

func TestJWTVerifierReportsUnavailableJWKS(t *testing.T) {
	signingKey, _ := testKeys(t)
	server := newJWKSServer(t)
	server.addKey(testKID, &signingKey.PublicKey)
	server.setFail(true)
	verifier := newVerifier(t, server, time.Minute)

	_, err := verifier.Verify(context.Background(), auth.Credentials{
		BearerToken: signToken(t, tokenOptions{kid: testKID, key: signingKey}),
	})
	if !errors.Is(err, auth.ErrKeyUnavailable) {
		t.Fatalf("Fehler = %v, erwartet ErrKeyUnavailable", err)
	}
}

func TestNewJWTVerifierValidatesConfig(t *testing.T) {
	tests := []struct {
		name string
		cfg  auth.JWKSConfig
	}{
		{name: "ohne Issuer", cfg: auth.JWKSConfig{Audience: testAudience, JWKSURL: "http://x/jwks"}},
		{name: "ohne Audience", cfg: auth.JWKSConfig{Issuer: testIssuer, JWKSURL: "http://x/jwks"}},
		{name: "ohne JWKS-URL", cfg: auth.JWKSConfig{Issuer: testIssuer, Audience: testAudience}},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := auth.NewJWTVerifier(testCase.cfg); err == nil {
				t.Fatal("Konfiguration wurde akzeptiert, erwartet wurde ein Fehler")
			}
		})
	}
}
