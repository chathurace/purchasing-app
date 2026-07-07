# Related documents for GRNs & Invoices

## Context

The purchasing app has a "related documents" feature that cross-links every record in a
procurement case. Today it covers the **PR → RFQ → Quotation → Contract** chain via three pieces
on each detail page:

- `ChainStepper` — the breadcrumb chain (Request ▸ RFQ ▸ Quotation ▸ Contract)
- `DirectParentCard` — the immediate upstream record
- `RelatedDocuments` — indirect ancestors, deeper descendants and siblings

All three are driven by one backend bundle, `GET /api/v1/purchase-requests/{id}/related`
(`relatedDocuments` struct), and the in-memory graph logic in
[caseGraph.ts](purchasing-app/frontend/src/lib/caseGraph.ts).

**GRNs and invoices are missing from this graph.** They hang off a signed contract (Phase 3),
are shown only in the contract page's `ContractFulfillment` section, and have no related-documents
view of their own. This change extends the case graph so GRNs/invoices participate fully:

- GRN and invoice detail pages get the stepper, direct-parent card, and related-documents section.
- The GRNs/invoices of a case are also surfaced as related-document groups on the **PR, RFQ and
  quotation** pages (per confirmed scope). The **contract** page is left as-is — its existing
  `ContractFulfillment` section already lists them, so no duplication there.

GRNs/invoices already carry a denormalized `purchase_request_id`, so the existing PR-anchored
`/related` endpoint is the right place to add them — no new endpoint or migration is needed.

## Backend

### 1. Repository — PR-scoped list methods
[backend/internal/repository/fulfillment.go](purchasing-app/backend/internal/repository/fulfillment.go)

Add two methods mirroring the existing `ListGRNs` / `ListInvoices` (which scope by `contractID`),
but filtering on `purchase_request_id`:

- `ListGRNsForPR(ctx, prID int64) ([]*GRN, error)` — same SELECT as `ListGRNs`, `WHERE g.purchase_request_id = $1`
- `ListInvoicesForPR(ctx, prID int64) ([]*Invoice, error)` — same SELECT as `ListInvoices`, `WHERE i.purchase_request_id = $1`

These return summaries (vendor name, dates, totals, status) — enough for cross-links.

### 2. Handler — extend the bundle
[backend/internal/handler/purchase_requests.go](purchasing-app/backend/internal/handler/purchase_requests.go) (`relatedDocuments`, `Related`)

- Add `GRNs []*repository.GRN \`json:"grns"\`` and `Invoices []*repository.Invoice \`json:"invoices"\`` to the `relatedDocuments` struct.
- In `Related()`, after loading contracts, call the two new methods and include them in the response. Stays finance-gated as today.

## Frontend types
[frontend/src/types/api.ts](purchasing-app/frontend/src/types/api.ts)

Add to the `RelatedDocuments` interface:
```ts
grns: GRN[];
invoices: Invoice[];
```

## Frontend graph logic
[frontend/src/lib/caseGraph.ts](purchasing-app/frontend/src/lib/caseGraph.ts)

- **`CaseKind`**: add `"grn" | "invoice"`. **`CaseStatus`**: add `InvoiceStatus`.
- **`CaseRecord.status`**: make optional (`status?`) — GRNs have no status.
- **Record builders**: add `grnRecord(g)` → `{kind:"grn", ref:grnRef(id), to:/grns/${id}, detail: vendor·received-date}` (no status); `invRecord(inv)` → `{kind:"invoice", ref:invRef(id), to:/invoices/${id}, detail: vendor·total, status: inv.status}`.
- **Lineage helper**: add a small `contractLineage(data, contractId)` returning `{contract, quotation, rfq}` (walk contract → `quotation_id` → quotation → `rfq_id` → rfq). Reuse it in `directParent`, `relatedGroups`, and `resolveLineage`.
- **`directParent`**: `grn`/`invoice` → the contract (`conRecord`), labels e.g. "Received against" / "Billed against"; fall back to PR if contract missing.
- **`resolveLineage` / `buildSteps`** (stepper): treat `grn`/`invoice` as a 5th leaf node after Contract.
  - `STEP_LABELS`: add `grn:"GRN"`, `invoice:"Invoice"`.
  - When `current.kind` is `grn`/`invoice`: resolve `conId` from the leaf, then derive `quoId`/`rfqId` via `contractLineage`; treat Contract (and PR/RFQ/Quo) as clickable **ancestors** and render the GRN/Invoice as the **current** 5th step. Contract pages are unchanged (no 5th node added for `contract`).
- **`relatedGroups`**:
  - `grn` (contract is the direct parent, shown in the card): groups = Purchase request, Request for quotation, Quotation (each if present), "Other goods received notes for this contract" (other GRNs, same `contract_id`), "Invoices for this contract".
  - `invoice`: symmetric — Purchase request, RFQ, Quotation, "Goods received notes for this contract", "Other invoices for this contract".
  - `pr`: append groups "Goods received (GRNs)" = all `data.grns`, "Invoices" = all `data.invoices`.
  - `rfq`: compute the contracts resulting from this RFQ (existing logic), then append GRNs/invoices whose `contract_id` is in that set.
  - `quotation`: compute contracts where `quotation_id === current.id`, then append their GRNs/invoices.
  - `contract`: **unchanged** (ContractFulfillment already lists them).

## Frontend rendering
[frontend/src/components/RecordCard.tsx](purchasing-app/frontend/src/components/RecordCard.tsx) (`RecordRow`)

Handle the new kinds: render no badge when `record.status` is undefined (GRN); route `invoice`
status through the existing `EntityStatusBadge` (it already includes `InvoiceStatus`/`paid`).

## Frontend pages

[frontend/src/pages/GrnDetailPage.tsx](purchasing-app/frontend/src/pages/GrnDetailPage.tsx) and
[frontend/src/pages/InvoiceDetailPage.tsx](purchasing-app/frontend/src/pages/InvoiceDetailPage.tsx),
following the `ContractDetailPage` pattern:

- Add `<ChainStepper prId={entity.purchase_request_id} current={{kind, id}} />` after the breadcrumb.
- Add `<DirectParentCard prId={...} current={...} />` after the main details card and **remove the
  now-redundant inline "Contract" `<Field>`** (the direct-parent card replaces it).
- Add `<RelatedDocuments prId={...} current={...} />` at the bottom.

No routing/hook changes: the components reuse `useRelatedDocuments(prId)`, and both entities already
expose `purchase_request_id`. All three components are finance-gated, which matches fulfillment access.

## Verification

1. **Build**: `cd backend && go build ./...`; `cd frontend && npx tsc --noEmit && npm run build`.
2. **Run** (per CLAUDE.md): start backend `go run ./cmd/server`, frontend `npm run dev`.
3. **Manual** (as a finance user, on a case that has a signed contract with ≥1 GRN and ≥1 invoice):
   - Open a **GRN** page: stepper shows Request ▸ RFQ ▸ Quotation ▸ Contract ▸ GRN (current); a
     "Received against" card links the contract; Related documents lists PR/RFQ/quotation and the
     sibling GRNs/invoices. Same for an **invoice** page.
   - Open the **PR / RFQ / quotation** pages: Related documents now include the case's GRNs/invoices.
   - Open the **contract** page: unchanged — GRNs/invoices appear only in the fulfillment section.
   - Cross-links navigate correctly; a contract with no quotation still resolves (PR fallback).
4. Confirm `GET /api/v1/purchase-requests/{id}/related` returns the new `grns`/`invoices` arrays.
