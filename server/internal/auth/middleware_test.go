package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func generateTestKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating test RSA key: %v", err)
	}
	return key
}

func signToken(t *testing.T, key *rsa.PrivateKey, claims jwt.MapClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	s, err := tok.SignedString(key)
	if err != nil {
		t.Fatalf("signing token: %v", err)
	}
	return s
}

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func TestMiddleware_MissingCookie(t *testing.T) {
	key := generateTestKey(t)
	mw := NewMiddleware(NewStaticKeyProvider(&key.PublicKey), NewDenylist())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	mw(okHandler()).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
	var body map[string]string
	json.NewDecoder(w.Body).Decode(&body)
	if body["error"] != "unauthenticated" {
		t.Errorf("expected error=unauthenticated, got %q", body["error"])
	}
}

func TestMiddleware_EmptyCookie(t *testing.T) {
	key := generateTestKey(t)
	mw := NewMiddleware(NewStaticKeyProvider(&key.PublicKey), NewDenylist())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "cc_token", Value: ""})
	w := httptest.NewRecorder()
	mw(okHandler()).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestMiddleware_InvalidToken(t *testing.T) {
	key := generateTestKey(t)
	mw := NewMiddleware(NewStaticKeyProvider(&key.PublicKey), NewDenylist())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "cc_token", Value: "not.a.token"})
	w := httptest.NewRecorder()
	mw(okHandler()).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestMiddleware_TokenSignedWithWrongKey(t *testing.T) {
	serverKey := generateTestKey(t)
	attackerKey := generateTestKey(t)
	mw := NewMiddleware(NewStaticKeyProvider(&serverKey.PublicKey), NewDenylist())

	token := signToken(t, attackerKey, jwt.MapClaims{
		"sub": "user1",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "cc_token", Value: token})
	w := httptest.NewRecorder()
	mw(okHandler()).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for wrong-key token, got %d", w.Code)
	}
}

func TestMiddleware_ExpiredToken(t *testing.T) {
	key := generateTestKey(t)
	mw := NewMiddleware(NewStaticKeyProvider(&key.PublicKey), NewDenylist())

	token := signToken(t, key, jwt.MapClaims{
		"sub": "user1",
		"exp": time.Now().Add(-time.Hour).Unix(),
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "cc_token", Value: token})
	w := httptest.NewRecorder()
	mw(okHandler()).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for expired token, got %d", w.Code)
	}
}

func TestMiddleware_ValidToken(t *testing.T) {
	key := generateTestKey(t)
	mw := NewMiddleware(NewStaticKeyProvider(&key.PublicKey), NewDenylist())

	token := signToken(t, key, jwt.MapClaims{
		"sub": "user1",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	req := httptest.NewRequest(http.MethodGet, "/api/schedule", nil)
	req.AddCookie(&http.Cookie{Name: "cc_token", Value: token})
	w := httptest.NewRecorder()
	mw(okHandler()).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for valid token, got %d", w.Code)
	}
}
