package auth

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"sync"
	"time"
)

var httpClient = &http.Client{Timeout: 10 * time.Second}

// JWKSRetryCount and JWKSRetryDelay control startup fetch behavior.
// Override in tests to avoid slow retries.
var (
	JWKSRetryCount = 5
	JWKSRetryDelay = 3 * time.Second
)

// KeyProvider returns the RSA public key for a given key ID.
// If kid is empty or not found, implementations should fall back to a default key.
type KeyProvider interface {
	PublicKey(kid string) *rsa.PublicKey
}

// StaticKeyProvider holds a fixed key — used in tests.
type StaticKeyProvider struct{ key *rsa.PublicKey }

func NewStaticKeyProvider(key *rsa.PublicKey) StaticKeyProvider {
	return StaticKeyProvider{key: key}
}

func (s StaticKeyProvider) PublicKey(_ string) *rsa.PublicKey { return s.key }

// JWKSKeyProvider fetches public keys from a JWKS endpoint at startup and
// refreshes them hourly so key rotation is picked up without a restart.
type JWKSKeyProvider struct {
	jwksURL string
	mu      sync.RWMutex
	keys    map[string]*rsa.PublicKey
	first   *rsa.PublicKey
}

func NewJWKSKeyProvider(ctx context.Context, jwksURL string) (*JWKSKeyProvider, error) {
	p := &JWKSKeyProvider{jwksURL: jwksURL}
	var lastErr error
	for i := 0; i < JWKSRetryCount; i++ {
		if err := p.Refresh(); err != nil {
			lastErr = err
			log.Printf("[auth] JWKS fetch attempt %d/%d failed: %v", i+1, JWKSRetryCount, err)
			if i < JWKSRetryCount-1 {
				time.Sleep(JWKSRetryDelay)
			}
			continue
		}
		go p.refreshLoop(ctx)
		return p, nil
	}
	return nil, fmt.Errorf("could not fetch JWKS from %s: %w", jwksURL, lastErr)
}

// PublicKey returns the RSA key matching kid, falling back to the first key.
func (p *JWKSKeyProvider) PublicKey(kid string) *rsa.PublicKey {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if kid != "" {
		if k, ok := p.keys[kid]; ok {
			return k
		}
	}
	return p.first
}

func (p *JWKSKeyProvider) Refresh() error {
	keys, first, err := fetchRSAKeys(p.jwksURL)
	if err != nil {
		return err
	}
	p.mu.Lock()
	p.keys = keys
	p.first = first
	p.mu.Unlock()
	return nil
}

func (p *JWKSKeyProvider) refreshLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := p.Refresh(); err != nil {
				log.Printf("[auth] JWKS refresh failed: %v", err)
			}
		case <-ctx.Done():
			return
		}
	}
}

type jwksResponse struct {
	Keys []jwk `json:"keys"`
}

type jwk struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	N   string `json:"n"`
	E   string `json:"e"`
}

// fetchRSAKeys fetches all RSA keys from the JWKS endpoint.
// Returns a map keyed by kid plus a pointer to the first key as the fallback.
func fetchRSAKeys(url string) (map[string]*rsa.PublicKey, *rsa.PublicKey, error) {
	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, nil, fmt.Errorf("GET %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("JWKS endpoint returned %d", resp.StatusCode)
	}
	var jwks jwksResponse
	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return nil, nil, fmt.Errorf("decoding JWKS: %w", err)
	}
	keys := make(map[string]*rsa.PublicKey)
	var first *rsa.PublicKey
	for _, k := range jwks.Keys {
		if k.Kty != "RSA" {
			continue
		}
		pub, err := jwkToRSA(k)
		if err != nil {
			return nil, nil, err
		}
		if first == nil {
			first = pub
		}
		if k.Kid != "" {
			keys[k.Kid] = pub
		}
	}
	if first == nil {
		return nil, nil, fmt.Errorf("no RSA key found in JWKS at %s", url)
	}
	return keys, first, nil
}

// fetchFirstRSAKey is kept for use in tests that only need a single key.
func fetchFirstRSAKey(url string) (*rsa.PublicKey, error) {
	_, first, err := fetchRSAKeys(url)
	return first, err
}

func jwkToRSA(k jwk) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(k.N)
	if err != nil {
		return nil, fmt.Errorf("decoding JWK n: %w", err)
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(k.E)
	if err != nil {
		return nil, fmt.Errorf("decoding JWK e: %w", err)
	}
	n := new(big.Int).SetBytes(nBytes)
	e := int(new(big.Int).SetBytes(eBytes).Int64())
	return &rsa.PublicKey{N: n, E: e}, nil
}
