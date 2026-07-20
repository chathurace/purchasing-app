-- Commercial details on the procurement recommendation, mirroring the PR's
-- commercial section: an estimated value + currency and the engagement type
-- (config-option lists on the client). Finance may refine these on the
-- recommendation independently of the requester's original PR figures.

ALTER TABLE pr_recommendations
    ADD COLUMN estimated_value NUMERIC(16,2) NOT NULL DEFAULT 0,
    ADD COLUMN currency        TEXT          NOT NULL DEFAULT '',
    ADD COLUMN engagement_type TEXT          NOT NULL DEFAULT '';
