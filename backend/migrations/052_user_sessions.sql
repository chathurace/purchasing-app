-- Backend-issued browser sessions (BFF pattern) — see docs/sessions.md.
--
-- Why this exists: session length used to be dictated entirely by the Asgardeo
-- token lifetimes plus whatever oidc-client-ts kept in the browser, so a user
-- was signed out as soon as the ID token could no longer be renewed — roughly
-- daily. The Asgardeo organisation is managed by another team, so its session
-- and token lifetimes cannot be raised. Instead the app issues its own session:
-- the SPA still logs in through Asgardeo exactly as before, hands the resulting
-- token to the backend once (POST /api/v1/auth/session), and gets back an
-- HttpOnly cookie valid for session.ttl_days (60 by default), independent of
-- any IdP lifetime.
--
-- Only the SHA-256 hash of the 256-bit random session token is stored, so a
-- database or backup disclosure does not yield usable session credentials.
CREATE TABLE IF NOT EXISTS user_sessions (
    id           BIGSERIAL   PRIMARY KEY,
    -- SHA-256 (hex) of the token in the cookie. UNIQUE doubles as the lookup
    -- index every authenticated request resolves.
    token_hash   TEXT        NOT NULL UNIQUE,
    user_id      BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_used_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at   TIMESTAMPTZ NOT NULL,
    revoked_at   TIMESTAMPTZ,
    -- Forensics only (both are client-controlled, so neither authenticates).
    user_agent   TEXT,
    ip           TEXT
);

-- "Sign out everywhere" (revoke by user) and the ON DELETE CASCADE scan.
CREATE INDEX IF NOT EXISTS idx_user_sessions_user_id ON user_sessions(user_id);

-- The periodic purge of expired/revoked rows.
CREATE INDEX IF NOT EXISTS idx_user_sessions_expires_at ON user_sessions(expires_at);
