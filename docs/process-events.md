# Process & audit events (as-built)

Two append-only tables record who did what, for later BPMN-style process analysis
and admin auditing. Migrations `034_process_events.sql` and `035_audit_events.sql`.
Recording is still **write-only at the mutating handlers** (best-effort, see below),
but an **admin-only read view** now exposes both logs — see **Audit log view** at the
end. `Repository.ListProcessEvents` remains the per-PR timeline read used by tests.

## Model

- **`process_events`** — one row per significant **business-process task** on a
  purchase request. `{purchase_request_id, action, qualifier, actor_id,
  actor_email, created_at}`.
- **`audit_events`** — one row per **non-process** mutation (master-data / admin /
  config). `{action, qualifier, entity_type, entity_id, detail, actor_id,
  actor_email, created_at}`.

### Rules

- **Fixed action set.** `action` values are Go constants in
  [`internal/model/events.go`](../backend/internal/model/events.go)
  (`ValidProcessActions` / `ValidAuditActions`). The repository **rejects** an
  action outside the set, so the catalog only changes when the code (the process)
  changes — like BPMN task types. There is deliberately no DB `CHECK` (it would
  force a migration per task); the Go set is the source of truth.
- **Action vs qualifier.** The `action` names the *task* (with the role/card baked
  in where the task is card-specific, e.g. `rec_approval_legal`). The decision
  direction is the `qualifier` — `approve|reject` (PR approval), `approve|revert`
  (recommendation cards), `sign|unsign` (contract), `received|approved|paid`
  (invoice status), `quotation|recommendation` (draft-contract source).
- **Human actions only.** PR status auto-advances (`under_review`,
  `vendor_selected`, `contract_prepared`, `order_signed`) happen in the repository
  as a side effect and are **not** recorded — they are derivable from the action
  sequence.
- **Best-effort + logged.** Events are written at the handler layer *after* the
  mutation succeeds, via `recordProcessEvent` / `recordAuditEvent`
  ([`internal/handler/events.go`](../backend/internal/handler/events.go)). A failed
  insert is logged with the request-scoped logger and never affects the response.
- **Actor snapshot.** `actor_id` is a FK (`ON DELETE SET NULL`); `actor_email` is
  an immutable snapshot taken from the request context, so analysis survives user
  renames/deletion and needs no join. The actor is captured from
  `middleware.UserFromCtx`, so it is recorded even for the repo funcs that drop the
  actor id (reject PR, select quotation, sign/unsign contract, revert card).
- **Timestamps** are DB-assigned (`created_at TIMESTAMPTZ DEFAULT NOW()`); the
  "date"/"time" fields from the spec are derived in queries
  (`created_at::date`, `created_at::time`).

## Process action catalog

| action | qualifier(s) | trigger (handler) |
|---|---|---|
| `submit_pr` | — | PR Create |
| `update_pr` | — | PR Update |
| `reject_pr` | — | PR Reject |
| `add_pr_approver` / `remove_pr_approver` | — | Add/Remove approver |
| `pr_approval` | `approve` \| `reject` | RecordApprovalDecision |
| `rerequest_pr_approval` | — | RequestApprovalAgain |
| `create_recommendation` / `update_recommendation` / `delete_recommendation` | — | recommendation CRUD |
| `rec_approval_legal` / `rec_approval_security` / `rec_approval_compliance` / `rec_approval_budget` | `approve` \| `revert` | SetRecApproval |
| `raise_rfi` / `clear_rfi` | — | Set/Delete recommendation RFI |
| `add_quotation` / `update_quotation` / `delete_quotation` | — | quotation CRUD |
| `select_quotation` | — | quotation Select |
| `add_draft_contract` | `quotation` \| `recommendation` | CreateFromQuotation / CreateRecommendationContract |
| `update_contract` / `delete_contract` | — | contract Update / DeleteRecommendationContract |
| `sign_contract` | `sign` \| `unsign` | Upload/Delete signed document |
| `create_grn` / `update_grn` / `delete_grn` | — | GRN CRUD |
| `create_invoice` / `update_invoice` / `delete_invoice` | — | invoice CRUD |
| `invoice_status` | `received` \| `approved` \| `paid` | invoice SetStatus |

