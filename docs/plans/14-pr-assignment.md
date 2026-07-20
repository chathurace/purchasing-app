# PR Assignment

## Context

Today, once a PR clears **team-lead approval**, *any* procurement user can start work on it
(add quotations, recommendations). There is no notion of ownership — nobody is accountable for a
given PR and two people can step on each other.

This adds a **PR assignment** step: after team-lead approval, a PR must be assigned to a member of
the procurement team before any procurement work can begin. The PR carries a single **assignee**
plus zero or more **collaborators** (also procurement users). Assignee + collaborators are the
people who may act on the PR; procurement_admin/admin can always act and can assign on others'
behalf.

Rules (confirmed with user):
- **Assign:** `procurement` users can assign a PR to **themselves** only; `procurement_admin`/`admin`
  can assign to **any** procurement user, and reassign anytime. Any procurement user can **claim**
  an unassigned PR. The current assignee can **unassign** (hand back) themselves.
- **Collaborators:** full work access (same as assignee). Managed by the **assignee** or
  `procurement_admin`/`admin`.
- **Gate order:** stacks on top of team-lead approval. Sequence is *TL approves → procurement
  assigns → quotations/recommendations*. (Assignment is naturally only possible after TL approval,
  since procurement can't see the PR before then.)
- **Work scoping:** once assigned, procurement mutations (quotation-create, recommendation-create)
  require the caller to be the **assignee, a collaborator, or procurement_admin/admin**. Plain
  procurement users who aren't on the PR can still see it in the queue but can't act.

The `assignee`/`collaborators` columns exist (empty) from PR creation; they are populated later by
procurement — the requester never picks them.

## Data model — migration `044_pr_assignment.sql`

Next number is **044** (`043` is the highest today).

```sql
ALTER TABLE purchase_requests
  ADD COLUMN assignee_id  BIGINT REFERENCES users(id),
  ADD COLUMN assigned_at  TIMESTAMPTZ,
  ADD COLUMN assigned_by  BIGINT REFERENCES users(id);

CREATE TABLE purchase_request_collaborators (
  purchase_request_id BIGINT NOT NULL REFERENCES purchase_requests(id) ON DELETE CASCADE,
  user_id             BIGINT NOT NULL REFERENCES users(id),
  added_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
  added_by            BIGINT REFERENCES users(id),
  PRIMARY KEY (purchase_request_id, user_id)
);
```

Single-column assignee mirrors `pr_recommendation_approvals.assignee_id` (migration `039`);
collaborators use a join table (multi-value, like the `pr_approvals` per-user pattern).

## Backend

### Event actions — `backend/internal/model/events.go`
- Add process action const `ProcessAssignPR = "assign_pr"` (assignee changes) and
  `ProcessUpdatePRCollaborators = "update_pr_collaborators"`.
- Register **both** in the `ValidProcessActions` `setOf(...)` list (~lines 113-125) — writes error
  otherwise. Reuse existing qualifiers `assign`/`unassign` (events.go ~62-77); add nothing new.

### Repository — `backend/internal/repository/repository.go` (+ reuse `teams.go`)
- **`PurchaseRequest` struct** (383-440): add `AssigneeID *int64`, `Assignee *UserSummary`,
  `AssignedAt *time.Time`, `Collaborators []UserSummary`.
- **Reads:** extend `GetPurchaseRequest` and `ListPurchaseRequests` (691-763) to `LEFT JOIN users`
  for the assignee and to load collaborators (batch/second query, as list-of-users). Populate the
  new struct fields.
- **Writers** (new, near the team-lead writers ~797-834):
  - `SetPRAssignee(ctx, prID, assigneeID *int64, actorID int64)` — sets `assignee_id/assigned_at/
    assigned_by` (all NULL when unassigning). Validate a non-nil assignee holds the procurement role
    via `UserHasRole(ctx, *assigneeID, model.RoleProcurement)` (`teams.go:137`).
  - `AddPRCollaborator(ctx, prID, userID, actorID)` / `RemovePRCollaborator(ctx, prID, userID)` —
    validate collaborator holds procurement role.
- **Authorization helpers** (reuse for gate + flags): a predicate
  `CallerCanWorkPR(pr, caller)` = assignee set **and** (caller is assignee OR in collaborators OR
  `HasProcurementAccess` with `procurement_admin`/`admin`). Put on the handler side (needs role set).

### Handlers — `backend/internal/handler/purchase_requests.go` + routes in `router.go`
Model closely on `TeamLeadDecision` (691-747) / the rec-assignee handlers.
- `PUT  /purchase-requests/{id}/assignee` — body `{ assignee_id: number|null }`.
  - `assignee_id == caller.id` → allowed for any procurement user (self-assign / claim).
  - `assignee_id != caller.id` (or reassigning an assigned PR to someone else) → requires
    `procurement_admin`/`admin`.
  - `assignee_id == null` (unassign) → allowed for current assignee or `procurement_admin`/`admin`.
  - Requires `IsTeamLeadApproved(pr.TeamLeadStatus)` (409 otherwise). After success:
    `recordProcessEvent(..., model.ProcessAssignPR, qualifier)` where qualifier is `assign`/`unassign`.
- `POST   /purchase-requests/{id}/collaborators`  body `{ user_id }` — assignee or admin.
- `DELETE /purchase-requests/{id}/collaborators/{userId}` — assignee or admin.
  - Both record `model.ProcessUpdatePRCollaborators`.
- **Per-caller capability flags** computed on read (added to the JSON PR): `my_can_assign` (can I
  change the assignee — true for procurement_admin/admin, or for self-assign when unassigned),
  `my_can_manage_collaborators`, `my_can_work` (assignee/collab/admin). Follows the existing
  `my_team_lead_actionable` convention.

### The work gate
Add alongside the existing `IsTeamLeadApproved` 409 checks:
- `backend/internal/handler/quotations.go:102-106`
- `backend/internal/handler/recommendations.go:119-121`

New check: if `pr.AssigneeID == nil` → 409 `"purchase request must be assigned before procurement
work can start"`; else if caller not assignee/collaborator/admin → 403 `"only the PR assignee or a
collaborator can perform this action"`.

## Frontend

### Types & API — `webapp/src/types/api.ts`, `webapp/src/api/purchaseRequests.ts`
- `PurchaseRequest` (298-348): add `assignee_id?`, `assignee?: UserSummary | null`,
  `collaborators?: UserSummary[]`, `assigned_at?`, and flags `my_can_assign?`,
  `my_can_manage_collaborators?`, `my_can_work?`.
- API fns (mirror `updateTeamLeadEmail` / `setRecAssignee`): `setPRAssignee(id, assigneeId|null)`,
  `addPRCollaborator(id, userId)`, `removePRCollaborator(id, userId)` — all return `PurchaseRequest`.

### New component — `webapp/src/components/AssignmentCard.tsx`
Modeled on `TeamLeadApprovalCard.tsx` (card shell, mutation+`invalidate()`+`ConfirmDialog` pattern).
- **Assignee** row: for procurement_admin/admin a `<select>` of procurement users (from
  `listTeams().find(t => t.key === "procurement").members`, the same source the rec legal/security
  assignee dropdown uses — RecommendationSection 869-874) with "Unassigned"; for a plain procurement
  user, an **"Assign to me" / "Unassign"** button. Gated by `pr.my_can_assign`.
- **Collaborators** row: `ApproverPicker` (`webapp/src/components/ApproverPicker.tsx`) seeded with
  procurement members as candidates and `pr.collaborators` as chips; add/remove call the API
  immediately. Shown only when `pr.my_can_manage_collaborators`.

### Wiring — `webapp/src/pages/PurchaseRequestDetailPage.tsx`
- Insert `<AssignmentCard pr={pr} me={me} />` between `TeamLeadApprovalCard` (224) and
  `ProcurementSection` (226).
- Gate `ProcurementSection` (226) and `RecommendationSection` (228) rendering on `pr.my_can_work`
  (fall back to a "assign this PR to start work" hint when assigned==false), so the UI matches the
  backend gate.
- Add `webapp/src/hooks/useIsProcurementAdmin.ts` (mirror `useIsAdmin.ts`).

### List page — `webapp/src/pages/PurchaseRequestListPage.tsx`
- Add an **Assignee** column; show an amber **"Unassigned"** badge for TL-approved-but-unassigned
  PRs and **"Assigned to you"** when `pr.assignee_id === me.id` (mirror the existing inline
  "Awaiting you" badge, 58-71).

## Verification
1. Migration: `psql -h localhost -d purchasing -f backend/migrations/044_pr_assignment.sql`.
2. Backend build + existing tests: `cd backend && go build ./... && go test ./internal/handler/ ./internal/repository/`.
   Add a handler test mirroring the team-lead gate test: quotation-create returns 409 while
   unassigned, 403 for a non-assignee procurement user, 200/201 for the assignee.
3. End-to-end via `/run` (or `verify` skill): as procurement, open a TL-approved PR → confirm
   quotations are blocked → self-assign → add a quotation succeeds. As procurement_admin, reassign
   to another procurement user and add a collaborator; confirm the collaborator can act and a
   third, unrelated procurement user cannot. Check the "Unassigned"/"Assigned to you" badges on the
   list page.
4. Confirm `process_events` has `assign_pr` / `update_pr_collaborators` rows after the actions.
5. Clean up any test PRs/rows created during verification (per CLAUDE.md).

## Docs (per CLAUDE.md conventions)
- Archive this plan to `docs/plans/NN-pr-assignment.md`.
- Add a companion `docs/pr-assignment.md` as-built and a CLAUDE.md status bullet.
