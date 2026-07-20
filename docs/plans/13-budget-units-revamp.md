# Plan 13 — Cost center → Budget unit revamp

Requested: rename "cost center" to "budget unit" (BU) and replace the primary/secondary-owner budget
approval with **value-based approval brackets**. Dev-only; delete existing data, no migration.

## Decisions (confirmed with the user)

1. **Full rename** — DB tables/columns, API routes, Go/TS identifiers, files and UI all move from
   cost center → budget unit.
2. **Approver pool** — a bracket lists one or more approvers; **any** qualified approver may approve
   and **all** are notified (the "use the first" rule was dropped).
3. **Bracket matching** — ordered list, **first match wins**; overlaps allowed, no strict validation.
4. **Currency** — a value only matches when its currency equals the BU currency; otherwise fall back
   to the highest bracket. No conversion.

Derived rules: a missing estimated value → highest bracket. The requester's estimated value drives a
creation-phase **preview**; the recommendation's estimated value drives the **actual** approval.

## Shape

- Migration `041`: rename `cost_centers`→`budget_units`, drop the owner model, rename the PR /
  invoice-allocation FK columns, drop legacy free-text `cost_center`, add `budget_unit_brackets` +
  `budget_unit_bracket_approvers`, add SQL function `resolve_budget_bracket(bu_id, value, currency)`.
  `TRUNCATE budget_units CASCADE` clears dev data.
- Backend: rewrite the repository BU model + bracket CRUD; `IsBudgetOwnerForPR` →
  `IsBudgetApproverForPR` + the two inlined list predicates, all via `resolve_budget_bracket`;
  `BudgetApproversForPR` / `BudgetApproversForValue`; handler `budget_units.go` with a bracket input
  and an `/approvers` preview endpoint; notify qualified approvers on recommendation create/update
  and via a budget "remind" endpoint; rename audit constants.
- Frontend: full rename of types/api/hooks/pages/nav; `BudgetUnitFields` bracket editor; requisition
  form BU picker + derived-approver preview (removing the free-text approver + duplicate coding cost
  center); recommendation budget card shows the resolved approvers + a Notify button.
- Docs: this plan, `docs/budget-units.md` (as-built), and the CLAUDE.md status entry.

As-built detail: see [budget-units.md](../budget-units.md).
