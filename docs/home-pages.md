# Role-based home pages (as-built)

The post-login landing page (`/`) is a **role-based dashboard**: count tiles plus a
"latest activity" feed, scoped to whatever the signed-in user does in the app. See
`docs/plans/18-role-home-pages.md` for the plan. Implements issue #2482.

## Sections (stacked; a user with several roles sees several)

A user may hold multiple roles at once, so the page renders **one section per role
group** the caller belongs to, mirroring how the nav gates its tabs:

- **My requests** (everyone — anyone can submit): `my_requests_count` / `completed_count`
  (`order_signed` or the reserved `completed`) + latest activity on the caller's own requests.
- **Approvals** (shown when `is_approver` — a named approver or a budget/legal/security/compliance
  recommendation-card actor): `pending_count` / `completed_count` (approved or rejected) +
  latest activity on the PRs awaiting/decided by the caller. Same PR set as the Approvals tab.
- **Procurement** (`procurement`/`procurement_admin`/`admin`): `pending_count` (active PRs before
  the order is signed) / `awaiting_delivery_count` (`order_signed`, not yet fully paid) /
  `completed_count` (`order_signed` with ≥1 invoice and every invoice `paid`) + latest activity
  on the procurement queue.

Count tiles link to the relevant filtered list (`/my-requests`, `/approvals`, `/requests`);
each activity row links to its PR detail page.

## Backend

One read-only endpoint, `GET /api/v1/home` (`handler/home.go`, `HomeHandler`), returns a
`HomeResponse{staff?, approvals?, procurement?}` — each block omitted when the caller doesn't
qualify. The staff block is always present; approvals is gated on `HasApprovableWork`;
procurement on `middleware.HasProcurementAccess`.

Aggregation lives in `repository/home.go` and **reuses the existing SQL predicate fragments**
from `repository.go` (`approvablePredicate`, `myApprovalStateExpr`, `teamLeadMatch`,
`budgetEmailMatch`) with the identical bind order `$1`=callerID, `$2`=the caller's team card types
(`text[]`), `$3`=lowercased email — so the dashboard's approval/visibility semantics stay in lockstep with
`ListPurchaseRequests`:

- `CountMyRequests` — total + `FILTER (WHERE status IN ('order_signed','completed'))`.
- `CountApprovals` — buckets `(myApprovalStateExpr)` over the scope=approvals PR set into
  pending vs completed.
- `CountProcurementRequests(isAdmin)` — over the procurement-visible set (admin: all; else
  team-lead-approved), excluding rejected/cancelled; splits `order_signed` into completed vs
  awaiting-delivery via an all-invoices-paid check on `invoices.purchase_request_id`/`status`.
- `RecentActivityForRequester|Approver|Procurement` — thin wrappers over `recentActivity`, which
  joins `process_events` to `purchase_requests` for the ref/title, newest first, `LIMIT 8`.

No migration and **no new process/audit action** — the endpoint is a read.

## Frontend

- `api/home.ts` (`getHome`) + `hooks/useHome.ts` (`useHome`, polls every 5s like the list hooks).
- Types + `activityLabel(action, qualifier)` (humanizes a process event) and `relativeTime(iso)`
  in `types/api.ts`.
- `pages/HomePage.tsx` renders the greeting + present sections; `StatTile` (linked `.app-card`)
  and `ActivityCard` are local components.
- `App.tsx`: `/` now renders `HomePage` (was a redirect to `/requests`); the `*` catch-all points
  at `/`. `Layout.tsx`: a "Home" `NavLink` is the first nav item.
