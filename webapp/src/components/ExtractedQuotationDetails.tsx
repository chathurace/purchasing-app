import { useState } from "react";
import { Alert, Box, Chip, Divider, Stack, Typography } from "@wso2/oxygen-ui";
import {
  ChevronDown,
  ChevronRight,
  Sparkles,
} from "@wso2/oxygen-ui-icons-react";
import { formatMoney } from "../types/api";
import type { ExtractionResponse, ExtractionSuggestion } from "../types/api";
import {
  reconciledTotal,
  sumTaxes,
  taxInclusiveTotal,
  type TotalReading,
} from "../lib/extractionTotals";

// Read-only presentation of what Claude read out of one quotation PDF, plus the
// presentational pieces the editable review panel reuses so both render the same
// layout: a line-item table and an invoice-style totals block.
//
// This is deliberately per *document*: a quotation's initial and final PDF are
// separate documents with separate extractions, and their figures and line items
// legitimately differ (that's the point of a final, post-negotiation quote). The
// quotation row itself stores only the headline total that was applied.

// confidenceTone maps the model's self-reported confidence to a chip colour. Low
// confidence is a scan or a heavy-guesswork read and deserves a closer look.
export const confidenceTone: Record<
  string,
  "success" | "warning" | "error" | "default"
> = {
  high: "success",
  medium: "warning",
  low: "error",
};

// money renders an amount, or an em dash when the document didn't state one. A
// missing figure must never render as 0.00 — that would read as a real value.
export function money(
  value: number | null | undefined,
  currency: string,
): string {
  return value == null ? "—" : formatMoney(value, currency);
}

// Grid template shared by the items table's header and rows. The trailing column is
// the per-row action (only used by the editable variant, blank when read-only).
export const itemsGrid = (withAction: boolean) =>
  `minmax(0, 1fr) 80px 116px 104px${withAction ? " 36px" : ""}`;

export function ItemsTableHeader({
  withAction = false,
}: {
  withAction?: boolean;
}) {
  return (
    <Box
      sx={{
        display: "grid",
        gridTemplateColumns: itemsGrid(withAction),
        gap: 1,
        alignItems: "center",
        pb: 0.5,
        borderBottom: 1,
        borderColor: "divider",
      }}
    >
      <HeaderCell>Description</HeaderCell>
      <HeaderCell align="right">Qty</HeaderCell>
      <HeaderCell align="right">Unit price</HeaderCell>
      <HeaderCell align="right">Amount</HeaderCell>
      {withAction && <span />}
    </Box>
  );
}

function HeaderCell({
  children,
  align = "left",
}: {
  children: React.ReactNode;
  align?: "left" | "right";
}) {
  return (
    <Typography
      variant="caption"
      color="text.secondary"
      sx={{
        fontWeight: 600,
        textAlign: align,
        textTransform: "uppercase",
        letterSpacing: 0.4,
      }}
    >
      {children}
    </Typography>
  );
}

// TotalsRow is one line of the totals block: a label on the left, a figure (or an
// input, in the editable panel) right-aligned.
export function TotalsRow({
  label,
  hint,
  value,
  strong,
}: {
  label: string;
  hint?: string;
  value: React.ReactNode;
  strong?: boolean;
}) {
  return (
    <Box
      sx={{
        display: "grid",
        gridTemplateColumns: "minmax(0, 1fr) 140px",
        gap: 1,
        alignItems: "center",
      }}
    >
      <Typography
        variant="body2"
        color={strong ? "text.primary" : "text.secondary"}
        sx={{ fontWeight: strong ? 600 : 400 }}
      >
        {label}
        {hint && (
          <Typography component="span" variant="caption" color="text.secondary">
            {" "}
            {hint}
          </Typography>
        )}
      </Typography>
      <Box sx={{ textAlign: "right", fontWeight: strong ? 600 : 400 }}>
        {typeof value === "string" ? (
          <Typography variant="body2">{value}</Typography>
        ) : (
          value
        )}
      </Box>
    </Box>
  );
}

