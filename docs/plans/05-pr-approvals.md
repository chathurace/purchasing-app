# Plan 05 — Purchase-request approvals

## Goal

When creating a purchase request, the requester names one or more **approvers**.
Each approver independently **approves or rejects with a comment**. A rejected
approval can be **re-requested** by the requester (resets that approver to
pending). **All approvers must approve before any RFQ can be raised** against the
request.

## Decisions (confirmed with the requester)

- **Approver pool:** any active user except the requester themselves.
- **Managing approvers:** the requester may add/remove approvers while the PR is
  editable (`submitted` / `under_review`). At least one approver is always
  required.
- **Required, not optional:** every new PR must carry ≥1 approver.
- **Notifications:** in-app **and** email. Email infrastructure added (disabled
  by default; logs instead of sends in dev).

## Design

Mirror the existing **contract review approvals** pattern: a separate table holds
per-approver decisions, and the RFQ step is gated on the rolled-up state. The
PR's own `status` is untouched.

- **`pr_approvals`** (migration `017`): one row per `(purchase_request_id,
  approver_id)`, `status ∈ {pending, approved, rejected}`, `comment`,
  `decided_at`. Re-request mutates the row in place (back to `pending`).
- **Gate:** `model.RFQAllowedByApprovals(total, approved)` — blocked only when
  `total > 0 && approved < total`. PRs with no approvers (legacy rows) are never
  blocked; new PRs always have ≥1, so the "all must approve" rule binds them.
  Enforced in both the RFQ handler (friendly 409) and inside `CreateRFQ`'s tx.

## Endpoints

- `GET  /api/v1/users/lookup` — active-user directory (id/email/name) for the
  picker; any authenticated user.
- `POST /api/v1/purchase-requests` — now takes `approver_ids` (≥1, excludes self).
- `POST /api/v1/purchase-requests/{id}/approvers` `{approver_id}` — add (owner, editable).
- `DELETE /api/v1/purchase-requests/{id}/approvers/{approverID}` — remove (keeps ≥1).
- `POST /api/v1/purchase-requests/{id}/approvers/{approverID}/request` — re-request a rejected one.
- `POST /api/v1/purchase-requests/{id}/approval` `{decision, comment}` — the
  calling approver records their own decision (`approve` requires nothing,
  `reject` requires a comment).

Access: `callerCanView` widened so named approvers can read the PR. The list
endpoint now returns, for non-finance users, PRs they own **or** approve, plus an
approval tally and the caller's own `my_approval_status`.

## Email

`internal/email`: a `Mailer` interface with an `SMTPMailer` (stdlib `net/smtp`,
no new deps) and a `LogMailer` no-op default. New optional `email:` config block.
Approvers are notified when asked (create / add / re-request); the requester is
notified on each decision. Sends are best-effort and async.

## Frontend

- `ApproverPicker` (search-and-add) and `ApprovalList` (per-approver rows with
  status, comment, and role-appropriate actions).
- New PR page: required approver selection.
- Detail page: approvals section — requester manages approvers + re-requests;
  the acting approver approves/rejects; RFQ form is replaced by a notice until
  all approvals land.
- List page: "Awaiting you" pill / `n/m approved` column.

## Tests

`pr_approvals_integration_test.go` walks: gate closed → partial → reject →
re-request → all approved → gate open + PR advances; last-approver guard;
non-approver rejected.
