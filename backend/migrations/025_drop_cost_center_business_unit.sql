-- Remove the business_unit association from cost centers. Budget ownership is no
-- longer derived from cost_centers.business_unit == purchase_requests.team; a PR
-- now references a real cost center directly (purchase_requests.cost_center_id,
-- migration 018) and its owner is the budget approver. See migration 023 for the
-- column this reverses.
DROP INDEX IF EXISTS idx_cost_centers_bu;

ALTER TABLE cost_centers DROP COLUMN IF EXISTS business_unit;
