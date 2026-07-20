# Audit logging for BPM-type analysis — `process_events` + `audit_events`

## Context

The app has **no DB-level audit trail** — the only record of "who did what" is domain
data (e.g. `pr_approvals.approver_id`) and stdout logs. The recent logging work
(request-scoped logger + access log) made actions *observable* but not *queryable*, and
explicitly deferred the DB audit trail to this feature.

We want two append-only tables for later analysis:

- **`process_events`** — significant **business-process** actions on a purchase request
  (BPMN-task style). Row = `{pr_id, timestamp, actor, action, qualifier}`. The set of
  `action` values is **fixed** — it changes only when the code (process) changes, like
  BPMN task types. Example tasks: submit PR, approve PR (legal), add quotation, add draft
  contract, sign contract.
- **`audit_events`** — everything else: master-data / admin / config mutations not tied to
  a PR lifecycle (add a vendor, delete a cost center, grant a role, connect storage).

### Decisions (confirmed with the user)

1. **Action vs qualifier**: the *action* is the task (with role/card baked in where the
   task is card-specific, e.g. `rec_approval_legal`). The decision direction
   (approve/reject/revert, sign/unsign, target invoice status) is a separate **`qualifier`**
   column — not a distinct action.
2. **Human actions only** — the PR status auto-advances in the repo (`under_review`,
   `vendor_selected`, `contract_prepared`, `order_signed`) as a *side effect*; these are
   derivable from the action sequence, so we do **not** write separate "system" rows.
3. **Best-effort + logged** — record the event right after the successful mutation; a failed
   insert is logged via `reqLog(r)` and never affects the response. No repo transaction
   threading.
4. **Tables only** — backend recording only. No new API/UI in v1 (query via SQL/BI).

## Approach

Recording happens at the **handler layer**, right after the successful repository mutation,
because several repo funcs drop the actor (`RejectPurchaseRequest`, `SelectQuotation`,
`SetContractSigned`/`ClearContractSigned`, `ClearRecApproval`) — but `middleware.UserFromCtx`
is always available in the handler. Two thin free-function helpers wrap the best-effort write
+ error logging. Action names are Go constants in `model` (single source of truth for the
"fixed set"), validated on write.

Timestamps are **DB-side** (`created_at TIMESTAMPTZ DEFAULT NOW()`), matching every existing
table — Go never passes a timestamp. "Date" and "time" from the spec are derived in queries
(`created_at::date`, `created_at::time`). Actor is stored as **both** `actor_id` (FK, canonical)
and `actor_email` (immutable snapshot, so analysis survives user renames/deletes and needs no join).

### 1. Migrations (apply with `psql -h localhost -d purchasing -f <file>`)

Highest existing migration is `033`. Add two, matching the `022_pr_recommendations.sql` style
(header comment block, aligned columns, inline FKs, `created_at DEFAULT NOW()`, trailing indexes).

**`backend/migrations/034_process_events.sql`** — append-only, PR-scoped:
```sql
CREATE TABLE process_events (
    id                  BIGSERIAL   PRIMARY KEY,
    purchase_request_id BIGINT      NOT NULL REFERENCES purchase_requests(id) ON DELETE CASCADE,
    action              TEXT        NOT NULL,                 -- fixed task constant (model.ProcessAction*)
    qualifier           TEXT        NOT NULL DEFAULT '',      -- approve|reject|revert|sign|unsign|received|approved|paid|quotation|recommendation|''
    actor_id            BIGINT      REFERENCES users(id) ON DELETE SET NULL,
    actor_email         TEXT        NOT NULL DEFAULT '',      -- snapshot of the actor at event time
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_process_events_pr    ON process_events(purchase_request_id, created_at);
CREATE INDEX idx_process_events_action ON process_events(action);
```

