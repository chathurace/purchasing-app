import { Box, Button, Stack, TextField, Typography } from "@wso2/oxygen-ui";
import { Plus } from "@wso2/oxygen-ui-icons-react";
import { NumberInput } from "./NumberInput";
import type { GRNInput } from "../types/api";

interface Props {
  value: GRNInput;
  onChange: (next: GRNInput) => void;
}

type GItem = { description: string; quantity: number };

export function GRNFields({ value, onChange }: Props) {
  const set = (patch: Partial<GRNInput>) => onChange({ ...value, ...patch });

  const setItem = (i: number, patch: Partial<GItem>) => {
    const items = value.items.map((it, idx) => (idx === i ? { ...it, ...patch } : it));
    set({ items });
  };

  return (
    <Stack spacing={2.5}>
      <Stack direction={{ xs: "column", sm: "row" }} spacing={1.5}>
        <TextField
          label="Received date"
          type="date"
          size="small"
          InputLabelProps={{ shrink: true }}
          value={value.received_date}
          onChange={(e) => set({ received_date: e.target.value })}
          sx={{ width: { xs: "100%", sm: 200 }, flexShrink: 0 }}
        />
        <TextField
          label="Received by"
          size="small"
          fullWidth
          value={value.received_by}
          onChange={(e) => set({ received_by: e.target.value })}
          placeholder="Name of the person who received the goods"
        />
      </Stack>

      <Box>
        <Box sx={{ mb: 1, display: "flex", alignItems: "center", justifyContent: "space-between" }}>
          <Typography variant="body2" sx={{ fontWeight: 600 }}>
            Items received
          </Typography>
          <Button
            variant="text"
            size="small"
            startIcon={<Plus size={16} />}
            onClick={() => set({ items: [...value.items, { description: "", quantity: 1 }] })}
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
                sx={{ width: 96, flexShrink: 0 }}
                min={0}
                step="any"
                value={it.quantity}
                onChange={(quantity: number) => setItem(i, { quantity })}
                placeholder="Qty"
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
        label="Note"
        size="small"
        fullWidth
        multiline
        minRows={3}
        value={value.note}
        onChange={(e) => set({ note: e.target.value })}
        placeholder="Condition, discrepancies, delivery reference…"
      />
    </Stack>
  );
}
