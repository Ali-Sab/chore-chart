package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	gojwt "github.com/golang-jwt/jwt/v5"

	"chore-chart/internal/auth"
	"chore-chart/internal/db"
)

// ─── Test fixtures ────────────────────────────────────────────────────────────

type testEnv struct {
	handler     http.Handler
	privKey     *rsa.PrivateKey
	tokenServer *httptest.Server
	cfg         serverConfig
	sqlDB       *sql.DB
	testGroupID int64
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()

	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating RSA key: %v", err)
	}

	var tokenServer *httptest.Server
	tokenServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tok := gojwt.NewWithClaims(gojwt.SigningMethodRS256, gojwt.MapClaims{
			"sub": "testuser",
			"exp": time.Now().Add(time.Hour).Unix(),
		})
		signed, _ := tok.SignedString(privKey)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"access_token":  signed,
			"refresh_token": "refresh-abc",
			"expires_in":    3600,
		})
	}))
	t.Cleanup(tokenServer.Close)

	keys := auth.NewStaticKeyProvider(&privKey.PublicKey)

	sqlDB, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("opening in-memory db: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })

	gid, err := db.CreateGroup(sqlDB, "test", "testuser")
	if err != nil {
		t.Fatalf("creating test group: %v", err)
	}
	if _, err := db.AddChore(sqlDB, gid, "Dishes", "testuser", 1.0); err != nil {
		t.Fatalf("adding test chore: %v", err)
	}

	cfg := serverConfig{
		authorizeURL: "https://auth.example.com/authorize",
		tokenURL:     tokenServer.URL + "/token",
		clientID:     "test-client-id",
		clientSecret: "test-client-secret",
		redirectURI:  "https://app.example.com/oauth/callback",
		keys:         keys,
		tokenClient:  tokenServer.Client(),
		db:           sqlDB,
	}

	handler := newHandler(context.Background(), cfg)
	return &testEnv{
		handler:     handler,
		privKey:     privKey,
		tokenServer: tokenServer,
		cfg:         cfg,
		sqlDB:       sqlDB,
		testGroupID: gid,
	}
}

// signToken creates a valid RS256 JWT signed with the test private key.
func (e *testEnv) signToken(t *testing.T, claims gojwt.MapClaims) string {
	t.Helper()
	if claims["exp"] == nil {
		claims["exp"] = time.Now().Add(time.Hour).Unix()
	}
	tok := gojwt.NewWithClaims(gojwt.SigningMethodRS256, claims)
	s, err := tok.SignedString(e.privKey)
	if err != nil {
		t.Fatalf("signing token: %v", err)
	}
	return s
}

// authedRequest builds a GET request with the testuser's token cookie.
func (e *testEnv) authedRequest(t *testing.T, method, path string, body string) *http.Request {
	t.Helper()
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	r.AddCookie(&http.Cookie{Name: "cc_token", Value: e.signToken(t, gojwt.MapClaims{"sub": "testuser"})})
	return r
}

// groupPath returns a path under the test group.
func (e *testEnv) groupPath(suffix string) string {
	return fmt.Sprintf("/api/groups/%d%s", e.testGroupID, suffix)
}

// startOAuth performs /oauth/start and returns the state value.
func (e *testEnv) startOAuth(t *testing.T, returnTo string) string {
	t.Helper()
	target := "/oauth/start"
	if returnTo != "" {
		target += "?return_to=" + url.QueryEscape(returnTo)
	}
	req := httptest.NewRequest(http.MethodGet, target, nil)
	w := httptest.NewRecorder()
	e.handler.ServeHTTP(w, req)
	if w.Code != http.StatusFound {
		t.Fatalf("oauth/start: expected 302, got %d", w.Code)
	}
	loc := w.Header().Get("Location")
	u, err := url.Parse(loc)
	if err != nil {
		t.Fatalf("parsing redirect: %v", err)
	}
	return u.Query().Get("state")
}