// TotalsBlock renders the quotation's totals as printed: subtotal, discount, each
// tax line kept separate, non-tax charges, then the grand total. totalSlot lets the
// editable review panel drop an input into the total row while everything above it
// stays as-read — only the total is stored on a quotation, so only it is editable.
// taxTotal / chargesTotal are the server-computed sums; they are shown only when
// there is more than one row to add up (a single tax line is already its own total).
export function TotalsBlock({
  suggestion,
  currency,
  totalSlot,
  taxTotal,
  chargesTotal,
}: {
  suggestion: ExtractionSuggestion;
  currency: string;
  totalSlot?: React.ReactNode;
  taxTotal?: number;
  chargesTotal?: number;
}) {
  const {
    subtotal_amount,
    discount_amount,
    taxes,
    other_charges,
    total_includes_tax,
  } = suggestion;
  const taxLines = taxes ?? [];
  const chargeLines = other_charges ?? [];
  const hasBreakdown =
    subtotal_amount != null ||
    discount_amount != null ||
    taxLines.length > 0 ||
    chargeLines.length > 0;
  const tax = taxTotal ?? sumTaxes(suggestion);
  const reading = taxInclusiveTotal(suggestion, tax);

  return (
    <Stack spacing={0.5}>
      {subtotal_amount != null && (
        <TotalsRow label="Subtotal" value={money(subtotal_amount, currency)} />
      )}
      {discount_amount != null && (
        <TotalsRow
          label="Discount"
          value={`− ${formatMoney(discount_amount, currency)}`}
        />
      )}
      {taxLines.map((t, i) => (
        <TotalsRow
          key={`tax-${i}`}
          label={t.label || "Tax"}
          hint={t.rate != null ? `${t.rate}%` : undefined}
          value={money(t.amount, currency)}
        />
      ))}
      {taxLines.length > 1 && taxTotal != null && (
        <TotalsRow label="Total tax" value={formatMoney(taxTotal, currency)} />
      )}
      {chargeLines.map((c, i) => (
        <TotalsRow
          key={`charge-${i}`}
          label={c.label || "Charge"}
          value={money(c.amount, currency)}
        />
      ))}
      {chargeLines.length > 1 && chargesTotal != null && (
        <TotalsRow label="Total charges" value={formatMoney(chargesTotal, currency)} />
      )}
      {hasBreakdown && <Divider sx={{ my: 0.5 }} />}
      <TotalsRow
        label={reading.addedTax ? "Total incl. tax" : "Total"}
        value={totalSlot ?? money(reading.value, currency)}
        strong
      />
      <Typography variant="caption" color="text.secondary" sx={{ textAlign: "right" }}>
        {totalNote(reading, taxLines.length, tax, currency, total_includes_tax)}
      </Typography>
    </Stack>
  );
}

// totalNote explains where the total came from. It matters most when the figure
// shown is not the figure printed: a tax-exclusive quotation is a common way to be
// surprised by 20% at invoice time, so the adjustment is stated, not silent.
function totalNote(
  reading: TotalReading,
  taxCount: number,
  tax: number,
  currency: string,
  includesTax: boolean | null,
): string {
  if (reading.derived) {
    return "No grand total printed — built from the breakdown above";
  }
  if (reading.addedTax) {
    const basis =
      includesTax === false
        ? "the PDF states tax is excluded"
        : "the PDF's total matches its pre-tax subtotal";
    return `${formatMoney(tax, currency)} tax added to the printed ${money(reading.stated, currency)} — ${basis}`;
  }
  if (taxCount > 0) return "Printed total already includes tax";
  return "No tax stated in the document";
}

