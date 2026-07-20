-- PR assignment. After team-lead approval, a PR must be assigned to a member of
-- the procurement team before any procurement work (quotations, recommendations)
-- can start. A PR carries a single assignee plus zero or more collaborators (also
-- procurement users); assignee + collaborators are the people who may act on it.
-- Columns exist (empty) from PR creation and are populated later by procurement.

ALTER TABLE purchase_requests
    ADD COLUMN assignee_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN assigned_at TIMESTAMPTZ,
    ADD COLUMN assigned_by BIGINT REFERENCES users(id) ON DELETE SET NULL;

CREATE TABLE purchase_request_collaborators (
    purchase_request_id BIGINT NOT NULL REFERENCES purchase_requests(id) ON DELETE CASCADE,
    user_id             BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    added_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    added_by            BIGINT REFERENCES users(id) ON DELETE SET NULL,
    PRIMARY KEY (purchase_request_id, user_id)
);
