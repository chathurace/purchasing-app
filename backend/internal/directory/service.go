package directory

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

// FallbackFunc supplies directory entries when SCIM is disabled — the app's own
// active DB users.
type FallbackFunc func(ctx context.Context) ([]User, error)

// Service serves the org directory. When SCIM is enabled it fetches the full
// user list from the identity server and caches it for the configured TTL,
// refreshing lazily; when disabled it defers to the fallback on every call.
type Service struct {
	client   *scimClient // nil when SCIM is disabled
	fallback FallbackFunc
	ttl      time.Duration
	log      zerolog.Logger

	refreshMu sync.Mutex   // serialises refreshes (avoids stampede)
	mu        sync.RWMutex // guards cached/expiry/lastFetch
	cached    []User
	expiry    time.Time
	lastFetch time.Time
}

// minForcedRefreshInterval caps how often a caller-forced refresh actually hits
// SCIM, so a burst of "no match" keystrokes can't stampede the identity server.
const minForcedRefreshInterval = 30 * time.Second

// New builds a directory Service. When cfg is nil (or scim disabled), the
// Service uses fallback for all lookups.
func New(enabled bool, cfg SCIMConfig, ttl time.Duration, fallback FallbackFunc, log zerolog.Logger) *Service {
	s := &Service{fallback: fallback, ttl: ttl, log: log}
	if enabled {
		s.client = newSCIMClient(cfg)
	}
	return s
}

// Enabled reports whether the Service is backed by SCIM (vs. the DB fallback).
func (s *Service) Enabled() bool { return s != nil && s.client != nil }

// List returns the directory, sorted by name then email. With SCIM disabled it
// returns fresh fallback data each call; with SCIM enabled it serves the cache,
// refreshing when stale (and serving stale data if a refresh fails).
//
// forceRefresh asks for a fresh SCIM fetch even when the cache is still within
// its TTL — used when a caller typed a name/email that matched nothing cached, so
// a newly-added directory user may be missing. It is rate-limited
// (minForcedRefreshInterval) so it can't stampede SCIM; if the last fetch was too
// recent the current cache is returned unchanged.
func (s *Service) List(ctx context.Context, forceRefresh bool) ([]User, error) {
	if s.client == nil {
		return sortUsers(nilToEmpty(mustFallback(ctx, s.fallback))), nil
	}

	s.mu.RLock()
	fresh := s.cached != nil && time.Now().Before(s.expiry)
	cached := s.cached
	s.mu.RUnlock()
	if fresh && !forceRefresh {
		return cached, nil
	}

	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()
	// Re-check: another goroutine may have refreshed while we waited.
	s.mu.RLock()
	fresh = s.cached != nil && time.Now().Before(s.expiry)
	recentlyFetched := s.cached != nil && time.Since(s.lastFetch) < minForcedRefreshInterval
	cached = s.cached
	s.mu.RUnlock()
	if fresh && !forceRefresh {
		return cached, nil
	}
	// A forced refresh that arrives right after a fetch is coalesced onto the
	// existing cache rather than hitting SCIM again.
	if forceRefresh && recentlyFetched {
		return cached, nil
	}

	users, err := s.client.listAll(ctx)
	if err != nil {
		if cached != nil {
			s.log.Warn().Err(err).Msg("scim directory refresh failed; serving stale cache")
			return cached, nil
		}
		s.log.Error().Err(err).Msg("scim directory fetch failed and no cache available")
		return nil, err
	}
	users = sortUsers(users)
	now := time.Now()
	s.mu.Lock()
	s.cached = users
	s.lastFetch = now
	s.expiry = now.Add(s.ttl)
	s.mu.Unlock()
	return users, nil
}

func mustFallback(ctx context.Context, f FallbackFunc) []User {
	if f == nil {
		return nil
	}
	users, err := f(ctx)
	if err != nil {
		return nil
	}
	return users
}

func nilToEmpty(u []User) []User {
	if u == nil {
		return []User{}
	}
	return u
}

func sortUsers(u []User) []User {
	sort.Slice(u, func(i, j int) bool {
		ni, nj := strings.ToLower(u[i].Name), strings.ToLower(u[j].Name)
		if ni != nj {
			return ni < nj
		}
		return u[i].Email < u[j].Email
	})
	return u
}
