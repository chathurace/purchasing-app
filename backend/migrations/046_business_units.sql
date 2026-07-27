-- "Budget units" → "business units".
--
-- The value-based approval-bracket model is dropped in favour of a much simpler
-- shape: a business unit is just a name, an optional description, and a flat list
-- of approvers. Budget-approval authority collapses to the existing
-- case-insensitive email match against purchase_requests.budget_approver_email
-- (the approver the requester picks from the unit's approver list), so
-- resolve_budget_approvers, the brackets, and the code/budget/currency/default
-- columns are no longer needed.
--
-- Dev-only: there is no data migration. Existing budget-unit rows are dropped.

-- 1. Drop the value-resolution machinery.
DROP FUNCTION IF EXISTS resolve_budget_approvers(BIGINT, NUMERIC, TEXT);
DROP TABLE IF EXISTS budget_unit_bracket_approvers;
DROP TABLE IF EXISTS budget_unit_brackets;

-- 2. Rename the table, its indexes and the referencing FK columns.
ALTER TABLE budget_units RENAME TO business_units;
ALTER INDEX idx_budget_units_name RENAME TO idx_business_units_name;
ALTER INDEX idx_budget_units_code RENAME TO idx_business_units_code;

ALTER TABLE purchase_requests RENAME COLUMN budget_unit_id TO business_unit_id;
ALTER TABLE invoice_cost_allocations RENAME COLUMN budget_unit_id TO business_unit_id;
ALTER INDEX idx_invoice_cost_allocations_budget_unit RENAME TO idx_invoice_cost_allocations_business_unit;

-- 3. Drop the now-unused columns.
ALTER TABLE business_units DROP COLUMN IF EXISTS code;
ALTER TABLE business_units DROP COLUMN IF EXISTS budget;
ALTER TABLE business_units DROP COLUMN IF EXISTS currency;
ALTER TABLE business_units DROP COLUMN IF EXISTS default_approver_id;
DROP INDEX IF EXISTS idx_business_units_code;

-- 4. Clear existing data (dev only) — old rows had no approvers under the new
--    model and the PR/allocation references become dangling, so start clean.
TRUNCATE business_units CASCADE;
UPDATE purchase_requests SET business_unit_id = NULL;

-- 5. Flat approver list per business unit (one or more). Any approver of the
--    unit may be picked as the PR's budget approver.
CREATE TABLE business_unit_approvers (
    business_unit_id BIGINT NOT NULL REFERENCES business_units(id) ON DELETE CASCADE,
    user_id          BIGINT NOT NULL REFERENCES users(id),
    position         INT    NOT NULL DEFAULT 0,
    PRIMARY KEY (business_unit_id, user_id)
);
