import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  Chip,
  Divider,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Typography,
} from "@wso2/oxygen-ui";
import { ChevronDown, ChevronRight, Scale } from "@wso2/oxygen-ui-icons-react";
import { ConfirmDialog } from "./ConfirmDialog";
import { NumberInput } from "./NumberInput";
import { RateComparisonChart } from "./RateComparisonChart";
import { money } from "./ExtractedQuotationDetails";
import {
  deleteQuotationComparison,
  generateQuotationComparison,
  updateQuotationComparison,
} from "../api/quotationComparison";
import { comparisonKey, useQuotationComparison } from "../hooks/useQuotationComparison";
import { ApiError } from "../api/client";
import { formatMoney, quoRef } from "../types/api";
import type {
  ComparisonItemRow,
  ComparisonQuote,
  ComparisonVendor,
  PurchaseRequest,
  QuotationComparison,
} from "../types/api";

// Quotation comparison card — the WSO2 vendor-quote comparison sheet laid out for a
// card instead of a spreadsheet. The sheet puts every vendor's two quotes across the
// page (three columns each) and runs out of width past two vendors; here the same
// data is stacked instead: one summary matrix (metrics × vendors) with the chart, then
// a per-vendor section carrying that vendor's line items and its two totals blocks.
//
// Every figure is derived server-side on each read, so a quotation edited after the
// comparison was generated shows up here on the next poll — nothing to regenerate.
// See docs/quotation-comparison.md.

// --- the button that lives in the Quotations card ---

// QuotationComparisonButton offers the comparison once a PR has two or more
// quotations. When any vendor has no final quote it asks first: standing a vendor's
// initial figures in for a final quote is an assumption the user has to make
// knowingly, and the card then keeps saying so.
export function QuotationComparisonButton({
  pr,
  quotationCount,
}: {
  pr: PurchaseRequest;
  quotationCount: number;
}) {
  const { data } = useQuotationComparison(pr.id);
  const [confirming, setConfirming] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const qc = useQueryClient();

  const generate = useMutation({
    mutationFn: (useInitialForFinal: boolean) =>
      generateQuotationComparison(pr.id, {
        // The requisition form no longer collects an estimated value, so this is
        // usually empty — procurement fills the approved budget in on the card.
        approved_budget: pr.estimated_value > 0 ? pr.estimated_value : null,
        currency: data?.currency || pr.currency || "",
        use_initial_for_final: useInitialForFinal,
      }),
    onSuccess: (res) => {
      qc.setQueryData(comparisonKey(pr.id), res);
      qc.invalidateQueries({ queryKey: comparisonKey(pr.id) });
      setConfirming(false);
      setError(null);
    },
    onError: (e) =>
      setError(e instanceof ApiError ? e.message : "Failed to generate the comparison"),
  });

  if (!data?.can_manage || quotationCount < 2 || data.exists) return null;
  const missing = data.missing_final ?? [];

  return (
    <>
      <Stack direction="row" spacing={1} alignItems="center">
        {error && (
          <Typography variant="caption" color="error">
            {error}
          </Typography>
        )}
        <Button
          variant="outlined"
          color="inherit"
          size="small"
          startIcon={<Scale size={16} />}
          disabled={generate.isPending}
          onClick={() => (missing.length > 0 ? setConfirming(true) : generate.mutate(false))}
        >
          {generate.isPending ? "Comparing…" : "Do quotation comparison"}
        </Button>
      </Stack>
      {confirming && (
        <ConfirmDialog
          title="Some vendors have no final quotation"
          confirmLabel="Compare on initial figures"
          busy={generate.isPending}
          onCancel={() => setConfirming(false)}
          onConfirm={() => generate.mutate(true)}
          message={
            <Stack spacing={1}>
              <Typography variant="body2">
                No final quotation has been read for {missing.join(", ")}.
              </Typography>
              <Typography variant="body2">
                Their initial figures can stand in for the final quote so the comparison
                can be made now. The card will show those columns as "initial figures —
                final quotation not available", and they will switch to the real numbers
                as soon as a final quotation PDF is uploaded and read.
              </Typography>
            </Stack>
          }
        />
      )}
    </>
  );
}

// --- the card itself ---