// cookie returns the named cookie from a recorder's response, or nil.
func cookie(w *httptest.ResponseRecorder, name string) *http.Cookie {
	resp := w.Result()
	for _, c := range resp.Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// ─── PKCE helpers ─────────────────────────────────────────────────────────────

func TestGenerateVerifier_Length(t *testing.T) {
	v := generateVerifier()
	if len(v) != 43 {
		t.Errorf("expected verifier length 43, got %d", len(v))
	}
	if _, err := base64.RawURLEncoding.DecodeString(v); err != nil {
		t.Errorf("verifier is not valid base64url: %v", err)
	}
}

func TestGenerateVerifier_Unique(t *testing.T) {
	a, b := generateVerifier(), generateVerifier()
	if a == b {
		t.Error("two verifiers should not be equal")
	}
}

func TestRandomState_Length(t *testing.T) {
	s := randomState()
	if len(s) != 22 {
		t.Errorf("expected state length 22, got %d", len(s))
	}
}

func TestRandomState_Unique(t *testing.T) {
	a, b := randomState(), randomState()
	if a == b {
		t.Error("two states should not be equal")
	}
}

func TestCodeChallenge_S256(t *testing.T) {
	result := codeChallenge("abc")
	if result == "" {
		t.Error("codeChallenge returned empty string")
	}
	if codeChallenge("abc") != codeChallenge("abc") {
		t.Error("codeChallenge is not deterministic")
	}
	if codeChallenge("abc") == codeChallenge("xyz") {
		t.Error("codeChallenge collision on different inputs")
	}
}

// ─── /oauth/start ─────────────────────────────────────────────────────────────

func TestOAuthStart_RedirectsToAuthorizeURL(t *testing.T) {
	e := newTestEnv(t)
	req := httptest.NewRequest(http.MethodGet, "/oauth/start", nil)
	w := httptest.NewRecorder()
	e.handler.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", w.Code)
	}
	loc := w.Header().Get("Location")
	if !strings.HasPrefix(loc, "https://auth.example.com/authorize") {
		t.Errorf("unexpected redirect location: %s", loc)
	}
}

func TestOAuthStart_RequiredParams(t *testing.T) {
	e := newTestEnv(t)
	req := httptest.NewRequest(http.MethodGet, "/oauth/start", nil)
	w := httptest.NewRecorder()
	e.handler.ServeHTTP(w, req)

	u, _ := url.Parse(w.Header().Get("Location"))
	q := u.Query()

	for _, param := range []string{"response_type", "client_id", "redirect_uri", "code_challenge", "code_challenge_method", "state"} {
		if q.Get(param) == "" {
			t.Errorf("missing query param %q in authorize redirect", param)
		}
	}
	if q.Get("response_type") != "code" {
		t.Errorf("response_type should be 'code', got %q", q.Get("response_type"))
	}
	if q.Get("code_challenge_method") != "S256" {
		t.Errorf("code_challenge_method should be 'S256', got %q", q.Get("code_challenge_method"))
	}
	if q.Get("client_id") != "test-client-id" {
		t.Errorf("unexpected client_id: %q", q.Get("client_id"))
	}
}

func TestOAuthStart_StateIsUnique(t *testing.T) {
	e := newTestEnv(t)
	s1 := e.startOAuth(t, "")
	s2 := e.startOAuth(t, "")
	if s1 == s2 {
		t.Error("two OAuth flows should have different states")
	}
}

func TestOAuthStart_ValidReturnTo(t *testing.T) {
	e := newTestEnv(t)
	state := e.startOAuth(t, "/dashboard")

	req := httptest.NewRequest(http.MethodGet, "/oauth/callback?state="+state+"&code=authcode", nil)
	w := httptest.NewRecorder()
	e.handler.ServeHTTP(w, req)

	if loc := w.Header().Get("Location"); loc != "/dashboard" {
		t.Errorf("expected redirect to /dashboard, got %q", loc)
	}
}

func TestOAuthStart_AbsoluteReturnToIsRejected(t *testing.T) {
	e := newTestEnv(t)
	state := e.startOAuth(t, "http://evil.com/steal")

	req := httptest.NewRequest(http.MethodGet, "/oauth/callback?state="+state+"&code=authcode", nil)
	w := httptest.NewRecorder()
	e.handler.ServeHTTP(w, req)

	if loc := w.Header().Get("Location"); loc != "/chores/" {
		t.Errorf("expected redirect to /chores/ for absolute URL, got %q", loc)
	}
}

