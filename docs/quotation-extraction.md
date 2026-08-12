# Quotation PDF extraction (Claude)

As-built reference. Plan: `docs/plans/20-quotation-pdf-extraction.md`.

Reads the commercial details out of a vendor's quotation PDF — vendor, currency, the
totals block (subtotal, discount, each tax line, shipping and other charges, grand
total), validity and line items — and presents them as **editable suggestions** a
procurement user confirms. Nothing is ever written onto a quotation automatically.

## Why it exists

The PR page's add-quotation form collected vendor + description + PDF and hardcoded
the rest (`total_amount: 0, currency: "USD", valid_until: null, items: []`), so every
new quotation started as a stub and the real numbers were only filled in if someone
later retyped them from the PDF on the quotation detail page.

## Configuration

Off by default — dev and CI need no API key, and the endpoints report unavailable
rather than failing.

```yaml
anthropic:
  enabled: false
  api_key: "<anthropic-api-key>"
  model: "claude-opus-5"       # default
  max_pdf_bytes: 20971520      # 20MB; the API caps a request at 32MB
  timeout_seconds: 180
```

`anthropic.api_key` is required when `enabled: true` (validated at load). The key is
used server-side only — the browser never sees it. `GET /quotation-extractions/status`
exposes only `{enabled, model, max_pdf_bytes}` so the UI can hide the feature.

## The two flows

Both share the same extraction call, staging table, and review UI.

**Pre-create (primary).** Drop the PDF on the PR page → "Read details from PDF" →
the add-quotation form is replaced by a pre-filled, fully editable review panel →
Submit issues **one** `CreateQuotation` with real values. The PDF was already stored
by the extraction call and is *adopted* into the new quotation's initial-PDF slot, so
the client never uploads the same bytes twice.

**Post-create.** Uploading a PDF into either slot of an existing quotation card
**reads it immediately** and opens the editable review inside that slot — the point of
attaching a quotation PDF is the numbers in it, so a separate "Read details" click was
busywork. Apply issues an `UpdateQuotation`; Discard leaves the stored read visible in
the slot's read-only panel. "Read details" remains for PDFs attached before this
existed, and **disables itself once that PDF has been read** (a re-read of the same
bytes costs a call and tells you nothing new). Replacing the PDF creates a new
document, which has no extraction, so the button re-enables on its own.

## What is shown, and where

A result is displayed **per document, not per quotation**. A quotation's initial and
final PDF are separate documents with separate extractions, and their figures and
line items legitimately differ — that is the point of a final, post-negotiation
quote — so collapsing them into one view would misrepresent one of them.

On the PR page each PDF slot therefore carries its own **"Read from this PDF"** panel
(`components/ExtractedQuotationDetails.tsx`), collapsed to a summary line (vendor ·
total · confidence · applied/not applied) and expanding to the full read: the field
grid, the line-item table, and an invoice-style totals block. The editable review
panel (`QuotationExtractionReview.tsx`) shares those presentational pieces, so
reviewing and reading back look the same.

This replaced the card's old **"More details" expander**, which showed one
card-level total / validity / line-item list fetched via `GetQuotation`. With figures
that differ per PDF, a single summary could only ever be right about one of them. The
card header now links to `/quotations/{id}`, which is where the stored quotation
record — including any **other documents** beyond the two primaries — is reachable.

**The tax breakdown lives with the extraction, not on the quotation.** A `quotations`
row stores a single `total_amount` / `currency` / `valid_until` / line items, so only
the grand total is editable in the review panel and only it is applied. Subtotal,
discount, tax lines and other charges stay in the extraction's `raw_json`, where they
remain visible per PDF indefinitely. This is deliberate rather than a gap: those
figures are a property of *a document*, and the two documents disagree. Adding
columns for them on `quotations` would force one document's breakdown to win.

`applied_at` is what tells a reader which PDF's numbers the quotation is actually
carrying — the panel shows it as an `applied` / `not applied` chip. While a read is
awaiting approval the editable panel replaces the read-only one in that slot: two
views of the same PDF side by side only invites confusion about which is live.

Two rules keep that chip honest, both of them regression-tested:

- **Both apply paths stamp it.** Create stamps it while adopting the staged PDF
  (`adoptExtractionDocument`); `UpdateQuotation` stamps it when the body carries an
  `extraction_id` (`markExtractionApplied`, which rejects an extraction belonging to
  another PR). Update used to ignore the field, so anything applied from a card slot
  read "not applied" forever — see `TestUpdateQuotationMarksExtractionApplied`.
