-- The IdP session id (`sid` claim) behind a browser session, captured from the
-- ID token when the session cookie is minted.
--
-- Why: OIDC back-channel logout (docs/sessions.md). When Asgardeo ends a session
-- — the user signs out there, or an admin terminates it — it POSTs a logout
-- token to the app naming `sid` and/or `sub`. Without this column the only thing
-- the app could do with a `sid` is ignore it and revoke *every* session that
-- person has, signing them out of unrelated browsers. Nullable: sessions minted
-- before this migration, or from an ID token with no `sid` claim, simply fall
-- back to sub-wide revocation.
ALTER TABLE user_sessions ADD COLUMN IF NOT EXISTS idp_sid TEXT;

-- Lookup path for a back-channel logout carrying a sid. Partial, since most
-- queries against this table go by token_hash and only these rows are relevant.
CREATE INDEX IF NOT EXISTS idx_user_sessions_idp_sid
    ON user_sessions(idp_sid) WHERE idp_sid IS NOT NULL;