func TestOAuthStart_EmptyReturnToDefaultsToChores(t *testing.T) {
	e := newTestEnv(t)
	state := e.startOAuth(t, "")

	req := httptest.NewRequest(http.MethodGet, "/oauth/callback?state="+state+"&code=authcode", nil)
	w := httptest.NewRecorder()
	e.handler.ServeHTTP(w, req)

	if loc := w.Header().Get("Location"); loc != "/chores/" {
		t.Errorf("expected /chores/, got %q", loc)
	}
}

func TestOAuthStart_MethodNotAllowed(t *testing.T) {
	e := newTestEnv(t)
	req := httptest.NewRequest(http.MethodPost, "/oauth/start", nil)
	w := httptest.NewRecorder()
	e.handler.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

// ─── /oauth/callback ──────────────────────────────────────────────────────────

func TestOAuthCallback_MethodNotAllowed(t *testing.T) {
	e := newTestEnv(t)
	req := httptest.NewRequest(http.MethodPost, "/oauth/callback", nil)
	w := httptest.NewRecorder()
	e.handler.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestOAuthCallback_ErrorParam(t *testing.T) {
	e := newTestEnv(t)
	req := httptest.NewRequest(http.MethodGet, "/oauth/callback?error=access_denied", nil)
	w := httptest.NewRecorder()
	e.handler.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); !strings.Contains(loc, "auth_error=access_denied") {
		t.Errorf("expected auth_error in redirect, got %q", loc)
	}
}

func TestOAuthCallback_MissingState(t *testing.T) {
	e := newTestEnv(t)
	req := httptest.NewRequest(http.MethodGet, "/oauth/callback?code=someCode", nil)
	w := httptest.NewRecorder()
	e.handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing state, got %d", w.Code)
	}
}

func TestOAuthCallback_UnknownState(t *testing.T) {
	e := newTestEnv(t)
	req := httptest.NewRequest(http.MethodGet, "/oauth/callback?state=notregistered&code=x", nil)
	w := httptest.NewRecorder()
	e.handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for unknown state, got %d", w.Code)
	}
}

func TestOAuthCallback_StateReplayPrevented(t *testing.T) {
	e := newTestEnv(t)
	state := e.startOAuth(t, "")

	do := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/oauth/callback?state="+state+"&code=x", nil)
		w := httptest.NewRecorder()
		e.handler.ServeHTTP(w, req)
		return w
	}

	if do().Code != http.StatusFound {
		t.Fatal("first callback should succeed (302)")
	}
	if do().Code != http.StatusBadRequest {
		t.Error("second callback with same state should fail (400)")
	}
}

func TestOAuthCallback_MissingCode(t *testing.T) {
	e := newTestEnv(t)
	state := e.startOAuth(t, "")

	req := httptest.NewRequest(http.MethodGet, "/oauth/callback?state="+state, nil)
	w := httptest.NewRecorder()
	e.handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing code, got %d", w.Code)
	}
}

func TestOAuthCallback_SetsTokenCookie(t *testing.T) {
	e := newTestEnv(t)
	state := e.startOAuth(t, "/chores/")

	req := httptest.NewRequest(http.MethodGet, "/oauth/callback?state="+state+"&code=authcode", nil)
	w := httptest.NewRecorder()
	e.handler.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d body: %s", w.Code, w.Body.String())
	}
	c := cookie(w, "cc_token")
	if c == nil {
		t.Fatal("cc_token cookie not set")
	}
	if !c.HttpOnly {
		t.Error("cc_token must be HttpOnly")
	}
	if !c.Secure {
		t.Error("cc_token must be Secure")
	}
	if c.SameSite != http.SameSiteStrictMode {
		t.Error("cc_token must be SameSite=Strict")
	}
	if c.MaxAge <= 0 {
		t.Errorf("cc_token MaxAge should be positive, got %d", c.MaxAge)
	}
}

func TestOAuthCallback_SetsRefreshCookie(t *testing.T) {
	e := newTestEnv(t)
	state := e.startOAuth(t, "")

	req := httptest.NewRequest(http.MethodGet, "/oauth/callback?state="+state+"&code=authcode", nil)
	w := httptest.NewRecorder()
	e.handler.ServeHTTP(w, req)

	c := cookie(w, "cc_refresh")
	if c == nil {
		t.Fatal("cc_refresh cookie not set")
	}
	if c.MaxAge != 30*24*3600 {
		t.Errorf("cc_refresh MaxAge should be 30 days, got %d", c.MaxAge)
	}
}

