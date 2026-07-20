# Budget units (formerly cost centers)

Budget units replace the old **cost centers**. The rename went end-to-end (DB, API routes, Go, TS,
UI) and the ownership model changed: instead of a primary + secondary owner, a budget unit now has
value-based **approval brackets** that resolve the budget approver dynamically.

Introduced by migration `041`; refined by `042` (a **default approver** on the unit + a **currency**
on each bracket). **Dev-only — there is no data migration.** Migration `041` renames the
tables/columns in place and then `TRUNCATE budget_units CASCADE`, which clears all existing
purchase-request data (cost-center rows had no brackets and the references would be dangling).

## Data model

- **`budget_units`** (was `cost_centers`): `id`, `code`, `name`, `description`, `budget`,
  `currency` (the budget figure's currency, informational), `is_active`, **`default_approver_id`**
  (FK → `users`; the catch-all approver), `created_by`, `created_at`, `updated_at`. The
  `primary_owner_id` column and the `cost_center_secondary_owners` table are dropped.
- **`budget_unit_brackets`**: `id`, `budget_unit_id` (FK, `ON DELETE CASCADE`), `position` (order —
  first match wins), **`currency`** (the bracket matches only this currency), `min_value`
  (inclusive), `max_value` (inclusive; **NULL = unbounded**). **Brackets are optional** — a unit may
  have **zero** brackets, in which case every request resolves to the default approver.
- **`budget_unit_bracket_approvers`**: `bracket_id` (FK, cascade), `user_id`, `position`. One or
  more per bracket; **all** are qualified approvers.
- FK renames: `purchase_requests.cost_center_id` → **`budget_unit_id`** (legacy free-text
  `purchase_requests.cost_center` dropped); `invoice_cost_allocations.cost_center_id` →
  **`budget_unit_id`**.

## Resolving the budget approver

SQL function **`resolve_budget_approvers(p_bu_id, p_value, p_currency) → SETOF user_id`** centralizes
the rule (used by every call site so the logic lives in one place — it replaced the earlier
`resolve_budget_bracket`, which returned a bracket id):

1. If a value is supplied and `p_currency` is non-empty, find the first bracket (by `position, id`)
   whose **own `currency`** equals `p_currency` and whose `[min_value, max_value]` contains the
   value; return **that bracket's approvers**.
2. Otherwise — no value, **no brackets at all**, no matching-currency bracket, or out of every
   range — return the unit's **`default_approver_id`** (if set).

**Any** returned approver may approve the budget card; **all** are notified. There is no "first
approver" preference. A budget unit always has a default approver (required on create/update), so
the resolution never comes back empty for a configured unit.

Two independent resolution points:

- **Creation-phase preview** — the *requester's* `purchase_requests.estimated_value` (+ currency).
  Endpoint `GET /api/v1/budget-units/{id}/approvers?value=&currency=` (any authenticated user; a
  blank value → default approver). Shown on the requisition form (step 3, "Budget approval") and on
  the PR detail page. Informational only — nothing is persisted.
- **Actual budget approval** — the *recommendation's* `pr_recommendations.estimated_value` +
  `currency`. Backend derivation: `Repository.IsBudgetApproverForPR` (the budget card's
  approve/comment gate) and the two list/dashboard predicates in `repository.go`
  (`approvablePredicate`, `myApprovalStateExpr`), all calling `resolve_budget_approvers`.
  `Repository.BudgetApproversForPR` returns the full set for notification.

Currency handling (v1): each bracket carries its own currency and matches only that currency (no
conversion); a value in a currency no bracket declares — or with no bracket range hit — falls back to
the default approver.

## Backend

- Repo: `repository/procurement.go` (`BudgetUnit`, `BudgetUnitBracket`, `BudgetUnitInput`, CRUD,
  `BudgetUnitInvoiceSummary`), `repository/recommendations.go` (`IsBudgetApproverForPR`,
  `BudgetApproversForPR`, `BudgetApproversForValue`).
- Handler: `handler/budget_units.go` (`BudgetUnitsHandler`) — `/api/v1/budget-units` CRUD + `/lookup`
  + `/{id}/approvers` + `/{id}/usage` + `/{id}/invoices`. Validation: name + default approver
  required; brackets are optional, but each supplied bracket needs a currency + ≥1 approver and
  `max ≥ min` when bounded.
- Notification: `PurchaseRequestsHandler.notifyBudgetApprovers` emails every qualified approver;
  called on recommendation create/update and by `RemindBudgetApprovers`
  (`POST .../recommendation/approvals/budget/remind`, procurement).
- Access: `middleware.HasBudgetUnitAdmin` (procurement_admin/admin) — also gates config options. The
  budget card keeps the admin bypass.
- Audit: `model.AuditCreateBudgetUnit` / `AuditUpdateBudgetUnit`, entity `model.EntityBudgetUnit`.

## Frontend

- Types: `BudgetUnit`, `BudgetUnitBracket(Input)`, `BudgetUnitInput`, `BudgetUnitSummary`,
  `BudgetUnitInvoiceSummary` in `types/api.ts`; `emptyBudgetUnit`, `budgetUnitRef`.
- API/hooks: `api/budgetUnits.ts`, `hooks/useBudgetUnits.ts` (incl. `useBudgetUnitApprovers`),
  `hooks/useCanManageBudgetUnits.ts`.
- UI: `pages/BudgetUnitListPage.tsx`, `pages/BudgetUnitDetailPage.tsx`,
  `components/BudgetUnitFields.tsx` (default-approver picker + the bracket editor: per-bracket
  currency + min/max + "no upper limit" + approver picker + reorder). Nav link "Budget units" at
  `/budget-units`.
- The UI calls the resolved approver(s) the **designated approver(s)**.
- Requisition form (`components/RequisitionForm.tsx`): step 1 picks the budget unit; step 3 has an
  editable **budget approver** (name/email, stored in `purchase_requests.budget_approver_name/email`)
  with a **↻ Use designated** button that copies the designated approver(s) into it, plus the
  designated approver(s) shown as a reference. If the entered approver differs from the designated
  one, the **Review** step flags it (non-blocking) — Save just submits.
- Recommendation budget card (`components/RecommendationSection.tsx`): shows the PR's named
  **Approver** (procurement can edit it inline via the ✎ icon, or click **↻ Use designated approver**
  when it differs) over `PUT /api/v1/purchase-requests/{id}/budget-approver`
  (`SetBudgetApprover`), and the **Designated** approver(s) with a ↻ re-check and a **Notify
  approvers** button.
