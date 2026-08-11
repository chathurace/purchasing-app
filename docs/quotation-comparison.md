# Quotation comparison

As-built reference for the vendor-quote comparison card (migration `050`). It is the
app's version of `resources/files/WSO2_Vendor_Quote_Comparison_Template.xlsx`: every
vendor's **initial quote** beside its **final (post-negotiation) quote**, with the
variances and savings that follow.

## Where it appears

- **Button** — "Do quotation comparison" in the **Quotations** card header on the PR
  page, shown to procurement once the PR has **two or more** quotations (a comparison
  of one is not a comparison) and no comparison exists yet.
- **Card** — `components/QuotationComparisonCard.tsx`, rendered at *page* level
  between the procurement section and the recommendation. Deliberately not inside
  `ProcurementSection`: the comparison is what the recommendation's approvers read,
  and they have no procurement access. It renders nothing until one is generated.

## Nothing is snapshotted

The stored row (`quotation_comparisons`, one per PR) holds **no figures** — only what
cannot be derived:

| column | why it is stored |
|---|---|
| `generated_at` / `generated_by` | that a comparison was made, by whom, when |
| `approved_budget` | the sheet's header field; the requisition form no longer collects an estimated value, so procurement types it on the card |
| `currency` | the currency the comparison is stated in |
| `use_initial_for_final` | the user's confirmed consent to stand a vendor's initial figures in for a final quote it lacks |

Every number on the card is assembled on **each read** by `assembleComparison`
(`internal/handler/comparisons.go`) from the PR's quotations, their stored items and
their per-document extractions. So "if a quotation is updated after generating the
comparison, update the comparison accordingly" needs no mechanism: editing a
quotation, or reading a final PDF that didn't exist before, changes the next read.
The card polls on the same 5s cadence as the quotation queries, and the PR page also
invalidates `["quotation-comparison", prId]` on every quotation mutation so the
change lands immediately.

Assembling it **server-side** is also what makes it readable by approvers: the
quotation and extraction endpoints are procurement-only, so a legal or budget
approver could never build this in the browser.

## Where each column's figures come from

Per vendor, per column (`ComparisonQuote.source`), in order of preference:

1. **`pdf`** — the extraction of *that slot's own PDF*. The rich case: the totals
   block as printed, with split taxes kept split (CGST + SGST are never summed).
2. **`record`** — the quotation row's stored total and items. What a hand-entered
   quotation has, and what every quotation has when extraction is not configured
   (`anthropic.enabled: false`, the default) — so the feature works without Claude.
   Not used for the *initial* column when the stored figures are known to be the
   final PDF's (its extraction is the applied one), which would misattribute them.
3. **`initial`** — the initial quote standing in for a missing final one. Only with
   `use_initial_for_final`, always labelled "Initial figures — final quotation not
   available", and the vendor stays in `missing_final` so the card keeps flagging it.

A quotation associated inline on the PR page (total `0`, no items) carries **no**
figures rather than `0.00` — a missing figure must never read as free.

Totals are always **tax-inclusive**, through `extraction.TaxInclusiveTotal` — the Go
twin of `webapp/src/lib/extractionTotals.ts`. The two must stay in step: the quotation
cards read a PDF through the TS version and this card through the Go one, so a
divergence would show the same PDF as two different totals on one page. Both have
unit tests over the same cases.

## The missing-final-quote confirmation

Before generating, the frontend already knows `missing_final` (the server computes it
whether or not a comparison exists), so the modal names the vendors. Confirming posts
`use_initial_for_final: true`. The gate is enforced server-side too: `POST` returns
**409** naming the vendors when finals are missing and the consent is absent. If a
*new* quotation without a final quote is added later, the card's warning banner
offers "Use initial figures" (a `PUT` that records the same consent).

## Metrics

Rows of the summary matrix (metrics × vendors — vendors grow sideways in the
spreadsheet and there are only ever a handful, so this way round fits a card):

- **Initial quote** / **Final quote** — tax-inclusive grand totals.
- **Negotiated saving** = initial − final.
- **Variance vs budget** = final − `approved_budget` (over budget shown in red).
- **Saving vs highest quote** = final − the highest final quote; the cheapest final
  quote is chipped "lowest".

Cross-vendor metrics are computed **only** for quotations in the comparison's
currency. A quote in another currency is shown with its own figures and left out —
converting it here would invent an exchange rate — and `mixed_currency` explains that
on the card.

Below the matrix, the **rate comparison** chart (`components/RateComparisonChart.tsx`)
plots initial vs final per vendor: a two-series grouped column chart, inline SVG,
using the validated categorical pair (blue `#2a78d6` / orange `#eb6834` on light,
`#3987e5` / `#d95926` on dark — both modes pass the lightness band, chroma floor, CVD
separation and 3:1 contrast checks). The matrix above it is the table view of the same
numbers, which is why the marks carry only a native `<title>`.

Each vendor then gets a collapsible **detail** section: the item table with the
initial and final quote's quantity, unit price and amount side by side (rows paired
server-side on the item description; an item only one quote lists keeps its row with
the other side blank), and the two totals blocks — subtotal, discount, each tax line,
non-tax charges, grand total, with a note whenever the total shown is not the total
printed.

The spreadsheet's **UoM** column has no counterpart: neither an extracted nor a
stored line item carries a unit of measure. The **procurement recommendation** block
at the foot of the sheet is deliberately out of scope — the recommendation is made
*after* the comparison and already has its own card.

## API

All under the PR, gated like the rest of procurement work (`HasProcurementAccess` +
team-lead approval 409 + `assignmentWorkGate`) except the read, which is open to
anyone who may view the PR (`callerCanView`):

| method | path | notes |
|---|---|---|
| `GET` | `/purchase-requests/{id}/quotation-comparison` | `exists: false` before one is generated; figures included for procurement (it needs `missing_final` for the modal), withheld from other viewers until it exists |
| `POST` | same | generate or regenerate (re-stamps generated by/at); 409 on <2 quotations or missing consent |
| `PUT` | same | edit approved budget / currency / consent only |
| `DELETE` | same | discards the record; the quotations are untouched |

Process events: `create_quotation_comparison`, `update_quotation_comparison`,
`delete_quotation_comparison` (no qualifiers).

## Tests

- `internal/extraction/totals_test.go` — the tax-inclusive/reconciled arithmetic.
- `internal/handler/comparisons_test.go` — `assembleComparison` directly: source
  precedence, the missing-final stand-in, the record fallback, mixed currency, item
  pairing.
- `internal/handler/comparison_integration_test.go` — the endpoints against the real
  schema: the 409 gate, generating, an approver's read, the budget edit re-deriving
  variances, a quotation edited *after* generation showing through, and the events.