func TestOAuthCallback_TokenServerError(t *testing.T) {
	e := newTestEnv(t)

	badServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer badServer.Close()
	e.cfg.tokenURL = badServer.URL + "/token"
	e.cfg.tokenClient = badServer.Client()
	e.handler = newHandler(context.Background(), e.cfg)

	state := e.startOAuth(t, "")
	req := httptest.NewRequest(http.MethodGet, "/oauth/callback?state="+state+"&code=x", nil)
	w := httptest.NewRecorder()
	e.handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadGateway {
		t.Errorf("expected 502 on bad token response, got %d", w.Code)
	}
}

func TestOAuthCallback_EmptyAccessToken(t *testing.T) {
	e := newTestEnv(t)

	badServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"access_token": "", "expires_in": 3600})
	}))
	defer badServer.Close()
	e.cfg.tokenURL = badServer.URL + "/token"
	e.cfg.tokenClient = badServer.Client()
	e.handler = newHandler(context.Background(), e.cfg)

	state := e.startOAuth(t, "")
	req := httptest.NewRequest(http.MethodGet, "/oauth/callback?state="+state+"&code=x", nil)
	w := httptest.NewRecorder()
	e.handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadGateway {
		t.Errorf("expected 502 for empty access_token, got %d", w.Code)
	}
}

func TestOAuthCallback_DefaultMaxAge(t *testing.T) {
	e := newTestEnv(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tok := gojwt.NewWithClaims(gojwt.SigningMethodRS256, gojwt.MapClaims{
			"sub": "u", "exp": time.Now().Add(time.Hour).Unix(),
		})
		signed, _ := tok.SignedString(e.privKey)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"access_token": signed})
	}))
	defer srv.Close()
	e.cfg.tokenURL = srv.URL + "/token"
	e.cfg.tokenClient = srv.Client()
	e.handler = newHandler(context.Background(), e.cfg)

	state := e.startOAuth(t, "")
	req := httptest.NewRequest(http.MethodGet, "/oauth/callback?state="+state+"&code=x", nil)
	w := httptest.NewRecorder()
	e.handler.ServeHTTP(w, req)

	c := cookie(w, "cc_token")
	if c == nil {
		t.Fatal("cc_token not set")
	}
	if c.MaxAge != 3600 {
		t.Errorf("expected default MaxAge 3600, got %d", c.MaxAge)
	}
}

// ─── /api/auth/logout ─────────────────────────────────────────────────────────

func TestLogout_ClearsBothCookies(t *testing.T) {
	e := newTestEnv(t)
	req := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	w := httptest.NewRecorder()
	e.handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	for _, name := range []string{"cc_token", "cc_refresh"} {
		c := cookie(w, name)
		if c == nil {
			t.Errorf("expected %s cookie in response", name)
			continue
		}
		if c.MaxAge != -1 {
			t.Errorf("%s cookie MaxAge should be -1 (delete), got %d", name, c.MaxAge)
		}
	}
}

func TestLogout_MethodNotAllowed(t *testing.T) {
	e := newTestEnv(t)
	req := httptest.NewRequest(http.MethodGet, "/api/auth/logout", nil)
	w := httptest.NewRecorder()
	e.handler.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

// ─── Protected API — auth failures ────────────────────────────────────────────

func TestAPI_Unauthenticated(t *testing.T) {
	e := newTestEnv(t)
	paths := []string{
		"/api/groups",
		e.groupPath("/schedule"),
		e.groupPath("/chores"),
		e.groupPath("/members"),
	}
	for _, path := range paths {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		e.handler.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s: expected 401 without cookie, got %d", path, w.Code)
		}
		var body map[string]string
		json.NewDecoder(w.Body).Decode(&body)
		if body["error"] != "unauthenticated" {
			t.Errorf("%s: expected error=unauthenticated, got %q", path, body["error"])
		}
	}
}

func TestAPI_InvalidToken(t *testing.T) {
	e := newTestEnv(t)
	for _, path := range []string{"/api/groups", e.groupPath("/schedule")} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.AddCookie(&http.Cookie{Name: "cc_token", Value: "not.a.real.token"})
		w := httptest.NewRecorder()
		e.handler.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s: expected 401 for bad token, got %d", path, w.Code)
		}
	}
}