export function QuotationComparisonCard({ prId }: { prId: number }) {
  const { data } = useQuotationComparison(prId);
  const qc = useQueryClient();
  const [error, setError] = useState<string | null>(null);
  const [editingBudget, setEditingBudget] = useState(false);
  const [budgetDraft, setBudgetDraft] = useState(0);
  const [removing, setRemoving] = useState(false);
  const [regenerating, setRegenerating] = useState(false);

  const settled = (res: QuotationComparison) => {
    qc.setQueryData(comparisonKey(prId), res);
    qc.invalidateQueries({ queryKey: comparisonKey(prId) });
    setError(null);
  };

  const update = useMutation({
    mutationFn: (input: { approved_budget: number | null; use_initial_for_final: boolean }) =>
      updateQuotationComparison(prId, {
        approved_budget: input.approved_budget,
        currency: data?.currency ?? "",
        use_initial_for_final: input.use_initial_for_final,
      }),
    onSuccess: (res) => {
      settled(res);
      setEditingBudget(false);
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : "Failed to update the comparison"),
  });

  const regenerate = useMutation({
    mutationFn: () =>
      generateQuotationComparison(prId, {
        approved_budget: data?.approved_budget ?? null,
        currency: data?.currency ?? "",
        // Regenerating re-confirms the stand-in only when there is something to
        // stand in for — it never quietly grants that consent for later.
        use_initial_for_final:
          (data?.missing_final ?? []).length > 0
            ? true
            : (data?.comparison?.use_initial_for_final ?? false),
      }),
    onSuccess: (res) => {
      settled(res);
      setRegenerating(false);
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : "Failed to regenerate"),
  });

  const remove = useMutation({
    mutationFn: () => deleteQuotationComparison(prId),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: comparisonKey(prId) });
      setRemoving(false);
      setError(null);
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : "Failed to remove the comparison"),
  });

  if (!data?.exists) return null;

  const { vendors, currency, comparison } = data;
  const missing = data.missing_final ?? [];
  const canManage = data.can_manage;
  const generatedOn = comparison?.generated_at
    ? new Date(comparison.generated_at).toLocaleString()
    : "";

  return (
    <Card variant="outlined" sx={{ mt: 3 }}>
      <CardContent>
        <Stack
          direction={{ xs: "column", sm: "row" }}
          justifyContent="space-between"
          alignItems={{ xs: "flex-start", sm: "center" }}
          gap={1}
          sx={{ mb: 1.5 }}
        >
          <Box>
            <Typography variant="subtitle1" sx={{ fontWeight: 600 }}>
              Quotation comparison
            </Typography>
            <Typography variant="caption" color="text.secondary">
              {comparison?.generator?.name || comparison?.generator?.email
                ? `Generated by ${comparison?.generator?.name || comparison?.generator?.email}`
                : "Generated"}
              {generatedOn ? ` · ${generatedOn}` : ""} · figures follow the quotations, so
              an edited quotation updates this card
            </Typography>
          </Box>
          {canManage && (
            <Stack direction="row" spacing={0.5} sx={{ flexShrink: 0 }}>
              <Button
                variant="text"
                size="small"
                disabled={regenerate.isPending}
                onClick={() => setRegenerating(true)}
              >
                Regenerate
              </Button>
              <Button
                variant="text"
                color="error"
                size="small"
                disabled={remove.isPending}
                onClick={() => setRemoving(true)}
              >
                Remove
              </Button>
            </Stack>
          )}
        </Stack>

        {error && (
          <Alert severity="error" sx={{ mb: 1.5 }}>
            {error}
          </Alert>
        )}

        {missing.length > 0 && (
          <Alert
            severity="warning"
            sx={{ mb: 1.5 }}
            action={
              canManage && !comparison?.use_initial_for_final ? (
                <Button
                  color="inherit"
                  size="small"
                  disabled={update.isPending}
                  onClick={() =>
                    update.mutate({
                      approved_budget: data.approved_budget,
                      use_initial_for_final: true,
                    })
                  }
                >
                  Use initial figures
                </Button>
              ) : undefined
            }
          >
            No final quotation has been read for {missing.join(", ")}.{" "}
            {comparison?.use_initial_for_final
              ? "Their initial figures are shown in the final-quote columns and will be replaced by the real ones once a final quotation PDF is read."
              : "Their final-quote columns are empty until a final quotation PDF is uploaded and read."}
          </Alert>
        )}

        {data.mixed_currency && (
          <Alert severity="info" sx={{ mb: 1.5 }}>
            Not every quotation is in {currency}. Quotes in another currency are shown with
            their own figures but left out of the cross-vendor comparison — converting them
            here would invent an exchange rate.
          </Alert>
        )}

        {/* Sheet header block: currency + the budget the quotes are measured against. */}
        <Stack
          direction="row"
          flexWrap="wrap"
          sx={{ mb: 2, gap: 3, alignItems: "flex-end" }}
        >
          <Meta label="Currency" value={currency || "—"} />
          <Box>
            <Typography
              variant="caption"
              color="text.secondary"
              sx={{ fontWeight: 600, display: "block" }}
            >
              Approved budget
            </Typography>
            {editingBudget ? (
              <Stack direction="row" spacing={0.5} alignItems="center">
                <NumberInput
                  value={budgetDraft}
                  onChange={setBudgetDraft}
                  min={0}
                  step="0.01"
                  style={{ width: 130 }}
                />
                <Button
                  variant="contained"
                  size="small"
                  disabled={update.isPending}
                  onClick={() =>
                    update.mutate({
                      approved_budget: budgetDraft > 0 ? budgetDraft : null,
                      use_initial_for_final: comparison?.use_initial_for_final ?? false,
                    })
                  }
                >
                  Save
                </Button>
                <Button
                  variant="text"
                  color="inherit"
                  size="small"
                  onClick={() => setEditingBudget(false)}
                >
                  Cancel
                </Button>
              </Stack>
            ) : (
              <Stack direction="row" spacing={1} alignItems="baseline">
                <Typography variant="body2">
                  {data.approved_budget == null
                    ? "Not set"
                    : formatMoney(data.approved_budget, currency)}
                </Typography>
                {canManage && (
                  <Button
                    variant="text"
                    size="small"
                    onClick={() => {
                      setBudgetDraft(data.approved_budget ?? 0);
                      setEditingBudget(true);
                    }}
                  >
                    {data.approved_budget == null ? "Set" : "Edit"}
                  </Button>
                )}
              </Stack>
            )}
          </Box>
          <Meta label="Quotations compared" value={String(vendors.length)} />
        </Stack>

        <SummaryMatrix data={data} />

        <Box sx={{ mt: 3 }}>
          <Typography variant="subtitle2" sx={{ fontWeight: 600, mb: 0.5 }}>
            Rate comparison
          </Typography>
          <RateComparisonChart
            currency={currency}
            data={vendors.map((v) => ({
              name: v.vendor_name,
              initial: v.initial.grand_total,
              final: v.final.grand_total,
              excluded: Boolean(currency && v.currency && v.currency !== currency),
            }))}
          />
        </Box>

        <Divider sx={{ my: 2 }} />

        <Typography variant="subtitle2" sx={{ fontWeight: 600, mb: 1 }}>
          Vendor detail
        </Typography>
        <Stack spacing={1}>
          {vendors.map((v) => (
            <VendorDetail key={v.quotation_id} vendor={v} comparisonCurrency={currency} />
          ))}
        </Stack>
      </CardContent>

      {removing && (
        <ConfirmDialog
          title="Remove this comparison?"
          message="The quotations themselves are untouched — you can run the comparison again at any time."
          confirmLabel="Remove"
          danger
          busy={remove.isPending}
          onConfirm={() => remove.mutate()}
          onCancel={() => setRemoving(false)}
        />
      )}
      {regenerating && (
        <ConfirmDialog
          title="Regenerate the comparison?"
          message={
            missing.length > 0
              ? `The figures are already live. Regenerating re-stamps who compared these quotations and when, and confirms comparing ${missing.join(", ")} on their initial figures.`
              : "The figures are already live. Regenerating re-stamps who compared these quotations and when."
          }
          confirmLabel="Regenerate"
          busy={regenerate.isPending}
          onConfirm={() => regenerate.mutate()}
          onCancel={() => setRegenerating(false)}
        />
      )}
    </Card>
  );
}

