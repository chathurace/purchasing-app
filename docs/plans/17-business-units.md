# Replace Budget Units with Business Units

## Context

The current **Budget Units** feature is over-engineered for how it's used: each unit carries a
code, a budget, a currency, a *default approver*, and an ordered list of value/currency **brackets**,
each bracket with its own approver set — resolved at approval time by the SQL function
`resolve_budget_approvers(bu_id, value, currency)`. In practice the requisition form doesn't even
collect a unit; it captures the budget approver as free text, and the recommendation budget card
falls back to matching that free-text email.

We are replacing this with a much simpler **Business Unit**: just a **name**, an **optional
description**, and a **flat list of approvers** (plus an active flag). On the requisition form the
requester picks a business unit, and the **Budget approver** field becomes a dropdown populated from
that unit's approvers. Budget-approval authority everywhere collapses to the existing
case-insensitive **email match** against `purchase_requests.budget_approver_email` (the person picked
from the dropdown), so brackets, currencies, per-value resolution, and the `resolve_budget_approvers`
function all go away. The serial **budget approval chain** (migration 045) is kept unchanged — its
base step is already governed by the budget approver via `IsBudgetApproverForPR`, which now becomes a
pure email match.

This is a **full rename** (tables, columns, routes, Go identifiers, UI), consistent with the prior
`cost_centers → budget_units` rename (migrations 041/042). Dev-only DB, no data migration.

## Backend

### Migration `backend/migrations/046_business_units.sql`
Follow the style of `041_budget_units.sql` / `042_...`:
- `DROP FUNCTION IF EXISTS resolve_budget_approvers(BIGINT, NUMERIC, TEXT);`
- `DROP TABLE budget_unit_bracket_approvers; DROP TABLE budget_unit_brackets;`
- `ALTER TABLE budget_units RENAME TO business_units;` + rename indexes (`idx_budget_units_*` → `idx_business_units_*`).
- Drop now-unused columns: `code`, `budget`, `currency`, `default_approver_id`.
- New `business_unit_approvers` (`business_unit_id BIGINT NOT NULL REFERENCES business_units(id) ON DELETE CASCADE`, `user_id BIGINT NOT NULL REFERENCES users(id)`, `position INT NOT NULL DEFAULT 0`, PK `(business_unit_id, user_id)`) — mirrors the old `budget_unit_bracket_approvers` shape.
- `ALTER TABLE purchase_requests RENAME COLUMN budget_unit_id TO business_unit_id;`
- `ALTER TABLE invoice_cost_allocations RENAME COLUMN budget_unit_id TO business_unit_id;` + rename its index.
- Dev reset (as 041 did): `TRUNCATE business_units CASCADE;` and null out `purchase_requests.business_unit_id` — the bracket→flat-approver change has no data migration.

### Repository — `backend/internal/repository/procurement.go`
Replace the whole budget-unit block (types + funcs, ~L190–510):
- `BusinessUnit` = `{ID, Name, Description, IsActive, Approvers []*UserSummary, CreatedAt, UpdatedAt}`. Drop `BudgetUnitBracket`, `Budget`, `Currency`, `Code`, `DefaultApprover(ID)`.
- `BusinessUnitInput` = `{Name, Description, IsActive, ApproverIDs []int64}`.
- `BusinessUnitSummary` = `{ID, Name}` (drop `Code`, `Currency`).
- Replace `populateBrackets`/`listBracketApprovers`/`populateDefaultApprover` with one `populateApprovers(ctx, *BusinessUnit)` reading `business_unit_approvers` (join users, `ORDER BY position, user_id`).
- Replace `replaceBrackets` with `replaceApprovers(ctx, tx, businessUnitID, approverIDs)`.
- `ListBusinessUnits`, `ListActiveBusinessUnits` (summary `id, name`), `GetBusinessUnit`, `CreateBusinessUnit`, `UpdateBusinessUnit`, `GetBusinessUnitUsage`, `GetBusinessUnitInvoiceSummary` — same shapes, simplified SQL/columns.
- Replace `BudgetApproversForValue` with `BusinessUnitApprovers(ctx, id)` → the flat approver list (for the form dropdown).
- `GetContract` (~L1076): join `business_units bu ON bu.id = pr.business_unit_id`; `Contract.BudgetUnit`→`BusinessUnit` (`BusinessUnitSummary`).

### Repository — `backend/internal/repository/recommendations.go`
- `IsBudgetApproverForPR` (L1179): drop the `resolve_budget_approvers` branch; qualify purely on the email match against `pr.budget_approver_email` (the existing fallback becomes the only path).
- `BudgetApproversForPR` (L1200): drop the `resolve_budget_approvers` UNION arm; keep the email-match arm.
- Delete `BudgetApproversForValue` (L1226). Leave `rec.estimated_value`/`currency` fields intact (display only; no longer drive resolution).

### Repository — `backend/internal/repository/repository.go`
- `budgetEmailMatch` (L659): unchanged.
- `approvablePredicate` (L662) budget branch: replace `(pr.budget_unit_id IS NOT NULL AND EXISTS(resolve_budget_approvers...)) OR (pr.budget_unit_id IS NULL AND budgetEmailMatch)` with just `budgetEmailMatch`. Keep the budget-step branch.
- `myApprovalStateExpr` (L694): same simplification (two spots).
- Rename `PurchaseRequest.BudgetUnitID`→`BusinessUnitID` (json `business_unit_id`), same on `PurchaseRequestInput`; update create/update/select SQL to `business_unit_id`.

