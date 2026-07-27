# Chained (serial) budget approvals — as built

*Companion to `docs/plans/15-chained-budget-approvals.md`. Migration `045`.*

A procurement recommendation's **budget** approval is a **serial chain of steps**
instead of a single flat card. Step 1 (the *base* step) is governed by the PR's
**named budget approver** (the person the requester picked from the business
unit's approvers; see `docs/business-units.md`); procurement may append further
**named** steps, each with its own approver (email-matched). Steps are decided one
after another, and every step must be **approved** before a quotation can be
selected.

## Data model

`pr_recommendation_budget_steps` (migration `045`):

| column | notes |
| --- | --- |
| `recommendation_id` | FK → `pr_recommendations`, cascade |
| `position` | 1 = base, 2… = additional, unique per recommendation |
| `approver_name` / `approver_email` | empty for the base step (governed by the PR's named budget approver); required for additional steps |
| `decision` | `pending` \| `approved` \| `rejected` |
| `decided_by` / `decided_at` | who decided, when (null when pending) |

Budget-card comments hang off a step via `pr_recommendation_comments.budget_step_id`
(legal/security comments leave it null). Migration `045` backfills a base step for
every existing budget card and re-homes existing budget comments onto it.

## The budget approvals row is a projection

The `pr_recommendation_approvals` budget row **stays** (so required-types, the
"N of M granted" badge, budget-approver view access, budget notifications, and the
fully-approved gate keep working unchanged). Its `approved_by`/`approved_at` are now
**derived** from the chain by `syncBudgetApprovalFromStepsTx` (in
`repository/recommendations.go`): stamped with the last step's decider/time iff every
step is approved, else null. Because steps are serial, "all approved" ⟺ "last step
approved", so **`recApprovedSQL` and `SelectQuotation` need no change** — a pending or
rejected step leaves the row unapproved.

## Serial rules (`SetBudgetStepDecision`)

- A step is decidable only when the **previous** step is `approved` (step 1 always).
- A step **locks** once the **next** step has made a decision — you may
  approve / reject / reverse only until then.
- `decision` values on the endpoint: `approve` | `reject` | `revert` (→ pending).
- A rejected step leaves the chain unapproved and blocks quotation selection.
- Editing the recommendation (`UpdateRecommendation`) resets every step to pending,
  keeping the step rows and their named approvers.

## Permissions (handler layer)

- **Decide** a step (`can_decide`): admins always; the **base** step is governed by
  the PR's named budget approver (`IsBudgetApproverForPR`, email match); an
  **additional** step matches on its `approver_email` (case-insensitive) — plus the
  serial rule above.
- **Manage** the chain (`can_manage`: add / edit / remove additional steps): any
  procurement user working the PR (`pr.my_can_work`), while the step is pending.
- **Comment** on a step: the step's approver or any procurement user.
- **View access**: a named step approver (email match) can view the PR and find it
  under **Approvals** even though they are not the base budget approver — added to
  `callerCanView`, `approvablePredicate`, `myApprovalStateExpr`, and `IsApproverForPR`
  (which gained a `callerEmail` argument; `$4` = caller email is now bound wherever
  `approvablePredicate` is used).

## API

All under `/purchase-requests/{id}/recommendation/budget-steps`:

| method / path | who | action |
| --- | --- | --- |
| `POST` `{approver_name, approver_email}` | procurement | add a step |
| `PUT /{stepID}` `{approver_name, approver_email}` | procurement | edit a pending additional step |
| `DELETE /{stepID}` | procurement | remove a pending additional step |
| `POST /{stepID}/decision` `{decision}` | step approver / admin | approve / reject / revert |
| `POST /{stepID}/remind` | procurement | re-notify the step approver |

Budget decisions no longer use `POST .../approvals/budget` (it 400s for budget);
budget comments post to `.../recommendation/comments` with `budget_step_id`.

## Requesting / removing individual approval cards

Procurement can add or drop a **single** approval card inline (the "Request X
approval" pills and each card's "Remove X approval" ✕), without the full-edit reset
of `UpdateRecommendation`:

| method / path | action |
| --- | --- |
| `POST .../recommendation/approvals/{type}/request` | require the card (budget seeds a base step); 409 if already required |
| `DELETE .../recommendation/approvals/{type}` | remove the card (its comments/docs, and budget steps, are deleted); 409 if it is the last card, 404 if not required |

Repository: `AddRecApprovalCard` / `RemoveRecApprovalCard` (`repository/recommendations.go`).
Process events: `request_rec_approval` / `remove_rec_approval`, qualifier = the type
(budget / legal / security). The "Remove" button confirms via `ConfirmDialog`.

## Notifications & events

- A step's approver is emailed when it becomes the **current** step (its predecessor
  is approved) and on add / remind (`notifyBudgetStepApprover` /
  `notifyBudgetStepIfCurrent`). The base step reuses `notifyBudgetApprovers`.
- Process events: step decisions record `rec_approval_budget` (qualifier
  `approve` / `reject` / `revert`); chain edits record `update_budget_chain`
  (`add` / `update` / `remove`).

## Frontend

`components/RecommendationSection.tsx`: the budget card renders `BudgetChain` →
`BudgetStepCard` (nested elevated rows, colour-coded by decision) plus a
"+ Add another approval step" affordance for procurement. Each step shows its
approver, decision chip, Approve/Reject (pending) or Reverse (decided) controls
gated on `can_decide`, edit/remove for procurement, and its own comment thread
(`CommentThread`, extracted and reused). The card-level approve toggle and comment
block are hidden for budget.
