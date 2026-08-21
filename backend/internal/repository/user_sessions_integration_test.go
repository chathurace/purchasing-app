package repository_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/cs/purchasing-app/internal/repository"
)

// noCap disables the absolute-lifetime ceiling, so the tests that are about the
// idle window aren't also asserting the cap. TestUserSessionAbsoluteCap covers
// the ceiling itself.
const noCap = time.Duration(0)

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
	if err := repo.CreateUserSession(ctx, hash, user.ID, time.Now().Add(time.Hour), "Go test UA", "203.0.113.7", ""); err != nil {
		t.Fatalf("create session: %v", err)
	}

	gotID, err := repo.TouchUserSession(ctx, hash, ttl, noCap)
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
	if _, err := repo.TouchUserSession(ctx, hash, ttl, noCap); !errors.Is(err, repository.ErrSessionInvalid) {
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

	if _, err := repo.TouchUserSession(ctx, "no-such-hash-"+strings.ToLower(t.Name()), ttl, noCap); !errors.Is(err, repository.ErrSessionInvalid) {
		t.Fatalf("unknown token: got %v, want ErrSessionInvalid", err)
	}

	expired := "expired-" + strings.ToLower(t.Name())
	if err := repo.CreateUserSession(ctx, expired, user.ID, time.Now().Add(-time.Minute), "", "", ""); err != nil {
		t.Fatalf("create expired session: %v", err)
	}
	// An expired row must not be resurrected by the GREATEST() slide.
	if _, err := repo.TouchUserSession(ctx, expired, ttl, noCap); !errors.Is(err, repository.ErrSessionInvalid) {
		t.Fatalf("expired token: got %v, want ErrSessionInvalid", err)
	}

	live := "live-" + strings.ToLower(t.Name())
	if err := repo.CreateUserSession(ctx, live, user.ID, time.Now().Add(ttl), "", "", ""); err != nil {
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
	if _, err := repo.DeleteExpiredUserSessions(ctx, noCap); err != nil {
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

// TestUserSessionAbsoluteCap covers the ceiling that stops the sliding idle
// window from keeping a session alive forever — the bound on how long an account
// disabled at the IdP keeps working here.
func TestUserSessionAbsoluteCap(t *testing.T) {
	repo, ctx := newTestRepo(t)
	user := newSessionTestUser(t, repo, ctx)
	const (
		ttl = 60 * 24 * time.Hour
		cap = 90 * 24 * time.Hour
	)

	hash := "cap-" + strings.ToLower(t.Name())
	if err := repo.CreateUserSession(ctx, hash, user.ID, time.Now().Add(ttl), "", "", ""); err != nil {
		t.Fatalf("create session: %v", err)
	}
	// Fresh row: within the cap, so it still authenticates.
	if _, err := repo.TouchUserSession(ctx, hash, ttl, cap); err != nil {
		t.Fatalf("touch inside cap: %v", err)
	}

	// Age it past the ceiling. expires_at is deliberately left in the future —
	// that is the whole point: the idle window says "alive", the cap says "no".
	if _, err := repo.Pool().Exec(ctx,
		`UPDATE user_sessions SET created_at = NOW() - INTERVAL '91 days' WHERE token_hash=$1`, hash); err != nil {
		t.Fatalf("age session: %v", err)
	}
	if _, err := repo.TouchUserSession(ctx, hash, ttl, cap); !errors.Is(err, repository.ErrSessionInvalid) {
		t.Fatalf("touch past cap: got %v, want ErrSessionInvalid", err)
	}
	// Cap disabled (max_days: 0) — the same row authenticates again, which is
	// exactly the behaviour the startup warning is about.
	if _, err := repo.TouchUserSession(ctx, hash, ttl, noCap); err != nil {
		t.Fatalf("touch past cap with the cap disabled: %v", err)
	}
	// And the purge collects over-cap rows.
	if _, err := repo.DeleteExpiredUserSessions(ctx, cap); err != nil {
		t.Fatalf("prune: %v", err)
	}
	var remaining int
	if err := repo.Pool().QueryRow(ctx, `SELECT COUNT(*) FROM user_sessions WHERE token_hash=$1`, hash).Scan(&remaining); err != nil {
		t.Fatalf("count: %v", err)
	}
	if remaining != 0 {
		t.Fatal("an over-cap session survived the purge")
	}
}

// TestUsersWithLiveSessions covers the candidate set the IdP offboarding sweep
// works from: only users who actually hold a live session, once each.
func TestUsersWithLiveSessions(t *testing.T) {
	repo, ctx := newTestRepo(t)
	user := newSessionTestUser(t, repo, ctx)
	const ttl = 60 * 24 * time.Hour
	slug := strings.ToLower(t.Name())

	contains := func(list []repository.UserSummary, id int64) bool {
		for _, u := range list {
			if u.ID == id {
				return true
			}
		}
		return false
	}

	// No sessions yet: not a candidate.
	before, err := repo.UsersWithLiveSessions(ctx)
	if err != nil {
		t.Fatalf("list candidates: %v", err)
	}
	if contains(before, user.ID) {
		t.Fatal("a user with no sessions is a candidate")
	}

	// Two live sessions must still yield exactly one candidate row (DISTINCT).
	for _, h := range []string{"one-" + slug, "two-" + slug} {
		if err := repo.CreateUserSession(ctx, h, user.ID, time.Now().Add(ttl), "", "", ""); err != nil {
			t.Fatalf("create session: %v", err)
		}
	}
	// Plus a dead one, which must not resurrect anybody.
	if err := repo.CreateUserSession(ctx, "dead-"+slug, user.ID, time.Now().Add(-time.Hour), "", "", ""); err != nil {
		t.Fatalf("create expired session: %v", err)
	}

	during, err := repo.UsersWithLiveSessions(ctx)
	if err != nil {
		t.Fatalf("list candidates: %v", err)
	}
	seen := 0
	for _, u := range during {
		if u.ID == user.ID {
			seen++
			if u.Email != user.Email {
				t.Errorf("candidate email = %q, want %q", u.Email, user.Email)
			}
		}
	}
	if seen != 1 {
		t.Fatalf("user appeared %d times, want exactly 1", seen)
	}

	// Revoked ⇒ no longer a candidate.
	if _, err := repo.RevokeAllUserSessions(ctx, user.ID); err != nil {
		t.Fatalf("revoke all: %v", err)
	}
	after, err := repo.UsersWithLiveSessions(ctx)
	if err != nil {
		t.Fatalf("list candidates: %v", err)
	}
	if contains(after, user.ID) {
		t.Fatal("a user whose sessions are revoked is still a candidate")
	}
}

// TestRevokeSessionsByIdPSIDAndSub covers what back-channel logout does with a
// verified token: a `sid` ends exactly that browser, a `sub` ends all of them.
func TestRevokeSessionsByIdPSIDAndSub(t *testing.T) {
	repo, ctx := newTestRepo(t)
	user := newSessionTestUser(t, repo, ctx)
	const ttl = 60 * 24 * time.Hour
	slug := strings.ToLower(t.Name())

	// Two browsers from two different IdP sessions, plus one with no sid (minted
	// before the sid column existed, or by an IdP that omits the claim).
	sessions := map[string]string{
		"tab-a-" + slug:  "idp-sid-a-" + slug,
		"tab-b-" + slug:  "idp-sid-b-" + slug,
		"legacy-" + slug: "",
	}
	for hash, sid := range sessions {
		if err := repo.CreateUserSession(ctx, hash, user.ID, time.Now().Add(ttl), "", "", sid); err != nil {
			t.Fatalf("create session %s: %v", hash, err)
		}
	}

	// sid-scoped revocation must touch one row only.
	revoked, gotUser, err := repo.RevokeUserSessionsByIdPSID(ctx, "idp-sid-a-"+slug)
	if err != nil {
		t.Fatalf("revoke by sid: %v", err)
	}
	if revoked != 1 || gotUser != user.ID {
		t.Fatalf("revoke by sid = (%d, %d), want (1, %d)", revoked, gotUser, user.ID)
	}
	if _, err := repo.TouchUserSession(ctx, "tab-a-"+slug, ttl, noCap); !errors.Is(err, repository.ErrSessionInvalid) {
		t.Fatalf("tab A still authenticates: %v", err)
	}
	if _, err := repo.TouchUserSession(ctx, "tab-b-"+slug, ttl, noCap); err != nil {
		t.Fatalf("tab B was signed out by another session's logout: %v", err)
	}

	// An unknown sid is not an error — the IdP broadcasts to every app.
	if revoked, _, err := repo.RevokeUserSessionsByIdPSID(ctx, "no-such-sid-"+slug); err != nil || revoked != 0 {
		t.Fatalf("unknown sid = (%d, %v), want (0, nil)", revoked, err)
	}

	// sub-scoped revocation is the fallback: everything still live goes.
	revoked, gotUser, err = repo.RevokeUserSessionsBySub(ctx, user.Sub)
	if err != nil {
		t.Fatalf("revoke by sub: %v", err)
	}
	if revoked != 2 || gotUser != user.ID {
		t.Fatalf("revoke by sub = (%d, %d), want (2, %d)", revoked, gotUser, user.ID)
	}
	// An unknown subject: nobody by that sub has signed in here.
	if revoked, _, err := repo.RevokeUserSessionsBySub(ctx, "no-such-sub-"+slug); err != nil || revoked != 0 {
		t.Fatalf("unknown sub = (%d, %v), want (0, nil)", revoked, err)
	}
}
