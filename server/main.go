package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"chore-chart/internal/auth"
	"chore-chart/internal/db"
	"chore-chart/internal/logging"
)

const scheduleYear = 2026

// ─── Domain types ─────────────────────────────────────────────────────────────

type Chore struct {
	Name         string
	Person       string
	CadenceWeeks float64
}

type ChoreEntry struct {
	Name   string `json:"name"`
	Person string `json:"person"`
}

// ─── Schedule builder ─────────────────────────────────────────────────────────

func buildSchedule(chores []Chore) map[string][]ChoreEntry {
	schedule := make(map[string][]ChoreEntry)
	start := time.Date(scheduleYear, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(scheduleYear, 12, 31, 0, 0, 0, 0, time.UTC)

	for _, chore := range chores {
		if chore.CadenceWeeks <= 0 {
			log.Fatalf("chore %q has invalid cadence_weeks %v", chore.Name, chore.CadenceWeeks)
		}
		cadMs := chore.CadenceWeeks * 7 * 24 * 3600 * 1000
		tMs := float64(start.UnixMilli())
		endMs := float64(end.UnixMilli())

		for tMs <= endMs {
			dayMs := math.Round(tMs/86400000) * 86400000
			d := time.UnixMilli(int64(dayMs)).UTC()
			if !d.Before(start) && !d.After(end) {
				key := d.Format("2006-01-02")
				schedule[key] = append(schedule[key], ChoreEntry{
					Name:   chore.Name,
					Person: chore.Person,
				})
			}
			tMs += cadMs
		}
	}

	for key := range schedule {
		sort.Slice(schedule[key], func(i, j int) bool {
			pi, pj := schedule[key][i].Person, schedule[key][j].Person
			if pi != pj {
				return pi < pj
			}
			return schedule[key][i].Name < schedule[key][j].Name
		})
	}

	return schedule
}

// ─── OAuth PKCE session store ─────────────────────────────────────────────────

type oauthSession struct {
	verifier string
	returnTo string
	at       time.Time
}

type sessionStore struct {
	mu sync.Mutex
	m  map[string]oauthSession
}

func newSessionStore(ctx context.Context) *sessionStore {
	s := &sessionStore{m: make(map[string]oauthSession)}
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				s.mu.Lock()
				for k, v := range s.m {
					if time.Since(v.at) > 10*time.Minute {
						delete(s.m, k)
					}
				}
				s.mu.Unlock()
			case <-ctx.Done():
				return
			}
		}
	}()
	return s
}

// ─── PKCE helpers ─────────────────────────────────────────────────────────────

