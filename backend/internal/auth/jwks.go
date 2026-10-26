package auth

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"sync"
	"time"
)

const (
	maxJWKSBodyBytes = 1 << 20 // 1 MiB
	minRSAKeyBits    = 2048
)

// errUnknownKey signalisiert ein kid, das auch nach Refresh nicht existiert.
var errUnknownKey = errors.New("unknown key id")

// jwk ist ein einzelner Schluessel aus einem JWKS-Dokument.
type jwk struct {
	KID string `json:"kid"`
	KTY string `json:"kty"`
	Alg string `json:"alg"`
	Use string `json:"use"`
	N   string `json:"n"`
	E   string `json:"e"`
}

type jwkSet struct {
	Keys []jwk `json:"keys"`
}

// keyCache laedt JWKS einmal und haelt die Schluessel im Speicher.
// Unbekannte kid loesen genau einen erneuten Abruf aus (Key-Rotation).
type keyCache struct {
	url    string
	client *http.Client
	ttl    time.Duration
	now    func() time.Time

	mu        sync.RWMutex
	keys      map[string]*rsa.PublicKey
	anyKey    *rsa.PublicKey
	fetchedAt time.Time
}

func newKeyCache(url string, ttl time.Duration, client *http.Client) *keyCache {
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	return &keyCache{
		url:    url,
		client: client,
		ttl:    ttl,
		now:    time.Now,
		keys:   make(map[string]*rsa.PublicKey),
	}
}

// key liefert den Schluessel zu kid; bei leerem kid den einzigen vorhandenen.
func (c *keyCache) key(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	if key, ok := c.cached(kid); ok {
		return key, nil
	}

	// Refresh und Pruefung unter exklusivem Lock (verhindert Thundering Herd).
	c.mu.Lock()
	defer c.mu.Unlock()

	if key, ok := c.cachedLocked(kid); ok {
		return key, nil
	}

	if err := c.refreshLocked(ctx); err != nil {
		return nil, err
	}
	if key, ok := c.cachedLocked(kid); ok {
		return key, nil
	}
	return nil, fmt.Errorf("%w: %q", errUnknownKey, kid)
}

func (c *keyCache) cached(kid string) (*rsa.PublicKey, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.cachedLocked(kid)
}

func (c *keyCache) cachedLocked(kid string) (*rsa.PublicKey, bool) {
	if c.fetchedAt.IsZero() || c.now().Sub(c.fetchedAt) > c.ttl {
		return nil, false
	}
	if kid == "" {
		if c.anyKey == nil {
			return nil, false
		}
		return c.anyKey, true
	}
	key, ok := c.keys[kid]
	return key, ok
}

func (c *keyCache) refreshLocked(ctx context.Context) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url, http.NoBody)
	if err != nil {
		return fmt.Errorf("%w: build jwks request: %w", ErrKeyUnavailable, err)
	}
	request.Header.Set("Accept", "application/json")

	response, err := c.client.Do(request)
	if err != nil {
		return fmt.Errorf("%w: fetch jwks: %w", ErrKeyUnavailable, err)
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxJWKSBodyBytes))
		return fmt.Errorf("%w: jwks status %d", ErrKeyUnavailable, response.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, maxJWKSBodyBytes))
	if err != nil {
		return fmt.Errorf("%w: read jwks: %w", ErrKeyUnavailable, err)
	}

	keys, anyKey, err := parseJWKS(body)
	if err != nil {
		return err
	}

	c.keys = keys
	c.anyKey = anyKey
	c.fetchedAt = c.now()
	return nil
}

// parseJWKS wandelt ein JWKS-Dokument in RSA-Public-Keys.
func parseJWKS(body []byte) (map[string]*rsa.PublicKey, *rsa.PublicKey, error) {
	var set jwkSet
	if err := json.Unmarshal(body, &set); err != nil {
		return nil, nil, fmt.Errorf("%w: decode jwks: %w", ErrKeyUnavailable, err)
	}
	if len(set.Keys) == 0 {
		return nil, nil, fmt.Errorf("%w: jwks contains no keys", ErrKeyUnavailable)
	}

	keys := make(map[string]*rsa.PublicKey, len(set.Keys))
	var anyKey *rsa.PublicKey
	for _, entry := range set.Keys {
		key, err := entry.rsaPublicKey()
		if err != nil {
			// Einzelne unbrauchbare Schluessel ueberspringen (Rotation/andere Verfahren).
			continue
		}
		if entry.KID != "" {
			keys[entry.KID] = key
		}
		if anyKey == nil {
			anyKey = key
		}
	}

	if len(keys) == 0 && anyKey == nil {
		return nil, nil, fmt.Errorf("%w: jwks contains no usable RSA keys", ErrKeyUnavailable)
	}
	return keys, anyKey, nil
}

func (entry jwk) rsaPublicKey() (*rsa.PublicKey, error) {
	if entry.KTY != "RSA" {
		return nil, fmt.Errorf("unsupported kty %q", entry.KTY)
	}
	if entry.Use != "" && entry.Use != "sig" {
		return nil, fmt.Errorf("unsupported use %q", entry.Use)
	}
	if entry.Alg != "" && entry.Alg != "RS256" {
		return nil, fmt.Errorf("unsupported alg %q", entry.Alg)
	}

	modulus, err := decodeBigInt(entry.N)
	if err != nil {
		return nil, err
	}
	exponent, err := decodeExponent(entry.E)
	if err != nil {
		return nil, err
	}

	key := &rsa.PublicKey{N: modulus, E: exponent}
	if key.Size()*8 < minRSAKeyBits {
		return nil, errors.New("rsa key too small")
	}
	return key, nil
}

func decodeBigInt(value string) (*big.Int, error) {
	if value == "" {
		return nil, errors.New("empty modulus")
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("decode modulus: %w", err)
	}
	modulus := new(big.Int).SetBytes(raw)
	if modulus.Sign() <= 0 {
		return nil, errors.New("invalid modulus")
	}
	return modulus, nil
}

func decodeExponent(value string) (int, error) {
	if value == "" {
		return 0, errors.New("empty exponent")
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return 0, fmt.Errorf("decode exponent: %w", err)
	}
	if len(raw) == 0 || len(raw) > 4 {
		return 0, errors.New("invalid exponent length")
	}
	exponent := 0
	for _, b := range raw {
		exponent = exponent<<8 | int(b)
	}
	if exponent < 3 || exponent%2 == 0 {
		return 0, errors.New("invalid exponent value")
	}
	return exponent, nil
}
