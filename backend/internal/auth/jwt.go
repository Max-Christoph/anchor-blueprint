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

	// RolesClaim benennt den Rollen-Claim (Default "roles").
	RolesClaim string
	// GroupsClaim benennt den Gruppen-Claim (Default "groups").
	GroupsClaim string
	// GroupRoleMap ordnet Gruppen-IDs zusaetzliche Rollen zu
	// (z. B. Entra-Objekt-ID -> "Admin").
	GroupRoleMap map[string][]string
}

// JWTVerifier validiert Bearer-Tokens (RS256) gegen ein JWKS.
type JWTVerifier struct {
	keys        *keyCache
	parser      *jwt.Parser
	rolesClaim  string
	groupsClaim string
	groupRoles  map[string][]string
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

	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Alg()}),
		jwt.WithIssuer(cfg.Issuer),
		jwt.WithAudience(cfg.Audience),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
		jwt.WithLeeway(cfg.Leeway),
	)

	rolesClaim := cfg.RolesClaim
	if rolesClaim == "" {
		rolesClaim = "roles"
	}
	groupsClaim := cfg.GroupsClaim
	if groupsClaim == "" {
		groupsClaim = "groups"
	}

	groupRoles := make(map[string][]string, len(cfg.GroupRoleMap))
	for group, roles := range cfg.GroupRoleMap {
		groupRoles[strings.ToLower(strings.TrimSpace(group))] = roles
	}

	return &JWTVerifier{
		keys:        newKeyCache(cfg.JWKSURL, cfg.CacheTTL, &http.Client{Timeout: timeout}),
		parser:      parser,
		rolesClaim:  rolesClaim,
		groupsClaim: groupsClaim,
		groupRoles:  groupRoles,
	}, nil
}

// Mode implementiert Verifier.
func (v *JWTVerifier) Mode() string { return ModeJWT }

// Verify prueft das Bearer-Token und mappt Claims in die Identitaet.
// Der Dev-Header wird im JWT-Modus bewusst ignoriert.
func (v *JWTVerifier) Verify(ctx context.Context, credentials Credentials) (httpx.Identity, error) {
	raw := strings.TrimSpace(credentials.BearerToken)
	if raw == "" {
		return httpx.Identity{}, ErrNoCredentials
	}

	claims := jwt.MapClaims{}
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

	subject, err := claims.GetSubject()
	if err != nil || strings.TrimSpace(subject) == "" {
		return httpx.Identity{}, fmt.Errorf("%w: missing sub claim", ErrInvalidToken)
	}

	groups := toStringList(claims[v.groupsClaim])
	roles := toStringList(claims[v.rolesClaim])
	for _, group := range groups {
		roles = append(roles, v.groupRoles[strings.ToLower(strings.TrimSpace(group))]...)
	}

	email := stringClaim(claims, "email")
	if email == "" {
		email = stringClaim(claims, "preferred_username")
	}

	return httpx.Identity{
		Subject: subject,
		Email:   email,
		Roles:   SplitList(strings.Join(roles, ",")),
		Groups:  groups,
		Method:  ModeJWT,
	}, nil
}

// stringClaim liest einen String-Claim defensiv.
func stringClaim(claims jwt.MapClaims, key string) string {
	value, ok := claims[key].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(value)
}

// classifyJWTError uebersetzt jwt-Fehler in die Paket-Fehlerklassen.
func classifyJWTError(err error) error {
	if errors.Is(err, jwt.ErrTokenExpired) {
		return fmt.Errorf("%w: %w", ErrTokenExpired, err)
	}
	return fmt.Errorf("%w: %w", ErrInvalidToken, err)
}
