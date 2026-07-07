-- User management (admin-maintained). Two changes to support admins adding
-- users *by email* before that person has ever logged in:
--
--  1. `sub` becomes nullable. An admin-invited user has an email (and optional
--     roles) but no OIDC subject yet. On their first login the auth layer claims
--     the pending row by matching email and fills in the real `sub`.
--  2. `is_active` lets admins deactivate a user (block login) without losing
--     their history. New and existing users default to active.
--
-- We also enforce case-insensitive email uniqueness so an invite and the
-- eventual login (which may differ only in case) resolve to the same row. The
-- old case-sensitive UNIQUE(email) is replaced by a unique index on lower(email).

ALTER TABLE users ALTER COLUMN sub DROP NOT NULL;
ALTER TABLE users ADD COLUMN is_active BOOLEAN NOT NULL DEFAULT TRUE;

-- Normalize existing emails to lowercase before adding the case-insensitive
-- unique index (no-op if already lowercase).
UPDATE users SET email = lower(email) WHERE email IS NOT NULL AND email <> lower(email);

ALTER TABLE users DROP CONSTRAINT IF EXISTS users_email_key;
CREATE UNIQUE INDEX users_email_lower_key ON users (lower(email)) WHERE email IS NOT NULL;