**`backend/migrations/035_audit_events.sql`** — append-only, entity-scoped:
```sql
CREATE TABLE audit_events (
    id          BIGSERIAL   PRIMARY KEY,
    action      TEXT        NOT NULL,                         -- fixed constant (model.AuditAction*)
    qualifier   TEXT        NOT NULL DEFAULT '',              -- role name for grant/revoke; active|inactive; ''
    entity_type TEXT        NOT NULL,                         -- vendor|cost_center|config_option|user|storage
    entity_id   BIGINT,                                       -- nullable (storage has no id)
    detail      TEXT        NOT NULL DEFAULT '',              -- human context, e.g. entity name
    actor_id    BIGINT      REFERENCES users(id) ON DELETE SET NULL,
    actor_email TEXT        NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_audit_events_entity ON audit_events(entity_type, entity_id);
CREATE INDEX idx_audit_events_action ON audit_events(action, created_at);
```
No DB `CHECK` on `action` (would force a migration per new task); the **Go constant set is the
enforcement** and the repo write validates against it. Remote DB: apply via the
`purchasing-db-deploy` skill.

### 2. Action constants — `backend/internal/model/events.go` (new)

`const` blocks (the fixed set) + a validation set. Enumerate exactly:

**Process actions** (with the qualifier each accepts):
| action | qualifier(s) | trigger |
|---|---|---|
| `submit_pr` | — | `Create` |
| `update_pr` | — | `Update` |
| `reject_pr` | — | `Reject` |
| `add_pr_approver` / `remove_pr_approver` | — | `AddApprover` / `RemoveApprover` |
| `pr_approval` | `approve` \| `reject` | `RecordApprovalDecision` |
| `rerequest_pr_approval` | — | `RequestApprovalAgain` |
| `create_recommendation` / `update_recommendation` / `delete_recommendation` | — | rec CRUD |
| `rec_approval_legal` / `rec_approval_security` / `rec_approval_budget` | `approve` \| `revert` | `SetRecApproval` / `ClearRecApproval` |
| `raise_rfi` / `clear_rfi` | — | `SetRecommendationRFI` / `DeleteRecommendationRFI` |
| `add_quotation` / `update_quotation` / `delete_quotation` | — | quotation CRUD |
| `select_quotation` | — | `Select` |
| `add_draft_contract` | `quotation` \| `recommendation` | `CreateFromQuotation` / `CreateRecommendationContract` |
| `update_contract` / `delete_contract` | — | `Update` / `DeleteRecommendationContract` |
| `sign_contract` | `sign` \| `unsign` | `UploadSignedDocument` / `DeleteSignedDocument` |
| `create_grn` / `update_grn` / `delete_grn` | — | GRN CRUD |
| `create_invoice` / `update_invoice` / `delete_invoice` | — | invoice CRUD |
| `invoice_status` | `received` \| `approved` \| `paid` | `SetStatus` (target status = qualifier) |

The `rec_approval_*` action is chosen from the URL `{type}` param (budget/legal/security).
Document-attachment ops (PR docs, draft-contract PDFs, RFI docs) are **excluded** from v1 as
sub-steps; can be added later if BPM analysis needs them.

**Audit actions**: `create_vendor`, `update_vendor`, `create_cost_center`,
`update_cost_center`, `create_config_option`, `update_config_option`, `delete_config_option`,
`create_user`, `grant_role` (qualifier=role), `revoke_role` (qualifier=role),
`set_user_active` (qualifier=`active`|`inactive`), `connect_storage`, `set_storage_folder`.

Also add `var validProcessActions`/`validAuditActions` sets used by the repo to reject an
unknown action (defensive; makes "fixed unless code changes" real).

### 3. Repository — `backend/internal/repository/events.go` (new)

