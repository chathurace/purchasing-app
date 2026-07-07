# Finance Role View (Phase 2) — Purchasing App

## Context

Phase 1 of `purchasing-app` shipped the **staff view**: staff create/edit purchase requests (PRs)
with items, links, and document uploads. The app already has OIDC auth, app-maintained roles
(`staff`, `finance`, `finance_admin`, `admin`), a Go/chi/pgx backend, and a React/Vite/TanStack
Query/Tailwind frontend.

This phase adds the **finance view** — the procurement workflow from `docs/Purchasing flow.pdf`,
covering PR → RFQ → quotation → contract → approvals → signing. The 11 requested pages stop at
contract signing; NetSuite PO, goods receipt (GRN), invoicing, and payment are explicitly **out of
scope** (future phases). The goal: let finance users turn a submitted PR into vetted vendor
quotations and an approved, signed contract, with full auditability and document attachments at
every stage.

### Confirmed decisions
1. **Vendors**: a reusable `vendors` table, referenced by quotations/contracts.
2. **Approvals**: an append-only list of approval records (approver, decision, comment, timestamp).
3. **Statuses auto-advance** as finance acts (e.g. creating an RFQ moves PR to `under_review`).
4. **Permissions**: any `finance` / `finance_admin` / `admin` user may perform **all** procurement
   actions. `staff` is unchanged (own PRs only).
5. **RFQ↔quotation**: a PR has many RFQs; an RFQ has many quotations; each quotation belongs to one
   RFQ and names one vendor. FKs: `quotation.rfq_id`, `rfq.purchase_request_id`.
6. **Quotation fields**: vendor, total amount, currency, validity date, notes, + optional line items
   (description, qty, unit price), + attached documents.
7. **Terminal PR status**: a **new `order_signed`** status. Signing a contract lands the PR on
   `order_signed`, reserving `completed` for the future payment phase.

### Guiding principle
Every RFQ/quotation/contract transitively belongs to exactly one PR, so we always resolve the owning
`purchase_request_id`. This lets us reuse `storage.LocalStore.Save(prID, …)` (path layout
`files/purchase-requests/<prID>/<uuid>/<name>`) unchanged for **all** document types, and reuse the
established repo/handler patterns (`replaceItemsLinks`, `load()`, `callerCanView`, compensating
`Storage.Delete` on DB failure).

---

## Backend

### Migrations (`backend/migrations/`, applied with `psql -h localhost -d purchasing -f <file>`)

- **`006_vendors.sql`** — `vendors(id, name, contact_name, email, phone, notes, created_by→users,
  created_at, updated_at)`. Index on `name`.
- **`007_rfqs.sql`** — `rfqs(id, purchase_request_id→purchase_requests ON DELETE CASCADE, title,
  details, status DEFAULT 'draft', created_by, created_at, updated_at)`. Indexes on
  `purchase_request_id`, `status`.
- **`008_quotations.sql`** — `quotations(id, rfq_id→rfqs ON DELETE CASCADE, vendor_id→vendors
  ON DELETE RESTRICT, total_amount NUMERIC(16,2), currency TEXT DEFAULT 'USD', valid_until DATE
  NULL, notes, status DEFAULT 'received', created_by, created_at, updated_at)` plus
  `quotation_items(id, quotation_id→quotations ON DELETE CASCADE, description, quantity
  NUMERIC(14,3), unit_price NUMERIC(16,2), position)` — mirrors `pr_items` so `replaceItemsLinks`
  applies verbatim.
- **`009_contracts.sql`** — `contracts(id, purchase_request_id→purchase_requests ON DELETE CASCADE
  [denormalized for path + PR advance], quotation_id→quotations ON DELETE SET NULL, vendor_id→vendors
  ON DELETE RESTRICT, title, total_amount, currency, terms, status DEFAULT 'draft',
  signed_document_id→documents ON DELETE SET NULL, created_by, created_at, updated_at)`.
- **`010_contract_approvals.sql`** — `contract_approvals(id, contract_id→contracts ON DELETE CASCADE,
  approver_id→users, decision TEXT ['approve'|'reject'], comment, created_at)`. Append-only; no
  unique constraint.
