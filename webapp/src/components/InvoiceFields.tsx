import {
  Box,
  Button,
  FormControlLabel,
  MenuItem,
  Radio,
  RadioGroup,
  Stack,
  TextField,
  Typography,
} from "@wso2/oxygen-ui";
import { Plus } from "@wso2/oxygen-ui-icons-react";
import {
  allocationsSum,
  allocationsValid,
  formatMoney,
  invoiceEffectiveTotal,
  invoiceItemsTotal,
} from "../types/api";
import type { AllocationMode, CostAllocationInput, InvoiceInput } from "../types/api";
import { useBusinessUnitLookup } from "../hooks/useBusinessUnits";
import { NullableNumberInput, NumberInput } from "./NumberInput";
import { CurrencyInput } from "./CurrencyInput";

interface Props {
  value: InvoiceInput;
  onChange: (next: InvoiceInput) => void;
}

type IItem = { description: string; quantity: number; unit_price: number };

// A numeric field with a trailing unit/currency suffix (e.g. "%", "USD"),
// keeping the specialised NumberInput's controlled behaviour intact.
function SuffixNumber({
  width,
  suffix,
  children,
}: {
  width: number | string;
  suffix: React.ReactNode;
  children: React.ReactNode;
}) {
  return (
    <Box sx={{ position: "relative", width, flexShrink: 0 }}>
      {children}
      <Typography
        variant="caption"
        color="text.secondary"
        sx={{
          position: "absolute",
          right: 10,
          top: "50%",
          transform: "translateY(-50%)",
          pointerEvents: "none",
        }}
      >
        {suffix}
      </Typography>
    </Box>
  );
}

