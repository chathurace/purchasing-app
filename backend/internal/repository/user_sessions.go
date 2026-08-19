package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// ErrSessionInvalid is returned when a session token is unknown, revoked, or
// expired. The caller cannot distinguish which, by design — the client is told
// only "not authenticated".
var ErrSessionInvalid = errors.New("session invalid, expired, or revoked")

// --- Backend-issued browser sessions (migration 052, docs/sessions.md) ---
//
// Rows hold only the SHA-256 hash of the session token, never the token itself.

// CreateUserSession stores a new session for userID expiring at expiresAt.
// userAgent and ip are recorded for forensics only — neither authenticates,
// since both are client-controlled.
func (r *Repository) CreateUserSession(ctx context.Context, tokenHash string, userID int64, expiresAt time.Time, userAgent, ip string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO user_sessions (token_hash, user_id, expires_at, user_agent, ip)
		VALUES ($1, $2, $3, $4, $5)`,
		tokenHash, userID, expiresAt, truncateStr(userAgent, 512), nilIfEmpty(ip),
	)
	if err != nil {
		return fmt.Errorf("session insert: %w", err)
	}
	return nil
}

// TouchUserSession resolves a session token hash to its user and slides the
// expiry forward, in a single statement so lookup and renewal cannot race.
//
// The sliding window is what makes a 60-day session mean "60 days idle" rather
// than "60 days since login": an active user's session is continuously renewed
// and never expires mid-use, while an abandoned one still dies on schedule.
// expires_at is only ever moved forward (GREATEST), so shortening
// session.ttl_days takes effect for new sessions without extending old ones.
//
// last_used_at is refreshed only when it is more than a minute stale: every
// authenticated request passes through here, and an unconditional UPDATE would
// mean a row write (and WAL record) per request per user.
func (r *Repository) TouchUserSession(ctx context.Context, tokenHash string, ttl time.Duration) (int64, error) {
	var userID int64
	err := r.pool.QueryRow(ctx, `
		UPDATE user_sessions
		   SET expires_at   = GREATEST(expires_at, NOW() + make_interval(secs => $2)),
		       last_used_at = CASE WHEN last_used_at < NOW() - INTERVAL '1 minute'
		                           THEN NOW() ELSE last_used_at END
		 WHERE token_hash = $1
		   AND revoked_at IS NULL
		   AND expires_at > NOW()
		RETURNING user_id`,
		tokenHash, ttl.Seconds(),
	).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrSessionInvalid
	}
	if err != nil {
		return 0, fmt.Errorf("session touch: %w", err)
	}
	return userID, nil
}

// RevokeUserSession marks a single session revoked (sign out on this browser).
// Revoking an already-revoked or unknown token is not an error — logout must be
// idempotent, and reporting "no such session" would leak token validity.
func (r *Repository) RevokeUserSession(ctx context.Context, tokenHash string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE user_sessions SET revoked_at = NOW()
		 WHERE token_hash = $1 AND revoked_at IS NULL`, tokenHash)
	if err != nil {
		return fmt.Errorf("session revoke: %w", err)
	}
	return nil
}

// RevokeAllUserSessions kills every live session a user has ("sign out
// everywhere"), returning the number ended. This is the lever that makes a
// 60-day cookie safe to hand out: access can be withdrawn immediately instead
// of waiting for the cookie to age out.
func (r *Repository) RevokeAllUserSessions(ctx context.Context, userID int64) (int64, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE user_sessions SET revoked_at = NOW()
		 WHERE user_id = $1 AND revoked_at IS NULL AND expires_at > NOW()`, userID)
	if err != nil {
		return 0, fmt.Errorf("session revoke all: %w", err)
	}
	return tag.RowsAffected(), nil
}

// DeleteExpiredUserSessions removes sessions that expired, or were revoked more
// than a day ago. The grace period keeps recently-revoked rows around briefly
// so a support question about "who signed out when" is still answerable.
func (r *Repository) DeleteExpiredUserSessions(ctx context.Context) (int64, error) {
	tag, err := r.pool.Exec(ctx, `
		DELETE FROM user_sessions
		 WHERE expires_at < NOW()
		    OR revoked_at < NOW() - INTERVAL '1 day'`)
	if err != nil {
		return 0, fmt.Errorf("session cleanup: %w", err)
	}
	return tag.RowsAffected(), nil
}

func truncateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}
