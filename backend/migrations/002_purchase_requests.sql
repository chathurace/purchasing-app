CREATE TABLE purchase_requests (
    id           BIGSERIAL   PRIMARY KEY,           -- displayed as PR-{id:06d}
    title        TEXT        NOT NULL DEFAULT '',    -- short summary for the list view
    requester_id BIGINT      NOT NULL REFERENCES users(id),
    cost_center  TEXT        NOT NULL DEFAULT '',
    comments     TEXT        NOT NULL DEFAULT '',
    status       TEXT        NOT NULL DEFAULT 'submitted',
        -- submitted | under_review | vendor_selected | contract_prepared
        -- | completed | rejected | cancelled
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_pr_requester ON purchase_requests(requester_id);
CREATE INDEX idx_pr_status    ON purchase_requests(status);
