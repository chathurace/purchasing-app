-- Multiple OIDC subjects per user — one human, several front ends.
--
-- Until now identity was `users.sub`, one subject per row. That holds while a
-- single SPA (and so a single Asgardeo application) is the only way in. It stops
-- holding the moment a second front end signs the same people in through a
-- second Asgardeo application: Asgardeo may issue an application-scoped `sub`,
-- so the same person arrives with a subject this app has never seen.
--
-- Without this table that login would fall through to the INSERT in
-- ProvisionUserOnLogin, whose `ON CONFLICT (sub)` does not catch the collision
-- that actually happens — `users_email_lower_key` (migration 015) — so the
-- request 500s and the person cannot use the app at all. See
-- docs/plans/21-one-wso2-port.md, Decision 5.
--
-- `users.sub` is deliberately LEFT IN PLACE and keeps holding whichever subject
-- was seen first: it is what `pending` is derived from (`sub IS NULL` means an
-- invited user who has never signed in) and what UpdateInvitedUser gates on.
-- This table is the authoritative *lookup*; that column is the primary subject.

CREATE TABLE user_identities (
    -- The OIDC subject. Primary key rather than a surrogate: a subject belongs
    -- to exactly one user, and the uniqueness is the point of the table.
    sub        TEXT        PRIMARY KEY,
    user_id    BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Reverse lookup: "which subjects does this user hold" (the Users page, and
-- ON DELETE CASCADE's own cleanup).
CREATE INDEX user_identities_user_id_idx ON user_identities (user_id);

-- Backfill every subject already recorded. Invited-but-never-logged-in rows have
-- sub IS NULL and get no identity row, which is correct — they have no subject
-- yet, and their first login claims the row by email exactly as before.
INSERT INTO user_identities (sub, user_id)
SELECT sub, id FROM users WHERE sub IS NOT NULL
ON CONFLICT (sub) DO NOTHING;
