-- Reshape contract approvals into typed "reviews". A review is one reviewer's
-- input of a given type (legal | security | budget | other), completed either as
-- a plain "submitted" comment or as a formal "approved" sign-off. The list stays
-- append-only (multiple reviews per type are allowed); the contract's own status
-- is the rolled-up state, recomputed from the set of approved review types.
ALTER TABLE contract_approvals RENAME TO contract_reviews;
ALTER TABLE contract_reviews RENAME COLUMN approver_id TO reviewer_id;
ALTER TABLE contract_reviews RENAME COLUMN decision   TO outcome;

ALTER TABLE contract_reviews
    ADD COLUMN review_type TEXT NOT NULL DEFAULT 'other';   -- legal | security | budget | other

-- Normalize legacy decision values to the new outcome vocabulary. Old "approve"
-- rows were formal sign-offs; everything else becomes a neutral submitted note.
UPDATE contract_reviews SET outcome = 'approved'  WHERE outcome = 'approve';
UPDATE contract_reviews SET outcome = 'submitted' WHERE outcome NOT IN ('approved');

ALTER INDEX idx_contract_approvals_contract RENAME TO idx_contract_reviews_contract;
CREATE INDEX idx_contract_reviews_type ON contract_reviews(contract_id, review_type);

-- New reviewer roles. Legal reviews require the legal role; security reviews the
-- security role. Budget/other reviews use the existing finance-access roles.
INSERT INTO roles (name) VALUES ('legal'), ('security') ON CONFLICT (name) DO NOTHING;