export function InvoiceFields({ value, onChange }: Props) {
  const set = (patch: Partial<InvoiceInput>) => onChange({ ...value, ...patch });
  const { data: businessUnits, isLoading: buLoading } = useBusinessUnitLookup();

  const setItem = (i: number, patch: Partial<IItem>) => {
    const items = value.items.map((it, idx) => (idx === i ? { ...it, ...patch } : it));
    set({ items });
  };

  // itemsTotal is the sum of the line items; total is the effective invoice
  // value (the entered total when given, else itemsTotal). The two can diverge —
  // the entered total wins and we only warn.
  const itemsTotal = invoiceItemsTotal(value.items);
  const total = invoiceEffectiveTotal(value);
  const hasEnteredTotal = value.entered_total != null;
  const mismatch =
    hasEnteredTotal && value.items.length > 0 && Math.abs((value.entered_total ?? 0) - itemsTotal) > 0.01;

  // --- business-unit allocation helpers ---
  const allocs = value.cost_allocations;
  const setAlloc = (i: number, patch: Partial<CostAllocationInput>) =>
    set({ cost_allocations: allocs.map((a, idx) => (idx === i ? { ...a, ...patch } : a)) });
  const addAlloc = () => set({ cost_allocations: [...allocs, { business_unit_id: 0, value: 0 }] });
  const removeAlloc = (i: number) =>
    set({ cost_allocations: allocs.filter((_, idx) => idx !== i) });
  const setMode = (mode: AllocationMode) => set({ allocation_mode: mode });

  const allocSum = allocationsSum(allocs);
  const allocTarget = value.allocation_mode === "percentage" ? 100 : total;
  const allocOk = allocationsValid(value.allocation_mode, allocs, total);
  // Business units already chosen on other rows, to avoid picking duplicates.
  const chosenIds = (i: number) =>
    new Set(allocs.filter((_, idx) => idx !== i).map((a) => a.business_unit_id));

  return (
    <Stack spacing={2.5}>
      <Stack direction={{ xs: "column", sm: "row" }} spacing={1.5}>
        <TextField
          label="Vendor invoice no."
          size="small"
          fullWidth
          value={value.vendor_invoice_no}
          onChange={(e) => set({ vendor_invoice_no: e.target.value })}
          placeholder="The vendor's own invoice number"
        />
        <Box
          component={CurrencyInput}
          sx={{ width: { xs: "100%", sm: 112 }, flexShrink: 0 }}
          value={value.currency}
          maxLength={3}
          onChange={(currency: string) => set({ currency })}
          placeholder="USD"
        />
      </Stack>

      <Stack direction={{ xs: "column", sm: "row" }} spacing={1.5}>
        <TextField
          label="Invoice date"
          type="date"
          size="small"
          InputLabelProps={{ shrink: true }}
          value={value.invoice_date}
          onChange={(e) => set({ invoice_date: e.target.value })}
          sx={{ width: { xs: "100%", sm: 200 }, flexShrink: 0 }}
        />
        <TextField
          label="Due date"
          type="date"
          size="small"
          InputLabelProps={{ shrink: true }}
          value={value.due_date ?? ""}
          onChange={(e) => set({ due_date: e.target.value || null })}
          sx={{ width: { xs: "100%", sm: 200 }, flexShrink: 0 }}
        />
      </Stack>

      {/* Line items */}
      <Box>
        <Box sx={{ mb: 1, display: "flex", alignItems: "center", justifyContent: "space-between" }}>
          <Typography variant="body2" sx={{ fontWeight: 600 }}>
            Line items
          </Typography>
          <Button
            variant="text"
            size="small"
            startIcon={<Plus size={16} />}
            onClick={() =>
              set({ items: [...value.items, { description: "", quantity: 1, unit_price: 0 }] })
            }
          >
            Add line
          </Button>
        </Box>
        <Stack spacing={1}>
          {value.items.length === 0 && (
            <Typography variant="body2" color="text.secondary">
              No line items.
            </Typography>
          )}
          {value.items.map((it, i) => (
            <Stack key={i} direction="row" spacing={1} alignItems="center">
              <TextField
                size="small"
                fullWidth
                value={it.description}
                onChange={(e) => setItem(i, { description: e.target.value })}
                placeholder="Description"
                sx={{ minWidth: 0, flex: 1 }}
              />
              <Box
                component={NumberInput}
                sx={{ width: 80, flexShrink: 0 }}
                min={0}
                step="any"
                value={it.quantity}
                onChange={(quantity: number) => setItem(i, { quantity })}
                placeholder="Qty"
              />
              <Box
                component={NumberInput}
                sx={{ width: 112, flexShrink: 0 }}
                min={0}
                step="any"
                value={it.unit_price}
                onChange={(unit_price: number) => setItem(i, { unit_price })}
                placeholder="Unit price"
              />
              <Button
                variant="outlined"
                color="inherit"
                size="small"
                onClick={() => set({ items: value.items.filter((_, idx) => idx !== i) })}
              >
                Remove
              </Button>
            </Stack>
          ))}
        </Stack>
        <Typography variant="body2" color="text.secondary" sx={{ mt: 1, textAlign: "right" }}>
          Line items total:{" "}
          <Box component="span" sx={{ fontWeight: 600, color: "text.primary" }}>
            {formatMoney(itemsTotal, value.currency)}
          </Box>
        </Typography>
      </Box>

      {/* Invoice total — may be entered directly; takes priority over the line items. */}
      <Box>
        <Typography variant="body2" sx={{ fontWeight: 600, mb: 0.5 }}>
          Invoice total
        </Typography>
        <Stack direction="row" spacing={1} alignItems="center">
          <SuffixNumber width={176} suffix={value.currency}>
            <Box
              component={NullableNumberInput}
              sx={{ width: "100%" }}
              min={0}
              step="any"
              value={value.entered_total}
              onChange={(entered_total: number | null) => set({ entered_total })}
              placeholder={itemsTotal.toFixed(2)}
            />
          </SuffixNumber>
          {hasEnteredTotal && (
            <Button variant="text" size="small" onClick={() => set({ entered_total: null })}>
              Use line items total
            </Button>
          )}
        </Stack>
        <Typography variant="caption" color="text.secondary" sx={{ display: "block", mt: 0.5 }}>
          Leave blank to use the line items total. When set, this value is the invoice total.
        </Typography>
        {mismatch && (
          <Typography variant="body2" color="warning.main" sx={{ mt: 0.5 }}>
            Entered total ({formatMoney(value.entered_total ?? 0, value.currency)}) differs from the
            line items total ({formatMoney(itemsTotal, value.currency)}). The entered total will be
            used.
          </Typography>
        )}
      </Box>

      {/* Budget-unit allocation */}
      <Box>
        <Box sx={{ mb: 1, display: "flex", alignItems: "center", justifyContent: "space-between", flexWrap: "wrap", gap: 1 }}>
          <Typography variant="body2" sx={{ fontWeight: 600 }}>
            Budget units
          </Typography>
          <Stack direction="row" spacing={1.5} alignItems="center">
            <RadioGroup
              row
              name="allocation_mode"
              value={value.allocation_mode}
              onChange={(e) => setMode(e.target.value as AllocationMode)}
            >
              <FormControlLabel
                value="percentage"
                control={<Radio size="small" />}
                label={<Typography variant="body2">Percentage</Typography>}
              />
              <FormControlLabel
                value="amount"
                control={<Radio size="small" />}
                label={<Typography variant="body2">Amount</Typography>}
              />
            </RadioGroup>
            <Button variant="text" size="small" startIcon={<Plus size={16} />} onClick={addAlloc}>
              Add budget unit
            </Button>
          </Stack>
        </Box>
        <Typography variant="caption" color="text.secondary" sx={{ display: "block", mb: 1 }}>
          Split this invoice across one or more budget units
          {value.allocation_mode === "percentage"
            ? " by percentage (must total 100%)"
            : " by amount (must total the invoice total)"}
          .
        </Typography>
        <Stack spacing={1}>
          {allocs.length === 0 && (
            <Typography variant="body2" color="text.secondary">
              No business units assigned.
            </Typography>
          )}
          {allocs.map((a, i) => {
            const taken = chosenIds(i);
            const resolved =
              value.allocation_mode === "percentage"
                ? (total * (a.value || 0)) / 100
                : total > 0
                  ? ((a.value || 0) / total) * 100
                  : 0;
            return (
              <Stack key={i} direction="row" spacing={1} alignItems="center">
                <TextField
                  select
                  size="small"
                  value={a.business_unit_id || ""}
                  onChange={(e) => setAlloc(i, { business_unit_id: Number(e.target.value) })}
                  sx={{ minWidth: 0, flex: 1 }}
                  SelectProps={{ displayEmpty: true }}
                >
                  <MenuItem value="">{buLoading ? "Loading…" : "Select business unit"}</MenuItem>
                  {(businessUnits ?? [])
                    .filter((c) => c.id === a.business_unit_id || !taken.has(c.id))
                    .map((c) => (
                      <MenuItem key={c.id} value={c.id}>
                        {c.name}
                      </MenuItem>
                    ))}
                </TextField>
                <SuffixNumber
                  width={128}
                  suffix={value.allocation_mode === "percentage" ? "%" : value.currency}
                >
                  <Box
                    component={NumberInput}
                    sx={{ width: "100%" }}
                    min={0}
                    step="any"
                    value={a.value}
                    onChange={(val: number) => setAlloc(i, { value: val })}
                  />
                </SuffixNumber>
                <Typography
                  variant="caption"
                  color="text.secondary"
                  sx={{ width: 144, flexShrink: 0, textAlign: "right" }}
                >
                  {value.allocation_mode === "percentage"
                    ? `= ${formatMoney(resolved, value.currency)}`
                    : `= ${resolved.toFixed(1)}%`}
                </Typography>
                <Button variant="outlined" color="inherit" size="small" onClick={() => removeAlloc(i)}>
                  Remove
                </Button>
              </Stack>
            );
          })}
        </Stack>
        {allocs.length > 0 && (
          <Typography
            variant="body2"
            sx={{ mt: 1, textAlign: "right" }}
            color={allocOk ? "text.secondary" : "error.main"}
          >
            Allocated:{" "}
            <Box component="span" sx={{ fontWeight: 600 }}>
              {value.allocation_mode === "percentage"
                ? `${allocSum.toFixed(2)}% / 100%`
                : `${formatMoney(allocSum, value.currency)} / ${formatMoney(allocTarget, value.currency)}`}
            </Box>
            {!allocOk && <Box component="span" sx={{ ml: 1 }}>— must add up before saving</Box>}
          </Typography>
        )}
      </Box>

      <TextField
        label="Note"
        size="small"
        fullWidth
        multiline
        minRows={3}
        value={value.note}
        onChange={(e) => set({ note: e.target.value })}
        placeholder="Any additional context"
      />
    </Stack>
  );
}
