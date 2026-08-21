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
// since both are client-controlled. idpSID is the `sid` claim of the ID token
// this session was minted from (may be empty), which is what lets a
// back-channel logout revoke exactly the browser the IdP is talking about.
func (r *Repository) CreateUserSession(ctx context.Context, tokenHash string, userID int64, expiresAt time.Time, userAgent, ip, idpSID string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO user_sessions (token_hash, user_id, expires_at, user_agent, ip, idp_sid)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		tokenHash, userID, expiresAt, truncateStr(userAgent, 512), nilIfEmpty(ip), nilIfEmpty(idpSID),
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
// maxLifetime is the absolute ceiling measured from created_at (0 disables it).
// Without it the sliding window means a session used at least once per TTL
// window lives forever, and since the IdP is never consulted again after mint,
// an account disabled at Asgardeo would keep full access indefinitely. The cap
// bounds that window even when nobody revokes anything by hand.
//
// last_used_at is refreshed only when it is more than a minute stale: every
// authenticated request passes through here, and an unconditional UPDATE would
// mean a row write (and WAL record) per request per user.
func (r *Repository) TouchUserSession(ctx context.Context, tokenHash string, ttl, maxLifetime time.Duration) (int64, error) {
	var userID int64
	err := r.pool.QueryRow(ctx, `
		UPDATE user_sessions
		   SET expires_at   = GREATEST(expires_at, NOW() + make_interval(secs => $2)),
		       last_used_at = CASE WHEN last_used_at < NOW() - INTERVAL '1 minute'
		                           THEN NOW() ELSE last_used_at END
		 WHERE token_hash = $1
		   AND revoked_at IS NULL
		   AND expires_at > NOW()
		   AND ($3 <= 0 OR created_at > NOW() - make_interval(secs => $3))
		RETURNING user_id`,
		tokenHash, ttl.Seconds(), maxLifetime.Seconds(),
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

// UsersWithLiveSessions lists the users who currently hold at least one live
// session, with their email. The candidate set for the IdP offboarding sweep
// (internal/offboard): users without a session need no action, so scanning the
// whole users table would be wasted work.
//
// Users with no email are skipped — the sweep matches identities by email, so
// there is nothing to compare them against.
func (r *Repository) UsersWithLiveSessions(ctx context.Context) ([]UserSummary, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT DISTINCT u.id, u.email, COALESCE(u.name, '')
		  FROM user_sessions s
		  JOIN users u ON u.id = s.user_id
		 WHERE s.revoked_at IS NULL
		   AND s.expires_at > NOW()
		   AND u.email IS NOT NULL
		   AND u.email <> ''
		 ORDER BY u.id`)
	if err != nil {
		return nil, fmt.Errorf("users with live sessions: %w", err)
	}
	defer rows.Close()
	var out []UserSummary
	for rows.Next() {
		var u UserSummary
		if err := rows.Scan(&u.ID, &u.Email, &u.Name); err != nil {
			return nil, fmt.Errorf("users with live sessions: %w", err)
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// RevokeUserSessionsByIdPSID revokes the sessions minted from one IdP session,
// returning how many were live. Used by back-channel logout when the logout
// token names a `sid`: the person may be signed in from several browsers, and
// only the one whose IdP session ended should be cut off.
//
// Also returns the owning user id (0 when nothing matched) so the caller can
// audit *whose* sessions ended — the logout token identifies a session, not
// necessarily a subject we can look up.
func (r *Repository) RevokeUserSessionsByIdPSID(ctx context.Context, sid string) (revoked int64, userID int64, err error) {
	rows, err := r.pool.Query(ctx, `
		UPDATE user_sessions SET revoked_at = NOW()
		 WHERE idp_sid = $1 AND revoked_at IS NULL AND expires_at > NOW()
		RETURNING user_id`, sid)
	if err != nil {
		return 0, 0, fmt.Errorf("session revoke by sid: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		if err := rows.Scan(&userID); err != nil {
			return 0, 0, fmt.Errorf("session revoke by sid: %w", err)
		}
		revoked++
	}
	return revoked, userID, rows.Err()
}

// RevokeUserSessionsBySub revokes every live session of the user with this IdP
// subject. The fallback for a back-channel logout with no `sid` (or a `sid` we
// never recorded), where the spec's intent is "this subject's session(s) are
// over" — so signing them out everywhere is the safe reading.
func (r *Repository) RevokeUserSessionsBySub(ctx context.Context, sub string) (revoked int64, userID int64, err error) {
	err = r.pool.QueryRow(ctx, `SELECT id FROM users WHERE sub = $1`, sub).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		// Unknown subject: nobody by that sub has ever signed in here. Not an
		// error — the IdP broadcasts to every registered app.
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, fmt.Errorf("session revoke by sub: %w", err)
	}
	revoked, err = r.RevokeAllUserSessions(ctx, userID)
	return revoked, userID, err
}

// DeleteExpiredUserSessions removes sessions that expired, were revoked more
// than a day ago, or aged past the absolute cap (0 disables that clause). The
// grace period keeps recently-revoked rows around briefly so a support question
// about "who signed out when" is still answerable.
func (r *Repository) DeleteExpiredUserSessions(ctx context.Context, maxLifetime time.Duration) (int64, error) {
	tag, err := r.pool.Exec(ctx, `
		DELETE FROM user_sessions
		 WHERE expires_at < NOW()
		    OR revoked_at < NOW() - INTERVAL '1 day'
		    OR ($1 > 0 AND created_at <= NOW() - make_interval(secs => $1))`,
		maxLifetime.Seconds())
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
