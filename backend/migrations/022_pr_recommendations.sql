-- Procurement recommendation: finance's proposal to proceed with a vendor for a
-- purchase request once quotations are in. One per PR. It carries the required
-- sign-offs (budget owner / legal / security), each an "approval card" the right
-- actor toggles approved and can comment on (with document attachments). All
-- required cards must be approved before a quotation may be selected / a contract
-- drafted (enforced in the repository).

CREATE TABLE pr_recommendations (
    id                  BIGSERIAL   PRIMARY KEY,           -- displayed as REC-{id:06d}
    purchase_request_id BIGINT      NOT NULL UNIQUE REFERENCES purchase_requests(id) ON DELETE CASCADE,
    vendor_id           BIGINT      NOT NULL REFERENCES vendors(id) ON DELETE RESTRICT,
    description         TEXT        NOT NULL DEFAULT '',
    created_by          BIGINT      REFERENCES users(id),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- The required approval cards. A row's presence means the card is required;
-- approved_by non-null means the card is approved (the per-card approve toggle).
CREATE TABLE pr_recommendation_approvals (
    recommendation_id BIGINT      NOT NULL REFERENCES pr_recommendations(id) ON DELETE CASCADE,
    approval_type     TEXT        NOT NULL,                 -- budget | legal | security
    approved_by       BIGINT      REFERENCES users(id),
    approved_at       TIMESTAMPTZ,
    PRIMARY KEY (recommendation_id, approval_type)
);

-- Comments left on an approval card (text and/or document attachments). Documents
-- attach via the shared documents table (owner_type 'pr_recommendation_comment',
-- owner_id = the comment id). Append-only.
CREATE TABLE pr_recommendation_comments (
    id                BIGSERIAL   PRIMARY KEY,
    recommendation_id BIGINT      NOT NULL REFERENCES pr_recommendations(id) ON DELETE CASCADE,
    approval_type     TEXT        NOT NULL,                 -- budget | legal | security
    author_id         BIGINT      NOT NULL REFERENCES users(id),
    comment           TEXT        NOT NULL DEFAULT '',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_pr_rec_comments ON pr_recommendation_comments(recommendation_id, approval_type);