// ExtractedQuotationDetails shows a stored extraction for one PDF. Collapsed it is a
// single summary line (so a card with two PDFs stays scannable); expanded it is the
// full read of that document — line items, totals block, dates and the model's notes.
export function ExtractedQuotationDetails({
  result,
  defaultOpen = false,
}: {
  result: ExtractionResponse;
  defaultOpen?: boolean;
}) {
  const [open, setOpen] = useState(defaultOpen);
  const { suggestion, extraction, items_total } = result;
  if (!suggestion) return null;

  const currency = suggestion.currency;
  const items = suggestion.items ?? [];
  const reading = taxInclusiveTotal(suggestion, result.tax_total);
  // Same non-blocking warning invoices use for entered_total: the document's total
  // wins, we only flag the divergence. Compared against the items *plus* the
  // document's discount, taxes and charges, so a taxed quotation isn't flagged
  // merely for being taxed.
  const built = reconciledTotal(suggestion, items_total, result.tax_total);
  const mismatch =
    items.length > 0 && reading.value != null && Math.abs(reading.value - built) > 0.01;

  return (
    <Box
      sx={{
        border: 1,
        borderColor: "divider",
        borderRadius: 2,
        bgcolor: "background.default",
        px: 1.5,
        py: 1,
      }}
    >
      <Box
        component="button"
        type="button"
        onClick={() => setOpen((v) => !v)}
        aria-expanded={open}
        sx={{
          display: "flex",
          alignItems: "center",
          gap: 0.75,
          width: "100%",
          border: 0,
          background: "none",
          p: 0,
          cursor: "pointer",
          textAlign: "left",
          font: "inherit",
          color: "text.primary",
        }}
      >
        {open ? <ChevronDown size={16} /> : <ChevronRight size={16} />}
        <Sparkles size={14} />
        <Typography variant="caption" sx={{ fontWeight: 600 }}>
          Read from this PDF
        </Typography>
        <Typography
          variant="caption"
          color="text.secondary"
          sx={{ minWidth: 0 }}
          noWrap
        >
          {suggestion.vendor_name || "vendor not stated"} ·{" "}
          {money(reading.value, currency)}
          {reading.addedTax && " incl. tax"}
        </Typography>
        <Box sx={{ flexGrow: 1 }} />
        {mismatch && (
          <Chip
            size="small"
            label="totals differ"
            color="warning"
            variant="outlined"
          />
        )}
        {suggestion.confidence && (
          <Chip
            size="small"
            label={suggestion.confidence}
            color={confidenceTone[suggestion.confidence] ?? "default"}
            variant="outlined"
          />
        )}
        <Chip
          size="small"
          variant="outlined"
          color={extraction.applied_at ? "success" : "default"}
          label={extraction.applied_at ? "applied" : "not applied"}
        />
      </Box>

      {open && (
        <Stack spacing={1.5} sx={{ mt: 1.5 }}>
          <Box
            sx={{
              display: "grid",
              gridTemplateColumns: { xs: "1fr 1fr", sm: "repeat(4, 1fr)" },
              columnGap: 2,
              rowGap: 1,
            }}
          >
            <Field
              label="Vendor (as printed)"
              value={suggestion.vendor_name || "—"}
            />
            <Field label="Currency" value={currency || "—"} />
            <Field label="Quote date" value={suggestion.quote_date ?? "—"} />
            <Field label="Valid until" value={suggestion.valid_until ?? "—"} />
            {suggestion.quote_reference && (
              <Field
                label="Quote reference"
                value={suggestion.quote_reference}
              />
            )}
          </Box>

          <Box>
            <ItemsTableHeader />
            {items.length === 0 ? (
              <Typography
                variant="body2"
                color="text.secondary"
                sx={{ pt: 0.75 }}
              >
                No itemised table in this PDF.
              </Typography>
            ) : (
              <Stack divider={<Divider />}>
                {items.map((it, i) => (
                  <Box
                    key={i}
                    sx={{
                      display: "grid",
                      gridTemplateColumns: itemsGrid(false),
                      gap: 1,
                      py: 0.75,
                      alignItems: "baseline",
                    }}
                  >
                    <Typography variant="body2">{it.description}</Typography>
                    <Typography variant="body2" sx={{ textAlign: "right" }}>
                      {it.quantity}
                    </Typography>
                    <Typography variant="body2" sx={{ textAlign: "right" }}>
                      {formatMoney(it.unit_price, "")}
                    </Typography>
                    <Typography variant="body2" sx={{ textAlign: "right" }}>
                      {formatMoney(it.quantity * it.unit_price, "")}
                    </Typography>
                  </Box>
                ))}
              </Stack>
            )}
          </Box>

          <Box sx={{ maxWidth: 420, ml: "auto", width: "100%" }}>
            <TotalsBlock
              suggestion={suggestion}
              currency={currency}
              taxTotal={result.tax_total}
              chargesTotal={result.charges_total}
            />
          </Box>

          {mismatch && (
            <Alert severity="warning">
              The total ({money(reading.value, currency)}) doesn’t match the line
              items with this document’s discount, taxes and charges applied (
              {formatMoney(built, currency)}). The document’s own total is what it
              asks for — check which is right.
            </Alert>
          )}

          {suggestion.notes && (
            <Alert severity="info" icon={false}>
              <Typography variant="body2">{suggestion.notes}</Typography>
            </Alert>
          )}

          <Typography variant="caption" color="text.secondary">
            {extraction.model}
            {extraction.filename ? ` · ${extraction.filename}` : ""}
            {extraction.applied_at
              ? " · applied to the quotation"
              : " · not applied — the quotation's own figures are unchanged"}
          </Typography>
        </Stack>
      )}
    </Box>
  );
}

function Field({ label, value }: { label: string; value: string }) {
  return (
    <Box sx={{ minWidth: 0 }}>
      <Typography
        variant="caption"
        color="text.secondary"
        sx={{ fontWeight: 600, display: "block" }}
      >
        {label}
      </Typography>
      <Typography variant="body2" sx={{ overflowWrap: "anywhere" }}>
        {value}
      </Typography>
    </Box>
  );
}
