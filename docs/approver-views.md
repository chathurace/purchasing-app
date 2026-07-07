# Approver views & role-based tabs (as-built)

Plan archive: `docs/plans/11-approver-views.md`.

Budget / legal / security approvers are also ordinary staff. This phase separates their two jobs
and aligns nav-tab visibility with role.

## Two jobs, two tabs

- **Requests** — a user's own submissions. Staff/approvers get `scope=mine`; finance/admin keep the
  full procurement queue (no scope).
- **Approvals** (new, `/approvals`) — PRs awaiting the caller's decision across **both** approval
  systems: the named `pr_approvals` approvers **and** the budget/legal/security recommendation
  cards. Two checkboxes (Pending on by default, Approved off) filter client-side on
  `my_approval_state`.

## Tab visibility

| Tab | staff | approver | finance | finance_admin | admin |
|---|---|---|---|---|---|
| Requests | ✅ own | ✅ own | ✅ all | ✅ all | ✅ all |
| Approvals | — | ✅ | ✅ | ✅ | ✅ |
| Quotations | — | ✅ RO, scoped | ✅ | ✅ | ✅ |
| Contracts | — | ✅ RO, scoped | ✅ | ✅ | ✅ |
| GRNs / Invoices | — | — | ✅ | ✅ | ✅ |
| Vendors / Cost centers / Settings | — | — | — | ✅ | ✅ |
| Users | — | — | — | — | ✅ |

Approvers get **read-only**, PR-scoped Quotations and Contracts — only rows attached to PRs they
approve. Vendors/Cost centers/Settings keep their prior `finance_admin`/`admin` gating.

## Backend

- **Shared predicate** — `approvablePredicate` in `internal/repository/repository.go` is the single
  definition of "caller is an approver on PR `pr`" (named approver, legal/security card actor by
  role, or budget owner of the PR's cost center). Binds `$1` caller / `$2` hasLegal / `$3`
  hasSecurity. Reused by every query below; `myApprovalStateExpr` is the matching state CASE.
- **PR list** — `ListPurchaseRequests(..., scope)` takes `PRScopeMine` / `PRScopeApprovals` /
  `PRScopeDefault`; handler maps `?scope=mine|approvals`. Every row now also carries
  `my_approval_state` (pending | approved | rejected) unified across both systems.
- **`/me`** — adds `is_approver` (via `HasApprovableWork`): true when the caller approves any PR
  they didn't submit. Drives the approver-only nav tabs, since budget owners / named approvers hold
  no distinguishing role.
- **Quotations** — `List`/`Get`/`DownloadDocument` allow finance **or** an approver on the PR
  (`ListQuotationsForApprover`, `IsApproverForPR`). Create/Update/Select/Delete stay finance-only.
- **Contracts** — `List`/`Get`/`DownloadDocument` allow finance **or** an approver on the PR
  (`ListContractsForApprover`), scoped to their PRs (replacing the old view-all `CanViewContract`,
  now removed). Mutations and `CreateFromQuotation` stay finance-only.

## Frontend

- `Me.is_approver` + `useIsApprover`; nav `canApprove = finance || isApprover` gates
  Approvals/Quotations/Contracts (`components/Layout.tsx`).
- `usePurchaseRequests` sends `scope=mine` for non-finance; `useApprovalRequests` sends
  `scope=approvals`. New `pages/ApprovalsListPage.tsx` at `/approvals`.
- Quotation/contract **detail** pages gate mutation controls on `useFinanceAccess()` so approvers see
  read-only; the related-case components (`RelatedDocuments`, `CaseSections`, `ChainStepper`,
  `ContractFulfillment`) already fetch only for finance and no-op otherwise.
