package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestJwkToRSA_RoundTrip(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	pub := &priv.PublicKey

	// Encode to JWK format
	nB64 := base64.RawURLEncoding.EncodeToString(pub.N.Bytes())
	e := big.NewInt(int64(pub.E))
	eB64 := base64.RawURLEncoding.EncodeToString(e.Bytes())

	result, err := jwkToRSA(jwk{Kty: "RSA", N: nB64, E: eB64})
	if err != nil {
		t.Fatalf("jwkToRSA: %v", err)
	}

	if result.N.Cmp(pub.N) != 0 {
		t.Error("N mismatch")
	}
	if result.E != pub.E {
		t.Errorf("E mismatch: want %d, got %d", pub.E, result.E)
	}
}

func TestJwkToRSA_InvalidN(t *testing.T) {
	_, err := jwkToRSA(jwk{Kty: "RSA", N: "!!!invalid!!!", E: "AQAB"})
	if err == nil {
		t.Error("expected error for invalid N, got nil")
	}
}

func TestJwkToRSA_InvalidE(t *testing.T) {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	nB64 := base64.RawURLEncoding.EncodeToString(priv.PublicKey.N.Bytes())
	_, err := jwkToRSA(jwk{Kty: "RSA", N: nB64, E: "!!!invalid!!!"})
	if err == nil {
		t.Error("expected error for invalid E, got nil")
	}
}

func TestFetchFirstRSAKey_ValidServer(t *testing.T) {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	pub := &priv.PublicKey
	nB64 := base64.RawURLEncoding.EncodeToString(pub.N.Bytes())
	eB64 := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes())

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"keys":[{"kty":"RSA","n":"` + nB64 + `","e":"` + eB64 + `"}]}`))
	}))
	defer srv.Close()

	got, err := fetchFirstRSAKey(srv.URL)
	if err != nil {
		t.Fatalf("fetchFirstRSAKey: %v", err)
	}
	if got.N.Cmp(pub.N) != 0 {
		t.Error("N mismatch")
	}
}

func TestFetchFirstRSAKey_NoRSAKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"keys":[{"kty":"EC"}]}`))
	}))
	defer srv.Close()

	_, err := fetchFirstRSAKey(srv.URL)
	if err == nil {
		t.Error("expected error when no RSA key in JWKS")
	}
}

func TestFetchFirstRSAKey_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := fetchFirstRSAKey(srv.URL)
	if err == nil {
		t.Error("expected error on 500 response")
	}
}

func TestFetchFirstRSAKey_EmptyKeys(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"keys":[]}`))
	}))
	defer srv.Close()

	_, err := fetchFirstRSAKey(srv.URL)
	if err == nil {
		t.Error("expected error for empty keys array")
	}
}

func TestNewJWKSKeyProvider_RetryOnFailure(t *testing.T) {
	// Override retry settings for speed
	origCount := JWKSRetryCount
	origDelay := JWKSRetryDelay
	JWKSRetryCount = 2
	JWKSRetryDelay = 0
	defer func() {
		JWKSRetryCount = origCount
		JWKSRetryDelay = origDelay
	}()

	attempts := 0
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	pub := &priv.PublicKey
	nB64 := base64.RawURLEncoding.EncodeToString(pub.N.Bytes())
	eB64 := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes())

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"keys":[{"kty":"RSA","n":"` + nB64 + `","e":"` + eB64 + `"}]}`))
	}))
	defer srv.Close()

	p, err := NewJWKSKeyProvider(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("expected success after retry, got: %v", err)
	}
	if p.PublicKey("") == nil {
		t.Error("expected non-nil public key")
	}
}

func TestStaticKeyProvider(t *testing.T) {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	p := NewStaticKeyProvider(&priv.PublicKey)
	if p.PublicKey("") != &priv.PublicKey {
		t.Error("StaticKeyProvider returned wrong key")
	}
}
