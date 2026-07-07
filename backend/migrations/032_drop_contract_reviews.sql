-- Contract review/approval is removed: approvals now happen only on the purchase
-- request (the procurement recommendation's approval cards). A contract is simply
-- draft until its signed PDF is attached, then signed. Drop the review tables.
DROP TABLE IF EXISTS contract_review_approvals;
DROP TABLE IF EXISTS contract_reviews;
