-- A procurement recommendation may optionally have a contract attached directly:
-- finance writes a description and attaches a PDF on the PR's recommendation
-- card. The contract is a normal row in `contracts` (so it shows up on the
-- contracts page and follows the usual review/sign lifecycle); this column links
-- the recommendation to its contract. ON DELETE SET NULL so removing the contract
-- leaves the recommendation intact.
ALTER TABLE pr_recommendations
    ADD COLUMN contract_id BIGINT REFERENCES contracts(id) ON DELETE SET NULL;
