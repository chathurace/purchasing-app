import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Box, Button, MenuItem, Stack, TextField, Typography } from "@wso2/oxygen-ui";
import { useVendors } from "../hooks/useVendors";
import { createVendor } from "../api/vendors";
import { ApiError } from "../api/client";
import { emptyVendor } from "./VendorFields";
import type { VendorInput } from "../types/api";

interface Props {
  value: number; // selected vendor id (0 = none)
  onChange: (vendorId: number) => void;
}

export function VendorSelect({ value, onChange }: Props) {
  const qc = useQueryClient();
  const { data: vendors } = useVendors();
  const [adding, setAdding] = useState(false);
  const [draft, setDraft] = useState<VendorInput>(emptyVendor);
  const [error, setError] = useState<string | null>(null);

  const createMutation = useMutation({
    mutationFn: () => createVendor({ ...draft, name: draft.name.trim() }),
    onSuccess: (v) => {
      qc.invalidateQueries({ queryKey: ["vendors"] });
      onChange(v.id);
      setAdding(false);
      setDraft(emptyVendor);
      setError(null);
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : "Failed to create vendor"),
  });

  return (
    <Box>
      <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", mb: 0.5 }}>
        <Typography component="label" variant="subtitle2" sx={{ fontWeight: 600 }}>
          Vendor
        </Typography>
        <Button variant="text" size="small" onClick={() => setAdding((a) => !a)}>
          {adding ? "Cancel" : "+ New vendor"}
        </Button>
      </Box>

      {!adding && (
        <TextField
          select
          size="small"
          fullWidth
          value={value || ""}
          onChange={(e) => onChange(Number(e.target.value))}
        >
          <MenuItem value="">Select a vendor…</MenuItem>
          {vendors
            // Only active vendors can be picked for new records, but keep the
            // currently selected vendor visible even if it was deactivated.
            ?.filter((v) => v.is_active || v.id === value)
            .map((v) => {
              const tags = [
                !v.is_active ? "inactive" : null,
                !v.registered ? "unregistered" : null,
              ].filter(Boolean);
              return (
                <MenuItem key={v.id} value={v.id}>
                  {v.name}
                  {tags.length ? ` (${tags.join(", ")})` : ""}
                </MenuItem>
              );
            })}
        </TextField>
      )}

      {adding && (
        <Stack
          spacing={1}
          sx={{ p: 1.5, borderRadius: 1, bgcolor: "background.default", border: 1, borderColor: "divider" }}
        >
          <TextField
            size="small"
            fullWidth
            value={draft.name}
            onChange={(e) => setDraft({ ...draft, name: e.target.value })}
            placeholder="Vendor name *"
          />
          <Stack direction="row" spacing={1}>
            <TextField
              size="small"
              fullWidth
              value={draft.contact_name}
              onChange={(e) => setDraft({ ...draft, contact_name: e.target.value })}
              placeholder="Contact name"
            />
            <TextField
              size="small"
              fullWidth
              value={draft.email}
              onChange={(e) => setDraft({ ...draft, email: e.target.value })}
              placeholder="Email"
            />
          </Stack>
          {error && (
            <Typography variant="body2" color="error.main">
              {error}
            </Typography>
          )}
          <Box>
            <Button
              variant="contained"
              size="small"
              disabled={createMutation.isPending || draft.name.trim() === ""}
              onClick={() => createMutation.mutate()}
            >
              {createMutation.isPending ? "Adding…" : "Add vendor"}
            </Button>
          </Box>
        </Stack>
      )}
    </Box>
  );
}
