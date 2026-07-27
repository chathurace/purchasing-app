# Chained (serial) budget approvals for a procurement recommendation

## Context

Today a recommendation's **budget** approval is a single flat card
(`pr_recommendation_approvals` row, PK `(recommendation_id, "budget")`): a boolean
approve↔revert toggle, satisfiable by *any* qualified budget approver of the PR's
budget unit. Procurement sometimes needs **several budget sign-offs in a fixed order**
(e.g. team budget owner → finance → CFO). This adds an optional, serial chain of extra
budget approval steps on top of the existing card.

Behaviour (confirmed with the user):
- **Base card = step 1** — keeps its current budget-unit designated-approver behaviour.
- **Additional steps are named per step (email-matched)** — procurement types an approver
  name + email for each; that person (case-insensitive email match) or an admin acts on it,
  same identity model as team-lead approval. Steps run **serially**.
- **Decision states are approve / reject / pending** (real tri-state). A rejected step blocks
  the chain; "reverse" returns a decided step to pending.
- An approver may approve/reject/reverse a step **only until the next step has made a
  decision** (then that step locks).
- All steps must be **approved** before a quotation can be selected (existing gate).

## Data model — migration `backend/migrations/045_budget_approval_chain.sql`

New table (base step + additional steps all live here):

```sql
CREATE TABLE pr_recommendation_budget_steps (
  id                BIGSERIAL PRIMARY KEY,
  recommendation_id BIGINT NOT NULL REFERENCES pr_recommendations(id) ON DELETE CASCADE,
  position          INT    NOT NULL,           -- 1 = base, 2.. = additional, sequential
  approver_name     TEXT   NOT NULL DEFAULT '',-- empty for base (budget-unit governed)
  approver_email    TEXT   NOT NULL DEFAULT '',
  decision          TEXT   NOT NULL DEFAULT 'pending'
                    CHECK (decision IN ('pending','approved','rejected')),
  decided_by        BIGINT REFERENCES users(id),
  decided_at        TIMESTAMPTZ,
  created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (recommendation_id, position)
);
ALTER TABLE pr_recommendation_comments
  ADD COLUMN budget_step_id BIGINT REFERENCES pr_recommendation_budget_steps(id) ON DELETE CASCADE;
```

Data migration in the same file (dev-only DB, but keep it correct):
- For every `pr_recommendations` that has a `budget` row in `pr_recommendation_approvals`,
  insert a **base step** (`position = 1`, empty approver fields) with `decision` derived from
  that row (`approved` if `approved_by` non-null else `pending`), copying `approved_by/at` into
  `decided_by/at`.
- Point existing budget comments (`approval_type='budget'`) at that base step's id.

**Design choice — the budget approvals row becomes a projection.** The existing
`pr_recommendation_approvals` budget row stays (so required-types, the "N of M granted" badge,
`callerCanView` budget-approver visibility, budget notifications, and especially the
fully-approved gate keep working *unchanged*). Its `approved_by/at` is now **derived**: set to
the last step's `decided_by/at` iff **all** steps are approved, else NULL. Because approvals are
serial, "all approved" ⟺ "last step approved". This means **`recApprovedSQL` and `SelectQuotation`
need no changes** — a rejected/pending step leaves the budget row unapproved.

## Backend

### Repository — `backend/internal/repository/recommendations.go`
- New structs `BudgetStep` (json: `id, position, is_base, approver_name, approver_email,
  decision, decided_by, decider, decided_at, comments, can_decide, can_manage`) and add
  `BudgetSteps []BudgetStep` to `RecApproval` (populated only for the budget card).
- `GetRecommendation`: load steps `ORDER BY position` for the rec; attach per-step comments
  (query `pr_recommendation_comments WHERE budget_step_id = $1`, with docs via
  `ListOwnedDocuments(OwnerRecommendationComment, commentID)` — unchanged owner type).
