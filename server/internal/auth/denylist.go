package auth

import (
	"sync"
	"time"
)

// Denylist rejects access tokens whose iat predates the user's logout time.
// Entries are reaped after one hour (the maximum token lifetime).
type Denylist struct {
	mu      sync.RWMutex
	entries map[string]time.Time // username -> logged_out_at
}

func NewDenylist() *Denylist {
	d := &Denylist{entries: make(map[string]time.Time)}
	go d.reapLoop()
	return d
}

func (d *Denylist) Add(username string, at time.Time) {
	d.mu.Lock()
	if existing, ok := d.entries[username]; !ok || at.After(existing) {
		d.entries[username] = at
	}
	d.mu.Unlock()
}

func (d *Denylist) IsDenied(username string, issuedAt time.Time) bool {
	d.mu.RLock()
	loggedOutAt, ok := d.entries[username]
	d.mu.RUnlock()
	return ok && issuedAt.Before(loggedOutAt)
}

func (d *Denylist) reapLoop() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		cutoff := time.Now().Add(-time.Hour)
		d.mu.Lock()
		for username, loggedOutAt := range d.entries {
			if loggedOutAt.Before(cutoff) {
				delete(d.entries, username)
			}
		}
		d.mu.Unlock()
	}
}
