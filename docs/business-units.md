# Business units (formerly budget units / cost centers)

**Business units** replace the earlier **budget units** (which had replaced **cost centers**). The
value-based approval-bracket model is gone: a business unit is now simply a **name**, an optional
**description**, and a **flat list of approvers**. The budget approver for a purchase request is the
one person the requester picks from the selected unit's approvers, matched thereafter by email.

Introduced by migration `046`, which renames `budget_units` → **`business_units`**, drops the
brackets / currency / value-resolution machinery, and — **dev-only, no data migration** — truncates
existing data (`TRUNCATE business_units CASCADE`, which also clears purchase-request data).

## Data model

- **`business_units`** (was `budget_units`): `id`, `name`, `description`, `is_active`, `created_by`,
  `created_at`, `updated_at`. The `code`, `budget`, `currency`, and `default_approver_id` columns are
  dropped.
- **`business_unit_approvers`**: `business_unit_id` (FK, `ON DELETE CASCADE`), `user_id` (FK →
  `users`), `position` (order), PK `(business_unit_id, user_id)`. One or more per unit; **all** are
  qualified budget approvers the requester may pick from.
- Dropped: `budget_unit_brackets`, `budget_unit_bracket_approvers`, and the SQL function
  `resolve_budget_approvers`.
- FK renames: `purchase_requests.budget_unit_id` → **`business_unit_id`**;
  `invoice_cost_allocations.budget_unit_id` → **`business_unit_id`**.

## Choosing and resolving the budget approver

There is no value/currency resolution anymore. On the **requisition form** the requester:

1. picks a **business unit** (dropdown of active units, `GET /api/v1/business-units/lookup`), then
2. picks a **budget approver** from that unit's approvers
   (`GET /api/v1/business-units/{id}/approvers`).

The chosen approver's name/email are stored in the free-text
`purchase_requests.budget_approver_name/email`. Budget-approval **authority** everywhere is a
case-insensitive **email match** against `budget_approver_email`:

- `Repository.IsBudgetApproverForPR` — the budget card's approve/comment gate (and the base step of
  the budget approval chain).
- `Repository.BudgetApproversForPR` — the set to notify.
- `budgetEmailMatch` — the SQL fragment folded into `approvablePredicate` and `myApprovalStateExpr`
  in `repository.go` (list/dashboard visibility). Binds `$4` = caller email.

Both endpoints (`/lookup` and `/{id}/approvers`) are open to any authenticated user, since they feed
the requisition-form dropdowns.

## Backend

- Repo: `repository/procurement.go` (`BusinessUnit`, `BusinessUnitInput`, `BusinessUnitSummary`, CRUD,
  `BusinessUnitApprovers`, `BusinessUnitInvoiceSummary`), `repository/recommendations.go`
  (`IsBudgetApproverForPR`, `BudgetApproversForPR`).
- Handler: `handler/business_units.go` (`BusinessUnitsHandler`) — `/api/v1/business-units` CRUD +
  `/lookup` + `/{id}/approvers` + `/{id}/usage` + `/{id}/invoices`. Validation: name required and at
  least one approver.
- Notification: `PurchaseRequestsHandler.notifyBudgetApprovers` emails the qualified approver(s);
  called on recommendation create/update and by `RemindBudgetApprovers`
  (`POST .../recommendation/approvals/budget/remind`, procurement).
- Access: `middleware.HasBusinessUnitAdmin` (procurement_admin/admin) — also gates config options. The
  budget card keeps the admin bypass.
- Audit: `model.AuditCreateBusinessUnit` / `AuditUpdateBusinessUnit`, entity `model.EntityBusinessUnit`.

## Frontend

- Types: `BusinessUnit`, `BusinessUnitInput`, `BusinessUnitSummary`, `BusinessUnitInvoiceSummary` in
  `types/api.ts`; `emptyBusinessUnit`.
- API/hooks: `api/businessUnits.ts`, `hooks/useBusinessUnits.ts` (incl. `useBusinessUnitApprovers`,
  `useBusinessUnitLookup`), `hooks/useCanManageBusinessUnits.ts`.
- UI: `pages/BusinessUnitListPage.tsx`, `pages/BusinessUnitDetailPage.tsx`,
  `components/BusinessUnitFields.tsx` (name + description + a single approver picker). Nav link
  "Business units" at `/business-units`.
- Requisition form (`components/RequisitionForm.tsx`, step "Vendor & budget"): a **Business unit**
  dropdown and a **Budget approver** dropdown populated from the selected unit's approvers; the
  selection fills `budget_approver_name/email`. Both are required (validated for IT / NON-IT).
- Recommendation budget card (`components/RecommendationSection.tsx`): shows the PR's named budget
  **Approver**; procurement changes it inline via a dropdown of the unit's approvers over
  `PUT /api/v1/purchase-requests/{id}/budget-approver` (`SetBudgetApprover`), with a **Notify
  approver** button. The serial budget approval chain's base step is governed by this named approver.
