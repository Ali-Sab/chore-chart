package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResolveAccountManagerEndpoints(t *testing.T) {
	tests := []struct {
		name         string
		publicURL    string
		internalURL  string
		wantPublic   string
		wantInternal string
		wantAuth     string
		wantJWKS     string
		wantToken    string
	}{
		{
			name:         "internal blank falls back to public",
			publicURL:    "https://pub.example/accounts",
			internalURL:  "",
			wantPublic:   "https://pub.example/accounts",
			wantInternal: "https://pub.example/accounts",
			wantAuth:     "https://pub.example/accounts/authorize",
			wantJWKS:     "https://pub.example/accounts/.well-known/jwks.json",
			wantToken:    "https://pub.example/accounts/token",
		},
		{
			name:         "internal whitespace falls back to public",
			publicURL:    "https://pub.example/accounts",
			internalURL:  "   ",
			wantPublic:   "https://pub.example/accounts",
			wantInternal: "https://pub.example/accounts",
			wantAuth:     "https://pub.example/accounts/authorize",
			wantJWKS:     "https://pub.example/accounts/.well-known/jwks.json",
			wantToken:    "https://pub.example/accounts/token",
		},
		{
			name:         "split: authorize public, jwks and token internal",
			publicURL:    "https://pub.example/accounts",
			internalURL:  "http://account-manager:3001",
			wantPublic:   "https://pub.example/accounts",
			wantInternal: "http://account-manager:3001",
			wantAuth:     "https://pub.example/accounts/authorize",
			wantJWKS:     "http://account-manager:3001/.well-known/jwks.json",
			wantToken:    "http://account-manager:3001/token",
		},
		{
			name:         "trailing slashes trimmed on both",
			publicURL:    "https://pub.example/accounts/",
			internalURL:  "http://account-manager:3001/",
			wantPublic:   "https://pub.example/accounts",
			wantInternal: "http://account-manager:3001",
			wantAuth:     "https://pub.example/accounts/authorize",
			wantJWKS:     "http://account-manager:3001/.well-known/jwks.json",
			wantToken:    "http://account-manager:3001/token",
		},
		{
			name:         "trailing slash trimmed on public only, internal blank",
			publicURL:    "https://pub.example/accounts/",
			internalURL:  "",
			wantPublic:   "https://pub.example/accounts",
			wantInternal: "https://pub.example/accounts",
			wantAuth:     "https://pub.example/accounts/authorize",
			wantJWKS:     "https://pub.example/accounts/.well-known/jwks.json",
			wantToken:    "https://pub.example/accounts/token",
		},
		{
			name:         "https internal accepted",
			publicURL:    "https://pub.example",
			internalURL:  "https://am.internal",
			wantPublic:   "https://pub.example",
			wantInternal: "https://am.internal",
			wantAuth:     "https://pub.example/authorize",
			wantJWKS:     "https://am.internal/.well-known/jwks.json",
			wantToken:    "https://am.internal/token",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveAccountManagerEndpoints(tc.publicURL, tc.internalURL)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			want := accountManagerEndpoints{
				publicBase:   tc.wantPublic,
				internalBase: tc.wantInternal,
				authorizeURL: tc.wantAuth,
				jwksURL:      tc.wantJWKS,
				tokenURL:     tc.wantToken,
			}
			if got != want {
				t.Errorf("got  %+v\nwant %+v", got, want)
			}
			for _, u := range []string{got.authorizeURL, got.jwksURL, got.tokenURL} {
				if strings.Contains(strings.TrimPrefix(strings.TrimPrefix(u, "https://"), "http://"), "//") {
					t.Errorf("double slash in %q", u)
				}
			}
		})
	}
}

func TestResolveAccountManagerEndpoints_InvalidInternal(t *testing.T) {
	for _, internal := range []string{
		"account-manager:3001",
		"ftp://am",
		"http://",
		"://bad",
	} {
		t.Run(internal, func(t *testing.T) {
			got, err := resolveAccountManagerEndpoints("https://pub.example/accounts", internal)
			if !errors.Is(err, errInvalidInternalURL) {
				t.Fatalf("err = %v, want errInvalidInternalURL", err)
			}
			if got != (accountManagerEndpoints{}) {
				t.Errorf("expected zero endpoints on error, got %+v", got)
			}
		})
	}
}

func TestResolveAccountManagerEndpoints_ErrorDoesNotLeakSecret(t *testing.T) {
	_, err := resolveAccountManagerEndpoints("https://pub.example/accounts", "ftp://u:s3cret@am")
	if !errors.Is(err, errInvalidInternalURL) {
		t.Fatalf("err = %v, want errInvalidInternalURL", err)
	}
	if strings.Contains(err.Error(), "s3cret") {
		t.Errorf("error leaks secret: %q", err.Error())
	}
}