- **`011_documents_owner_and_pr_reject.sql`** — generalize the existing `documents` table (lowest
  friction; avoids 3 near-identical doc tables):
  - `ALTER TABLE documents ADD COLUMN owner_type TEXT NOT NULL DEFAULT 'purchase_request'`
    (`purchase_request|rfq|quotation|contract`), `ADD COLUMN owner_id BIGINT`.
  - Backfill: `UPDATE documents SET owner_id = purchase_request_id WHERE owner_id IS NULL`.
  - Index `(owner_type, owner_id)`. `purchase_request_id` stays `NOT NULL` so storage paths +
    PR-scoped cleanup keep working for every doc.
  - `ALTER TABLE purchase_requests ADD COLUMN rejection_reason TEXT NOT NULL DEFAULT ''` (so PR
    rejection doesn't clobber the requester's `comments`).

### Model (`backend/internal/model/`)
- Add status constants to `model.go`: RFQ (`draft|sent|responses_received|closed|cancelled`),
  Quotation (`received|under_evaluation|selected|rejected`), Contract
  (`draft|pending_approval|approved|rejected|signed`), decisions (`approve|reject`), and **new PR
  status `StatusOrderSigned = "order_signed"`**.
- New `transitions.go` with small pure guard functions (e.g. `NextPRStatusForRFQCreated(cur) (string,
  bool)`) so all auto-advance rules live in one place.

**Auto-advance rules** (each PR-advancing UPDATE is conditional — `WHERE status = $expected` — so
actions are idempotent and never move a PR backward):

| Action | Entity effect | PR effect |
|---|---|---|
| Create RFQ from PR | RFQ = `draft` | `submitted → under_review` |
| Create quotation on RFQ | quotation = `received`; RFQ → `responses_received` | — |
| Select quotation | quotation → `selected` | `under_review → vendor_selected` |
| Create contract from quotation | contract = `draft`; quotation → `selected` | `vendor_selected → contract_prepared` |
| Add approval (approve, no active reject) | contract → `approved` | — |
| Add approval (reject) | contract → `rejected` | — |
| Attach signed PDF (contract approved) | contract → `signed` | `contract_prepared → order_signed` |
| Reject PR (with comment) | open RFQs → `cancelled`, non-selected quotations → `rejected`, draft/pending contracts → `rejected` (signed contracts untouched) | → `rejected` |

### Access control (`backend/internal/middleware/auth.go`)
Add one helper:
```go
func HasFinanceAccess(ctx context.Context) bool {
    return HasRole(ctx, model.RoleFinance) || HasRole(ctx, model.RoleFinanceAdmin) || HasRole(ctx, model.RoleAdmin)
}
```
(`HasRole` already treats admin as wildcard.) Every procurement handler starts with a
`HasFinanceAccess` gate → 403 otherwise. Refactor the existing inline triple-checks in
`purchase_requests.go` (`List`, `callerCanView`) to use it.

### Repository (`backend/internal/repository/procurement.go`, same `*Repository` receiver)
New structs (`Vendor`, `RFQ`, `Quotation`, `QuotationItem`, `Contract`, `ContractApproval`) with
`pgtype` scanning for nullables, mirroring existing `Item`/`Document` style. Methods:
- Vendors: `CreateVendor`, `UpdateVendor`, `ListVendors`, `GetVendor`.
- RFQs: `CreateRFQ` (tx: insert + conditional PR advance), `UpdateRFQ`, `ListRFQs(prID *int64)`,
  `GetRFQ`, `DeleteRFQ` (returns owned-doc `stored_path`s to unlink after commit).
- Quotations: `CreateQuotation` (tx: insert + items + RFQ→`responses_received`), `UpdateQuotation`
  (tx: update + `replaceQuotationItems` — copy of `replaceItemsLinks`), `ListQuotations(rfqID
  *int64)`, `GetQuotation` (items + vendor + owning prID), `SelectQuotation` (tx: quote→selected,
  PR→vendor_selected), `DeleteQuotation`.
- Contracts: `CreateContractFromQuotation` (tx: resolve prID+vendor+currency from quotation, insert
  contract `draft`, quote→selected, PR→contract_prepared), `UpdateContract`, `ListContracts(prID
  *int64)`, `GetContract` (approvals + vendor + signed doc), `AddContractApproval` (tx: insert +
  recompute contract status), `SetContractSigned` (tx, guard contract=`approved`: contract→signed,
  PR→order_signed).
