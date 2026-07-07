# Contracts (as-built)

The contract is a **document container** attached to a purchase request: a set of
draft-contract PDFs and a single signed-contract PDF, each with its own notes.
Contract review/approval was removed (migration `032`) — approvals happen only on
the PR, via the procurement recommendation's approval cards.

## Model

- `contracts` row (migration `009`): `purchase_request_id`, `quotation_id` (origin,
  nullable), `vendor_id`, `title`, `total_amount`, `currency`, `status`,
  `signed_document_id`. `terms` still exists on the row but is no longer surfaced
  in the UI (per-document `notes` replaced it).
- **Documents** live in the shared `documents` table with `owner_type = 'contract'`,
  `owner_id = contract.id`, and a `notes` column (migration `031`). The signed PDF
  is the document pointed to by `contracts.signed_document_id`; every *other*
  contract-owned document is a **draft contract**. `GetContract` splits them:
  `documents` = drafts, `signed_document` = the signed PDF (both carry `notes`).

## Status

Only two live states (plus `rejected`, reached when the owning PR is rejected):

- **`draft`** — the default; the contract has zero or more draft PDFs and no signed PDF.
- **`signed`** — set when a signed PDF is attached; reverts to `draft` if it is removed.

Signing advances the PR to `order_signed`; removing the signed PDF rewinds the PR to
`contract_prepared` (`SetContractSigned` / `ClearContractSigned` in
`repository/procurement.go`). Removing the signed PDF is refused if the contract
already has GRNs or invoices.

## Endpoints (`handler/contracts.go`, router under `/api/v1`)

- `GET  /contracts`, `GET /contracts/{id}` — list / detail.
- `POST /quotations/{id}/contracts` — draft a contract from a quotation
  (`CreateFromQuotation`, **gated** on the recommendation being fully approved).
- Draft PDFs: `POST /contracts/{id}/documents` (PDF + optional `notes`),
  `PUT /contracts/{id}/documents/{docID}` (edit `notes`),
  `GET .../download`, `DELETE .../{docID}`.
- Signed PDF: `POST /contracts/{id}/signed-document` (PDF + optional `notes`;
  allowed from `draft` or `signed` — re-upload **replaces** and deletes the old
  file), `DELETE /contracts/{id}/signed-document` (revert to `draft`).

The recommendation **contract card** creates/links a contract without the approval
gate — see the "Procurement recommendation" entry in `CLAUDE.md` (migration `029`,
`pr_recommendations.contract_id`; `CreateRecommendationContract` /
`DeleteRecommendationContract`).

## Frontend

- `components/ContractContent.tsx` — the shared **draft contracts** + **signed
  contract** UI (upload PDF + notes, edit notes, remove). Used by both the contract
  page and the recommendation contract card so they behave identically.
- `pages/ContractDetailPage.tsx` — header (vendor, amount, status) + `ContractContent`
  + fulfillment (GRNs/invoices once signed). No review/approval section.
- `components/RecommendationSection.tsx` — the contract card wraps `ContractContent`
  in an elevated sub-card; the first draft upload creates the contract.

## Fulfillment

Unchanged (see `docs/plans/03-invoices-grns.md`): once `signed`, finance records GRNs
and invoices against the contract.
