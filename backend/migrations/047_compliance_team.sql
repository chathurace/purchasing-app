-- Compliance team + compliance recommendation approval card.
--
-- Mirrors Security exactly: a `compliance` role whose holders are the Compliance
-- team (membership *is* the role — see 038_teams.sql), a seeded teams row so the
-- Settings page renders its card, and — because the approval-card set is validated
-- in Go, not in SQL (pr_recommendation_approvals.approval_type is free text) — no
-- schema change is needed for the new card itself.

INSERT INTO roles (name) VALUES ('compliance') ON CONFLICT (name) DO NOTHING;

INSERT INTO teams (key, name, member_role) VALUES
    ('compliance', 'Compliance', 'compliance')
ON CONFLICT (key) DO NOTHING;
