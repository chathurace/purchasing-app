-- Cost-center revamp → "budget units" (BU).
--
-- A budget unit replaces the old primary/secondary-owner model with value-based
-- approval brackets: each BU has one or more ordered brackets (min/max value,
-- max NULL = unbounded), and each bracket names one or more approvers. The
-- budget approver for a purchase request is derived from the BU plus an
-- estimated value + currency (see resolve_budget_bracket below).
--
-- Dev-only: there is no data migration. Existing cost-center rows are dropped.

-- 1. Drop the old owner model.
DROP TABLE IF EXISTS cost_center_secondary_owners;
ALTER TABLE cost_centers DROP COLUMN IF EXISTS primary_owner_id;

-- 2. Rename the table, its indexes and the referencing FK columns.
ALTER TABLE cost_centers RENAME TO budget_units;
ALTER INDEX idx_cost_centers_name RENAME TO idx_budget_units_name;
ALTER INDEX idx_cost_centers_code RENAME TO idx_budget_units_code;

ALTER TABLE purchase_requests RENAME COLUMN cost_center_id TO budget_unit_id;
-- The legacy free-text cost_center label is no longer maintained.
ALTER TABLE purchase_requests DROP COLUMN IF EXISTS cost_center;

ALTER TABLE invoice_cost_allocations RENAME COLUMN cost_center_id TO budget_unit_id;
ALTER INDEX idx_invoice_cost_allocations_cost_center RENAME TO idx_invoice_cost_allocations_budget_unit;

-- 3. Clear existing data (dev only) — old rows have no brackets and the PR/
--    allocation references become dangling, so start clean.
TRUNCATE budget_units CASCADE;
UPDATE purchase_requests SET budget_unit_id = NULL;

-- 4. Approval brackets: ordered value ranges per budget unit. min_value is
--    inclusive; max_value is inclusive and NULL means unbounded (any value).
--    Resolution walks brackets by (position, id) and the first match wins.
CREATE TABLE budget_unit_brackets (
    id             BIGSERIAL     PRIMARY KEY,
    budget_unit_id BIGINT        NOT NULL REFERENCES budget_units(id) ON DELETE CASCADE,
    position       INT           NOT NULL DEFAULT 0,
    min_value      NUMERIC(16,2) NOT NULL DEFAULT 0,
    max_value      NUMERIC(16,2),  -- NULL = unbounded
    created_at     TIMESTAMPTZ   NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_budget_unit_brackets_bu ON budget_unit_brackets(budget_unit_id, position, id);

-- 5. Approvers per bracket (one or more). All approvers of the resolved bracket
--    are qualified to approve the budget card; there is no "first" preference.
CREATE TABLE budget_unit_bracket_approvers (
    bracket_id BIGINT NOT NULL REFERENCES budget_unit_brackets(id) ON DELETE CASCADE,
    user_id    BIGINT NOT NULL REFERENCES users(id),
    position   INT    NOT NULL DEFAULT 0,
    PRIMARY KEY (bracket_id, user_id)
);

-- 6. resolve_budget_bracket picks the bracket that governs a given value for a
--    budget unit:
--      * if a value is supplied AND its currency matches the BU currency, the
--        first bracket (by position, id) whose [min_value, max_value] contains
--        the value wins;
--      * otherwise (no value, currency mismatch, or no bracket matched) the
--        highest bracket is assumed — unbounded max first, else the largest max.
--    Returns NULL when the BU has no brackets. STABLE so it can be used freely
--    inside SELECT predicates.
CREATE OR REPLACE FUNCTION resolve_budget_bracket(
    p_bu_id    BIGINT,
    p_value    NUMERIC,
    p_currency TEXT
) RETURNS BIGINT AS $$
DECLARE
    v_bracket_id  BIGINT;
    v_bu_currency TEXT;
BEGIN
    IF p_bu_id IS NULL THEN
        RETURN NULL;
    END IF;
    SELECT currency INTO v_bu_currency FROM budget_units WHERE id = p_bu_id;

    IF p_value IS NOT NULL AND p_currency IS NOT NULL AND p_currency = v_bu_currency THEN
        SELECT b.id INTO v_bracket_id
        FROM budget_unit_brackets b
        WHERE b.budget_unit_id = p_bu_id
          AND b.min_value <= p_value
          AND (b.max_value IS NULL OR p_value <= b.max_value)
        ORDER BY b.position, b.id
        LIMIT 1;
        IF v_bracket_id IS NOT NULL THEN
            RETURN v_bracket_id;
        END IF;
    END IF;

    -- Fallback: the highest-value bracket.
    SELECT b.id INTO v_bracket_id
    FROM budget_unit_brackets b
    WHERE b.budget_unit_id = p_bu_id
    ORDER BY (b.max_value IS NULL) DESC, b.max_value DESC NULLS LAST, b.position, b.id
    LIMIT 1;
    RETURN v_bracket_id;
END;
$$ LANGUAGE plpgsql STABLE;
