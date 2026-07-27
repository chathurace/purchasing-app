import { Box, Button, Stack, TextField, Typography } from "@wso2/oxygen-ui";
import { Plus } from "@wso2/oxygen-ui-icons-react";
import { VendorSelect } from "./VendorSelect";
import { NumberInput } from "./NumberInput";
import { CurrencyInput } from "./CurrencyInput";
import type { QuotationInput } from "../types/api";

interface Props {
  value: QuotationInput;
  onChange: (next: QuotationInput) => void;
}

type QItem = { description: string; quantity: number; unit_price: number };

export function QuotationFields({ value, onChange }: Props) {
  const set = (patch: Partial<QuotationInput>) => onChange({ ...value, ...patch });

  const setItem = (i: number, patch: Partial<QItem>) => {
    const items = value.items.map((it, idx) => (idx === i ? { ...it, ...patch } : it));
    set({ items });
  };

  return (
    <Stack spacing={2.5}>
      <VendorSelect value={value.vendor_id} onChange={(vendor_id) => set({ vendor_id })} />

      <Stack direction={{ xs: "column", sm: "row" }} spacing={1.5}>
        <Box sx={{ flex: 1 }}>
          <Typography variant="body2" sx={{ fontWeight: 500, mb: 0.5 }}>
            Total amount
          </Typography>
          <Box
            component={NumberInput}
            sx={{ width: "100%" }}
            min={0}
            step="any"
            value={value.total_amount}
            onChange={(total_amount: number) => set({ total_amount })}
          />
        </Box>
        <Box sx={{ width: { xs: "100%", sm: 112 }, flexShrink: 0 }}>
          <Typography variant="body2" sx={{ fontWeight: 500, mb: 0.5 }}>
            Currency
          </Typography>
          <Box
            component={CurrencyInput}
            sx={{ width: "100%" }}
            value={value.currency}
            maxLength={3}
            onChange={(currency: string) => set({ currency })}
            placeholder="USD"
          />
        </Box>
        <TextField
          label="Valid until"
          type="date"
          size="small"
          InputLabelProps={{ shrink: true }}
          value={value.valid_until ?? ""}
          onChange={(e) => set({ valid_until: e.target.value || null })}
          sx={{ width: { xs: "100%", sm: 200 }, flexShrink: 0, alignSelf: "flex-end" }}
        />
      </Stack>

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
      </Box>

      <TextField
        label="Notes"
        size="small"
        fullWidth
        multiline
        minRows={3}
        value={value.notes}
        onChange={(e) => set({ notes: e.target.value })}
        placeholder="Any additional context"
      />
    </Stack>
  );
}