func TestAPI_ExpiredToken(t *testing.T) {
	e := newTestEnv(t)
	tok := gojwt.NewWithClaims(gojwt.SigningMethodRS256, gojwt.MapClaims{
		"sub": "u", "exp": time.Now().Add(-time.Hour).Unix(),
	})
	expired, _ := tok.SignedString(e.privKey)

	req := httptest.NewRequest(http.MethodGet, "/api/groups", nil)
	req.AddCookie(&http.Cookie{Name: "cc_token", Value: expired})
	w := httptest.NewRecorder()
	e.handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for expired token, got %d", w.Code)
	}
}

// ─── /api/groups — list and create ───────────────────────────────────────────

func TestAPI_Groups_List_EmptyForNewUser(t *testing.T) {
	e := newTestEnv(t)
	token := e.signToken(t, gojwt.MapClaims{"sub": "stranger"})

	req := httptest.NewRequest(http.MethodGet, "/api/groups", nil)
	req.AddCookie(&http.Cookie{Name: "cc_token", Value: token})
	w := httptest.NewRecorder()
	e.handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var groups []db.Group
	json.NewDecoder(w.Body).Decode(&groups)
	if len(groups) != 0 {
		t.Errorf("expected empty list for new user, got %d groups", len(groups))
	}
}

func TestAPI_Groups_List_IncludesUserGroups(t *testing.T) {
	e := newTestEnv(t)
	req := e.authedRequest(t, http.MethodGet, "/api/groups", "")
	w := httptest.NewRecorder()
	e.handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var groups []db.Group
	json.NewDecoder(w.Body).Decode(&groups)
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if groups[0].Name != "test" {
		t.Errorf("expected group name 'test', got %q", groups[0].Name)
	}
	if len(groups[0].Members) != 1 || groups[0].Members[0] != "testuser" {
		t.Errorf("unexpected members: %v", groups[0].Members)
	}
}

func TestAPI_Groups_Create(t *testing.T) {
	e := newTestEnv(t)
	req := e.authedRequest(t, http.MethodPost, "/api/groups", `{"name":"new-group"}`)
	w := httptest.NewRecorder()
	e.handler.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body: %s", w.Code, w.Body.String())
	}
	var g db.Group
	json.NewDecoder(w.Body).Decode(&g)
	if g.Name != "new-group" {
		t.Errorf("expected group name 'new-group', got %q", g.Name)
	}
	if len(g.Members) != 1 || g.Members[0] != "testuser" {
		t.Errorf("creator should be the first member, got %v", g.Members)
	}
}

func TestAPI_Groups_Create_DuplicateName(t *testing.T) {
	e := newTestEnv(t)
	req := e.authedRequest(t, http.MethodPost, "/api/groups", `{"name":"test"}`)
	w := httptest.NewRecorder()
	e.handler.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409 for duplicate name, got %d", w.Code)
	}
}

func TestAPI_Groups_Create_EmptyName(t *testing.T) {
	e := newTestEnv(t)
	req := e.authedRequest(t, http.MethodPost, "/api/groups", `{"name":"  "}`)
	w := httptest.NewRecorder()
	e.handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty name, got %d", w.Code)
	}
}

// ─── /api/groups/{id}/schedule ────────────────────────────────────────────────

func TestAPI_Groups_Schedule_ValidToken(t *testing.T) {
	e := newTestEnv(t)
	req := e.authedRequest(t, http.MethodGet, e.groupPath("/schedule"), "")
	w := httptest.NewRecorder()
	e.handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected application/json, got %q", ct)
	}
	if cc := w.Header().Get("Cache-Control"); cc != "private, max-age=300" {
		t.Errorf("expected Cache-Control: private, max-age=300, got %q", cc)
	}
	var schedule map[string][]ChoreEntry
	if err := json.NewDecoder(w.Body).Decode(&schedule); err != nil {
		t.Fatalf("decoding schedule body: %v", err)
	}
	if len(schedule) == 0 {
		t.Error("expected non-empty schedule for group with chores")
	}
}