- `syncBudgetApprovalFromStepsTx(tx, recID)`: recompute the budget row's `approved_by/at` from
  the steps (all approved → last step's decider/time; else NULL). Call after every step mutation.
- Step ops (mirror the existing terse repo style):
  - `AddBudgetStep(prID, name, email)` — append at `max(position)+1`; only if a budget row exists.
  - `UpdateBudgetStep(prID, stepID, name, email)` — additional steps only, only while `pending`.
  - `DeleteBudgetStep(prID, stepID)` — additional steps only, only while `pending`; resequence positions; then sync.
  - `SetBudgetStepDecision(prID, stepID, decision, deciderID)` — enforce serial gate in-tx (prior
    step approved; next step still pending), write decision + decided_by/at (NULL for `pending`),
    then `syncBudgetApprovalFromStepsTx`. `ErrInvalidState` on violation.
- `CreateRecommendation` / `UpdateRecommendation`: when budget is a required type, ensure a base
  step exists; when budget is removed, its steps cascade. `UpdateRecommendation` already resets
  approval cards — also reset every budget step to `pending` (clear `decided_by/at`), keeping the
  step rows and their named approvers.
- Extend `AddRecComment` (+ `GetRecComment` scoping) to accept an optional `budgetStepID` so budget
  comments attach to a step (`approval_type='budget'`, `budget_step_id` set).

### Handler — `backend/internal/handler/recommendations.go`, `purchase_requests.go`, `router.go`
- Capability flags, computed alongside `attachRecActionable` (`purchase_requests.go:214`):
  - `can_decide` per step = **qualified** (base → `IsBudgetApproverForPR` or admin; additional →
    case-insensitive `approver_email` match or admin) **AND actionable** (prior step approved or is
    step 1) **AND** (step `pending` → may approve/reject) **OR** (step decided **and next step
    pending** → may reverse). Reuse the email-match helper used by team-lead approval.
  - `can_manage` per step = caller has `pr.my_can_work` (procurement) and the step is additional and
    still `pending` (edit/remove; add is a rec-level affordance also gated on `my_can_work`).
- New routes under `/purchase-requests/{id}/recommendation/budget-steps` (register near
  `router.go:117-132`), each `ensureCollaborator` + `recordProcessEvent` best-effort like siblings:
  - `POST` add · `PUT /{stepID}` edit · `DELETE /{stepID}` remove — procurement (`my_can_work`).
  - `POST /{stepID}/decision` `{decision: "approve"|"reject"|"revert"}` — the step's approver/admin.
  - `POST /{stepID}/remind` — re-notify the step's approver (procurement).
  - `POST /{stepID}/comments` (+ doc upload reusing the existing comment-doc handlers) — or extend
    the existing `POST /comments` to take `budget_step_id`.
- Budget decisions now flow **exclusively** through the step endpoints; `SetRecApproval` stays for
  legal/security. (The old `POST /approvals/budget` will simply no longer be called by the UI.)
- Notifications (reuse `internal/email` mailer + `app_base_url` PR link, best-effort, logged in dev):
  notify a step's approver when it becomes the current step (prior step approved) and on
  add/remind. Base step keeps the existing `notifyBudgetApprovers` path.

### Events — `backend/internal/model/events.go`, `backend/internal/handler/events.go`
- Reuse `ProcessRecApprovalBudget` for step decisions with qualifiers `approve`/`reject`/`revert`
  (`QualifierReject`/`QualifierRevert` already exist).
- Add process action `update_budget_chain` (qualifiers add/update/remove) to `ValidProcessActions`.

## Frontend

### Types — `webapp/src/types/api.ts`
- `BudgetStep` interface matching the backend json; add `budget_steps?: BudgetStep[]` to `RecApproval`.

### API client — `webapp/src/api/recommendations.ts`
- `addBudgetStep`, `updateBudgetStep`, `deleteBudgetStep`, `setBudgetStepDecision`,
  `remindBudgetStep`, and step-scoped comment add (extend `addRecComment` with an optional stepId).
  All return the refreshed `PurchaseRequest` (comment add returns `RecComment`), matching siblings.

### Component — `webapp/src/components/RecommendationSection.tsx` (`ApprovalCard`, `isBudget` block ~1017-1130)
- Render **step 1 (base)** as today (named/designated approver + notify) plus tri-state decision
  controls gated on `step.can_decide`.
- Add an **"Add another approval step"** affordance (visible when `pr.my_can_work`) → name+email
  inputs → `addBudgetStep`.
- Render each **additional step** as a nested `SubCard` (reuse `SubCard`/`IconTile`/`TONE`): position
  label, approver name/email (inline-editable + remove for procurement while `pending`), a
  decision chip (pending / approved=emerald / rejected=red), decision controls
  (**Approve** / **Reject** when pending; **Reverse decision** when decided) gated on `can_decide`,
  a comment thread (reuse the existing comment block keyed to the step), and **Send reminder**.
  Reject uses the existing comment box / a `ConfirmDialog` to capture a reason.
- Locked steps (prior not approved) render greyed/disabled via `can_decide=false`; a rejected step
  shows the chain blocked.
- All mutations reuse the local `invalidate(["purchase-requests", prId])` refetch pattern.

## Docs
- Archive this plan to `docs/plans/06-chained-budget-approvals.md`; add as-built
  `docs/chained-budget-approvals.md`; extend the **Procurement recommendation** section of
  `CLAUDE.md` to describe the budget chain.

## Verification (end-to-end, then clean up test data per CLAUDE.md)
1. Apply migrations: `for f in backend/migrations/*.sql; do psql -h localhost -d purchasing -f "$f"; done`
   (045 applies cleanly; existing recs gain a base step). `cd backend && go run ./cmd/server`; `cd webapp && npm run dev`.
2. Create a PR → team-lead approve → assign to a procurement user → add a quotation → add a
   recommendation with **budget** required.
3. On the PR page budget card: approve the **base** step. Enable additional approvals; add step 2
   (email of a second test user) and step 3 (email of a third).
4. Confirm **serial gating**: step 2 is actionable only after base approved; step 3 only after step 2.
   Steps not yet reachable render locked.
5. Log in as the step-2 email user → confirm they (and admin) can decide step 2, others cannot.
6. **Reject** step 2 → confirm the PR's quotation cannot be selected (gate blocks) and step 3 stays locked.
7. **Reverse** step 2 back to pending while step 3 is pending; then approve step 2, decide step 3, and
   confirm step 2 **locks** (no reverse) once step 3 has decided.
8. Approve all steps → confirm the budget card reads approved and **Select quotation** succeeds.
9. Verify comments + reminder emails (logged in dev) work on an additional step. Delete all test data.
