-- Per-approver sign-off on a purchase request. When a PR is created the
-- requester names one or more approvers; each approver independently approves or
-- rejects (with a comment). All approvers must approve before any RFQ can be
-- raised against the PR. A rejected approval can be re-requested by the
-- requester, which resets that one approver's row back to pending.
--
-- One row per (PR, approver): the row IS the current decision, so re-requesting
-- mutates it in place rather than appending. The PR's own status is untouched —
-- the approval gate is derived from this table, mirroring how contract review
-- approvals gate contract signing.
CREATE TABLE pr_approvals (
    id                  BIGSERIAL   PRIMARY KEY,
    purchase_request_id BIGINT      NOT NULL REFERENCES purchase_requests(id) ON DELETE CASCADE,
    approver_id         BIGINT      NOT NULL REFERENCES users(id),
    status              TEXT        NOT NULL DEFAULT 'pending',  -- pending | approved | rejected
    comment             TEXT        NOT NULL DEFAULT '',         -- the approver's decision comment
    decided_at          TIMESTAMPTZ,                             -- set when approved/rejected, cleared on re-request
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (purchase_request_id, approver_id)
);
CREATE INDEX idx_pr_approvals_pr       ON pr_approvals(purchase_request_id);
CREATE INDEX idx_pr_approvals_approver ON pr_approvals(approver_id);