- PR: `RejectPurchaseRequest(ctx, id, comment, by)` (tx: PR→rejected + cascade-cancel children).
- Generic docs: `AddOwnedDocument`, `ListOwnedDocuments`, `GetOwnedDocument`, `DeleteOwnedDocument`
  (owner_type + owner_id scoped). Reimplement existing `AddDocument`/`ListDocuments`/etc. to delegate
  with `owner_type="purchase_request"`, keeping `purchase_requests.go` untouched.

### Handlers (new files mirroring `purchase_requests.go`; each with private `load()` + finance gate)
- `handler/vendors.go`, `handler/rfqs.go`, `handler/quotations.go`, `handler/contracts.go`.
- `handler/documents.go` — shared `uploadOwnedDoc(...)` helper extracting the multipart + extension
  allowlist + `Storage.Save(prID, …)` + record + rollback-`Delete` dance (reused by all entities).
- Extend `purchase_requests.go` with `Reject` only.

### Routes (`backend/internal/handler/router.go`, all inside the authenticated group)
- Vendors: `GET/POST /vendors`, `GET/PUT /vendors/{id}`.
- PR-scoped: `GET/POST /purchase-requests/{id}/rfqs`, `POST /purchase-requests/{id}/reject` (body
  `{comment}`).
- RFQs: `GET /rfqs`, `GET/PUT/DELETE /rfqs/{id}`, `GET/POST /rfqs/{id}/quotations`, RFQ documents
  (`POST` + `GET .../download` + `DELETE`).
- Quotations: `GET /quotations`, `GET/PUT/DELETE /quotations/{id}`, `POST /quotations/{id}/select`,
  `POST /quotations/{id}/contracts` (create contract from quotation), quotation documents.
- Contracts: `GET /contracts`, `GET/PUT /contracts/{id}`, `POST /contracts/{id}/approvals` (body
  `{decision, comment}`), `POST /contracts/{id}/signed-document` (multipart, pdf only), contract
  documents.

---

## Frontend

### Types & helpers (`frontend/src/types/api.ts`)
Add `Vendor`, `RFQ`/`RFQStatus`, `Quotation`/`QuotationItem`/`QuotationStatus`,
`Contract`/`ContractStatus`, `ContractApproval`, and `*Input` types. Add `"order_signed"` to
`PRStatus`. Add `rfqRef`/`quoRef`/`conRef` helpers next to `prRef`.

### API modules & hooks
- `api/vendors.ts`, `api/rfqs.ts`, `api/quotations.ts`, `api/contracts.ts` — thin `apiFetch` /
  `downloadFile` wrappers mirroring `api/purchaseRequests.ts` (incl. upload/download/delete document).
- `hooks/useVendors.ts`, `hooks/useRFQs.ts`+`useRFQ.ts`, `hooks/useQuotations.ts`+`useQuotation.ts`,
  `hooks/useContracts.ts`+`useContract.ts` — `useQuery` with `refetchInterval: 5000`. Mutations live
  in pages (matching existing detail-page pattern), invalidating both the entity key and
  `["purchase-requests", prId]` when an action auto-advances the PR.
- `hooks/useFinanceAccess.ts` — `me.roles` includes finance/finance_admin/admin. UX-only gate;
  backend enforces.

### Shared components (`frontend/src/components/`)
- `DocumentList.tsx` — extract the existing `DocumentRow` + upload-label block from
  `PurchaseRequestDetailPage` into a reusable component (callbacks for upload/download/delete), reused
  on RFQ/quotation/contract pages.
- `EntityStatusBadge.tsx` — generalize `StatusBadge` with label/color maps per entity kind (or keep
  `StatusBadge` for PR + add thin wrappers).
- `VendorSelect.tsx` — vendor dropdown with inline "+ new vendor".
- `QuotationFields.tsx` (vendor, total, currency, validity, notes, line items) and
  `ContractFields.tsx` (title, total, currency, terms) — controlled forms in the `RequestFields`
  style.
- `ApprovalList.tsx` — renders approvals + "Add approval" form (decision radio + comment).
- `formatMoney(amount, currency)` util — never sum across currencies.

