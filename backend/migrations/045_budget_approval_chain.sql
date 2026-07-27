-- Chained (serial) budget approvals for a procurement recommendation.
--
-- The recommendation's budget approval becomes an ordered chain of steps instead
-- of a single flat card. Step 1 (the "base" step) keeps the existing budget-unit
-- designated-approver behaviour; additional steps each name an approver (by email,
-- case-insensitive match like team-lead approval) and are decided serially, one
-- after the other. Each step is approve / reject / pending.
--
-- The existing pr_recommendation_approvals budget row stays and becomes a
-- PROJECTION of the chain: its approved_by/approved_at are set to the last step's
-- decider/time iff every step is approved, else NULL. Because steps are serial,
-- "all approved" == "last step approved", so the fully-approved gate
-- (recApprovedSQL / SelectQuotation) keeps working unchanged.

CREATE TABLE pr_recommendation_budget_steps (
    id                BIGSERIAL   PRIMARY KEY,
    recommendation_id BIGINT      NOT NULL REFERENCES pr_recommendations(id) ON DELETE CASCADE,
    position          INT         NOT NULL,                 -- 1 = base, 2.. = additional, sequential
    approver_name     TEXT        NOT NULL DEFAULT '',      -- empty for base (budget-unit governed)
    approver_email    TEXT        NOT NULL DEFAULT '',
    decision          TEXT        NOT NULL DEFAULT 'pending'
                      CHECK (decision IN ('pending', 'approved', 'rejected')),
    decided_by        BIGINT      REFERENCES users(id),
    decided_at        TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (recommendation_id, position)
);
CREATE INDEX idx_pr_rec_budget_steps ON pr_recommendation_budget_steps(recommendation_id, position);

-- Budget-card comments now hang off a specific step (legal/security comments leave
-- this NULL and stay keyed by approval_type).
ALTER TABLE pr_recommendation_comments
    ADD COLUMN budget_step_id BIGINT REFERENCES pr_recommendation_budget_steps(id) ON DELETE CASCADE;

-- Backfill: every recommendation that already has a budget card gets a base step,
-- carrying over its current approve state.
INSERT INTO pr_recommendation_budget_steps
    (recommendation_id, position, decision, decided_by, decided_at)
SELECT ra.recommendation_id,
       1,
       CASE WHEN ra.approved_by IS NOT NULL THEN 'approved' ELSE 'pending' END,
       ra.approved_by,
       ra.approved_at
FROM pr_recommendation_approvals ra
WHERE ra.approval_type = 'budget';

-- Re-home existing budget comments onto their recommendation's base step.
UPDATE pr_recommendation_comments c
SET budget_step_id = s.id
FROM pr_recommendation_budget_steps s
WHERE c.approval_type = 'budget'
  AND s.recommendation_id = c.recommendation_id
  AND s.position = 1;
