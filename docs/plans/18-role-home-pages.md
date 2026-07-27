# Role-based home pages (issue #2482)

## Context

Today the app has no landing page: `/` just redirects to `/requests` (procurement) or
`/my-requests` (everyone else). Issue #2482 asks for a **home page whose cards surface the
most relevant information for the signed-in user's role(s)** — count tiles plus a "latest
activity" feed. This gives each role an at-a-glance overview and quick jump-off points.

A user may hold several roles at once, so the home page shows **stacked sections**, one per
applicable role group (mirroring how the nav gates tabs):

- **Staff** (everyone — anyone can submit): My requests / Completed / Latest activity on my requests
- **Approver** (legal / security / budget card actor, or named approver — i.e. `is_approver`):
  Pending approvals / Completed approvals / Latest activity on pending approvals
- **Procurement** (`procurement`/`procurement_admin`/`admin`): Pending requests / Awaiting delivery /
  Completed / Latest activity on procurement queue

### Decisions (confirmed with user)
- **"Completed" = `order_signed`** (contract signed / order placed) — the terminal state today;
  the reserved `completed` status is also counted for forward-compat.
- **Procurement split**: `pending` = active PRs before `order_signed`
  (`submitted`/`under_review`/`vendor_selected`/`contract_prepared`, excluding rejected/cancelled);
  `awaiting delivery` = `order_signed` and **not** fully paid; `completed` = `order_signed` with
  ≥1 invoice and **all** its invoices `paid`. (`invoices.purchase_request_id` + `status` drive this.)
- **Home lives at `/`** (replaces the redirect) with a **"Home"** nav link as the first item.
- **Cards/rows are clickable**: count tiles link to the relevant filtered list; each activity row
  links to its PR detail page.

## Backend

Everything is computed in **one new authenticated endpoint** `GET /api/v1/home`, returning role-scoped
blocks (each omitted when N/A). This is warranted because two data needs aren't in the existing PR list:
the procurement completed/awaiting split (needs invoice aggregation) and "latest activity" (the
`process_events` table is not yet exposed). No migration needed.

### New handler — `backend/internal/handler/home.go`
`HomeHandler{Repo, Log}` with `Get(w, r)`, following the `/me` handler shape (`users.go`):
read `user := middleware.UserFromCtx(ctx)`, roles via `middleware.HasProcurementAccess` /
`HasRole(ctx, RoleLegal|RoleSecurity)` / `HasRole(ctx, RoleAdmin)`, assemble the response, `writeJSON`.

Response struct (coalesce nil activity slices to `[]`):
```go
type HomeResponse struct {
    Staff       *StaffHome       `json:"staff,omitempty"`        // always present
    Approvals   *ApprovalsHome   `json:"approvals,omitempty"`    // when is_approver
    Procurement *ProcurementHome `json:"procurement,omitempty"`  // when HasProcurementAccess
}
// each *Home: a few int counts + RecentActivity []HomeActivity
type HomeActivity struct {
    PurchaseRequestID int64; Reference, Title, Action, Qualifier, ActorEmail string
    CreatedAt time.Time // json snake_case tags
}
```
- Staff block always included (everyone submits; matches "My requests" always-visible nav).
- Approvals block gated on `HasApprovableWork(...)` (reuse; already backs `is_approver`).
- Procurement block gated on `HasProcurementAccess`.
- Register `r.Get("/home", homeH.Get)` inside the authenticated `/api/v1` group in
  `handler/router.go`, and add the handler to wiring next to `prs`/`users`.

### New repository methods — `backend/internal/repository/home.go`
Reuse the existing SQL fragments in `repository.go` (`approvablePredicate`, `myApprovalStateExpr`,
`teamLeadMatch`, `budgetEmailMatch`) and bind order `$1`=callerID, `$2`=hasLegal, `$3`=hasSecurity,
`$4`=`normEmail(email)` — identical to `ListPurchaseRequests`/`HasApprovableWork`.

- `CountMyRequests(ctx, callerID) (total, completed int)` —
  `count(*)` and `count(*) FILTER (WHERE status IN ('order_signed','completed'))` over
  `requester_id = $1` (the `FILTER` idiom already used in `ApprovalTally`).
