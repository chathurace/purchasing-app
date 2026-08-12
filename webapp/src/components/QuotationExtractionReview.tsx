import { useEffect, useMemo, useState } from "react";
import {
  Alert,
  Box,
  Button,
  Chip,
  Divider,
  IconButton,
  Stack,
  TextField,
  Typography,
} from "@wso2/oxygen-ui";
import { Plus, Sparkles, Trash } from "@wso2/oxygen-ui-icons-react";
import { formatMoney } from "../types/api";
import type { ExtractionResponse, QuotationInput } from "../types/api";
import { VendorSelect } from "./VendorSelect";
import { NumberInput } from "./NumberInput";
import { CurrencyInput } from "./CurrencyInput";
import {
  ItemsTableHeader,
  TotalsBlock,
  confidenceTone,
  itemsGrid,
  money,
} from "./ExtractedQuotationDetails";
import { reconciledTotal, taxInclusiveTotal } from "../lib/extractionTotals";

type Item = { description: string; quantity: number; unit_price: number };

interface Props {
  result: ExtractionResponse;
  /** Label for the confirm button ("Add quotation" on create, "Apply" on a card). */
  submitLabel: string;
  submitting?: boolean;
  onSubmit: (input: QuotationInput) => void;
  onDiscard: () => void;
  /** Prefilled description/notes carried over from the form the user was filling. */
  initialNotes?: string;
}