func generateVerifier() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		log.Fatal("generating code verifier:", err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func codeChallenge(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

func randomState() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		log.Fatal("generating state:", err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// ─── Discovery ────────────────────────────────────────────────────────────────

func fetchEndSessionEndpoint(ctx context.Context, discoveryBase, publicBase, redirectURI string) string {
	base := strings.TrimRight(discoveryBase, "/")

	publicOrigin := ""
	if pub, err := url.Parse(redirectURI); err == nil && pub.Scheme != "" && pub.Host != "" {
		publicOrigin = pub.Scheme + "://" + pub.Host
	}
	fallback := publicOrigin + "/logout"
	if publicOrigin == "" {
		fallback = strings.TrimRight(publicBase, "/") + "/logout"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/.well-known/oauth-authorization-server", nil)
	if err != nil {
		return fallback
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return fallback
	}
	defer resp.Body.Close()

	var doc struct {
		EndSessionEndpoint string `json:"end_session_endpoint"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil || doc.EndSessionEndpoint == "" {
		return fallback
	}
	if publicOrigin == "" {
		return doc.EndSessionEndpoint
	}
	u, err := url.Parse(doc.EndSessionEndpoint)
	if err != nil {
		return fallback
	}
	return publicOrigin + u.RequestURI()
}

// ─── mustEnv ──────────────────────────────────────────────────────────────────

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("required environment variable %s is not set", key)
	}
	return v
}

// ─── Server config & handler ──────────────────────────────────────────────────

type serverConfig struct {
	authorizeURL  string
	tokenURL      string
	endSessionURL string
	clientID      string
	clientSecret  string
	redirectURI   string
	keys          auth.KeyProvider
	tokenClient   *http.Client
	db            *sql.DB
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// requireMember parses the {id} path param and verifies username is a member.
// Returns (groupID, true) on success; writes an error response and returns (0, false) otherwise.
func requireMember(w http.ResponseWriter, r *http.Request, sqlDB *sql.DB, username string) (int64, bool) {
	idStr := r.PathValue("id")
	groupID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || groupID <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid group id"})
		return 0, false
	}
	isMember, err := db.IsMember(sqlDB, groupID, username)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "database error"})
		return 0, false
	}
	if !isMember {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "not a member of this group"})
		return 0, false
	}
	return groupID, true
}

// newHandler wires all routes and returns the root http.Handler.
func newHandler(ctx context.Context, cfg serverConfig) http.Handler {
	if cfg.tokenClient == nil {
		cfg.tokenClient = &http.Client{Timeout: 10 * time.Second}
	}

	store := newSessionStore(ctx)
	mux := http.NewServeMux()

	// ─── OAuth start ──────────────────────────────────────────────────────────
	mux.HandleFunc("/oauth/start", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		returnTo := r.URL.Query().Get("return_to")
		if returnTo == "" || !strings.HasPrefix(returnTo, "/") || strings.Contains(returnTo, "://") {
			returnTo = "/chores/"
		}

		verifier := generateVerifier()
		state := randomState()

		store.mu.Lock()
		store.m[state] = oauthSession{verifier: verifier, returnTo: returnTo, at: time.Now()}
		store.mu.Unlock()

		params := url.Values{
			"response_type":         {"code"},
			"client_id":             {cfg.clientID},
			"redirect_uri":          {cfg.redirectURI},
			"code_challenge":        {codeChallenge(verifier)},
			"code_challenge_method": {"S256"},
			"state":                 {state},
		}
		http.Redirect(w, r, cfg.authorizeURL+"?"+params.Encode(), http.StatusFound)
	})

	// ─── OAuth callback ───────────────────────────────────────────────────────
	mux.HandleFunc("/oauth/callback", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		q := r.URL.Query()
		if errParam := q.Get("error"); errParam != "" {
			http.Redirect(w, r, "/chores/?auth_error="+url.QueryEscape(errParam), http.StatusFound)
			return
		}

		state := q.Get("state")
		code := q.Get("code")

		store.mu.Lock()
		sess, ok := store.m[state]
		if ok {
			delete(store.m, state)
		}
		store.mu.Unlock()

		if !ok || state == "" {
			http.Error(w, "invalid state", http.StatusBadRequest)
			return
		}
		if code == "" {
			http.Error(w, "missing code", http.StatusBadRequest)
			return
		}

		resp, err := cfg.tokenClient.PostForm(cfg.tokenURL, url.Values{
			"grant_type":    {"authorization_code"},
			"code":          {code},
			"redirect_uri":  {cfg.redirectURI},
			"client_id":     {cfg.clientID},
			"client_secret": {cfg.clientSecret},
			"code_verifier": {sess.verifier},
		})
		if err != nil {
			log.Printf("[oauth/callback] token exchange error: %v", err)
			http.Error(w, "token exchange failed", http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Printf("[oauth/callback] reading token response body: %v", err)
			http.Error(w, "token exchange failed", http.StatusBadGateway)
			return
		}
		var tokenResp struct {
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
			ExpiresIn    int    `json:"expires_in"`
		}
		if err := json.Unmarshal(body, &tokenResp); err != nil || tokenResp.AccessToken == "" {
			log.Printf("[oauth/callback] bad token response: %s", body)
			http.Error(w, "invalid token response", http.StatusBadGateway)
			return
		}

		maxAge := tokenResp.ExpiresIn
		if maxAge <= 0 {
			maxAge = 3600
		}
		http.SetCookie(w, &http.Cookie{
			Name:     "cc_token",
			Value:    tokenResp.AccessToken,
			Path:     "/",
			MaxAge:   maxAge,
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteStrictMode,
		})
		if tokenResp.RefreshToken != "" {
			http.SetCookie(w, &http.Cookie{
				Name:     "cc_refresh",
				Value:    tokenResp.RefreshToken,
				Path:     "/",
				MaxAge:   30 * 24 * 3600,
				HttpOnly: true,
				Secure:   true,
				SameSite: http.SameSiteStrictMode,
			})
		}
		http.Redirect(w, r, sess.returnTo, http.StatusFound)
	})

	// ─── Logout ───────────────────────────────────────────────────────────────
	postLogoutURI := func() string {
		u, err := url.Parse(cfg.redirectURI)
		if err != nil {
			return cfg.redirectURI
		}
		u.Path = strings.TrimSuffix(u.Path, "/callback") + "/start"
		u.RawQuery = ""
		return u.String()
	}()
	endSessionRedirect := cfg.endSessionURL + "?post_logout_redirect_uri=" + url.QueryEscape(postLogoutURI)

	mux.HandleFunc("/api/auth/logout", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		for _, name := range []string{"cc_token", "cc_refresh"} {
			http.SetCookie(w, &http.Cookie{Name: name, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: true})
		}
		writeJSON(w, http.StatusOK, map[string]string{"location": endSessionRedirect})
	})

	// ─── Backchannel logout ───────────────────────────────────────────────────
	denylist := auth.NewDenylist()

	mux.HandleFunc("/api/auth/backchannel-logout", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		tokenStr := r.FormValue("logout_token")
		if tokenStr == "" {
			http.Error(w, "missing logout_token", http.StatusBadRequest)
			return
		}
		kid := auth.KidFromToken(tokenStr)
		claims, err := auth.VerifyToken(tokenStr, cfg.keys.PublicKey(kid))
		if err != nil {
			log.Printf("backchannel logout: invalid token: %v", err)
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}
		events, ok := claims["events"].(map[string]any)
		if !ok {
			http.Error(w, "not a logout token", http.StatusBadRequest)
			return
		}
		if _, ok := events["http://schemas.openid.net/event/backchannel-logout"]; !ok {
			http.Error(w, "not a logout token", http.StatusBadRequest)
			return
		}
		sub, _ := claims["sub"].(string)
		if sub == "" {
			http.Error(w, "missing sub", http.StatusBadRequest)
			return
		}
		denylist.Add(sub, time.Now())
		w.WriteHeader(http.StatusNoContent)
	})

	// ─── Protected API ────────────────────────────────────────────────────────
	authMiddleware := auth.NewMiddleware(cfg.keys, denylist)
	protected := http.NewServeMux()

	// GET /api/me — returns the authenticated user's username.
	protected.HandleFunc("GET /api/me", func(w http.ResponseWriter, r *http.Request) {
		username, _ := r.Context().Value(auth.UsernameKey).(string)
		writeJSON(w, http.StatusOK, map[string]string{"username": username})
	})

	// GET /api/groups — list all groups the authenticated user belongs to.
	protected.HandleFunc("GET /api/groups", func(w http.ResponseWriter, r *http.Request) {
		username, _ := r.Context().Value(auth.UsernameKey).(string)
		groups, err := db.GetGroupsForUser(cfg.db, username)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "database error"})
			return
		}
		if groups == nil {
			groups = []db.Group{}
		}
		writeJSON(w, http.StatusOK, groups)
	})

	// POST /api/groups — create a new group; creator becomes the first member.
	// Body: {"name": "home"}
	protected.HandleFunc("POST /api/groups", func(w http.ResponseWriter, r *http.Request) {
		username, _ := r.Context().Value(auth.UsernameKey).(string)
		var body struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Name) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
			return
		}
		gid, err := db.CreateGroup(cfg.db, strings.TrimSpace(body.Name), username)
		if err != nil {
			if strings.Contains(err.Error(), "UNIQUE constraint") {
				writeJSON(w, http.StatusConflict, map[string]string{"error": "a group with that name already exists"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "database error"})
			return
		}
		g, err := db.GetGroup(cfg.db, gid)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "database error"})
			return
		}
		writeJSON(w, http.StatusCreated, g)
	})

	// GET /api/groups/{id}/schedule — compute and return the year schedule for a group.
	protected.HandleFunc("GET /api/groups/{id}/schedule", func(w http.ResponseWriter, r *http.Request) {
		username, _ := r.Context().Value(auth.UsernameKey).(string)
		groupID, ok := requireMember(w, r, cfg.db, username)
		if !ok {
			return
		}
		dbChores, err := db.GetChores(cfg.db, groupID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "database error"})
			return
		}
		chores := make([]Chore, len(dbChores))
		for i, c := range dbChores {
			chores[i] = Chore{Name: c.Name, Person: c.AssignedTo, CadenceWeeks: c.CadenceWeeks}
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		json.NewEncoder(w).Encode(buildSchedule(chores))
	})

	// GET /api/groups/{id}/chores — list all chores for a group.
	protected.HandleFunc("GET /api/groups/{id}/chores", func(w http.ResponseWriter, r *http.Request) {
		username, _ := r.Context().Value(auth.UsernameKey).(string)
		groupID, ok := requireMember(w, r, cfg.db, username)
		if !ok {
			return
		}
		chores, err := db.GetChores(cfg.db, groupID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "database error"})
			return
		}
		if chores == nil {
			chores = []db.Chore{}
		}
		writeJSON(w, http.StatusOK, chores)
	})

	// POST /api/groups/{id}/chores — add a chore to a group.
	// Body: {"name": "Kitchen Trash", "assigned_to": "alison", "cadence_weeks": 0.5}
	// assigned_to must be an existing group member.
	protected.HandleFunc("POST /api/groups/{id}/chores", func(w http.ResponseWriter, r *http.Request) {
		username, _ := r.Context().Value(auth.UsernameKey).(string)
		groupID, ok := requireMember(w, r, cfg.db, username)
		if !ok {
			return
		}
		var body struct {
			Name         string  `json:"name"`
			AssignedTo   string  `json:"assigned_to"`
			CadenceWeeks float64 `json:"cadence_weeks"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		body.Name = strings.TrimSpace(body.Name)
		body.AssignedTo = strings.TrimSpace(body.AssignedTo)
		if body.Name == "" || body.AssignedTo == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name and assigned_to are required"})
			return
		}
		if body.CadenceWeeks <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cadence_weeks must be positive"})
			return
		}
		isMember, err := db.IsMember(cfg.db, groupID, body.AssignedTo)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "database error"})
			return
		}
		if !isMember {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "assigned_to must be a group member"})
			return
		}
		id, err := db.AddChore(cfg.db, groupID, body.Name, body.AssignedTo, body.CadenceWeeks)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "database error"})
			return
		}
		writeJSON(w, http.StatusCreated, db.Chore{
			ID:           id,
			Name:         body.Name,
			AssignedTo:   body.AssignedTo,
			CadenceWeeks: body.CadenceWeeks,
		})
	})

	// DELETE /api/groups/{id}/chores/{choreId} — remove a chore from a group.
	protected.HandleFunc("DELETE /api/groups/{id}/chores/{choreId}", func(w http.ResponseWriter, r *http.Request) {
		username, _ := r.Context().Value(auth.UsernameKey).(string)
		groupID, ok := requireMember(w, r, cfg.db, username)
		if !ok {
			return
		}
		choreID, err := strconv.ParseInt(r.PathValue("choreId"), 10, 64)
		if err != nil || choreID <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid chore id"})
			return
		}
		if err := db.RemoveChore(cfg.db, groupID, choreID); err != nil {
			if err.Error() == "chore not found" {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "chore not found"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "database error"})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	// POST /api/groups/{id}/members — add a user to a group.
	// Body: {"username": "bob"}
	// Cannot add yourself; must be added by an existing member.
	protected.HandleFunc("POST /api/groups/{id}/members", func(w http.ResponseWriter, r *http.Request) {
		username, _ := r.Context().Value(auth.UsernameKey).(string)
		groupID, ok := requireMember(w, r, cfg.db, username)
		if !ok {
			return
		}
		var body struct {
			Username string `json:"username"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Username) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "username is required"})
			return
		}
		target := strings.TrimSpace(body.Username)
		if target == username {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cannot add yourself to a group"})
			return
		}
		if err := db.AddMember(cfg.db, groupID, target); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "database error"})
			return
		}
		g, err := db.GetGroup(cfg.db, groupID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "database error"})
			return
		}
		writeJSON(w, http.StatusOK, g)
	})

	// DELETE /api/groups/{id}/members/{username} — remove a member from a group.
	// Any member can remove any other member, or leave themselves.
	// If the last member leaves, the group is deleted.
	protected.HandleFunc("DELETE /api/groups/{id}/members/{username}", func(w http.ResponseWriter, r *http.Request) {
		actor, _ := r.Context().Value(auth.UsernameKey).(string)
		groupID, ok := requireMember(w, r, cfg.db, actor)
		if !ok {
			return
		}
		target := r.PathValue("username")
		if target == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "username is required"})
			return
		}
		groupDeleted, err := db.RemoveMember(cfg.db, groupID, target)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "database error"})
			return
		}
		if groupDeleted {
			writeJSON(w, http.StatusOK, map[string]any{"group_deleted": true})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.Handle("/api/", authMiddleware(protected))

	// In production, nginx serves /chores/ from client/dist directly.
	mux.Handle("/", http.FileServer(http.Dir("../client/dist")))

	return mux
}

// ─── main ─────────────────────────────────────────────────────────────────────

func main() {
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "./chore-chart.db"
	}

	sqlDB, err := db.Open(dbPath)
	if err != nil {
		log.Fatal("opening database:", err)
	}
	defer sqlDB.Close()

	if err := db.Seed(sqlDB, "data/seed.json"); err != nil {
		log.Fatal("seeding database:", err)
	}
	log.Println("database ready")

	accountManagerURL := mustEnv("ACCOUNT_MANAGER_URL")
	clientID := mustEnv("OAUTH_CLIENT_ID")
	clientSecret := mustEnv("OAUTH_CLIENT_SECRET")
	redirectURI := mustEnv("OAUTH_REDIRECT_URI")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	ep, err := resolveAccountManagerEndpoints(accountManagerURL, os.Getenv("ACCOUNT_MANAGER_INTERNAL_URL"))
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("account-manager: public %s, server-side %s", ep.publicBase, ep.internalBase)
	keys, err := auth.NewJWKSKeyProvider(ctx, ep.jwksURL)
	if err != nil {
		log.Fatal("fetching JWKS:", err)
	}
	endSessionURL := fetchEndSessionEndpoint(ctx, ep.internalBase, ep.publicBase, redirectURI)
	log.Printf("end_session_endpoint: %s", endSessionURL)

	cfg := serverConfig{
		authorizeURL:  ep.authorizeURL,
		tokenURL:      ep.tokenURL,
		endSessionURL: endSessionURL,
		clientID:      clientID,
		clientSecret:  clientSecret,
		redirectURI:   redirectURI,
		keys:          keys,
		db:            sqlDB,
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	srv := &http.Server{Addr: ":" + port, Handler: logging.NewMiddleware(newHandler(ctx, cfg))}
	go func() {
		log.Printf("listening on :%s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown error: %v", err)
	}
}