- `CountApprovals(ctx, callerID, email, hasLegal, hasSecurity) (pending, completed int)` — over the
  same PR set as scope=approvals (`requester_id <> $1 AND (teamLeadMatch OR (team_lead_status='approved'
  AND approvablePredicate))`), bucket on `(myApprovalStateExpr)` = `pending` vs `approved|rejected`.
- `CountProcurementRequests(ctx, isAdmin, seesAll) (pending, awaiting, completed int)` — over the
  procurement-visible set (admin: all; else team-lead-approved), excluding rejected/cancelled:
  `pending` = status in the four pre-order_signed states; then a per-PR "all invoices paid" check
  (`NOT EXISTS(... invoices WHERE status <> 'paid')` combined with `EXISTS(... invoices ...)`) splits
  `order_signed` into `completed` vs `awaiting`.
- `RecentActivity(ctx, filter, args, limit) ([]HomeActivity)` — one helper that selects from
  `process_events pe JOIN purchase_requests pr` (for `reference`/`title`), `ORDER BY pe.created_at DESC
  LIMIT $n`, with the PR-set predicate passed in. Called three times with the requester /
  approvable / procurement-visible predicates (same fragments as the counts). Limit = 8.

## Frontend

### API + hook
- `webapp/src/api/home.ts`: `getHome = () => apiFetch<HomeResponse>("/api/v1/home")`.
- `webapp/src/hooks/useHome.ts`: `useHome()` — `useQuery({ queryKey: ["home"], queryFn: getHome,
  refetchInterval: 5000 })` (matches the list hooks).
- Types in `webapp/src/types/api.ts`: `HomeResponse`, `StaffHome`, `ApprovalsHome`,
  `ProcurementHome`, `HomeActivity`.

### Page — `webapp/src/pages/HomePage.tsx`
- Greeting header (`me.name`), then render each present block as a section. Each section = a heading +
  a row of stat tiles + a "Latest activity" card.
- **StatTile** (small local component or in a new `components/StatTile.tsx`): `.app-card p-6` with a big
  count, label, optional accent, wrapped in a `<Link to=...>` — My requests→`/my-requests`,
  Pending approvals→`/approvals`, procurement tiles→`/requests`.
- **ActivityList**: rows of `<Link to={/requests/:id}>` showing the PR ref + a humanized action label +
  actor + relative time. Add an `ACTION_LABELS: Record<string,string>` map in the types/labels file
  (e.g. `submit_pr`→"Request submitted", `pr_approval`+approve→"Approved", `sign_contract`+sign→
  "Contract signed", `assign_pr`+assign→"Assigned", `select_quotation`→"Quotation selected", …),
  keyed by action with qualifier refinement; fall back to a title-cased action.
- Reuse `.app-card`, `.badge`, `StatusBadge`, page-header classes, and `prReference`.

### Wiring
- `webapp/src/App.tsx`: replace `<Route path="/" element={<Navigate to="/requests" .../>}>` with
  `<Route path="/" element={<HomePage />} />`. Leave the `*` catch-all pointing at `/`.
- `webapp/src/components/Layout.tsx`: add `<NavLink to="/" className={linkCls} end>Home</NavLink>` as the
  first nav item.

## Docs
- Copy this plan to `docs/plans/17-role-home-pages.md` (plan-archival convention).
- Add a short as-built `docs/home-pages.md` and a Status bullet in `CLAUDE.md`.
- Add `ProcessGetHome`? No new process/audit action (read-only endpoint — nothing to record).

## Verification
1. `cd backend && go build ./... && go vet ./...`; `cd webapp && npm run build` (tsc) — must pass.
2. Seed a little data (a couple PRs across statuses, an approval, a signed contract + paid invoice),
   then `curl -H "Authorization: Bearer <tok>" localhost:8080/api/v1/home` as: a plain staff user
   (only `staff` block), an approver (adds `approvals`), and a procurement user (adds `procurement`);
   confirm counts and that `recent_activity` rows carry ref/title/action. **Clean up all test data after.**
3. Run the app (backend + `npm run dev`), log in, confirm `/` shows the right stacked sections per role,
   tiles navigate to the filtered lists, and activity rows open the PR. Check a multi-role user
   (admin) sees all three sections.
4. Run existing repo integration tests (`go test ./internal/repository/...`) to ensure no regression.