function Meta({ label, value }: { label: string; value: string }) {
  return (
    <Box>
      <Typography
        variant="caption"
        color="text.secondary"
        sx={{ fontWeight: 600, display: "block" }}
      >
        {label}
      </Typography>
      <Typography variant="body2">{value}</Typography>
    </Box>
  );
}

// --- summary matrix (the sheet's FINAL SUMMARY, transposed to fit a card) ---

// Metrics are rows and vendors are columns: vendors grow sideways in the spreadsheet
// and there are only ever a handful, while the metric list is fixed — so this way
// round the table stays readable and only needs to scroll on many vendors.
function SummaryMatrix({ data }: { data: QuotationComparison }) {
  const { vendors, currency } = data;
  const cell = (v: ComparisonVendor, value: number | null, signed = false) => {
    if (value == null) return "—";
    const cur = v.currency || currency;
    return signed ? signedMoney(value, cur) : formatMoney(value, cur);
  };

  const rows: {
    label: string;
    hint?: string;
    value: (v: ComparisonVendor) => string;
    strong?: boolean;
    tone?: (v: ComparisonVendor) => "default" | "over" | "under";
  }[] = [
    {
      label: "Initial quote",
      value: (v) => cell(v, v.initial.grand_total),
    },
    {
      label: "Final quote",
      hint: "incl. tax",
      value: (v) => cell(v, v.final.grand_total),
      strong: true,
    },
    {
      label: "Negotiated saving",
      hint: "initial − final",
      value: (v) => cell(v, v.negotiated_saving, true),
    },
    {
      label: "Variance vs budget",
      value: (v) => cell(v, v.variance_vs_budget, true),
      tone: (v) =>
        v.variance_vs_budget == null
          ? "default"
          : v.variance_vs_budget > 0
            ? "over"
            : "under",
    },
    {
      label: "Saving vs highest quote",
      value: (v) => cell(v, v.saving_vs_highest, true),
    },
  ];

  return (
    <TableContainer sx={{ overflowX: "auto" }}>
      <Table size="small" sx={{ minWidth: 520 }}>
        <TableHead>
          <TableRow>
            <TableCell sx={{ fontWeight: 600 }}>Metric</TableCell>
            {vendors.map((v) => (
              <TableCell key={v.quotation_id} align="right" sx={{ fontWeight: 600 }}>
                <Stack direction="row" spacing={0.5} alignItems="center" justifyContent="flex-end">
                  <span>{v.vendor_name}</span>
                  {v.lowest && (
                    <Chip size="small" color="success" variant="outlined" label="lowest" />
                  )}
                </Stack>
                <Typography variant="caption" color="text.secondary" sx={{ display: "block" }}>
                  {quoRef(v.quotation_id)}
                  {v.final_is_initial ? " · no final quote" : ""}
                </Typography>
              </TableCell>
            ))}
          </TableRow>
        </TableHead>
        <TableBody>
          {rows.map((row) => (
            <TableRow key={row.label}>
              <TableCell sx={{ fontWeight: row.strong ? 600 : 400 }}>
                {row.label}
                {row.hint && (
                  <Typography component="span" variant="caption" color="text.secondary">
                    {" "}
                    {row.hint}
                  </Typography>
                )}
              </TableCell>
              {vendors.map((v) => {
                const tone = row.tone?.(v) ?? "default";
                return (
                  <TableCell
                    key={v.quotation_id}
                    align="right"
                    sx={{
                      fontWeight: row.strong ? 600 : 400,
                      fontVariantNumeric: "tabular-nums",
                      color:
                        tone === "over"
                          ? "error.main"
                          : tone === "under"
                            ? "success.main"
                            : "text.primary",
                      bgcolor: v.lowest && row.strong ? "action.hover" : undefined,
                    }}
                  >
                    {row.value(v)}
                  </TableCell>
                );
              })}
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </TableContainer>
  );
}

// --- per-vendor detail ---

function VendorDetail({
  vendor,
  comparisonCurrency,
}: {
  vendor: ComparisonVendor;
  comparisonCurrency: string;
}) {
  const [open, setOpen] = useState(false);
  const headline = vendor.final.grand_total ?? vendor.initial.grand_total;
  const cur = vendor.currency || comparisonCurrency;

  return (
    <Box sx={{ border: 1, borderColor: "divider", borderRadius: 1.5, px: 1.5, py: 1 }}>
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
        <Typography variant="body2" sx={{ fontWeight: 600 }}>
          {vendor.vendor_name}
        </Typography>
        <Typography variant="caption" color="text.secondary" noWrap sx={{ minWidth: 0 }}>
          {quoRef(vendor.quotation_id)} · {money(headline, cur)}
          {vendor.negotiated_saving != null && vendor.negotiated_saving !== 0
            ? ` · negotiated ${signedMoney(vendor.negotiated_saving, cur)}`
            : ""}
        </Typography>
        <Box sx={{ flexGrow: 1 }} />
        {vendor.lowest && <Chip size="small" color="success" variant="outlined" label="lowest" />}
        {vendor.final_is_initial && (
          <Chip size="small" color="warning" variant="outlined" label="no final quote" />
        )}
        {(vendor.initial.items_mismatch || vendor.final.items_mismatch) && (
          <Chip size="small" color="warning" variant="outlined" label="totals differ" />
        )}
      </Box>

      {open && (
        <Stack spacing={2} sx={{ mt: 1.5 }}>
          {vendor.notes && (
            <Typography variant="body2" color="text.secondary" sx={{ whiteSpace: "pre-wrap" }}>
              {vendor.notes}
            </Typography>
          )}
          <ItemsTable rows={vendor.items} currency={cur} />
          <Box
            sx={{
              display: "grid",
              gridTemplateColumns: { xs: "1fr", md: "1fr 1fr" },
              gap: 2,
            }}
          >
            <QuoteTotals title="Initial quote" quote={vendor.initial} fallbackCurrency={cur} />
            <QuoteTotals
              title="Final quote"
              quote={vendor.final}
              fallbackCurrency={cur}
              standIn={vendor.final_is_initial}
            />
          </Box>
        </Stack>
      )}
    </Box>
  );
}

// ItemsTable is the sheet's item block: one row per item with the initial and final
// quote's quantity, unit price and amount beside each other. Rows are paired on the
// server by description, so a blank side means that quote didn't list the item.
function ItemsTable({ rows, currency }: { rows: ComparisonItemRow[]; currency: string }) {
  if (rows.length === 0) {
    return (
      <Typography variant="body2" color="text.secondary">
        No itemised lines on either quotation.
      </Typography>
    );
  }
  const num = (v: number | null, digits = 2) =>
    v == null ? "—" : v.toLocaleString(undefined, { minimumFractionDigits: digits, maximumFractionDigits: digits });

  return (
    <TableContainer sx={{ overflowX: "auto" }}>
      <Table size="small" sx={{ minWidth: 680 }}>
        <TableHead>
          <TableRow>
            <TableCell rowSpan={2} sx={{ fontWeight: 600 }}>
              Item
            </TableCell>
            <TableCell colSpan={3} align="center" sx={{ fontWeight: 600 }}>
              Initial quote
            </TableCell>
            <TableCell
              colSpan={3}
              align="center"
              sx={{ fontWeight: 600, borderLeft: 1, borderColor: "divider" }}
            >
              Final quote
            </TableCell>
          </TableRow>
          <TableRow>
            {["Qty", "Unit price", "Amount"].map((h) => (
              <TableCell key={`i-${h}`} align="right" sx={{ color: "text.secondary" }}>
                {h}
              </TableCell>
            ))}
            {["Qty", "Unit price", "Amount"].map((h, i) => (
              <TableCell
                key={`f-${h}`}
                align="right"
                sx={{
                  color: "text.secondary",
                  borderLeft: i === 0 ? 1 : 0,
                  borderColor: "divider",
                }}
              >
                {h}
              </TableCell>
            ))}
          </TableRow>
        </TableHead>
        <TableBody>
          {rows.map((row, i) => (
            <TableRow key={`${row.description}-${i}`}>
              <TableCell sx={{ maxWidth: 320 }}>{row.description}</TableCell>
              <TableCell align="right" sx={numeric}>
                {num(row.initial_quantity, 0)}
              </TableCell>
              <TableCell align="right" sx={numeric}>
                {num(row.initial_unit_price)}
              </TableCell>
              <TableCell align="right" sx={numeric}>
                {num(row.initial_amount)}
              </TableCell>
              <TableCell align="right" sx={{ ...numeric, borderLeft: 1, borderColor: "divider" }}>
                {num(row.final_quantity, 0)}
              </TableCell>
              <TableCell align="right" sx={numeric}>
                {num(row.final_unit_price)}
              </TableCell>
              <TableCell align="right" sx={numeric}>
                {num(row.final_amount)}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
      <Typography variant="caption" color="text.secondary" sx={{ display: "block", mt: 0.5 }}>
        Amounts in {currency}. A blank side means that quotation didn't list the item.
      </Typography>
    </TableContainer>
  );
}

const numeric = { fontVariantNumeric: "tabular-nums" } as const;

// QuoteTotals is one quote's additional-cost block, as printed: subtotal, discount,
// every tax line kept separate (CGST + SGST are never summed into "tax"), non-tax
// charges, then the tax-inclusive total the comparison is made on.
function QuoteTotals({
  title,
  quote,
  fallbackCurrency,
  standIn,
}: {
  title: string;
  quote: ComparisonQuote;
  fallbackCurrency: string;
  standIn?: boolean;
}) {
  const cur = quote.currency || fallbackCurrency;
  return (
    <Box sx={{ border: 1, borderColor: "divider", borderRadius: 1.5, p: 1.5 }}>
      <Stack direction="row" justifyContent="space-between" alignItems="baseline" sx={{ mb: 0.5 }}>
        <Typography variant="caption" sx={{ fontWeight: 600 }}>
          {title}
        </Typography>
        {quote.applied && (
          <Chip size="small" variant="outlined" color="success" label="on the quotation" />
        )}
      </Stack>
      <Typography variant="caption" color="text.secondary" sx={{ display: "block", mb: 1 }}>
        {sourceNote(quote, standIn)}
      </Typography>

      {!quote.available ? (
        <Typography variant="body2" color="text.secondary">
          No figures available.
        </Typography>
      ) : (
        <Stack spacing={0.25}>
          {quote.items.length > 0 && (
            <TotalLine label="Items" value={formatMoney(quote.items_total, cur)} />
          )}
          {quote.subtotal != null && (
            <TotalLine label="Subtotal" value={formatMoney(quote.subtotal, cur)} />
          )}
          {quote.discount != null && (
            <TotalLine label="Discount" value={`− ${formatMoney(quote.discount, cur)}`} />
          )}
          {quote.taxes.map((t, i) => (
            <TotalLine
              key={`tax-${i}`}
              label={`${t.label}${t.rate != null ? ` ${t.rate}%` : ""}`}
              value={money(t.amount, cur)}
            />
          ))}
          {quote.taxes.length > 1 && (
            <TotalLine label="Total tax" value={formatMoney(quote.tax_total, cur)} />
          )}
          {quote.charges.map((c, i) => (
            <TotalLine key={`chg-${i}`} label={c.label} value={money(c.amount, cur)} />
          ))}
          {quote.charges.length > 1 && (
            <TotalLine label="Total charges" value={formatMoney(quote.charges_total, cur)} />
          )}
          <Divider sx={{ my: 0.5 }} />
          <TotalLine
            label={quote.added_tax ? "Grand total incl. tax" : "Grand total"}
            value={money(quote.grand_total, cur)}
            strong
          />
          {(quote.added_tax || quote.derived_total) && (
            <Typography variant="caption" color="text.secondary" sx={{ textAlign: "right" }}>
              {quote.derived_total
                ? "No grand total printed — built from the breakdown"
                : `${formatMoney(quote.tax_total, cur)} tax added to the printed ${money(quote.stated_total, cur)}`}
            </Typography>
          )}
          {quote.items_mismatch && (
            <Typography variant="caption" color="warning.main" sx={{ mt: 0.5 }}>
              The total doesn't match this quote's own line items — check which is right.
            </Typography>
          )}
          {quote.valid_until && (
            <Typography variant="caption" color="text.secondary" sx={{ mt: 0.5 }}>
              Valid until {quote.valid_until}
            </Typography>
          )}
        </Stack>
      )}
    </Box>
  );
}

function TotalLine({
  label,
  value,
  strong,
}: {
  label: string;
  value: string;
  strong?: boolean;
}) {
  return (
    <Box
      sx={{
        display: "grid",
        gridTemplateColumns: "minmax(0, 1fr) auto",
        gap: 1,
        alignItems: "baseline",
      }}
    >
      <Typography
        variant="body2"
        color={strong ? "text.primary" : "text.secondary"}
        sx={{ fontWeight: strong ? 600 : 400 }}
      >
        {label}
      </Typography>
      <Typography
        variant="body2"
        sx={{ fontWeight: strong ? 600 : 400, textAlign: "right", ...numeric }}
      >
        {value}
      </Typography>
    </Box>
  );
}

// sourceNote names where a column's figures came from — the card must never present
// a stand-in as if it were read off the vendor's own final quotation.
function sourceNote(quote: ComparisonQuote, standIn?: boolean): string {
  if (standIn) return "Initial figures — final quotation not available";
  if (!quote.available) return "Not uploaded or not read yet";
  switch (quote.source) {
    case "pdf":
      return `Read from ${quote.filename || "this PDF"}${quote.confidence ? ` · ${quote.confidence} confidence` : ""}`;
    case "record":
      return "From the quotation record";
    default:
      return "";
  }
}

// signedMoney shows direction explicitly: a variance or saving of −5,000 vs +5,000
// are opposite outcomes and must never look alike.
function signedMoney(value: number, currency: string): string {
  if (value === 0) return formatMoney(0, currency);
  const sign = value > 0 ? "+" : "−";
  return `${sign}${formatMoney(Math.abs(value), currency)}`;
}
