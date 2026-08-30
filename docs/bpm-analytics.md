# BPM analytics (as-built)

A read-only **Analytics** section in the sidebar, above Admin, over the process
data the app already records. **Phase 1 is one subsection — Purchase requests:**
the list of every PR, and per PR its process-event flow drawn top-to-bottom. No
migration, no new event action, no writes of any kind.

Analytics is deliberately a *group* rather than a single nav item — more
subsections are expected to join it, and adding one is a new entry in
`analyticsItems` plus a route.

## Access

**admin / procurement_admin only** — the same gate as the [Audit
log](process-events.md#audit-log-view), enforced by the *same* function:
`handler.hasEventLogAccess` (extracted from `EventsHandler.requireAccess` and now
shared with `AnalyticsHandler.requireAccess`). That is not a coincidence to be
tidied away later: the flow view returns the **same `process_events` rows** the
audit log does, so the two must never diverge in who may read them.

Frontend flag `useCanViewAnalytics` (in `hooks/useAnalytics.ts`) mirrors it for
nav visibility and the placeholder card. It is a separate hook from
`useCanViewAuditLog` despite the identical rule today, so either gate can move
without a rename.

## No visibility gate — on purpose

Every other PR read narrows by caller: the team-lead gate hides a PR until it is
approved, and `callerCanView` / `ListPurchaseRequests` scope the rest. The
analytics reads apply **none of that** — no team-lead gate, no scope, no
per-caller SQL binds at all (`repository/analytics.go` takes no caller
arguments). Two consequences worth knowing:

- The list includes PRs still waiting on their team lead, which a
  `procurement_admin` cannot see on the Purchase requests page.
- `GET /analytics/purchase-requests/{id}` does **not** go through the gated PR
  read, so a `procurement_admin` can analyse the flow of a request they could not
  open on the PR page.

This is sound only *because* of the access rule above: the audience already sees
every PR's events in the audit log, so gating the analytics view would hide
nothing while making the numbers wrong.

## Backend

`repository/analytics.go`:

- **`AnalyticsPR`** — `{id, reference, title, status, priority, created_at,
  requester, assignee}`. Deliberately narrow: these pages are about the *process*,
  not the request's contents. It doubles as the flow page's header.
- **`ListAnalyticsPRs(ctx, sort, desc, limit)`** — every PR, ordered and capped.
  Sorting is **server-side** because the read is capped: sorting the page locally
  would reorder a slice of the data and call it an ordering. `limit` is clamped to
  `[1, 2000]` (default 500) by `analyticsPRLimit`.
- **`analyticsPROrderBy`** resolves the column through a `switch` — never
  interpolated from the request — and the direction is one of two literals, so the
  clause is safe to concatenate. Notes on the ordering itself:
  - the person columns sort on `lower(COALESCE(NULLIF(name,''), email))`, the
    exact string the table renders, so the visible order matches the clicked
    header;
  - the assignee column is `NULLS LAST` in **both** directions — unassigned PRs
    belong at the bottom, not flooding page one of an ascending sort;
  - every ordering breaks ties on `id`, so a capped page is deterministic.
- **`GetAnalyticsPR`** returns `pgx.ErrNoRows` for an unknown id (→ 404).

`Repository.ListProcessEvents` (in `repository/events.go`) is the timeline read —
it already returned a PR's events oldest-first and now also `LEFT JOIN`s `users`
for `actor_name`, which is what the timeline renders. It stays **unbounded**: this
is one request's own history, not a table scan, and truncating a flow diagram
would misrepresent the process. `FilterProcessEvents` remains its cross-PR,
newest-first, capped counterpart for the audit log.

`handler/analytics.go` — `AnalyticsHandler`, read-only:

| Route | Returns |
|---|---|
| `GET /api/v1/analytics/purchase-requests?sort=&dir=&limit=` | `AnalyticsPR[]` |
| `GET /api/v1/analytics/purchase-requests/{id}` | `PRFlowResponse{purchase_request, events}` |

`?sort=` accepts `created_at` (default) \| `requester` \| `assignee`; an unknown
value falls back to `created_at` rather than erroring. `?dir=` accepts
`asc`\|`desc`, defaulting to **each column's natural direction** — newest-first
for the timestamp, A→Z for a person — which is what that header reads as on its
first click. The flow endpoint returns header *and* events in one round trip.

## Frontend

- `api/analytics.ts`, `hooks/useAnalytics.ts` (`useAnalyticsPRs` keeps the
  previous page visible while a re-sort is in flight, so the table doesn't blank
  out on every header click; `usePRFlow`).
- `pages/AnalyticsPRListPage.tsx` — `TableSortLabel` headers on the three
  sortable columns; clicking the active column flips it, a new column starts at
  its natural direction. Rows are click-through, and the reference is also a real
  link so it stays keyboard-reachable and middle-clickable.
- `pages/PRAnalyticsPage.tsx` — header (reference, title, priority, status,
  created, requester, assignee), three stat tiles derived from the log (steps
  recorded / first-to-last span / time since the last step), then the flow.
- `components/ProcessFlowTimeline.tsx` — the visualization.
- `lib/eventLabels.ts` — `humanizeEventToken` (now shared with the Audit log page,
  which used a local copy), `eventOutcome`, `formatDuration`.
- `components/AnalyticsAccessDenied.tsx` — the shared placeholder.

### The flow visualization

One continuous vertical rail, oldest event at the top, one node per event
(action, its qualifier as a chip, actor, timestamp). The **connector between two
nodes carries the elapsed time** between them — that placement is the point: the
waiting time is a property of the gap, not of either step, and finding where a
request sat is what a BPM view is read for.

Colour carries **outcome only**, and only from the reserved status roles
(`success` / `error` / `warning` for approve-or-sign / reject / revert-or-unsign,
derived from the *qualifier* — see `eventOutcome`); every other step is neutral,
which is most of them. Status colour never carries meaning alone: each node pairs
it with an icon *and* the visible qualifier chip. Node tiles are **outlined rather
than filled** so the glyph keeps its contrast in light and dark without
hand-picking a contrast-text colour per palette step. All text — action, actor,
timestamp — wears theme text tokens, so the only colour left on screen is the
decisions.

Because every value on the timeline is already rendered as text, the flow needs
no separate table view to be readable without colour.

## Testing

`handler/analytics_integration_test.go` drives the real handlers against the dev
DB: the role gate (staff/procurement/legal → 403; admin/procurement_admin → 200),
the ungated list (a PR whose team lead has *not* approved still appears), the flow
read (header + `submit_pr` with `actor_name` resolved, and 404 on an unknown id),
and the sort contract (each direction reorders, an unknown column falls back
rather than erroring, unassigned rows sort last both ways). Test rows are removed
in `t.Cleanup` — PR first, then the requester it references.
