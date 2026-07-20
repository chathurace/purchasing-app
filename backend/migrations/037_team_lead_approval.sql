-- 037_team_lead_approval.sql
-- Team lead approval: every PR names a team lead by email who must approve it
-- before finance can see or act on it. Decision state lives directly on the PR
-- (one team lead per PR).

ALTER TABLE purchase_requests
    ADD COLUMN team_lead_email   TEXT NOT NULL DEFAULT '',
    ADD COLUMN team_lead_status  TEXT NOT NULL DEFAULT 'pending'
        CHECK (team_lead_status IN ('pending', 'approved', 'rejected')),
    ADD COLUMN team_lead_notes   TEXT NOT NULL DEFAULT '',
    ADD COLUMN team_lead_decided_at TIMESTAMPTZ,
    ADD COLUMN team_lead_decided_by BIGINT REFERENCES users(id);

-- Grandfather every pre-existing PR to 'approved' so in-flight requests stay
-- visible to finance. New PRs created by the app default to 'pending'.
UPDATE purchase_requests SET team_lead_status = 'approved';

-- Visibility lookups match the caller's (lowercased) email against team_lead_email.
CREATE INDEX idx_purchase_requests_team_lead_email
    ON purchase_requests (team_lead_email);
