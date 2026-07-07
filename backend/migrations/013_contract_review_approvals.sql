-- Approval is now tracked per review TYPE per contract, separately from the
-- append-only review log. A row's presence means that review type is approved;
-- the approve toggle inserts/deletes it, and submitting a new review of that
-- type clears it (back to pending approval). The contract's rolled-up status is
-- derived from this table, not from individual review rows.
CREATE TABLE contract_review_approvals (
    contract_id BIGINT      NOT NULL REFERENCES contracts(id) ON DELETE CASCADE,
    review_type TEXT        NOT NULL,                 -- legal | security | budget | other
    approved_by BIGINT      REFERENCES users(id),
    approved_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (contract_id, review_type)
);

-- Carry over existing approvals: any review type that already had an approved
-- review stays approved (latest approver wins) so currently-approved contracts
-- are not silently knocked back to pending.
INSERT INTO contract_review_approvals (contract_id, review_type, approved_by, approved_at)
SELECT DISTINCT ON (contract_id, review_type)
       contract_id, review_type, reviewer_id, created_at
FROM contract_reviews
WHERE outcome = 'approved'
ORDER BY contract_id, review_type, created_at DESC
ON CONFLICT DO NOTHING;
