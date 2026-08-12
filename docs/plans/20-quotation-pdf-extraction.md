# Plan 20 — Quotation PDF extraction (Claude)

Extract vendor, currency, total, validity and line items from an uploaded quotation
PDF using the Anthropic API, and surface the result as **editable suggestions** the
procurement user confirms — never as a silent write.

## Why

The inline "add quotation" form on the PR page collects vendor + description + the
initial PDF, then **hardcodes the interesting fields**
(`PurchaseRequestDetailPage.tsx` → `createMutation`):

```ts
total_amount: 0, currency: "USD", valid_until: null, items: []
```

So every new quotation starts as a stub, and total/currency/validity/line items only
get filled in if someone later opens the quotation detail page and retypes what is
already written in the PDF. That is the gap this closes.

## Shape of the solution

One Claude call per PDF: read the blob through the existing `storage.Store.Open()`,
base64 it into a `document` content block, and constrain the reply with **structured
outputs** (`output_config.format` + a JSON schema). Result lands in a **staging
table**, never directly in `quotations`.

Two entry points, sharing all the machinery:

1. **Pre-create (primary).** Drop the PDF on the PR page → extraction runs → the
   create form comes back pre-filled and fully editable → one `CreateQuotation` with
   real values. The already-stored blob is *adopted* into the initial-PDF slot, so
   the client never uploads the same bytes twice.
2. **Post-create.** An "Extract from PDF" button on the quotation card, for
   quotations created by hand and for final (post-negotiation) PDFs that change the
   numbers. Review → Apply → `UpdateQuotation`.

### Trust rules (the part that matters)

- **Nothing is auto-applied.** Extraction writes to `quotation_extractions`; a human
  applies it.
- **Vendor is a name, not an ID.** The model returns a string; the backend
  ranks candidates from `vendors` and the user picks. Letting the model choose a
  `vendor_id` is how a quote ends up attached to the wrong company.
- **Every field is nullable**, and the prompt says omit rather than guess. A missing
  total is recoverable; a hallucinated one is not.
- **Reconcile the total against the line items** — reusing the existing invoice
  `entered_total` pattern: if `sum(qty × unit_price) ≠ total_amount`, warn
  (non-blocking) and let the extracted total win.
- Disabled by default (`anthropic.enabled: false`), so dev and CI need no key. The
  key stays server-side; the browser never sees it.

## Backend

### Migration `049_quotation_extractions.sql`

```
quotation_extractions(
  id, purchase_request_id → purchase_requests (ON DELETE CASCADE),
  document_id → documents (ON DELETE CASCADE),
  quotation_id → quotations (ON DELETE SET NULL),   -- null for pre-create
  status text,          -- pending | succeeded | failed
  model text, raw_json jsonb, error_message text,
  input_tokens int, output_tokens int,
  created_at, created_by, applied_at, applied_by
)
```

Unique partial index on `document_id` where not failed → re-extraction is idempotent.

### `internal/extraction` (new package)

Mirrors `internal/directory`: config-gated, `Enabled()` false ⇒ no-op.

- `Service.Extract(ctx, filename string, pdf []byte) (*Result, error)`
- Model `claude-opus-5`, `thinking: {type: "adaptive"}`, structured output schema.
- Streaming (`Messages.NewStreaming`) — extraction runs can be slow and a large
  `max_tokens` on a non-streaming call risks an HTTP timeout.
- Prompt-caches the system prompt + schema; the PDF goes after the breakpoint.
- Returns `Result{Suggestion, Model, InputTokens, OutputTokens, RawJSON}`.

`Suggestion` fields: `vendor_name`, `currency`, `total_amount`, `valid_until`,
`quote_reference`, `quote_date`, `items[]{description, quantity, unit_price}`,
`confidence`, `notes`.

### Config

New `anthropic:` block (`enabled` false default, `api_key`, `model`,
`max_pdf_bytes`, `timeout_seconds`), validated only when enabled.

### Repository

- `CreateExtraction` / `GetExtraction` / `GetExtractionForDocument`
- `FinishExtraction(id, status, rawJSON, tokens, errMsg)`
- `MarkExtractionApplied(id, userID)`
- `MatchVendors(ctx, name)` — ranked candidates for the extracted vendor name.

### Handlers + routes

```
POST /api/v1/purchase-requests/{id}/quotation-extractions   (multipart file)
     → stores blob under the PR dir as an orphan doc, extracts, returns
       { extraction, suggestion, vendor_matches }
GET  /api/v1/quotation-extractions/{id}
POST /api/v1/quotations/{id}/extract?slot=initial|final
```

`POST /purchase-requests/{id}/quotations` gains an optional
`extraction_id`: on create the staged document is adopted into the initial slot via
the existing `SetQuotationDocument`, and the extraction row is marked applied.

Gated exactly like the rest of procurement work: `HasProcurementAccess` +
team-lead-approval 409 + `assignmentWorkGate`. New process event
`extract_quotation` (qualifier `initial`/`final`) added to `ValidProcessActions`.

## Frontend

- `api/quotationExtractions.ts` + types.
- `QuotationExtractionReview` component: renders the suggestion as an editable
  form (vendor picker seeded with matches, currency, total, validity, line-item
  table), with confidence and the total-mismatch warning.
- PR-page create form: file input triggers extraction, review panel replaces the
  bare vendor+description form, Submit posts `extraction_id`.
- Quotation card: "Extract from PDF" per PDF slot when one is attached.
- Everything is hidden when `GET /me`-adjacent capability flag says extraction is
  disabled.

## Verification

The four PDFs in `resources/files/` are the eval set — note
`SUSE - Patrocínio FEBRABAN 26.pdf` is Portuguese and likely BRL, which exercises
non-USD currency and `1.234,56` decimal formatting.

## Out of scope

- Citations / page provenance (`citations: {enabled: true}` is incompatible with
  `output_config.format` — returns 400). Revisit with a second call if needed.
- Auto-extraction on upload. The plumbing supports it; the trigger stays manual.
- Batch/queue infrastructure. Extraction is request-scoped.
