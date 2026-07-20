# Team lead approval

## Context

Today a purchase request (PR) is visible to finance/admin the moment it's submitted, and there is
no mandatory gate before finance starts procurement work. The business wants an explicit **team
lead sign-off**: every PR must be approved by the requester's team lead before finance can see it or
act on it. The requester names their team lead by email on the requisition form; the team lead
reviews it from the Approvals page and approves/rejects with notes; until approval the PR is hidden
from everyone except the requester, the team lead, and admins.

Per the clarifications: we **keep** the existing optional multi-approver "Approvals" feature
untouched and add a **separate** "Team lead approval" card (so the original "rename" note becomes
"add a new card"). A rejected PR is **editable & resubmittable**. Team-lead approval is enforced
both as a **visibility gate** and as an **explicit server-side gate** on finance mutations.

Key existing anchors (confirmed):
- `callerCanView` — `backend/internal/handler/purchase_requests.go:105`
- `ListPurchaseRequests` / `approvablePredicate` / `myApprovalStateExpr` — `backend/internal/repository/repository.go:546-654`
- `HasApprovableWork` (drives `/me` `is_approver` nav flag) — `repository.go:671`
- `CreatePurchaseRequest` / `UpdatePurchaseRequest` — `repository.go:426`, `~:478`
- Caller identity: `middleware.UserFromCtx(ctx).Email` (stored **lowercased**) — `middleware/auth.go:236`, `repository.go:32`
- Requisition form business-justification field — `webapp/src/components/RequisitionForm.tsx:413-415`
- Existing multi-approver card (leave as-is) — `webapp/src/components/ApprovalList.tsx:42`
- Email infra: `notifyApprovalRequested` / `notifyDecision` — `purchase_requests.go:654,677`

## Backend

### 1. Migration `backend/migrations/037_team_lead_approval.sql`
Add columns to `purchase_requests`:
- `team_lead_email TEXT NOT NULL DEFAULT ''`
- `team_lead_status TEXT NOT NULL DEFAULT 'pending'` (values: `pending|approved|rejected`, CHECK)
- `team_lead_notes TEXT NOT NULL DEFAULT ''`
- `team_lead_decided_at TIMESTAMPTZ`
- `team_lead_decided_by BIGINT REFERENCES users(id)`

Then **grandfather existing rows** so in-flight PRs stay visible: `UPDATE purchase_requests SET
team_lead_status = 'approved';` (runs once, before app writes new pending rows). Add
`CREATE INDEX ... ON purchase_requests (team_lead_email)` for the visibility lookup.

### 2. Model — `backend/internal/model/`
- `model.go`: reuse the existing `pending/approved/rejected` string constants; add
  `IsTeamLeadApproved(status string) bool` helper.
- `events.go`: add `ProcessTeamLeadApproval` to `ValidProcessActions` (qualifier `approve|reject`).

### 3. Repository — `backend/internal/repository/repository.go`
- Add to `PurchaseRequest` struct: `TeamLeadEmail`, `TeamLeadStatus`, `TeamLeadNotes`,
  `TeamLeadDecidedAt *time.Time`, `TeamLeadDecidedBy *int64` (JSON `team_lead_*`).
- Add `TeamLeadEmail` to `PurchaseRequestInput`.
- `CreatePurchaseRequest`: include `team_lead_email` in the INSERT (lowercased); status defaults to
  `pending` via the column default.
- `UpdatePurchaseRequest`: update `team_lead_email` (lowercased) and **reset the decision** to
  `pending` (clear notes/decided_at/decided_by) when the new email differs from the stored one OR
  the current status is `rejected` — this implements "edit & resubmit". Mirror the recommendation
  "editing resets cards to pending" pattern.
