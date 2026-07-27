import { Box, Checkbox, FormControlLabel, Stack, TextField, Typography } from "@wso2/oxygen-ui";
import type { VendorInput } from "../types/api";

// A fresh, active vendor draft. Shared by the management page and the inline
// VendorSelect quick-add form.
export const emptyVendor: VendorInput = {
  name: "",
  contact_name: "",
  email: "",
  phone: "",
  notes: "",
  is_active: true,
  tax_id: "",
  address_line: "",
  city: "",
  postal_code: "",
  country: "",
  website: "",
  registered: false,
};

interface Props {
  value: VendorInput;
  onChange: (next: VendorInput) => void;
}

// VendorFields edits everything except the active/inactive status, which is
// controlled separately via an Activate/Deactivate action.
export function VendorFields({ value, onChange }: Props) {
  const set = (patch: Partial<VendorInput>) => onChange({ ...value, ...patch });

  return (
    <Stack spacing={2}>
      <TextField
        label="Name"
        required
        size="small"
        fullWidth
        value={value.name}
        onChange={(e) => set({ name: e.target.value })}
        placeholder="Vendor / company name"
      />

      <Stack direction={{ xs: "column", sm: "row" }} spacing={1.5}>
        <TextField
          label="Contact name"
          size="small"
          fullWidth
          value={value.contact_name}
          onChange={(e) => set({ contact_name: e.target.value })}
          placeholder="Primary contact"
        />
        <TextField
          label="Tax / registration ID"
          size="small"
          fullWidth
          value={value.tax_id}
          onChange={(e) => set({ tax_id: e.target.value })}
          placeholder="e.g. VAT / EIN"
        />
      </Stack>

      <Stack direction={{ xs: "column", sm: "row" }} spacing={1.5}>
        <TextField
          label="Email"
          size="small"
          fullWidth
          value={value.email}
          onChange={(e) => set({ email: e.target.value })}
          placeholder="billing@vendor.com"
        />
        <TextField
          label="Phone"
          size="small"
          fullWidth
          value={value.phone}
          onChange={(e) => set({ phone: e.target.value })}
          placeholder="+1 555 000 0000"
        />
      </Stack>

      <TextField
        label="Website"
        size="small"
        fullWidth
        value={value.website}
        onChange={(e) => set({ website: e.target.value })}
        placeholder="https://vendor.com"
      />

      <TextField
        label="Address"
        size="small"
        fullWidth
        value={value.address_line}
        onChange={(e) => set({ address_line: e.target.value })}
        placeholder="Street address"
      />

      <Stack direction={{ xs: "column", sm: "row" }} spacing={1.5}>
        <TextField
          label="City"
          size="small"
          fullWidth
          value={value.city}
          onChange={(e) => set({ city: e.target.value })}
        />
        <TextField
          label="Postal code"
          size="small"
          value={value.postal_code}
          onChange={(e) => set({ postal_code: e.target.value })}
          sx={{ width: { xs: "100%", sm: 128 }, flexShrink: 0 }}
        />
        <TextField
          label="Country"
          size="small"
          fullWidth
          value={value.country}
          onChange={(e) => set({ country: e.target.value })}
        />
      </Stack>

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

      <FormControlLabel
        sx={{ alignItems: "flex-start", m: 0 }}
        control={
          <Checkbox
            size="small"
            checked={value.registered}
            onChange={(e) => set({ registered: e.target.checked })}
          />
        }
        label={
          <Box sx={{ pt: 0.5 }}>
            <Typography variant="body2" sx={{ fontWeight: 600 }}>
              Registered vendor
            </Typography>
            <Typography variant="caption" color="text.secondary">
              Mark vendors that have completed formal registration. Shown when picking a vendor on
              quotations and recommendations.
            </Typography>
          </Box>
        }
      />
    </Stack>
  );
}
