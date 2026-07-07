# Purchase-request approvals (as-built)

Each purchase request carries one or more **approvers** who must all approve
before procurement (RFQs) can begin. This is independent of the PR `status`,
mirroring how contract review approvals gate contract signing.

## Data

`pr_approvals` (migration `017`): one row per `(purchase_request_id,
approver_id)`.

| column                | meaning                                            |
| --------------------- | -------------------------------------------------- |
| `status`              | `pending` \| `approved` \| `rejected`              |
| `comment`             | the approver's decision comment                    |
| `decided_at`          | set on approve/reject, cleared on re-request       |

Unique on `(purchase_request_id, approver_id)`. Re-requesting a rejected approval
updates the row back to `pending` (it is not append-only).

## The gate

`model.RFQAllowedByApprovals(total, approved)` returns `false` only when
`total > 0 && approved < total`. So:

- A new PR (always ≥1 approver) is blocked until **every** approver approves.
- Legacy PRs with zero approvers are never blocked.

Enforced twice: the RFQ handler returns `409` with a friendly message, and
`Repository.CreateRFQ` re-checks inside its transaction (returns
`ErrInvalidState`) so the gate holds under races.

## Roles & access

- **Requester** (while PR is editable — `submitted`/`under_review`): names
  approvers at creation, and afterwards adds/removes them and re-requests a
  rejected approval. A PR must always keep ≥1 approver (`RemoveApprover` guard).
- **Approver** (any active user except the requester): records their own
  decision on their own row. `approve` needs no comment; `reject` requires one.
- **View access:** `callerCanView` allows the owner, finance/admin, **and** named
  approvers — so an approver who is only `staff` can open the PR.

## API

| Method & path                                                  | Who            |
| -------------------------------------------------------------- | -------------- |
| `GET /users/lookup`                                            | any auth user  |
| `POST /purchase-requests` (`approver_ids`, ≥1, not self)       | requester      |
| `POST /purchase-requests/{id}/approvers` `{approver_id}`       | owner/editable |
| `DELETE /purchase-requests/{id}/approvers/{approverID}`        | owner/editable |
| `POST /purchase-requests/{id}/approvers/{approverID}/request`  | owner/editable |
| `POST /purchase-requests/{id}/approval` `{decision, comment}`  | the approver   |

List reads (`GET /purchase-requests`) now also return, for non-finance users,
PRs they own **or** approve, plus `approvals_total`, `approvals_approved`, and
the caller's `my_approval_status`.

## Email

`internal/email` — `Mailer` with an SMTP implementation and a logging no-op.
Configured under `email:` in `config.yaml` (disabled by default → logs, so dev
needs no SMTP server). Approvers are emailed when asked (create / add /
re-request); the requester is emailed on each decision. All sends are async and
best-effort — a failure is logged and never blocks the action.

## UI

- `ApproverPicker` — search active users and add them.
- `ApprovalList` — per-approver status, comment, and role-appropriate actions
  (decide / remove / re-request).
- New PR form requires at least one approver. Detail page shows the approvals
  section; the RFQ form is replaced by a notice until all approvals land. The
  request list shows an "Awaiting you" pill or an `n/m approved` summary.