// discoveryServer serves the discovery doc at the well-known path with the
// given status and body, and records every request path.
func discoveryServer(t *testing.T, status int, body string) (*httptest.Server, *[]string) {
	t.Helper()
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.URL.Path != "/.well-known/oauth-authorization-server" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, &paths
}

func TestFetchEndSessionEndpoint_DiscoveryFromDiscoveryBase(t *testing.T) {
	srv, paths := discoveryServer(t, http.StatusOK,
		`{"end_session_endpoint":"https://pub.example/accounts/logout"}`)

	got := fetchEndSessionEndpoint(context.Background(), srv.URL,
		"https://pub.example/accounts", "https://pub.example/chores/oauth/callback")

	if got != "https://pub.example/accounts/logout" {
		t.Errorf("got %q", got)
	}
	if len(*paths) != 1 || (*paths)[0] != "/.well-known/oauth-authorization-server" {
		t.Errorf("discovery paths = %v, want one request to /.well-known/oauth-authorization-server", *paths)
	}
}

func TestFetchEndSessionEndpoint_TrailingSlashDiscoveryBase(t *testing.T) {
	srv, paths := discoveryServer(t, http.StatusOK,
		`{"end_session_endpoint":"https://pub.example/accounts/logout"}`)

	got := fetchEndSessionEndpoint(context.Background(), srv.URL+"/",
		"https://pub.example/accounts", "https://pub.example/chores/oauth/callback")

	if got != "https://pub.example/accounts/logout" {
		t.Errorf("got %q", got)
	}
	if len(*paths) != 1 || (*paths)[0] != "/.well-known/oauth-authorization-server" {
		t.Errorf("discovery paths = %v", *paths)
	}
}

func TestFetchEndSessionEndpoint_RebasesInternalHostOntoRedirectOrigin(t *testing.T) {
	srv, _ := discoveryServer(t, http.StatusOK,
		`{"end_session_endpoint":"http://account-manager:3001/logout"}`)

	got := fetchEndSessionEndpoint(context.Background(), srv.URL,
		"https://pub.example/accounts", "https://pub.example/chores/oauth/callback")

	if got != "https://pub.example/logout" {
		t.Errorf("got %q, want https://pub.example/logout", got)
	}
}

func TestFetchEndSessionEndpoint_Discovery500FallsBackToPublicLogout(t *testing.T) {
	srv, paths := discoveryServer(t, http.StatusInternalServerError, `oops`)

	got := fetchEndSessionEndpoint(context.Background(), srv.URL,
		"https://pub.example/accounts", "https://pub.example/chores/oauth/callback")

	if got != "https://pub.example/logout" {
		t.Errorf("got %q, want https://pub.example/logout", got)
	}
	if len(*paths) != 1 {
		t.Errorf("expected discovery to be attempted once, paths = %v", *paths)
	}
}

func TestFetchEndSessionEndpoint_InvalidRedirectURIAndDiscovery500(t *testing.T) {
	srv, _ := discoveryServer(t, http.StatusInternalServerError, `oops`)

	got := fetchEndSessionEndpoint(context.Background(), srv.URL,
		"https://pub.example/accounts", "not-a-url")

	if got != "https://pub.example/accounts/logout" {
		t.Errorf("got %q, want https://pub.example/accounts/logout", got)
	}
	if strings.Contains(got, srv.URL) {
		t.Errorf("fallback %q must not contain the internal discovery URL %q", got, srv.URL)
	}
}

func TestFetchEndSessionEndpoint_InvalidRedirectURIPublicBaseTrailingSlash(t *testing.T) {
	srv, _ := discoveryServer(t, http.StatusInternalServerError, `oops`)

	got := fetchEndSessionEndpoint(context.Background(), srv.URL,
		"https://pub.example/accounts/", "not-a-url")

	if got != "https://pub.example/accounts/logout" {
		t.Errorf("got %q, want https://pub.example/accounts/logout", got)
	}
}

func TestFetchEndSessionEndpoint_InvalidRedirectURIWithValidDocReturnsDocValue(t *testing.T) {
	srv, _ := discoveryServer(t, http.StatusOK,
		`{"end_session_endpoint":"https://pub.example/accounts/logout"}`)

	got := fetchEndSessionEndpoint(context.Background(), srv.URL,
		"https://pub.example/accounts", "not-a-url")

	if got != "https://pub.example/accounts/logout" {
		t.Errorf("got %q, want doc value unchanged", got)
	}
}
