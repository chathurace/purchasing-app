# PR assignment (as-built)

After a PR clears **team-lead approval**, it must be **assigned** to a member of the procurement
team before any procurement work (quotations, recommendations) can start. This gives every in-flight
PR a clear owner.

Assignment tracks the owner — it does **not** lock others out. Once a PR is assigned, *any*
`procurement`/`procurement_admin` user may work on it; a user who does non-readonly work on a PR they
don't already own is **automatically recorded as a collaborator**. So the **assignee** is the owner
and **collaborators** are the procurement users who have also worked on it (plus any added manually
by the assignee/admin).

Sequence: *team lead approves → procurement assigns → quotations / recommendation*. Assignment is
only possible after team-lead approval, because procurement can't see the PR before then.

The same card also carries the PR's **priority** — see [Priority](#priority) below.

## Data model — migration `044_pr_assignment.sql`

- `purchase_requests.assignee_id` (FK `users`, `ON DELETE SET NULL`), `assigned_at`, `assigned_by`.
- `purchase_request_collaborators (purchase_request_id, user_id, added_at, added_by)`, PK
  `(purchase_request_id, user_id)`, `ON DELETE CASCADE` on the PR.

The columns exist (empty) from PR creation and are populated later by procurement — the requester
never picks them.

Migration `051_pr_priority.sql` adds `purchase_requests.priority` — `TEXT NOT NULL DEFAULT 'P3'`
with `CHECK (priority IN ('P1','P2','P3'))`, so every PR (including every pre-existing row) starts
at P3.

## Rules

| Action | Who |
| --- | --- |
| Claim an **unassigned** PR (self-assign) | any `procurement` user |
| Unassign / hand back **your own** assignment | the current assignee, or `procurement_admin`/`admin` |
| Assign / reassign to **another** user | `procurement_admin`/`admin` |
| Add / remove **collaborators** manually | the assignee, or `procurement_admin`/`admin` |
| Perform procurement **work** (quotations, recommendation, …) on an assigned PR | any `procurement`/`procurement_admin` user (auto-added as a collaborator) |

A valid assignee/collaborator must be able to do procurement work — they hold `procurement`,
`procurement_admin`, or `admin`. (Membership of the Procurement team is the plain `procurement`
role, but a `procurement_admin` does procurement work too and must be assignable, e.g. claiming a
PR for themselves.) The repository validates this (`SetPRAssignee` / `AddPRCollaborator` via
`CanBeAssignedPR`), returning `ErrNotProcurementUser` → HTTP 400 otherwise. On the client the
assignee dropdown lists the Procurement team plus the current user when they're a
`procurement_admin`/`admin` not already on the team.

## Backend

- **Repository** (`internal/repository/repository.go`): `SetPRAssignee`, `AddPRCollaborator`,
  `RemovePRCollaborator`, `listCollaborators`. `GetPurchaseRequest` / `ListPurchaseRequests` join the
  assignee (`LEFT JOIN users`) and `GetPurchaseRequest` loads collaborators.
- **Handlers** (`internal/handler/assignment.go`): `SetAssignee` (`PUT .../assignee`, body
  `{assignee_id}`, `null` to unassign), `AddCollaborator` (`POST .../collaborators`, `{user_id}`),
  `RemoveCollaborator` (`DELETE .../collaborators/{userID}`). Routes in `router.go`.
- **Work gate** (`assignmentWorkGate`): called next to the existing team-lead 409 in `quotations.go`
  (`Create`) and `recommendations.go` (`CreateRecommendation`). Returns **409** when the PR is
  unassigned; once assigned, any procurement user (already confirmed by the handler's
  `HasProcurementAccess` check) proceeds.
- **Auto-collaborator** (`ensureCollaborator` → `repository.EnsurePRCollaborator`): called after a
  successful procurement mutation (quotation create/update/select; recommendation create/update/delete;
  recommendation contract create/delete; RFI raise/clear; budget-approver set). If the acting user is a
  `procurement`/`procurement_admin` and isn't already the assignee or a collaborator, they're inserted
  as a collaborator (and an `update_pr_collaborators` process event recorded). Best-effort; idempotent;
  skips the requester and team-card/budget approvers (not procurement) and plain admins.
