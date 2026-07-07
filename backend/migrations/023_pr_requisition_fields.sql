-- Richer purchase-requisition form (WSO2 SOP-85000). Queryable/core fields are
-- promoted to columns; the rest of the structured form lives in a JSONB blob.
-- Budget ownership is no longer a per-PR cost-center pick — it is derived from
-- the requester's team/business unit, matched against a cost center tagged with
-- that business unit (see cost_centers.business_unit below).

ALTER TABLE purchase_requests
    ADD COLUMN team                  TEXT          NOT NULL DEFAULT '',  -- step 1 Team / Business unit (drives budget owner)
    ADD COLUMN entity                TEXT          NOT NULL DEFAULT '',  -- WSO2 legal entity
    ADD COLUMN category              TEXT          NOT NULL DEFAULT '',  -- IT | NON-IT
    ADD COLUMN estimated_value       NUMERIC(16,2) NOT NULL DEFAULT 0,
    ADD COLUMN currency              TEXT          NOT NULL DEFAULT '',
    ADD COLUMN budget_approver_name  TEXT          NOT NULL DEFAULT '',
    ADD COLUMN budget_approver_email TEXT          NOT NULL DEFAULT '',
    ADD COLUMN details               JSONB         NOT NULL DEFAULT '{}';

-- Tag cost centers with the business unit they fund. The budget owner of a PR is
-- the owner of the cost center whose business_unit matches the PR's team.
ALTER TABLE cost_centers
    ADD COLUMN business_unit TEXT NOT NULL DEFAULT '';

CREATE INDEX idx_cost_centers_bu ON cost_centers(business_unit);