func TestAPI_Groups_Schedule_ResponseIsStable(t *testing.T) {
	e := newTestEnv(t)

	body := func() string {
		req := e.authedRequest(t, http.MethodGet, e.groupPath("/schedule"), "")
		w := httptest.NewRecorder()
		e.handler.ServeHTTP(w, req)
		return w.Body.String()
	}
	if body() != body() {
		t.Error("schedule response should be stable across requests")
	}
}

func TestAPI_Groups_Schedule_EmptyForGroupWithNoChores(t *testing.T) {
	e := newTestEnv(t)

	// Create a second group with no chores; add testuser as member.
	gid, _ := db.CreateGroup(e.sqlDB, "empty-group", "testuser")

	req := e.authedRequest(t, http.MethodGet, fmt.Sprintf("/api/groups/%d/schedule", gid), "")
	w := httptest.NewRecorder()
	e.handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var schedule map[string][]ChoreEntry
	json.NewDecoder(w.Body).Decode(&schedule)
	if len(schedule) != 0 {
		t.Errorf("expected empty schedule for group with no chores, got %d days", len(schedule))
	}
}

// ─── /api/groups/{id}/chores ─────────────────────────────────────────────────

func TestAPI_Groups_Chores_ValidToken(t *testing.T) {
	e := newTestEnv(t)
	req := e.authedRequest(t, http.MethodGet, e.groupPath("/chores"), "")
	w := httptest.NewRecorder()
	e.handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var chores []db.Chore
	if err := json.NewDecoder(w.Body).Decode(&chores); err != nil {
		t.Fatalf("decoding chores body: %v", err)
	}
	if len(chores) == 0 {
		t.Error("expected at least one chore")
	}
	if chores[0].Name != "Dishes" {
		t.Errorf("expected 'Dishes', got %q", chores[0].Name)
	}
}

func TestAPI_Groups_AddChore(t *testing.T) {
	e := newTestEnv(t)
	body := `{"name":"Laundry","assigned_to":"testuser","cadence_weeks":2}`
	req := e.authedRequest(t, http.MethodPost, e.groupPath("/chores"), body)
	w := httptest.NewRecorder()
	e.handler.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body: %s", w.Code, w.Body.String())
	}
	var chore db.Chore
	json.NewDecoder(w.Body).Decode(&chore)
	if chore.Name != "Laundry" {
		t.Errorf("expected 'Laundry', got %q", chore.Name)
	}
	if chore.ID == 0 {
		t.Error("expected non-zero chore ID")
	}
}

func TestAPI_Groups_AddChore_NonMemberAssignee(t *testing.T) {
	e := newTestEnv(t)
	body := `{"name":"Laundry","assigned_to":"nobody","cadence_weeks":1}`
	req := e.authedRequest(t, http.MethodPost, e.groupPath("/chores"), body)
	w := httptest.NewRecorder()
	e.handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for non-member assignee, got %d", w.Code)
	}
}

func TestAPI_Groups_AddChore_InvalidCadence(t *testing.T) {
	e := newTestEnv(t)
	body := `{"name":"Laundry","assigned_to":"testuser","cadence_weeks":-1}`
	req := e.authedRequest(t, http.MethodPost, e.groupPath("/chores"), body)
	w := httptest.NewRecorder()
	e.handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid cadence, got %d", w.Code)
	}
}

func TestAPI_Groups_RemoveChore(t *testing.T) {
	e := newTestEnv(t)

	// Get the chore ID first.
	req := e.authedRequest(t, http.MethodGet, e.groupPath("/chores"), "")
	w := httptest.NewRecorder()
	e.handler.ServeHTTP(w, req)
	var chores []db.Chore
	json.NewDecoder(w.Body).Decode(&chores)
	if len(chores) == 0 {
		t.Fatal("no chores to remove")
	}
	choreID := chores[0].ID

	req = e.authedRequest(t, http.MethodDelete, fmt.Sprintf("%s/%d", e.groupPath("/chores"), choreID), "")
	w = httptest.NewRecorder()
	e.handler.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
}

func TestAPI_Groups_RemoveChore_NotFound(t *testing.T) {
	e := newTestEnv(t)
	req := e.authedRequest(t, http.MethodDelete, e.groupPath("/chores/99999"), "")
	w := httptest.NewRecorder()
	e.handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for missing chore, got %d", w.Code)
	}
}

