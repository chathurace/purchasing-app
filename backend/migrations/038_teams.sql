-- Teams: named groups (Legal, Security, Finance) whose membership is a role and
-- which carry a shared team email for notifications. Membership is *derived* from
-- the member_role (a user is a member iff they hold that role) — adding/removing a
-- member simply grants/revokes the role — so there is no separate membership table.
-- The seeded set is fixed for now; member_role is display-only (the app's approval
-- actor logic is keyed on the role, so remapping it is intentionally not exposed).

CREATE TABLE teams (
    id          BIGSERIAL PRIMARY KEY,
    key         TEXT NOT NULL UNIQUE,               -- stable identifier used in URLs
    name        TEXT NOT NULL,
    member_role TEXT NOT NULL REFERENCES roles(name),
    team_email  TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO teams (key, name, member_role) VALUES
    ('legal',    'Legal',    'legal'),
    ('security', 'Security', 'security'),
    ('finance',  'Finance',  'finance');
