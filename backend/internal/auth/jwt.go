package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/Max-Christoph/anchor-blueprint/backend/internal/httpx"
)

// JWKSConfig konfiguriert die RS256-JWT-Validierung gegen ein JWKS.
type JWKSConfig struct {
	Issuer      string
	Audience    string
	JWKSURL     string
	CacheTTL    time.Duration
	HTTPTimeout time.Duration
	Leeway      time.Duration
}

// Claims sind die aus dem Token gelesenen Felder.
type Claims struct {
	jwt.RegisteredClaims
	Email             string   `json:"email"`
	PreferredUsername string   `json:"preferred_username"`
	Roles             []string `json:"roles"`
	Groups            []string `json:"groups"`
}

// JWTVerifier validiert Bearer-Tokens (RS256) gegen ein JWKS.
type JWTVerifier struct {
	keys     *keyCache
	parser   *jwt.Parser
	issuer   string
	audience string
}

// NewJWTVerifier erzeugt den Verifier und validiert die Konfiguration.
func NewJWTVerifier(cfg JWKSConfig) (*JWTVerifier, error) {
	if strings.TrimSpace(cfg.Issuer) == "" {
		return nil, errors.New("OIDC_ISSUER is required for AUTH_MODE=jwt")
	}
	if strings.TrimSpace(cfg.Audience) == "" {
		return nil, errors.New("OIDC_AUDIENCE is required for AUTH_MODE=jwt")
	}
	if strings.TrimSpace(cfg.JWKSURL) == "" {
		return nil, errors.New("OIDC_JWKS_URL is required for AUTH_MODE=jwt")
	}

	timeout := cfg.HTTPTimeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	client := &http.Client{Timeout: timeout}

	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Alg()}),
		jwt.WithIssuer(cfg.Issuer),
		jwt.WithAudience(cfg.Audience),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
		jwt.WithLeeway(cfg.Leeway),
	)

	return &JWTVerifier{
		keys:     newKeyCache(cfg.JWKSURL, cfg.CacheTTL, client),
		parser:   parser,
		issuer:   cfg.Issuer,
		audience: cfg.Audience,
	}, nil
}

// Mode implementiert Verifier.
func (v *JWTVerifier) Mode() string { return ModeJWT }

// Verify prueft das Bearer-Token und liefert die Identitaet.
// Der Dev-Header wird im JWT-Modus bewusst ignoriert.
func (v *JWTVerifier) Verify(ctx context.Context, credentials Credentials) (httpx.Identity, error) {
	raw := strings.TrimSpace(credentials.BearerToken)
	if raw == "" {
		return httpx.Identity{}, ErrNoCredentials
	}

	claims := &Claims{}
	var keyErr error

	keyfunc := func(token *jwt.Token) (any, error) {
		kid, _ := token.Header["kid"].(string)
		key, err := v.keys.key(ctx, kid)
		if err != nil {
			keyErr = err
			return nil, err
		}
		return key, nil
	}

	token, err := v.parser.ParseWithClaims(raw, claims, keyfunc)
	if err != nil {
		if keyErr != nil {
			if errors.Is(keyErr, errUnknownKey) {
				return httpx.Identity{}, fmt.Errorf("%w: %w", ErrInvalidToken, keyErr)
			}
			return httpx.Identity{}, keyErr
		}
		return httpx.Identity{}, classifyJWTError(err)
	}
	if !token.Valid {
		return httpx.Identity{}, ErrInvalidToken
	}
	if strings.TrimSpace(claims.Subject) == "" {
		return httpx.Identity{}, fmt.Errorf("%w: missing sub claim", ErrInvalidToken)
	}

	email := claims.Email
	if email == "" {
		email = claims.PreferredUsername
	}

	return httpx.Identity{
		Subject: claims.Subject,
		Email:   email,
		Roles:   claims.Roles,
		Groups:  claims.Groups,
		Method:  ModeJWT,
	}, nil
}

// classifyJWTError uebersetzt jwt-Fehler in die Paket-Fehlerklassen.
func classifyJWTError(err error) error {
	switch {
	case errors.Is(err, jwt.ErrTokenExpired):
		return fmt.Errorf("%w: %w", ErrTokenExpired, err)
	case errors.Is(err, jwt.ErrTokenUnverifiable):
		if errors.Is(err, ErrKeyUnavailable) {
			return err
		}
		return fmt.Errorf("%w: %w", ErrInvalidToken, err)
	default:
		return fmt.Errorf("%w: %w", ErrInvalidToken, err)
	}
}
