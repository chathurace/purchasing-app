-- Cost centers are managed master data (admin / finance_admin) referenced by
-- purchase requests. Like vendors, they are soft-deleted via is_active rather
-- than removed, so history is preserved.
CREATE TABLE cost_centers (
    id               BIGSERIAL     PRIMARY KEY,           -- displayed as CC-{id:06d}
    code             TEXT          NOT NULL DEFAULT '',
    name             TEXT          NOT NULL,
    description      TEXT          NOT NULL DEFAULT '',
    primary_owner_id BIGINT        REFERENCES users(id),
    budget           NUMERIC(16,2) NOT NULL DEFAULT 0,
    currency         TEXT          NOT NULL DEFAULT '',
    is_active        BOOLEAN       NOT NULL DEFAULT TRUE,
    created_by       BIGINT        REFERENCES users(id),
    created_at       TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ   NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_cost_centers_name ON cost_centers(name);
-- code is optional, but unique when present
CREATE UNIQUE INDEX idx_cost_centers_code ON cost_centers(code) WHERE code <> '';

-- Secondary owners: many-to-many (mirror of pr_approvals shape).
CREATE TABLE cost_center_secondary_owners (
    cost_center_id BIGINT NOT NULL REFERENCES cost_centers(id) ON DELETE CASCADE,
    user_id        BIGINT NOT NULL REFERENCES users(id),
    PRIMARY KEY (cost_center_id, user_id)
);

-- Link purchase requests to the master (nullable; the legacy free-text
-- cost_center column is kept for history and for PRs created before this).
ALTER TABLE purchase_requests ADD COLUMN cost_center_id BIGINT REFERENCES cost_centers(id);
