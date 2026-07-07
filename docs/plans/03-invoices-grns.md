# Phase 3 — Invoices & GRNs (contract fulfillment)

Once a contract is **signed**, finance users can record one or more **Goods Received
Notes (GRNs)** and **Invoices** against it. This is the fulfillment layer that sits
below the procurement chain (PR → RFQ → Quotation → Contract); GRNs and invoices are
children of a contract, not new steps in that linear chain.

## Decisions (from the requester)

- **Line items** on both GRNs and invoices (GRN lines = qty received; invoice lines = qty × unit price).
- **Invoice lifecycle**: `received → approved → paid` (revertible one step each way).
- **Over-billing**: warn, don't block. The contract exposes `invoiced_total`; the UI shows
  remaining-to-invoice and flags when invoices exceed the contract total.
- **PR status unchanged**: stays at `order_signed` (this phase does not use the reserved
  `completed` status — that remains for a future payment phase).

## Data model (migration `014_grns_invoices.sql`)

- `grns` — contract_id, purchase_request_id + vendor_id (denormalized, like contracts),
  received_date, received_by (free text), note, created_by, timestamps. No status (a GRN is
  a receipt record). `grn_items` (description, quantity, position).
- `invoices` — same denormalized keys, vendor_invoice_no, invoice_date, due_date,
  total_amount (derived server-side from items), currency, status, note, approved_by/at,
  paid_date, created_by, timestamps. `invoice_items` (description, quantity, unit_price, position).
- Documents reuse the shared `documents` table via new owner types `grn` / `invoice`;
  files still live under the owning PR's storage dir.

## Backend

- `model`: invoice statuses, `ValidInvoiceTransition`, owner-type constants.
- `repository/fulfillment.go`: GRN + Invoice CRUD, item-replace (mirrors quotations),
  `SetInvoiceStatus`, `contractInvoicedTotal`. Create gates on contract `signed`
  (else `ErrInvalidState` → 409). `GetContract` now populates `invoiced_total`.
- `handler/grns.go`, `handler/invoices.go`: finance-gated; create/list nested under
  `/contracts/{id}`, entity ops at top level; invoice status setter validates transitions.

## Frontend

- types/api: GRN, Invoice (+ items, inputs, statuses), `grnRef`/`invRef`, invoice status badge.
- api + hooks per entity; list pages + nav; New + Detail pages (line items, documents,
  invoice approve / mark-paid actions).
- Contract detail page gains **Goods received** and **Invoices** sections (only once signed),
  with the invoiced-total summary + over-billing warning.
