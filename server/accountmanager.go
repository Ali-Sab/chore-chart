package main

import (
	"errors"
	"net/url"
	"strings"
)

// accountManagerEndpoints holds the account-manager URLs chore-chart uses.
// authorizeURL is browser-facing (public base); jwksURL, tokenURL and internalBase
// (discovery) are server-side (internal base).
type accountManagerEndpoints struct {
	publicBase   string
	internalBase string
	authorizeURL string
	jwksURL      string
	tokenURL     string
}

var errInvalidInternalURL = errors.New("ACCOUNT_MANAGER_INTERNAL_URL must be an absolute http(s) URL, e.g. http://account-manager:3001")

// resolveAccountManagerEndpoints builds endpoints from ACCOUNT_MANAGER_URL (publicURL)
// and the optional ACCOUNT_MANAGER_INTERNAL_URL (internalURL; blank = use publicURL).
func resolveAccountManagerEndpoints(publicURL, internalURL string) (accountManagerEndpoints, error) {
	pub := strings.TrimRight(strings.TrimSpace(publicURL), "/")
	internal := strings.TrimRight(strings.TrimSpace(internalURL), "/")
	if internal == "" {
		internal = pub
	} else {
		u, err := url.Parse(internal)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return accountManagerEndpoints{}, errInvalidInternalURL
		}
	}
	return accountManagerEndpoints{
		publicBase:   pub,
		internalBase: internal,
		authorizeURL: pub + "/authorize",
		jwksURL:      internal + "/.well-known/jwks.json",
		tokenURL:     internal + "/token",
	}, nil
}