- **Per-caller flags** (`attachAssignmentActionable`, set on detail reads, mirrors
  `my_team_lead_actionable`): `my_can_assign`, `my_can_manage_collaborators`, and `my_can_work` (true
  once the PR is assigned, for any procurement caller).
- **Process events** (`internal/model/events.go`): `assign_pr` (qualifier `assign`/`unassign`) and
  `update_pr_collaborators`, both registered in `ValidProcessActions`. Recorded best-effort after the
  mutation.

## Frontend

- **Types / API** (`types/api.ts`, `api/purchaseRequests.ts`): `assignee_id`/`assignee`/`assigned_at`/
  `collaborators` + the three `my_can_*` flags on `PurchaseRequest`; `setPRAssignee`,
  `addPRCollaborator`, `removePRCollaborator`.
- **`components/AssignmentCard.tsx`**: rendered between `TeamLeadApprovalCard` and the procurement
  sections on the PR detail page. Admins get a `<select>` of Procurement-team members (from
  `listTeams().find(t => t.key === "procurement").members`); a plain procurement user gets
  "Assign to me" / "Unassign me". Collaborators use `ApproverPicker` (procurement members as
  candidates). Gated by `my_can_assign` / `my_can_manage_collaborators`.
- **Gating**: `ProcurementSection` and recommendation **authoring** (`RecommendationSection`'s
  `procurement` capability) are gated on `my_can_work`, matching the backend. Approver card actions
  (comment/approve/assign) are unaffected — they use the per-card flags.
- **List page**: an **Assignee** column — "Assigned to you" (indigo), the assignee's name, or an
  amber "Unassigned" badge for a team-lead-approved-but-unassigned PR.

## Priority

Procurement triages every PR as **P1 (red, high)**, **P2 (yellow, medium)** or **P3 (green, normal —
the default)**. It's a queue-wide signal, not an ownership one, so the rules are simpler than
assignment's:

| Action | Who |
| --- | --- |
| Change the priority | any `procurement` / `procurement_admin` user (and `admin`, as everywhere) |
| See the priority | anyone who can see the PR |

Unlike procurement *work*, setting the priority does **not** require the PR to be assigned — it's
how an unassigned PR gets triaged in the first place. It does require team-lead approval, because
procurement can't see the PR before then (the endpoint 409s, matching `SetAssignee`).

- **Backend**: `repository.SetPRPriority` (re-validates against `model.ValidPRPriority`);
  `handler.SetPriority` at `PUT /purchase-requests/{id}/priority`, body `{priority}` — 403 for a
  non-procurement caller, 409 before team-lead approval, 400 on an unknown value. `priority` rides
  along on both the list and detail selects; the per-caller flag `my_can_set_priority` is set in
  `attachAssignmentActionable` (independent of whether the PR is assigned). Process event
  `update_pr_priority`, with the new priority as the **qualifier** (`P1`/`P2`/`P3`).
- **Frontend**: `PRPriority` + the ordered `PR_PRIORITIES` option list (value/label/colour) and the
  `prPriority`/`prPriorityColor` helpers in `types/api.ts`; `setPRPriority` in
  `api/purchaseRequests.ts`. The `AssignmentCard` shows a coloured `<select>` when
  `my_can_set_priority`, otherwise a read-only chip. The list page shows a **Priority** column
  (coloured `P1`/`P2`/`P3` chip) between Title and Status.

## Tests

`assignment_integration_test.go` (`TestSetPriority`) covers the priority endpoint: a new PR defaults
to P3, procurement raises it to P1 (unassigned), the requester gets 403, an unknown value 400, and a
pre-team-lead-approval attempt 409.

`internal/handler/team_lead_integration_test.go` (`TestTeamLeadApprovalFlow`) drives the real
handlers: unassigned → quotation create 409; self-assign 200 (+ an `assign_pr` process event);
assignee → quotation create passes the gate; a *different* procurement user → also succeeds and is
auto-added as a collaborator. `assignment_integration_test.go` (`TestProcurementAdminSelfAssign`)
covers a `procurement_admin` (without the plain `procurement` role) assigning a PR to themselves.