// ─── /api/groups/{id}/members ─────────────────────────────────────────────────

func TestAPI_Groups_AddMember(t *testing.T) {
	e := newTestEnv(t)
	body := `{"username":"alice"}`
	req := e.authedRequest(t, http.MethodPost, e.groupPath("/members"), body)
	w := httptest.NewRecorder()
	e.handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body: %s", w.Code, w.Body.String())
	}
	var g db.Group
	json.NewDecoder(w.Body).Decode(&g)
	found := false
	for _, m := range g.Members {
		if m == "alice" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected alice in members, got %v", g.Members)
	}
}

func TestAPI_Groups_AddMember_Self(t *testing.T) {
	e := newTestEnv(t)
	body := `{"username":"testuser"}`
	req := e.authedRequest(t, http.MethodPost, e.groupPath("/members"), body)
	w := httptest.NewRecorder()
	e.handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 when adding yourself, got %d", w.Code)
	}
}

func TestAPI_Groups_AddMember_IsIdempotent(t *testing.T) {
	e := newTestEnv(t)
	body := `{"username":"alice"}`

	for i := 0; i < 2; i++ {
		req := e.authedRequest(t, http.MethodPost, e.groupPath("/members"), body)
		w := httptest.NewRecorder()
		e.handler.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("add #%d: expected 200, got %d", i+1, w.Code)
		}
	}
	// Verify alice appears only once.
	req := e.authedRequest(t, http.MethodGet, "/api/groups", "")
	w := httptest.NewRecorder()
	e.handler.ServeHTTP(w, req)
	var groups []db.Group
	json.NewDecoder(w.Body).Decode(&groups)
	count := 0
	for _, m := range groups[0].Members {
		if m == "alice" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("alice should appear exactly once, found %d times", count)
	}
}

func TestAPI_Groups_RemoveMember(t *testing.T) {
	e := newTestEnv(t)

	// Add another member first so the group survives.
	db.AddMember(e.sqlDB, e.testGroupID, "alice")

	req := e.authedRequest(t, http.MethodDelete, e.groupPath("/members/alice"), "")
	w := httptest.NewRecorder()
	e.handler.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
}

func TestAPI_Groups_RemoveMember_LastMemberDeletesGroup(t *testing.T) {
	e := newTestEnv(t)

	req := e.authedRequest(t, http.MethodDelete, e.groupPath("/members/testuser"), "")
	w := httptest.NewRecorder()
	e.handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 with group_deleted, got %d", w.Code)
	}
	var body map[string]any
	json.NewDecoder(w.Body).Decode(&body)
	if body["group_deleted"] != true {
		t.Errorf("expected group_deleted=true, got %v", body)
	}

	// Group should be gone.
	req = e.authedRequest(t, http.MethodGet, "/api/groups", "")
	w = httptest.NewRecorder()
	e.handler.ServeHTTP(w, req)
	var groups []db.Group
	json.NewDecoder(w.Body).Decode(&groups)
	if len(groups) != 0 {
		t.Errorf("expected 0 groups after last member left, got %d", len(groups))
	}
}

// ─── Cross-group isolation ────────────────────────────────────────────────────

func TestAPI_Groups_NonMember_Forbidden(t *testing.T) {
	e := newTestEnv(t)
	token := e.signToken(t, gojwt.MapClaims{"sub": "stranger"})

	for _, path := range []string{
		e.groupPath("/schedule"),
		e.groupPath("/chores"),
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.AddCookie(&http.Cookie{Name: "cc_token", Value: token})
		w := httptest.NewRecorder()
		e.handler.ServeHTTP(w, req)

		if w.Code != http.StatusForbidden {
			t.Errorf("%s: expected 403 for non-member, got %d", path, w.Code)
		}
	}
}