### Other backend
- Rename `handler/budget_units.go` → `business_units.go`: `BusinessUnitsHandler`; input struct = `{Name, Description, IsActive, ApproverIDs []int64}`; `toRepo` validates name + ≥1 approver (dedupe ids, keep pattern). Drop `bracketInput`. `Approvers` handler → `GET /business-units/{id}/approvers` returning the flat list (no `value`/`currency` params) via `Repo.BusinessUnitApprovers`.
- `middleware/auth.go`: `HasBudgetUnitAdmin`→`HasBusinessUnitAdmin`; update refs in `handler/config_options.go`.
- `model/events.go`: `AuditCreateBudgetUnit`/`AuditUpdateBudgetUnit` → `create_business_unit`/`update_business_unit` (const names + values); `EntityBudgetUnit`→`business_unit`.
- `router.go`: `/budget-units*` → `/business-units*`, handler var rename, drop query params on `/approvers`.
- `handler/purchase_requests.go`, `handler/recommendations.go`, `handler/invoices.go`, `repository/fulfillment.go`: rename `BudgetUnitID`→`BusinessUnitID` (PR + `CostAllocation`) and update comments. `SetBudgetApprover` and the budget-step handlers are unchanged (still email-based).
- Update `backend/internal/repository/procurement_integration_test.go` (already modified in the tree) to the new types/table.

## Frontend (`webapp/`)

- Rename `api/budgetUnits.ts`→`api/businessUnits.ts`, `hooks/useBudgetUnits.ts`→`useBusinessUnits.ts`, `hooks/useCanManageBudgetUnits.ts`→`useCanManageBusinessUnits.ts`. `getBusinessUnitApprovers(id)` / `useBusinessUnitApprovers(id)` take just an id.
- `types/api.ts`: `BusinessUnit {id, name, description, is_active, approvers: UserSummary[], created_at, updated_at}`, `BusinessUnitInput {name, description, is_active, approver_ids: number[]}`, `BusinessUnitSummary {id, name}`, `BusinessUnitUsage`, invoice-summary types kept. Drop bracket types, `budget`/`currency`/`code`/`default_approver*`, `budgetUnitRef`. Add `emptyBusinessUnit`. PR type `business_unit_id`/`business_unit?`; invoice alloc `business_unit_id`.
- Pages: `BudgetUnitListPage`→`BusinessUnitListPage` (drop bracket-count/budget columns), `BudgetUnitDetailPage`→`BusinessUnitDetailPage` (drop `BracketView`/budget/currency/default-approver; show the flat approver list; keep usage + invoices panels). `components/BudgetUnitFields.tsx`→`BusinessUnitFields.tsx`: name, description, active, and a single `<ApproverPicker>` for the approver list (reuse the existing component; drop `BracketRow` and reorder logic).
- `App.tsx` routes `/business-units`, `/business-units/:id`; `Layout.tsx` nav label "Business units" + `/business-units`, gated by `useCanManageBusinessUnits`; `SettingsPage.tsx` uses the renamed hook.
- **`RequisitionForm.tsx`** (Budget approval section, ~L670–678): add a **Business unit** dropdown fed by `lookupBusinessUnits()` (sets `value.business_unit_id`); replace the two free-text approver inputs with a **Budget approver** dropdown populated from `useBusinessUnitApprovers(business_unit_id)` — selecting an approver sets `budget_approver_name`/`budget_approver_email`. Remove the redundant free-text "Business unit (BU)" field from the *Budget details* grid (L656–658, the `d.business_unit` coding label) to avoid two "Business unit" fields. Update validation (L130–132) and empty-form defaults.
- **`PurchaseRequestDetailPage.tsx`** `ReadOnlyView`: use `useBusinessUnitLookup` to show the **Business unit** name; show **Budget approver** (from `budget_approver_*`). Remove the "Designated approver" row and the `useBudgetUnitApprovers(value,currency)` call. Thread `business_unit_id` through the edit state.
- **`RecommendationSection.tsx`** budget card: replace `useBudgetUnitApprovers(value,currency)` + `designated`/`approverMismatch`/"Use designated approver" with a **Budget approver dropdown** sourced from `useBusinessUnitApprovers(businessUnitId)`; the selection drives `setBudgetApprover(...)`. Keep `remindBudgetApprovers`, `BudgetChain`/`BudgetStepCard`, and step comments unchanged (base step still email-governed).

## Docs
- Add `docs/plans/17-business-units.md` (archive this plan) and `docs/business-units.md` (rename/rewrite `docs/budget-units.md`).
- Update `CLAUDE.md`: rewrite the **Budget units** bullet as **Business units**, and adjust the budget-approver references in the Phase 1 / recommendation / chained-budget-approvals bullets.
- Update `docs/process-events.md` for the renamed audit actions.

## Verification
1. `for f in backend/migrations/0*.sql; do psql -h localhost -d purchasing -f "$f"; done` on a fresh `purchasing` DB (or just apply 046) — confirm no errors.
2. `cd backend && go build ./... && go vet ./... && go test ./internal/repository/...`.
3. `cd webapp && npm run build` (typecheck) — expect zero references to removed budget-unit symbols.
4. Run the app (`go run ./cmd/server` + `npm run dev`) and drive end-to-end (use the `/verify` skill): as procurement_admin create a **Business unit** with 2 approvers; as staff create a PR, pick that unit, confirm the **Budget approver** dropdown lists exactly those approvers; submit; as the picked approver confirm the PR appears under Approvals and the recommendation **budget card + chain base step** can be decided. Then clean up the test data.