- **At most one per quotation.** `MarkExtractionApplied` clears the stamp on the
  quotation's other extractions, because a quotation holds a single total: applying
  the final PDF's figures genuinely un-applies the initial PDF's. Without this both
  slots would claim "applied" and the chip would stop answering its one question —
  see `TestApplyingOneExtractionUnappliesTheOther`.

Both stamps are **best-effort**: the quotation write has already succeeded, so a
failure to stamp is logged rather than turned into an error the user can't act on.

## The total is always tax-inclusive

`lib/extractionTotals.ts` holds this arithmetic — pure functions, shared by both
panels, so the figure shown, the figure warned about and the figure saved cannot
drift apart. A quotation stores one total, and it is the amount actually payable:

- Most quotations print a tax-inclusive grand total, which is used unchanged.
- `total_includes_tax: false` ("plus taxes", "tax extra") → tax is **added**.
- The document didn't say, but its printed total equals a pre-tax base (the subtotal,
  or the subtotal less the discount) → tax is **added**; arithmetic settles what the
  wording didn't.
- No grand total printed at all → **derived** as subtotal − discount + tax + charges.
- Anything else keeps the printed total. Adding tax on top of an explicit grand total
  would overstate the commitment, which is the more expensive way to be wrong.

Whenever the figure offered isn't the figure printed, the totals block says so in
words underneath it ("43.20 EUR tax added to the printed 240.00 EUR — …"). A silent
adjustment to a number someone is about to commit to would be worse than none.

The line-item cross-check is made **at the same level as the total** — items less the
discount, plus taxes and charges (`reconciledTotal`). Comparing a tax-inclusive total
against a pre-tax item sum would flag every taxed quotation as a mismatch; with no
discount, tax or charges it reduces to the plain item sum, as before.

## Endpoints

| Method | Path | Notes |
|---|---|---|
| `GET` | `/api/v1/quotation-extractions/status` | Any authenticated user; drives UI gating |
| `POST` | `/api/v1/purchase-requests/{id}/quotation-extractions` | multipart `file`; stages + extracts before a quotation exists |
| `POST` | `/api/v1/quotations/{id}/extract?slot=initial\|final` | Re-reads an attached PDF |
| `GET` | `/api/v1/quotation-extractions/{id}` | Re-fetch a staged result |
| `GET` | `/api/v1/purchase-requests/{id}/quotation-extractions` | Every succeeded result on the PR, one per document; drives the per-slot panels |

The list read is gated like `ListForPR` (procurement access) and skips vendor
ranking — matches exist to help *pick* a vendor while applying, and ranking every
extraction on the page would be a query each. The frontend fetches it once per PR
(`usePRExtractions`, key `["quotation-extractions","pr",id]`) and each card looks up
its two slots by `document_id`.

`POST /purchase-requests/{id}/quotations` gained an optional `extraction_id`, which
triggers the adoption described above.

Gated exactly like the rest of procurement work: `HasProcurementAccess`, the
team-lead-approval 409, and `assignmentWorkGate`. A successful extraction records
process event `extract_quotation` (qualifier `initial` / `final` / `staged`) and
auto-enrols the actor as a collaborator.

## Data model (migration `049`)

`quotation_extractions` stages every attempt — it is the audit trail and the reason
re-extraction is cheap and idempotent:

- `quotation_id` is **null** in the pre-create flow; set on adoption.
- `raw_json` holds the model's validated output verbatim; `input_tokens` /
  `output_tokens` give cost visibility.
- `status` is `pending` → `succeeded` | `failed`. A unique partial index on
  `document_id` **where status <> 'failed'** means re-running against the same PDF
  replaces the live row, while failures are retained as a record.
- `applied_at` / `applied_by` stamp the human confirmation.

The staged PDF is an ordinary `documents` row under the PR's storage directory, with
owner type **`quotation_extraction`** — a staging state, not a resting place. On
adoption `SetDocumentOwner` re-points it at the quotation and
`SetQuotationDocument` fills the initial slot; the stored bytes never move.

## Trust rules

These are the load-bearing design decisions, not incidental details.

- **Nothing is auto-applied.** Extraction writes to the staging table; a human
  applies it.
- **Vendor is a name, not an ID.** The model returns a string; `MatchVendors` ranks
  candidates from `vendors` (100 exact / 80 prefix / 60 substring / 40 shared
  significant word, after normalising case, punctuation and legal forms like
  `Ltd`/`Ltda`/`GmbH`). The UI preselects only on a score ≥ 80 and otherwise makes
  the user choose — attaching a quote to the wrong company is worse than asking.
  When **nothing** ranks, the vendor has to be created before the quotation can be
  saved, so `VendorSelect` opens its new-vendor form with the extracted name already
  in it (`suggestedName` / `defaultAdding`) — one click instead of retyping a name we
  just read. It stays a deliberate act, and while the field still holds exactly what
  was read the form says so: a model can pick the *buyer's* name off the page, and
  this creates a real vendor record.