func TestAPI_DifferentUsers_GetDifferentSchedules(t *testing.T) {
	privKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	keys := auth.NewStaticKeyProvider(&privKey.PublicKey)

	sqlDB, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("opening in-memory db: %v", err)
	}
	defer sqlDB.Close()

	groupA, _ := db.CreateGroup(sqlDB, "house-a", "alice")
	db.AddChore(sqlDB, groupA, "Dishes", "alice", 1.0)

	groupB, _ := db.CreateGroup(sqlDB, "house-b", "bob")
	db.AddChore(sqlDB, groupB, "Mowing", "bob", 1.0)

	cfg := serverConfig{
		authorizeURL: "https://auth.example.com/authorize",
		tokenURL:     "https://auth.example.com/token",
		clientID:     "cid", clientSecret: "cs",
		redirectURI: "https://app.example.com/callback",
		keys:        keys,
		db:          sqlDB,
	}
	handler := newHandler(context.Background(), cfg)

	sign := func(sub string) string {
		tok := gojwt.NewWithClaims(gojwt.SigningMethodRS256, gojwt.MapClaims{
			"sub": sub, "exp": time.Now().Add(time.Hour).Unix(),
		})
		s, _ := tok.SignedString(privKey)
		return s
	}

	for _, tc := range []struct {
		user      string
		groupID   int64
		wantChore string
		notChore  string
	}{
		{"alice", groupA, "Dishes", "Mowing"},
		{"bob", groupB, "Mowing", "Dishes"},
	} {
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/groups/%d/schedule", tc.groupID), nil)
		req.AddCookie(&http.Cookie{Name: "cc_token", Value: sign(tc.user)})
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("%s: expected 200, got %d", tc.user, w.Code)
			continue
		}
		body := w.Body.String()
		if !strings.Contains(body, tc.wantChore) {
			t.Errorf("%s: expected %q in schedule", tc.user, tc.wantChore)
		}
		if strings.Contains(body, tc.notChore) {
			t.Errorf("%s: found %q in schedule but should not have", tc.user, tc.notChore)
		}
	}

	// Cross-access: alice cannot access bob's group schedule.
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/groups/%d/schedule", groupB), nil)
	req.AddCookie(&http.Cookie{Name: "cc_token", Value: sign("alice")})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("alice cross-access: expected 403, got %d", w.Code)
	}
}

// ─── JWK kid matching ─────────────────────────────────────────────────────────

func TestAPI_KidMatching(t *testing.T) {
	privKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	pub := &privKey.PublicKey

	nB64 := base64.RawURLEncoding.EncodeToString(pub.N.Bytes())
	eB64 := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes())

	jwksSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]any{
				{"kty": "RSA", "kid": "key1", "n": nB64, "e": eB64},
			},
		})
	}))
	defer jwksSrv.Close()

	auth.JWKSRetryCount = 1
	auth.JWKSRetryDelay = 0
	keys, err := auth.NewJWKSKeyProvider(context.Background(), jwksSrv.URL)
	if err != nil {
		t.Fatalf("NewJWKSKeyProvider: %v", err)
	}

	sqlDB, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("opening in-memory db: %v", err)
	}
	defer sqlDB.Close()
	db.CreateGroup(sqlDB, "g", "u")

	cfg := serverConfig{
		authorizeURL: "https://auth.example.com/authorize",
		tokenURL:     "https://auth.example.com/token",
		clientID:     "cid", clientSecret: "cs",
		redirectURI:  "https://app.example.com/callback",
		keys:         keys,
		db:           sqlDB,
	}
	handler := newHandler(context.Background(), cfg)

	tok := gojwt.New(gojwt.SigningMethodRS256)
	tok.Header["kid"] = "key1"
	tok.Claims = gojwt.MapClaims{"sub": "u", "exp": time.Now().Add(time.Hour).Unix()}
	signed, _ := tok.SignedString(privKey)

	req := httptest.NewRequest(http.MethodGet, "/api/groups", nil)
	req.AddCookie(&http.Cookie{Name: "cc_token", Value: signed})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 with matching kid, got %d", w.Code)
	}

	tok2 := gojwt.New(gojwt.SigningMethodRS256)
	tok2.Header["kid"] = "unknown-kid"
	tok2.Claims = gojwt.MapClaims{"sub": "u", "exp": time.Now().Add(time.Hour).Unix()}
	signed2, _ := tok2.SignedString(privKey)

	req2 := httptest.NewRequest(http.MethodGet, "/api/groups", nil)
	req2.AddCookie(&http.Cookie{Name: "cc_token", Value: signed2})
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Errorf("expected 200 with unknown kid falling back to first key, got %d", w2.Code)
	}
}