Document-attachment ops (PR docs, draft-contract PDFs, RFI docs) are treated as
sub-steps and are **not** recorded in v1.

## Audit action catalog

| action | qualifier | entity_type | trigger |
|---|---|---|---|
| `create_vendor` / `update_vendor` | — | `vendor` | vendor Create/Update |
| `create_business_unit` / `update_business_unit` | — | `business_unit` | business-unit Create/Update |
| `create_config_option` | list key | `config_option` | config-option Create |
| `update_config_option` / `delete_config_option` | — | `config_option` | config-option Update/Delete |
| `create_user` | — | `user` | user Create (invite) |
| `update_user` | — | `user` | Update (edit invited user's email/name) |
| `grant_role` / `revoke_role` | role name | `user` | AddRole / RemoveRole |
| `set_user_active` | `active` \| `inactive` | `user` | SetActive |
| `connect_storage` / `set_storage_folder` | — | `storage` (no id) | storage Connect / SetFolder |

## Example queries

```sql
-- Full task timeline for a PR
SELECT created_at, action, qualifier, actor_email
FROM process_events WHERE purchase_request_id = $1 ORDER BY created_at;

-- Cycle time submit -> first sign, per PR
SELECT purchase_request_id,
       min(created_at) FILTER (WHERE action='submit_pr')     AS submitted,
       min(created_at) FILTER (WHERE action='sign_contract' AND qualifier='sign') AS signed
FROM process_events GROUP BY purchase_request_id;

-- Who granted admin, when
SELECT created_at, actor_email, entity_id AS target_user
FROM audit_events WHERE action='grant_role' AND qualifier='admin' ORDER BY created_at;
```

## Audit log view (issue #2502)

A read view over both logs for **admin / procurement_admin**, at `/audit`
("Audit log" in the Admin nav group). One page, a segmented control switching two
sections:

- **Process events** — `process_events`, newest-first across all PRs.
- **System events** — `audit_events`, newest-first.

**Filters** (server-side, debounced) shared by both sections: **actor**
(case-insensitive substring against the `actor_email` snapshot *and* the actor's
current joined name), **action** (exact, from a dropdown populated by the fixed Go
catalog), and a **date range** (inclusive `created_at::date` bounds). Process events
add a **PR id** filter. Results are capped at the latest **2000** rows per section
(`defaultEventLimit` 500 / `maxEventLimit` 2000 in `repository/events.go`); the UI
notes when the cap is hit.

Backend:

- `Repository.FilterProcessEvents` / `FilterAuditEvents` (`repository/events.go`)
  build the shared WHERE via `eventWheres` and `LEFT JOIN users` to surface
  `actor_name` (display-only; the immutable audit key stays `actor_email`).
- `EventsHandler` (`handler/events_view.go`) serves `GET /api/v1/events/process`,
  `/events/audit`, and `/events/actions` (the two action catalogs, from
  `model.SortedProcessActions`/`SortedAuditActions`). All three are gated to
  **admin / procurement_admin** (`EventsHandler.requireAccess` — `HasRole(admin)
  || HasRole(procurement_admin)`); the log carries sensitive actions (role grants,
  deactivations) so it is not open to plain procurement/staff.

Frontend: `pages/AuditEventsPage.tsx` (+ `api/events.ts`, `hooks/useEvents.ts`,
`hooks/useCanViewAuditLog.ts`; nav gated on the `canViewAuditLog` flag).
The view is read-only — no mutations, no new event action.

## Adding a new action

1. Add the constant to `internal/model/events.go` and to the matching
   `ValidProcessActions` / `ValidAuditActions` set.
2. Call `recordProcessEvent` / `recordAuditEvent` at the handler, right after the
   successful mutation.
3. Add a row to the catalog above.

No migration is needed — the column is plain `TEXT` and the Go set is the gate.
