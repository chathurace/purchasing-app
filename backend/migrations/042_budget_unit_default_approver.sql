-- Budget-unit refinements:
--   * a budget unit has a single DEFAULT approver — the catch-all when a PR
--     matches no bracket (this replaces the old "highest bracket" fallback);
--   * each bracket carries its OWN currency, so matching keys on the bracket's
--     currency + value range rather than the budget unit's currency.

ALTER TABLE budget_units
    ADD COLUMN default_approver_id BIGINT REFERENCES users(id);

ALTER TABLE budget_unit_brackets
    ADD COLUMN currency TEXT NOT NULL DEFAULT '';

-- Dev backfill: default the catch-all approver to whoever created the unit so
-- existing rows resolve to a real user.
UPDATE budget_units SET default_approver_id = created_by WHERE default_approver_id IS NULL;

-- resolve_budget_approvers returns the set of qualified budget-approver user ids
-- for a budget unit at a given value + currency:
--   * the approvers of the FIRST bracket (by position) whose currency matches
--     and whose [min_value, max_value] contains the value (max NULL = unbounded);
--   * otherwise (no value, no currency, or no matching bracket) the budget
--     unit's default approver, if set.
-- Replaces resolve_budget_bracket (dropped below).
CREATE OR REPLACE FUNCTION resolve_budget_approvers(
    p_bu_id    BIGINT,
    p_value    NUMERIC,
    p_currency TEXT
) RETURNS TABLE(user_id BIGINT) AS $$
DECLARE
    v_bracket_id BIGINT;
    v_default    BIGINT;
BEGIN
    IF p_bu_id IS NULL THEN
        RETURN;
    END IF;

    IF p_value IS NOT NULL AND p_currency IS NOT NULL AND p_currency <> '' THEN
        SELECT b.id INTO v_bracket_id
        FROM budget_unit_brackets b
        WHERE b.budget_unit_id = p_bu_id
          AND b.currency = p_currency
          AND b.min_value <= p_value
          AND (b.max_value IS NULL OR p_value <= b.max_value)
        ORDER BY b.position, b.id
        LIMIT 1;
    END IF;

    IF v_bracket_id IS NOT NULL THEN
        RETURN QUERY
            SELECT ba.user_id
            FROM budget_unit_bracket_approvers ba
            WHERE ba.bracket_id = v_bracket_id
            ORDER BY ba.position, ba.user_id;
        RETURN;
    END IF;

    -- No bracket matched → the default approver (if any).
    SELECT default_approver_id INTO v_default FROM budget_units WHERE id = p_bu_id;
    IF v_default IS NOT NULL THEN
        RETURN QUERY SELECT v_default;
    END IF;
END;
$$ LANGUAGE plpgsql STABLE;

DROP FUNCTION IF EXISTS resolve_budget_bracket(BIGINT, NUMERIC, TEXT);
