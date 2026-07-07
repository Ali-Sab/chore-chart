package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"
)

// ctxKey is an unexported type for context keys in this package.
type ctxKey string

// UsernameKey is the context key under which the authenticated username (JWT sub) is stored.
const UsernameKey ctxKey = "username"

func deny(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	if err := json.NewEncoder(w).Encode(map[string]string{"error": "unauthenticated"}); err != nil {
		log.Printf("[auth] encoding 401 response: %v", err)
	}
}

// KidFromToken extracts the "kid" header claim from a JWT without verifying its signature.
func KidFromToken(tokenStr string) string {
	parts := strings.SplitN(tokenStr, ".", 3)
	if len(parts) < 2 {
		return ""
	}
	hBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return ""
	}
	var header map[string]any
	if err := json.Unmarshal(hBytes, &header); err != nil {
		return ""
	}
	kid, _ := header["kid"].(string)
	return kid
}

// NewMiddleware returns an HTTP middleware that requires a valid RS256 JWT in
// the "cc_token" httpOnly cookie. On success it stores the JWT sub claim as the
// authenticated username in the request context under UsernameKey.
// Returns 401 JSON on missing or invalid token.
func NewMiddleware(keys KeyProvider, denylist *Denylist) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie("cc_token")
			if err != nil || cookie.Value == "" {
				deny(w)
				return
			}
			kid := KidFromToken(cookie.Value)
			claims, err := VerifyToken(cookie.Value, keys.PublicKey(kid))
			if err != nil {
				deny(w)
				return
			}
			sub, _ := claims["sub"].(string)
			iat, _ := claims["iat"].(float64)
			if denylist.IsDenied(sub, time.Unix(int64(iat), 0)) {
				deny(w)
				return
			}
			ctx := context.WithValue(r.Context(), UsernameKey, sub)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
