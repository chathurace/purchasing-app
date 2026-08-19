package repository_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/cs/purchasing-app/internal/repository"
)

// newSessionTestUser creates a throwaway user (and cleans it up), since every
// session row is FK-bound to one.
func newSessionTestUser(t *testing.T, repo *repository.Repository, ctx context.Context) *repository.User {
	t.Helper()
	slug := strings.ToLower(t.Name())
	user, err := repo.UpsertUser(ctx, "session-sub-"+slug, "session-"+slug+"@example.com", "Session Tester")
	if err != nil {
		t.Fatalf("upsert user: %v", err)
	}
	t.Cleanup(func() { _, _ = repo.Pool().Exec(ctx, `DELETE FROM users WHERE id=$1`, user.ID) })
	return user
}

// TestUserSessionLifecycle covers the whole cookie-session path: mint, resolve +
// slide the idle expiry, then revoke.
func TestUserSessionLifecycle(t *testing.T) {
	repo, ctx := newTestRepo(t)
	user := newSessionTestUser(t, repo, ctx)

	const ttl = 60 * 24 * time.Hour
	hash := "hash-" + strings.ToLower(t.Name())
	// Deliberately short so the slide below is observable.
	if err := repo.CreateUserSession(ctx, hash, user.ID, time.Now().Add(time.Hour), "Go test UA", "203.0.113.7"); err != nil {
		t.Fatalf("create session: %v", err)
	}

	gotID, err := repo.TouchUserSession(ctx, hash, ttl)
	if err != nil {
		t.Fatalf("touch session: %v", err)
	}
	if gotID != user.ID {
		t.Fatalf("touch resolved user %d, want %d", gotID, user.ID)
	}

	// Sliding window: the touch above must have pushed expiry out to ~now+ttl,
	// which is what makes "60 days" mean 60 days *idle*.
	var expiresAt time.Time
	if err := repo.Pool().QueryRow(ctx, `SELECT expires_at FROM user_sessions WHERE token_hash=$1`, hash).Scan(&expiresAt); err != nil {
		t.Fatalf("read expires_at: %v", err)
	}
	if remaining := time.Until(expiresAt); remaining < ttl-time.Hour {
		t.Fatalf("expiry was not slid forward: %s remaining, want ~%s", remaining, ttl)
	}

	if err := repo.RevokeUserSession(ctx, hash); err != nil {
		t.Fatalf("revoke session: %v", err)
	}
	if _, err := repo.TouchUserSession(ctx, hash, ttl); !errors.Is(err, repository.ErrSessionInvalid) {
		t.Fatalf("touch after revoke: got %v, want ErrSessionInvalid", err)
	}
	// Revoking twice is a no-op — logout must be idempotent.
	if err := repo.RevokeUserSession(ctx, hash); err != nil {
		t.Fatalf("second revoke: %v", err)
	}
}

// TestUserSessionRejectsUnknownAndExpired checks the two ways a presented cookie
// is refused, and that "sign out everywhere" kills only live sessions.
func TestUserSessionRejectsUnknownAndExpired(t *testing.T) {
	repo, ctx := newTestRepo(t)
	user := newSessionTestUser(t, repo, ctx)
	const ttl = 60 * 24 * time.Hour

	if _, err := repo.TouchUserSession(ctx, "no-such-hash-"+strings.ToLower(t.Name()), ttl); !errors.Is(err, repository.ErrSessionInvalid) {
		t.Fatalf("unknown token: got %v, want ErrSessionInvalid", err)
	}

	expired := "expired-" + strings.ToLower(t.Name())
	if err := repo.CreateUserSession(ctx, expired, user.ID, time.Now().Add(-time.Minute), "", ""); err != nil {
		t.Fatalf("create expired session: %v", err)
	}
	// An expired row must not be resurrected by the GREATEST() slide.
	if _, err := repo.TouchUserSession(ctx, expired, ttl); !errors.Is(err, repository.ErrSessionInvalid) {
		t.Fatalf("expired token: got %v, want ErrSessionInvalid", err)
	}

	live := "live-" + strings.ToLower(t.Name())
	if err := repo.CreateUserSession(ctx, live, user.ID, time.Now().Add(ttl), "", ""); err != nil {
		t.Fatalf("create live session: %v", err)
	}
	n, err := repo.RevokeAllUserSessions(ctx, user.ID)
	if err != nil {
		t.Fatalf("revoke all: %v", err)
	}
	if n != 1 {
		t.Fatalf("revoke all reported %d session(s), want 1 (the expired row is already dead)", n)
	}

	// The purge clears both the expired and the just-revoked-in-the-past rows;
	// the freshly revoked one is kept for its one-day grace period.
	if _, err := repo.Pool().Exec(ctx, `UPDATE user_sessions SET revoked_at = NOW() - INTERVAL '2 days' WHERE token_hash=$1`, live); err != nil {
		t.Fatalf("age revoked row: %v", err)
	}
	if _, err := repo.DeleteExpiredUserSessions(ctx); err != nil {
		t.Fatalf("prune: %v", err)
	}
	var remaining int
	if err := repo.Pool().QueryRow(ctx, `SELECT COUNT(*) FROM user_sessions WHERE user_id=$1`, user.ID).Scan(&remaining); err != nil {
		t.Fatalf("count remaining: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("%d session row(s) survived the purge, want 0", remaining)
	}
}
