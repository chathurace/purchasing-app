import { Box, Stack, TextField, Typography } from "@wso2/oxygen-ui";
import { NumberInput } from "./NumberInput";
import { CurrencyInput } from "./CurrencyInput";
import type { ContractInput } from "../types/api";

interface Props {
  value: ContractInput;
  onChange: (next: ContractInput) => void;
}

// Shared styling for the specialized numeric/currency inputs (NumberInput /
// CurrencyInput keep their own controlled behaviour, so they render as native
// inputs styled here to match the MUI TextFields, using theme tokens only).
const fieldSx = {
  width: "100%",
  boxSizing: "border-box",
  border: "1px solid",
  borderColor: "divider",
  borderRadius: 1,
  bgcolor: "background.paper",
  color: "text.primary",
  fontFamily: "inherit",
  fontSize: "0.875rem",
  px: 1.5,
  py: 1,
  "&:focus": { outline: "none", borderColor: "primary.main" },
  "&::placeholder": { color: "text.disabled" },
} as const;

function Field({
  label,
  sx,
  children,
}: {
  label: React.ReactNode;
  sx?: object;
  children: React.ReactNode;
}) {
  return (
    <Box sx={sx}>
      <Typography variant="body2" sx={{ fontWeight: 500, mb: 0.5 }}>
        {label}
      </Typography>
      {children}
    </Box>
  );
}

export function ContractFields({ value, onChange }: Props) {
  const set = (patch: Partial<ContractInput>) => onChange({ ...value, ...patch });

  return (
    <Stack spacing={2.5}>
      <Field label="Title">
        <TextField
          size="small"
          fullWidth
          value={value.title}
          onChange={(e) => set({ title: e.target.value })}
          placeholder="Contract title"
        />
      </Field>

      <Stack direction="row" spacing={1.5}>
        <Field label="Total amount" sx={{ flex: 1 }}>
          <Box
            component={NumberInput}
            sx={fieldSx}
            min={0}
            step="any"
            value={value.total_amount}
            onChange={(total_amount: number) => set({ total_amount })}
          />
        </Field>
        <Field label="Currency" sx={{ width: 112, flexShrink: 0 }}>
          <Box
            component={CurrencyInput}
            sx={fieldSx}
            value={value.currency}
            maxLength={3}
            onChange={(currency: string) => set({ currency })}
            placeholder="USD"
          />
        </Field>
      </Stack>

      <Field label="Terms">
        <TextField
          size="small"
          fullWidth
          multiline
          minRows={5}
          value={value.terms}
          onChange={(e) => set({ terms: e.target.value })}
          placeholder="Key contract terms"
        />
      </Field>
    </Stack>
  );
}