Structs `ProcessEvent` / `AuditEvent` (json tags, `CreatedAt time.Time`), plus methods on
`*Repository` using the standard single-statement `QueryRow(...).Scan(...)` pattern (see
`AddOwnedDocument`, `AddRecComment`):
```go
func (r *Repository) AddProcessEvent(ctx, prID int64, action, qualifier string, actorID int64, actorEmail string) error
func (r *Repository) AddAuditEvent(ctx, action, qualifier, entityType string, entityID *int64, detail string, actorID int64, actorEmail string) error
```
Each validates `action` against the model set (returns an error the caller logs). No new
`Deps`/`main.go` wiring — these are methods on the existing injected `*Repository`.

### 4. Handler helpers — `backend/internal/handler/events.go` (new)

Free functions (multiple handler types record events), mirroring `reqLog`:
```go
func recordProcessEvent(r *http.Request, repo *repository.Repository, prID int64, action, qualifier string) {
    u := middleware.UserFromCtx(r.Context())
    var id int64; var email string
    if u != nil { id, email = u.ID, u.Email }
    if err := repo.AddProcessEvent(r.Context(), prID, action, qualifier, id, email); err != nil {
        reqLog(r).Error().Err(err).Str("action", action).Int64("pr_id", prID).Msg("record process event")
    }
}
func recordAuditEvent(r *http.Request, repo *repository.Repository, action, qualifier, entityType string, entityID *int64, detail string) { /* same shape */ }
```

### 5. Call sites (the bulk of the work)

Insert one `recordProcessEvent(...)` / `recordAuditEvent(...)` call right after each successful
mutation, before the response is written. ~35 process-event sites across
`purchase_requests.go`, `recommendations.go`, `quotations.go`, `contracts.go`, `grns.go`,
`invoices.go`; ~13 audit-event sites across `vendors.go`, `cost_centers.go`,
`config_options.go`, `users.go`, `storage.go`. PR id comes from the loaded entity
(`pr.ID`, `q.PurchaseRequestID`, `c.PurchaseRequestID`, `g.PurchaseRequestID`,
`inv.PurchaseRequestID`); qualifier from the branch (decision, target status, card type,
contract source).

Representative example (`RecordApprovalDecision`, purchase_requests.go):
```go
// after the decision is persisted, before reloadPR:
recordProcessEvent(r, h.Repo, pr.ID, model.ProcessActionPRApproval, decision) // decision = "approve"|"reject"
```

### 6. Docs

- New `docs/process-events.md` (as-built): the two tables, the fixed action/qualifier
  catalog, and the "human-actions-only, best-effort" rules.
- Update `CLAUDE.md` status section with a short "Audit & process events" bullet + migration
  `034`/`035` references.

## Verification

1. **Migrations**: `psql -h localhost -d purchasing -f backend/migrations/034_process_events.sql`
   and `…035…`; confirm tables/indexes with `\d process_events` / `\d audit_events`.
2. **Build/test**: `cd backend && go build ./... && go vet ./... && go test ./...`.
3. **Repo integration test** (extend `repository/*_integration_test.go` pattern): insert a
   process event + an audit event, read back, assert `action`/`qualifier`/`actor_email`/
   `created_at`; assert `AddProcessEvent` with an unknown action returns an error.
4. **End-to-end** (via `/run` or `/verify`): start backend, authenticate, then
   `POST /purchase-requests` (→ expect `submit_pr`), `POST …/{id}/quotations`
   (→ `add_quotation`), `POST …/{id}/approval` (→ `pr_approval`,`approve`), and a master-data
   op like `POST /vendors` (→ `create_vendor` in `audit_events`). Query:
   `SELECT purchase_request_id, action, qualifier, actor_email, created_at FROM process_events ORDER BY id;`
   and the same for `audit_events`. Confirm rows exist with correct actor + no rows for the
   auto-advance status transitions.
5. Grep check: every action string written by a handler exists in the `model` constant set.

## Out of scope (noted for later)

- Backfill of events for PRs that already exist (v1 starts recording from deploy).
- Read API / UI timeline (deferred per decision 4).
- Document-attachment process events.
- DB `CHECK` constraint on `action` (Go constants enforce instead).
