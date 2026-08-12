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
  /**
   * Name to seed the new-vendor form with — e.g. the vendor read off a quotation PDF.
   * Seeded at mount, so pass a `key` that changes when the suggestion does.
   */
  suggestedName?: string;
  /**
   * Start with the new-vendor form open instead of the picker. Used when we already
   * know nothing in the list matches, so creating the vendor is a single click.
   */
  defaultAdding?: boolean;
}

export function VendorSelect({ value, onChange, suggestedName, defaultAdding = false }: Props) {
  const qc = useQueryClient();
  const { data: vendors } = useVendors();
  const seededName = suggestedName?.trim() ?? "";
  const [adding, setAdding] = useState(defaultAdding);
  const [draft, setDraft] = useState<VendorInput>(() =>
    seededName ? { ...emptyVendor, name: seededName } : emptyVendor,
  );
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
          {/* Flagged while the name is still exactly what was read: a model can pick
              the buyer's name off the page instead of the issuer's, and this form
              creates a real vendor record. */}
          {seededName !== "" && draft.name === seededName && (
            <Typography variant="caption" color="text.secondary">
              Name read from the PDF — check it before adding.
            </Typography>
          )}
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
