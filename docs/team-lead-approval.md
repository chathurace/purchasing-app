# Team lead approval (as-built)

Every purchase request names a **team lead** (by email) who must approve it before
procurement can see or act on it. Until the team lead approves, the PR is visible only
to the requester, the team lead, and admins. Plan archive: `docs/plans/13-team-lead-approval.md`.

## Data model

Migration `034` … `037`. All state lives on `purchase_requests` (one team lead per PR):

| column | notes |
| --- | --- |
| `team_lead_email` | lowercased; the named team lead. Required on create. |
| `team_lead_status` | `pending` \| `approved` \| `rejected` (default `pending`). |
| `team_lead_notes` | the team lead's decision notes. |
| `team_lead_decided_at` | timestamp of the last decision (nullable). |
| `team_lead_decided_by` | user id who decided (nullable FK). |

The migration **grandfathers all pre-existing PRs to `approved`** so in-flight
requests stay visible; new PRs default to `pending`.

## Identity by email

The team lead is matched by a **case-insensitive email comparison** — no user id is
stored. This works even if the named person has never logged in: they are
auto-provisioned as `staff` on first login (the existing pending-invite path), and
the PR then appears in their Approvals queue. Emails are stored lowercased
(`normEmail`), and the caller's side is normalized too (`callerIsTeamLead`).

## Visibility gate

Enforced in two places (backend):

- **Detail / download** — `handler.callerCanView`: requester, admin, or team lead
  always; everyone else (procurement, named approvers, recommendation-card actors) only
  once `team_lead_status = 'approved'`.
- **Lists** — `repository.ListPurchaseRequests` (`scope=default`/`approvals`) and
  `HasApprovableWork` (the `/me` `is_approver` nav flag). A `teamLeadMatch` SQL
  fragment surfaces the team lead's own queue at any status; other actors surface
  only after approval. Admins see everything; procurement (non-admin) see own +
  team-lead-of + any team-lead-approved PR.

The caller's unified `my_approval_state` / `my_approval_status` (Approvals-tab
filter + "Awaiting you" badge) also reflect `team_lead_status` when the caller is
the team lead.

## Explicit procurement gate

Belt-and-suspenders on top of visibility: quotation create (`handler/quotations.go`)
and recommendation create (`handler/recommendations.go`) return **409 "purchase
request is awaiting team lead approval"** unless `model.IsTeamLeadApproved`.
Everything else downstream needs a quotation/recommendation, so these two entry
points cover the phase.

## Decision flow

- Endpoint: `POST /api/v1/purchase-requests/{id}/team-lead-approval`
  `{ decision: approve|reject, notes }`. Only the team lead (email match) or an
  admin may act (`handler.TeamLeadDecision`). **Notes are required on reject.** The
  decision may be revised at any time ("Edit decision").
- Recorded as process event `team_lead_approval` (qualifier `approve|reject`).
- The requester is emailed on each decision; the team lead is emailed on submission
  (`notifyTeamLeadRequested` / `notifyDecision`, best-effort).

## Edit & resubmit

`UpdatePurchaseRequest` resets the decision to `pending` (clearing notes / decided
fields) when the **team-lead email changes** or the current status is **rejected**
— so an edited/rejected PR goes back through review. An `approved` PR whose team
lead is unchanged keeps its approval across edits.

## Frontend

- Requisition form (`RequisitionForm.tsx`): a required **"Team lead (for approval)"**
  email field below business justification (step 1), with a non-empty + format check.
- PR detail (`TeamLeadApprovalCard.tsx`, rendered above the existing named-approver
  "Approvals" card): shows the team lead + status; for the actor, a notes textarea +
  Approve/Reject buttons, each confirmed via `ConfirmDialog` (the app's first modal).
  Once decided it shows "Approved/Rejected on <date/time>" + notes and an "Edit
  decision" button.

## Relationship to the named-approver system

This is **separate** from the optional multi-approver "Approvals" feature
(`pr_approvals`), which is unchanged. A PR can have both a team lead (mandatory,
gating) and named approvers (optional, informational).