- `GetPurchaseRequest` + `ListPurchaseRequests`: SELECT the new columns.
- New `RecordTeamLeadDecision(ctx, prID, deciderID int64, decision, notes string) error` — sets
  status/notes/decided_at=now()/decided_by; allowed from any current status (supports "edit
  decision").

**Visibility (the core change).** Thread caller email + isAdmin through the list/lookup functions.
Add a SQL fragment `teamLeadMatch` = `(pr.team_lead_email <> '' AND pr.team_lead_email = $N)` (both
sides lowercased). Rework `ListPurchaseRequests` WHERE by scope:
- `mine` — unchanged (`requester_id = $1`).
- default:
  - **admin** → no WHERE (sees all).
  - **finance (non-admin)** → `requester_id=$1 OR teamLeadMatch OR team_lead_status='approved'`.
  - **other** → `requester_id=$1 OR teamLeadMatch OR (team_lead_status='approved' AND approvablePredicate)`.
- `approvals` → `requester_id<>$1 AND ( teamLeadMatch OR (team_lead_status='approved' AND approvablePredicate) )`
  — so card actors surface only after team-lead approval; the team lead surfaces their own queue.

Extend `myApprovalStateExpr` and the `my_approval_status` subquery so that when the caller is the
team lead, their state/status reflect `team_lead_status` (drives the Approvals-tab pending/reviewed
filter and the "Awaiting you" badge). `HasApprovableWork` (→ `/me` `is_approver`) must also return
true when the caller is a team lead of any PR they didn't submit, so the Approvals nav tab appears
for team leads who lack finance/approver roles. Update signatures of `ListPurchaseRequests`,
`HasApprovableWork` (and callers in `handler/purchase_requests.go` + `handler/users.go`) to pass
`callerEmail` and `isAdmin` (`middleware.HasRole(ctx, model.RoleAdmin)`).

### 4. Handler — `backend/internal/handler/purchase_requests.go`
- `callerCanView`: allow requester, admin, or `user.Email == pr.TeamLeadEmail`; only if
  `pr.TeamLeadStatus == approved` fall through to the existing broad rules (finance / named approver
  / rec-card actor). This also gates `DownloadDocument`.
- `Create`: reject if `team_lead_email` is empty or equals the requester's email (400). After
  create, email the team lead (reuse the `notifyApprovalRequested` send pattern).
- New handler `TeamLeadDecision` (route `POST /api/v1/purchase-requests/{id}/team-lead-approval`,
  body `{decision, notes}`): load PR; allow only the team lead (email match) or admin (else 403);
  validate decision; require `notes` on reject (matches existing reject-needs-comment convention);
  `RecordTeamLeadDecision`; `recordProcessEvent(..., ProcessTeamLeadApproval, decision)`; email the
  requester (reuse `notifyDecision`); reload & return the PR.
- Add `my_team_lead_actionable` (bool) to the PR JSON on `Get`, computed like `attachRecActionable`
  (caller is team lead or admin) so the client shows controls without re-deriving email matching.
- Register the route in `handler/router.go` next to the existing approval routes.

### 5. Explicit finance gate
Add a small helper `requireTeamLeadApproved(pr) (ok bool)` and enforce at the two entry points that
begin finance work (everything downstream needs these): quotation create/associate in
`handler/quotations.go` and recommendation create in `handler/recommendations.go`. Return **409**
"purchase request is awaiting team lead approval" when `pr.TeamLeadStatus != approved`.

## Frontend

### 6. Types & API — `webapp/src/types/api.ts`, `webapp/src/api/purchaseRequests.ts`
- `PurchaseRequest`: add `team_lead_email`, `team_lead_status: ApprovalStatus`, `team_lead_notes`,
  `team_lead_decided_at: string | null`, `team_lead_decided_by: number | null`,
  `my_team_lead_actionable?: boolean`.
- `PurchaseRequestInput`: add `team_lead_email: string`.
- Add `recordTeamLeadDecision(id, decision, notes)` → `POST .../team-lead-approval`.

### 7. Requisition form — `webapp/src/components/RequisitionForm.tsx`
- Add a required `<Field label="Team lead (for approval)">` `<input type="email">` immediately below
  the business-justification textarea in `StepRequester`, bound to `value.team_lead_email` via
  `setTop`. Default it in `emptyRequisition()`.
- Add to step-1 validation in `missingOn(1)`: non-empty + basic email-format check.

### 8. Confirm dialog — new `webapp/src/components/ConfirmDialog.tsx`
Reusable modal (`fixed inset-0` overlay + centered panel, title/message/confirm/cancel) — the app
has no modal today. Used for the approve/reject confirmation.

### 9. Team lead approval card — new `webapp/src/components/TeamLeadApprovalCard.tsx`
Rendered in `webapp/src/pages/PurchaseRequestDetailPage.tsx` alongside (separate from) the existing
`ApprovalList`. Titled **"Team lead approval"**. Shows the team lead email + a status pill.
- When `pr.my_team_lead_actionable` and (pending, or user clicked "Edit decision"): a **Notes**
  textarea + **Approve** / **Reject** buttons; each button opens `ConfirmDialog` and on confirm
  calls `recordTeamLeadDecision`, invalidating `["purchase-requests", pr.id]` and
  `["purchase-requests"]`.
- When decided: show **"Approved/Rejected on <decided_at>"** via `new Date(...).toLocaleString()`
  plus the notes, and (for the actor) an **"Edit decision"** button re-revealing the form.
- Non-actors (requester/finance): read-only status view.

### 10. Approvals page copy — `webapp/src/pages/ApprovalsListPage.tsx`
Update the subtitle to mention "team lead" alongside budget/legal/security (rows already flow in via
`my_approval_state`).

## Docs & conventions
- Archive this plan to `docs/plans/13-team-lead-approval.md`.
- Add `docs/team-lead-approval.md` (as-built): the email-match model, visibility gate, explicit
  finance gate, edit-&-resubmit reset.
- Update `CLAUDE.md` status section with a "Team lead approval" bullet and note the pre-approval
  visibility restriction.

## Verification
1. `psql -h localhost -d purchasing -f backend/migrations/037_team_lead_approval.sql`; confirm
   existing PRs backfilled to `approved`.
2. `cd backend && go build ./... && go test ./...` — fix any list-visibility integration tests
   whose expectations change; add a test that a submitted (pending) PR is invisible to a finance
   user and visible to the team-lead-email user + admin, and becomes visible to finance after
   approval.
3. End-to-end via the app (`/run`): create a PR naming a team-lead email → confirm finance user
   cannot see it and gets 409 attempting a quotation → log in as the team-lead user, see it under
   Approvals, approve via the modal → finance now sees it and can add a quotation → separately test
   reject (finance still blocked) then requester edit-&-resubmit resets to pending → team lead
   "Edit decision".
