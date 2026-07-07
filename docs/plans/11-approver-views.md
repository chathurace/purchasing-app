# Approver dual-view UX + role-based tab visibility

## Context

Budget / legal / security approvers are also ordinary staff who submit their own purchase
requests. Today the **Requests** list conflates two jobs into one screen: for a non-finance user it
returns a *union* of "PRs I submitted" **and** "PRs awaiting my approval" (see the WHERE clause in
`ListPurchaseRequests`, [repository.go:523](backend/internal/repository/repository.go#L523)). There
is no way to separate the two, no dedicated place to triage approvals, and approvers can't reach the
Quotations/Contracts context for the PRs they're deciding on (quotations 403 non-finance;
contracts show *all* rows, unscoped).

This change gives approvers **two clearly separated jobs** and aligns tab visibility with role:

- **Requests tab** — only PRs the logged-in user submitted (their own requisitions).
- **Approvals tab** (new) — PRs awaiting *their* decision, across **both** approval mechanisms:
  the `pr_approvals` named approvers **and** the budget/legal/security recommendation cards.
  Two checkboxes filter by the caller's own state; **Pending** is checked by default, **Approved**
  off.
- Approvers also get **read-only, PR-scoped** Quotations and Contracts tabs — limited to the
  quotations/contracts attached to PRs they are an approver on.

### Final tab-visibility matrix (effective)

| Tab | staff | approver | finance | finance_admin | admin |
|---|---|---|---|---|---|
| Requests | ✅ (own) | ✅ (own) | ✅ (all) | ✅ (all) | ✅ (all) |
| Approvals | — | ✅ | ✅ | ✅ | ✅ |
| Quotations | — | ✅ (scoped, RO) | ✅ | ✅ | ✅ |
| Contracts | — | ✅ (scoped, RO) | ✅ | ✅ | ✅ |
| GRNs / Invoices | — | — | ✅ | ✅ | ✅ |
| Vendors / Cost centers | — | — | — | ✅ | ✅ |
| Settings | — | — | — | ✅ | ✅ |
| Users | — | — | — | — | ✅ |

"approver" = holds `legal`/`security` role, **or** is a named approver on any PR, **or** owns a cost
center that makes them a budget-card actor. Vendors/Cost centers/Settings keep their **current**
`finance_admin`/`admin` gating (the matrix's "finance = all except users/settings" is read as the
finance *group*, not a broadening of plain `finance`). Requests for finance/admin stays "all PRs"
(their procurement queue); everyone else sees only their own.

---

## Backend (`backend/internal/…`)

### 1. Factor the "approvable PRs" predicate (`repository/repository.go`)

The recommendation/approver EXISTS branches already live inside `ListPurchaseRequests`
([repository.go:523](backend/internal/repository/repository.go#L523)). Extract the approval-assignment
portion into a reusable SQL fragment constant (params: `$1` userID, `$2` hasLegal, `$3` hasSecurity):

```
-- caller is a named approver, OR a legal/security card actor, OR the budget owner
EXISTS (SELECT 1 FROM pr_approvals a WHERE a.purchase_request_id = pr.id AND a.approver_id = $1)
OR EXISTS (SELECT 1 FROM pr_recommendation_approvals ra
             JOIN pr_recommendations rec ON rec.id = ra.recommendation_id
            WHERE rec.purchase_request_id = pr.id AND (
                  (ra.approval_type='legal'    AND $2)
               OR (ra.approval_type='security' AND $3)
               OR (ra.approval_type='budget'   AND pr.cost_center_id IS NOT NULL AND EXISTS (
                     SELECT 1 FROM cost_centers cc WHERE cc.id = pr.cost_center_id AND (
                        cc.primary_owner_id = $1
                     OR EXISTS (SELECT 1 FROM cost_center_secondary_owners s
                                 WHERE s.cost_center_id = cc.id AND s.user_id = $1))))))
```

Reused by items 2–6 below so the definition of "an approver" stays in one place.

### 2. PR list — `scope` query param (`handler/purchase_requests.go` `List` + repo)

Add `?scope=mine|approvals` (default = current behavior, keeps finance's all-PRs queue intact):

- `scope=mine` → `WHERE pr.requester_id = $1` (for **any** role).
- `scope=approvals` → `WHERE (<approvable predicate>) AND pr.requester_id <> $1` (a caller never
  approves their own PR).
- no scope → unchanged (`seesAll` union), so nothing else breaks.

`List` reads the param and passes a scope enum into `ListPurchaseRequests` (extend its signature).

### 3. PR summary — `my_approval_state` (`repository.go` list projection + `model`)

The existing row already exposes `my_approval_status` from `pr_approvals` only
([repository.go:530](backend/internal/repository/repository.go#L530)). Add a unified
`my_approval_state ∈ {pending, approved, rejected}` computed with scalar subqueries over **both**
systems:

- `pending` if I have a `pr_approvals` row still `pending`, **or** a legal/security/budget card I can
  act on that is not yet `approved`;
- else `rejected` if I rejected a `pr_approvals` row (cards have no reject);
- else `approved`.

Drives the Approvals-tab checkboxes. Keep `my_approval_status` as-is for the Requests "Approvals"
column.

### 4. `/me` — `is_approver` capability (`handler` for `/api/v1/me` + `model.Me`)

Roles alone can't tell the nav whether to show the Approvals/Quotations/Contracts tabs (budget
owners and named approvers have no role). Add `is_approver bool` to the `/me` payload, computed as
`EXISTS (SELECT 1 FROM purchase_requests pr WHERE <approvable predicate> AND pr.requester_id <> me)`.
One cheap query; guarantees the tab is non-empty when shown.

### 5. Quotations list + detail — scoped read for approvers (`handler/quotations.go`)

`quotations.purchase_request_id` exists (migration 021). Relax the blanket `HasFinanceAccess` 403 on
**`List`** and **`Get`** only (mutations — Create/Select/Delete — stay finance-only):

- finance → all rows (current `ListQuotations(ctx, nil)`).
- else → new `ListQuotationsForApprover(ctx, userID, hasLegal, hasSecurity)` =
  `… WHERE q.purchase_request_id IN (SELECT id FROM purchase_requests pr WHERE <approvable predicate>)`.
- `Get`: allow if finance **or** new `IsApproverForPR(ctx, q.purchase_request_id, …)` (per-PR form
  of the predicate); else 403.

### 6. Contracts list + detail — scope to involved PRs (`handler/contracts.go`)

`contracts.purchase_request_id` is `NOT NULL` with index `idx_contracts_pr` (migration 009). Replace
the `CanViewContract`-returns-all behavior:

- `List`: finance → all; else → `ListContractsForApprover(…)` scoped by the same
  `purchase_request_id IN (<approvable PRs>)` subquery. (Legal/security are a subset of the predicate,
  so they keep access, now correctly narrowed to their PRs; budget-owner approvers gain it.)
- `Get`/`loadViewable` ([contracts.go:281](backend/internal/handler/contracts.go#L281)): allow if
  finance/legal/security **or** `IsApproverForPR(ctx, c.purchase_request_id, …)`. Mutating `load`
  path and `CreateFromQuotation` stay `HasFinanceAccess`.

New repo helper `IsApproverForPR` (single-PR predicate) is reused by items 5 and 6.

---

## Frontend (`frontend/src`)

### 7. Types + capability hook

- `types/api.ts`: add `is_approver: boolean` to `Me`; add `my_approval_state: "pending" | "approved"
  | "rejected"` to the PR summary type.
- `hooks/useIsApprover.ts`: derive from `useMe()` (mirrors `useFinanceAccess.ts`).

### 8. Nav gating (`components/Layout.tsx`)

Add the **Approvals** `NavLink` (route `/approvals`) after Requests, and update gates
([Layout.tsx](frontend/src/components/Layout.tsx#L26)):

- Approvals: `finance || isApprover`
- Quotations: `finance || isApprover` (was `finance`)
- Contracts: `finance || isApprover` (replaces `finance || reviewer`; `reviewer` ⊂ `isApprover`)
- GRNs/Invoices/Vendors/Cost centers/Users/Settings: **unchanged**.

### 9. Requests page — own PRs only for non-finance

- `api/purchaseRequests.ts`: `listPurchaseRequests(scope?: "mine" | "approvals")` appends `?scope=`.
- `hooks/usePurchaseRequests.ts`: pass `scope="mine"` when `!useFinanceAccess()` (finance keeps the
  full queue). Key the query by scope.

### 10. New Approvals page + route

- `pages/ApprovalsListPage.tsx` — reuses the Requests table layout. Two checkboxes: **Pending**
  (default on) and **Approved** (off), filtering client-side on `my_approval_state`
  (`approved` bucket also surfaces `rejected` rows with a distinct red badge, since cards have no
  reject and a rejected named-approval is "acted"). Empty-state copy when no assignments.
- `api/purchaseRequests.ts`: `listApprovalRequests = () => listPurchaseRequests("approvals")`.
- `hooks/useApprovalRequests.ts`: `useQuery(["purchase-requests","approvals"], …, {refetchInterval:
  5000})` (same polling pattern as the existing list hooks).
- `App.tsx`: add `<Route path="/approvals" element={<ApprovalsListPage/>} />` inside the
  `RequireAuth`/`Layout` group.

### 11. Quotations / Contracts pages

No structural change — once the backend opens scoped read, the existing list/detail pages render the
scoped rows. Verify the detail pages hide finance-only mutation controls for non-finance viewers
(create/select/delete quotation, contract draft/sign actions) — gate those UI blocks on
`useFinanceAccess()` if not already.

---

## Verification

1. **Migrations**: none required (all columns already exist). Run backend + frontend per
   `CLAUDE.md` quick start.
2. **Seed roles/data**: as admin at `/users`, create a `legal` user, a `security` user, and set a
   cost-center owner (budget approver). Have a `staff` user submit a PR naming one of them as a
   named approver; as finance, add a quotation + a recommendation with legal/security/budget cards.
3. **Approver dual view**: log in as the legal user →
   - **Requests** tab shows only PRs *they* submitted (not the ones awaiting them).
   - **Approvals** tab lists the PR(s) awaiting them; only **Pending** checked by default; ticking
     **Approved** reveals ones they've approved. Approve a card → it moves buckets on next poll.
   - **Quotations**/**Contracts** tabs show only rows tied to their involved PR(s), read-only (no
     create/select/sign controls); a quotation/contract on an unrelated PR is not listed and its
     detail URL returns 403.
4. **Role gating**: plain `staff` sees only Requests (own). `finance` sees Requests(all), Approvals,
   Quotations, Contracts, GRNs, Invoices — but not Vendors/Cost centers/Settings/Users. `admin`
   sees all. Confirm direct-URL access to a forbidden list still 403s server-side.
5. **Regression**: finance Requests still shows all PRs; existing recommendation-card approve/reject
   and named-approval flows unchanged.
6. Run `cd backend && go build ./... && go vet ./...` and `cd frontend && npm run build`.