- **Omitted means omitted.** Every field is nullable and the prompt says omit rather
  than guess. A missing total surfaces as blank, never as `0`; the review panel lists
  what the document didn't state.
- **The total is cross-checked against the line items** — the same non-blocking
  mismatch warning invoices use for `entered_total`. The stated total wins.
- **Split taxes stay split.** `taxes` is a list, not a field: CGST + SGST, or ICMS +
  PIS + COFINS, are separate rows and summing them would destroy the reviewer's
  ability to reconcile against the document. A row with a rate but no printed amount
  is kept (that is real information); a row with neither is dropped.
- **"No tax stated" ≠ "tax of 0.00".** An absent tax section yields an empty list and
  a null `total_includes_tax`, and the panel says so in words, rather than implying a
  zero-rated quote. `total_includes_tax` is read from what the document *says*, not
  inferred from arithmetic.
- **Refusals and truncation are surfaced, not swallowed.** `stop_reason` is checked
  before reading content, so a safety refusal or a `max_tokens` cut becomes a clear
  message instead of an empty suggestion.

## Model call

`internal/extraction` mirrors `internal/directory`: config-gated, `Enabled()` false
⇒ `ErrDisabled`. One streaming request per PDF:

- `claude-opus-5`, adaptive thinking, `output_config.format` with a JSON schema
  (`additionalProperties: false`, every field nullable).
- Streaming, because extraction on a long quotation can run a while and a large
  `max_tokens` on a non-streaming call risks an HTTP timeout.
- The system prompt is the cached prefix (`cache_control: ephemeral`) and must stay
  byte-stable — it interpolates nothing per request. The PDF follows it.
- The prompt is explicit about the failure modes that matter commercially: the vendor
  is the *issuer* (the buyer's own name is on the page too), `1.234,56` is European
  formatting, ambiguous `05/03/2026` dates should be omitted rather than guessed.

## Verification

`internal/extraction/extraction_test.go` drives the real SDK against a fake Messages
endpoint (`Config.BaseURL`), asserting the request shape (document block, base64
round-trip, schema attached, adaptive thinking with no `budget_tokens`, cached system
block) and the response handling (normalisation, nil-preservation, refusal,
truncation, malformed JSON). `extractions_integration_test.go` exercises the SQL
against the real schema: stage → finish → adopt, the re-run/replace behaviour, and
the guards that stop an extraction being adopted twice or across PRs.

One schema constraint is load-bearing and easy to reintroduce: the `confidence`
property carries **`enum` with no `type`**. The API's schema validator checks each
enum value against a *single* declared type, so pairing an enum with the
`["string","null"]` union that every other field uses is rejected with
`400 Enum value 'high' does not match declared type '["string","null"]'`. The enum
alone — with `null` as a member — constrains the field fully.
`TestSchemaEnumsCarryNoType` fails offline if that combination reappears, and also
checks that every property is in `required` (structured outputs express optionality
as nullability, so a property missing from `required` is a silent hole).

`live_schema_test.go` is the opt-in live counterpart — a schema mistake only shows up
as a 400 from the real API, which no fake endpoint can catch:

```bash
PURCHASING_LIVE_ANTHROPIC_KEY=sk-ant-... go test ./internal/extraction -run Live -v
```

**Partly verified live.** The schema is accepted and a synthetic text quotation
round-trips correctly: split CGST/SGST kept separate, discount and shipping read,
`1.234,56`-style comma decimals normalised, `total_includes_tax` set from the
document's wording, and nulls preserved for what wasn't stated. **Accuracy against
real PDFs is still untested.** To check it:

```bash
# backend/config.yaml
anthropic:
  enabled: true
  api_key: "sk-ant-..."
```

then upload each PDF in `resources/files/` through the PR page. Those four are a
reasonable first eval set — note `SUSE - Patrocínio FEBRABAN 26.pdf` is Portuguese
and likely BRL, which exercises non-USD currency and `1.234,56` decimal formatting.
Watch for: vendor picked as the issuer (not WSO2), currency inferred or correctly
left blank, and the total matching the document's grand total.

## Out of scope

- **Citations / page provenance.** `citations: {enabled: true}` is incompatible with
  `output_config.format` (400). Would need a second call.
- **Auto-extraction on upload.** The plumbing supports it; the trigger is manual so
  cost is user-initiated and the user is present to review.
- **Queue/batch infrastructure.** Extraction is request-scoped.