### Pages → routes (under the authenticated `<Layout>` in `App.tsx`)
| # | Page | Route | Component |
|---|---|---|---|
| 1 | Login | `/login` | exists |
| 2 | List PRs (finance sees all — already supported) | `/requests` | exists |
| 3 | PR detail + create RFQs + reject | `/requests/:id` | **extend** `PurchaseRequestDetailPage` (finance-gated RFQ list, "Create RFQ", "Reject with comment") |
| 4 | List RFQs | `/rfqs` | `RFQListPage` |
| 5 | RFQ page (associate quotations) | `/rfqs/:id` | `RFQDetailPage` |
| 6 | Create quotation | `/rfqs/:id/quotations/new` | `NewQuotationPage` |
| 7 | List quotations | `/quotations` | `QuotationListPage` |
| 8 | Edit quotation + create contract | `/quotations/:id` | `QuotationDetailPage` |
| 9 | Create contract | `/quotations/:id/contracts/new` | `NewContractPage` |
| 10 | List contracts | `/contracts` | `ContractListPage` |
| 11 | View/edit contract + approvals + signed PDF | `/contracts/:id` | `ContractDetailPage` |

### Layout (`frontend/src/components/Layout.tsx`)
Add finance-gated nav links (`useFinanceAccess`): **RFQs**, **Quotations**, **Contracts**.
"Requests" stays visible to all. Optionally a `RequireRole` wrapper redirects non-finance users from
finance-only routes (backend still enforces).

---

## Key edge cases (handled in the design)
- **Document cleanup on delete**: docs are keyed on PR, not the entity, so they don't cascade.
  `DeleteRFQ`/`DeleteQuotation` must select owned docs, delete rows in-tx, and `Storage.Delete` the
  files after commit (DB-first ordering). Forbid deleting an RFQ with quotations / a quotation with a
  contract → 409.
- **Concurrency** (5s refetch + multiple finance users): all PR advances are conditional UPDATEs;
  only ever move forward.
- **PR rejection** requires a non-empty comment (stored in new `rejection_reason`); cascades to open
  children but never touches a signed contract.
- **Currency**: free 3-letter code, `NUMERIC` only, no cross-currency math; contract copies the
  quotation's currency.
- **Vendor in use**: `ON DELETE RESTRICT` → surface 409, not 500.
- **Multi-write actions** (`CreateContractFromQuotation`, `SetContractSigned`) run in a single tx;
  doc upload + record reuses the proven compensating-delete pattern.

---

## Verification (end-to-end, local)
1. **Migrations**: `for f in backend/migrations/0{06,07,08,09,10,11}*.sql; do psql -h localhost -d
   purchasing -f "$f"; done`. Confirm new tables via `\dt` and `\d documents` shows
   `owner_type`/`owner_id`.
2. **Backend**: `cd backend && go build ./... && go run ./cmd/server`. Add procurement repo tests
   mirroring the existing repository integration tests.
3. **Frontend**: `cd frontend && npm install && npm run dev` → http://localhost:5173.
4. **Roles**: log in as `bootstrap_admin.email` (auto-admin) for all finance actions; a second SSO
   user (auto-`staff`) granted `finance` confirms gating.
5. **Click-through (the 11 pages)**:
   - Open a `submitted` PR → "Create RFQ" → PR badge flips to `under_review`.
   - `/rfqs` → open RFQ → "Add quotation" (pick/create vendor, totals, currency, validity, line
     items, attach PDF) → RFQ → `responses_received`.
   - `/quotations` → open → edit; "Select" → PR `vendor_selected`; "Create contract" → PR
     `contract_prepared`.
   - `/contracts` → open → add `approve` approval (→ `approved`); attach signed PDF → contract
     `signed`, **PR → `order_signed`**.
   - Negative: `staff` gets 403/redirect on `/rfqs|/quotations|/contracts`; reject a PR with a
     comment → child RFQs/quotations move to cancelled/rejected; delete a referenced vendor → 409.
6. **Storage**: confirm RFQ/quotation/contract files land under
   `files/purchase-requests/<prID>/<uuid>/<name>` and download with correct `Content-Disposition`.

## Plan archival (per CLAUDE.md convention)
Before implementing, copy this plan to `purchasing-app/docs/plans/02-finance-view.md`.