// QuotationExtractionReview renders what Claude read out of a quotation PDF as an
// editable form, laid out like the quotation edit view: a line-item table and an
// invoice-style totals block. Nothing here is authoritative — the user corrects
// anything wrong and confirms, and only then does the parent write it. Fields the
// model left blank stay blank rather than being defaulted, so a missing value is
// visible as missing.
//
// The tax/subtotal/charge lines are shown as printed but are not editable: a
// quotation row stores only the grand total, so those figures stay with the
// extraction (where they remain visible per PDF — see ExtractedQuotationDetails).
export function QuotationExtractionReview({
  result,
  submitLabel,
  submitting,
  onSubmit,
  onDiscard,
  initialNotes = "",
}: Props) {
  const { suggestion, vendor_matches: matches } = result;

  // Preselect a vendor only on a confident match. Anything weaker is left unset so
  // the user makes the call — attaching a quote to the wrong company is worse than
  // asking. (Scores: 100 exact, 80 prefix, 60 substring, 40 shared word.)
  const bestMatch = matches[0];
  const [vendorId, setVendorId] = useState(
    bestMatch && bestMatch.score >= 80 ? bestMatch.vendor.id : 0,
  );

  // A named vendor that ranked against nothing in the vendor list has to be created
  // before this quotation can be saved.
  const extractedVendor = suggestion?.vendor_name?.trim() ?? "";
  const noVendorMatch = extractedVendor !== "" && matches.length === 0;
  // The total offered for saving is the tax-inclusive one — the amount actually
  // payable. Where the PDF printed a tax-exclusive total, the tax is added and the
  // adjustment is stated below the field rather than applied silently.
  const reading = suggestion
    ? taxInclusiveTotal(suggestion, result.tax_total)
    : { value: null, addedTax: false, derived: false, stated: null };

  const [currency, setCurrency] = useState(suggestion?.currency ?? "");
  const [total, setTotal] = useState(reading.value ?? 0);
  const [validUntil, setValidUntil] = useState(suggestion?.valid_until ?? "");
  const [notes, setNotes] = useState(() =>
    buildNotes(initialNotes, suggestion?.quote_reference),
  );
  const [items, setItems] = useState<Item[]>(suggestion?.items ?? []);

  // Reseed when a re-extraction returns a different result for the same mount.
  useEffect(() => {
    setVendorId(bestMatch && bestMatch.score >= 80 ? bestMatch.vendor.id : 0);
    setCurrency(suggestion?.currency ?? "");
    setTotal(reading.value ?? 0);
    setValidUntil(suggestion?.valid_until ?? "");
    setItems(suggestion?.items ?? []);
    // initialNotes is the user's own typing — deliberately not reseeded here.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [result.extraction.id]);

  const itemsTotal = useMemo(
    () => items.reduce((sum, it) => sum + it.quantity * it.unit_price, 0),
    [items],
  );
  // Same non-blocking warning invoices use for entered_total: the entered total wins,
  // we only flag the divergence. The items are compared at the same level as the
  // total — with the document's discount, taxes and charges applied — so a taxed
  // quotation isn't flagged merely for being taxed.
  const built = suggestion ? reconciledTotal(suggestion, itemsTotal, result.tax_total) : itemsTotal;
  const mismatch = items.length > 0 && total > 0 && Math.abs(total - built) > 0.01;

  const setItem = (i: number, patch: Partial<Item>) =>
    setItems((prev) =>
      prev.map((it, idx) => (idx === i ? { ...it, ...patch } : it)),
    );

  const missing: string[] = [];
  if (!suggestion?.currency) missing.push("currency");
  // A derived total counts as read — only a document that gave us nothing to add up
  // leaves this blank.
  if (reading.value == null) missing.push("total");
  if (!suggestion?.valid_until) missing.push("validity date");

  const submit = () =>
    onSubmit({
      vendor_id: vendorId,
      total_amount: total,
      currency: currency.trim() || "USD",
      valid_until: validUntil || null,
      notes: notes.trim(),
      items: items
        .filter((it) => it.description.trim() !== "")
        .map((it) => ({
          description: it.description.trim(),
          quantity: it.quantity,
          unit_price: it.unit_price,
        })),
      extraction_id: result.extraction.id,
    });

  return (
    <Box
      sx={{
        p: 2,
        border: 1,
        borderColor: "divider",
        borderRadius: 2,
        bgcolor: "action.hover",
      }}
    >
      <Stack direction="row" alignItems="center" spacing={1} sx={{ mb: 1.5 }}>
        <Sparkles size={16} />
        <Typography variant="subtitle2" sx={{ fontWeight: 600 }}>
          Read from {result.extraction.filename || "the PDF"}
        </Typography>
        {suggestion?.confidence && (
          <Chip
            size="small"
            label={`${suggestion.confidence} confidence`}
            color={confidenceTone[suggestion.confidence] ?? "default"}
            variant="outlined"
          />
        )}
      </Stack>

      <Alert severity="info" sx={{ mb: 2 }}>
        These values were read from the PDF automatically. Check them against
        the document and correct anything wrong — nothing is saved until you
        confirm.
      </Alert>

      {suggestion?.notes && (
        <Alert severity="warning" sx={{ mb: 2 }}>
          {suggestion.notes}
        </Alert>
      )}

      {missing.length > 0 && (
        <Alert severity="warning" sx={{ mb: 2 }}>
          Not stated in the document: {missing.join(", ")}. Fill these in if you
          need them.
        </Alert>
      )}

      <Stack spacing={2}>
        <Box>
          {/* No match in the vendor list means the vendor has to be created before
              this quotation can be saved, so the form opens with the extracted name
              already in it — one click, instead of retyping a name we just read.
              Keyed on the extraction so a re-read reseeds it. */}
          <VendorSelect
            key={result.extraction.id}
            value={vendorId}
            onChange={setVendorId}
            suggestedName={extractedVendor}
            defaultAdding={noVendorMatch}
          />
          {suggestion?.vendor_name && (
            <Typography
              variant="caption"
              color="text.secondary"
              sx={{ mt: 0.5, display: "block" }}
            >
              PDF says “{suggestion.vendor_name}”
              {noVendorMatch &&
                " — no existing vendor matches, so it is prefilled above; check it and click Add vendor."}
              {vendorId === 0 &&
                matches.length > 0 &&
                " — pick the matching vendor, or “+ New vendor” to create it (the name is prefilled)."}
            </Typography>
          )}
          {matches.length > 1 && (
            <Stack
              direction="row"
              spacing={0.5}
              sx={{ mt: 0.5, flexWrap: "wrap", gap: 0.5 }}
            >
              {matches.map((m) => (
                <Chip
                  key={m.vendor.id}
                  size="small"
                  label={m.vendor.name}
                  variant={m.vendor.id === vendorId ? "filled" : "outlined"}
                  onClick={() => setVendorId(m.vendor.id)}
                />
              ))}
            </Stack>
          )}
        </Box>

        <Stack direction={{ xs: "column", sm: "row" }} spacing={1.5}>
          <Box sx={{ width: { sm: 140 } }}>
            <Typography variant="caption" color="text.secondary">
              Currency
            </Typography>
            <CurrencyInput
              value={currency}
              onChange={setCurrency}
              maxLength={3}
            />
          </Box>
          <Box sx={{ width: { sm: 180 } }}>
            <Typography variant="caption" color="text.secondary">
              Valid until
            </Typography>
            <TextField
              type="date"
              size="small"
              fullWidth
              value={validUntil}
              onChange={(e) => setValidUntil(e.target.value)}
            />
          </Box>
          {suggestion?.quote_date && (
            <Box sx={{ width: { sm: 180 } }}>
              <Typography
                variant="caption"
                color="text.secondary"
                sx={{ display: "block" }}
              >
                Quote date (as printed)
              </Typography>
              <Typography variant="body2" sx={{ pt: 1 }}>
                {suggestion.quote_date}
              </Typography>
            </Box>
          )}
        </Stack>

        <Box>
          <Stack
            direction="row"
            justifyContent="space-between"
            alignItems="center"
            sx={{ mb: 0.5 }}
          >
            <Typography
              variant="caption"
              color="text.secondary"
              sx={{ fontWeight: 600 }}
            >
              Line items
            </Typography>
            <Button
              variant="text"
              size="small"
              startIcon={<Plus size={14} />}
              onClick={() =>
                setItems((p) => [
                  ...p,
                  { description: "", quantity: 1, unit_price: 0 },
                ])
              }
            >
              Add item
            </Button>
          </Stack>
          <ItemsTableHeader withAction />
          {items.length === 0 ? (
            <Typography
              variant="body2"
              color="text.secondary"
              sx={{ pt: 0.75 }}
            >
              No itemised table found in the PDF.
            </Typography>
          ) : (
            <Stack spacing={0.75} sx={{ pt: 0.75 }}>
              {items.map((it, i) => (
                <Box
                  key={i}
                  sx={{
                    display: "grid",
                    gridTemplateColumns: itemsGrid(true),
                    gap: 1,
                    alignItems: "center",
                  }}
                >
                  <TextField
                    size="small"
                    fullWidth
                    placeholder="Description"
                    value={it.description}
                    onChange={(e) =>
                      setItem(i, { description: e.target.value })
                    }
                  />
                  <NumberInput
                    value={it.quantity}
                    onChange={(quantity) => setItem(i, { quantity })}
                    min={0}
                    step="any"
                  />
                  <NumberInput
                    value={it.unit_price}
                    onChange={(unit_price) => setItem(i, { unit_price })}
                    min={0}
                    step="0.01"
                  />
                  <Typography variant="body2" sx={{ textAlign: "right" }}>
                    {formatMoney(it.quantity * it.unit_price, "")}
                  </Typography>
                  <IconButton
                    size="small"
                    aria-label="Remove item"
                    onClick={() =>
                      setItems((p) => p.filter((_, idx) => idx !== i))
                    }
                  >
                    <Trash size={16} />
                  </IconButton>
                </Box>
              ))}
            </Stack>
          )}
          {items.length > 0 && (
            <Typography
              variant="caption"
              color="text.secondary"
              sx={{ display: "block", textAlign: "right", mt: 0.5 }}
            >
              Line items sum to {formatMoney(itemsTotal, currency)}
            </Typography>
          )}
        </Box>

        {/* The totals block as printed on the PDF. Only the grand total is editable —
            it is the one figure a quotation row stores, and it is the tax-inclusive
            amount payable. */}
        <Box sx={{ maxWidth: 420, ml: { sm: "auto" }, width: "100%" }}>
          {suggestion && (
            <TotalsBlock
              suggestion={suggestion}
              currency={currency}
              taxTotal={result.tax_total}
              chargesTotal={result.charges_total}
              totalSlot={
                <NumberInput
                  value={total}
                  onChange={setTotal}
                  min={0}
                  step="0.01"
                />
              }
            />
          )}
          {/* Only once the user has edited away from what we read — the totals block's
              own note already explains any tax adjustment we made. */}
          {reading.value != null && total !== reading.value && (
            <Typography
              variant="caption"
              color="text.secondary"
              sx={{ display: "block", textAlign: "right", mt: 0.5 }}
            >
              Read from the PDF as {money(reading.value, currency)}
            </Typography>
          )}
        </Box>

        {mismatch && (
          <Alert severity="warning">
            The total ({formatMoney(total, currency)}) doesn’t match the line items
            with this document’s discount, taxes and charges applied (
            {formatMoney(built, currency)}). The total above is what gets saved —
            check which is right.
          </Alert>
        )}

        <TextField
          label="Description / notes"
          size="small"
          fullWidth
          multiline
          minRows={2}
          value={notes}
          onChange={(e) => setNotes(e.target.value)}
        />

        <Divider />

        <Stack direction="row" spacing={1} alignItems="center">
          <Button
            variant="contained"
            onClick={submit}
            disabled={submitting || vendorId === 0}
          >
            {submitting ? "Saving…" : submitLabel}
          </Button>
          <Button variant="text" onClick={onDiscard} disabled={submitting}>
            Discard
          </Button>
          <Box sx={{ flexGrow: 1 }} />
          <Typography variant="caption" color="text.secondary">
            {result.extraction.model}
          </Typography>
        </Stack>
        {vendorId === 0 && (
          <Typography variant="caption" color="text.secondary">
            Select a vendor to continue.
          </Typography>
        )}
      </Stack>
    </Box>
  );
}

// buildNotes seeds the notes field with whatever the user had already typed, plus
// the vendor's own quote reference when the PDF printed one (it has no column of its
// own and is useful to keep).
function buildNotes(existing: string, quoteRef?: string): string {
  const parts = [existing.trim()];
  if (quoteRef?.trim()) parts.push(`Quote ref: ${quoteRef.trim()}`);
  return parts.filter(Boolean).join("\n");
}
