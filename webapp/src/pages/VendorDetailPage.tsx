import { useState } from "react";
import { Link, useParams } from "react-router-dom";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  Chip,
  CircularProgress,
  Link as MuiLink,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import { useVendor, useVendorUsage } from "../hooks/useVendors";
import { useCanManageVendors } from "../hooks/useCanManageVendors";
import { updateVendor } from "../api/vendors";
import { ApiError } from "../api/client";
import { VendorFields } from "../components/VendorFields";
import { useConfirmAction } from "../components/ConfirmDialog";
import { vendorRef, type Vendor, type VendorInput } from "../types/api";

function toInput(v: Vendor): VendorInput {
  return {
    name: v.name,
    contact_name: v.contact_name,
    email: v.email,
    phone: v.phone,
    notes: v.notes,
    is_active: v.is_active,
    tax_id: v.tax_id,
    address_line: v.address_line,
    city: v.city,
    postal_code: v.postal_code,
    country: v.country,
    website: v.website,
    registered: v.registered,
  };
}

export function VendorDetailPage() {
  const { id } = useParams();
  const vendorId = Number(id);
  const canManage = useCanManageVendors();
  const qc = useQueryClient();
  const { data: vendor, isLoading, error } = useVendor(vendorId);
  const { data: usage } = useVendorUsage(vendorId);

  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState<VendorInput | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  // Deactivation is how a vendor is retired (they can't be deleted) — confirm it.
  const [confirmNode, confirmAction] = useConfirmAction();

  const invalidate = () => {
    qc.invalidateQueries({ queryKey: ["vendors"] });
    qc.invalidateQueries({ queryKey: ["vendors", vendorId] });
  };

  const save = useMutation({
    mutationFn: (input: VendorInput) => updateVendor(vendorId, { ...input, name: input.name.trim() }),
    onSuccess: () => {
      invalidate();
      setEditing(false);
      setActionError(null);
    },
    onError: (e) => setActionError(e instanceof ApiError ? e.message : "Failed to save"),
  });

  const toggleActive = useMutation({
    mutationFn: () =>
      updateVendor(vendorId, { ...toInput(vendor!), is_active: !vendor!.is_active }),
    onSuccess: invalidate,
    onError: (e) => setActionError(e instanceof ApiError ? e.message : "Failed to update status"),
  });

  if (!canManage) {
    return (
      <Box sx={{ maxWidth: 1200, mx: "auto", p: { xs: 2, md: 4 } }}>
        <Card variant="outlined" sx={{ borderStyle: "dashed" }}>
          <CardContent sx={{ textAlign: "center", py: 6 }}>
            <Typography color="text.secondary">
              You need the admin or procurement_admin role to manage vendors.
            </Typography>
          </CardContent>
        </Card>
      </Box>
    );
  }
  if (isLoading)
    return (
      <Box sx={{ maxWidth: 1200, mx: "auto", p: { xs: 2, md: 4 } }}>
        <CircularProgress size={24} />
      </Box>
    );
  if (error || !vendor)
    return (
      <Box sx={{ maxWidth: 1200, mx: "auto", p: { xs: 2, md: 4 } }}>
        <Alert severity="error">Failed to load vendor.</Alert>
      </Box>
    );

  const startEdit = () => {
    setDraft(toInput(vendor));
    setEditing(true);
    setActionError(null);
  };

  return (
    <Box sx={{ maxWidth: 1200, mx: "auto", p: { xs: 2, md: 4 } }}>
      <Stack direction="row" spacing={1} alignItems="center" sx={{ mb: 2 }}>
        <MuiLink component={Link} to="/vendors" variant="body2">
          Vendors
        </MuiLink>
        <Typography variant="body2" color="text.secondary">
          /
        </Typography>
        <Typography variant="body2" color="text.secondary">
          {vendorRef(vendor.id)}
        </Typography>
      </Stack>

      <Stack
        direction="row"
        justifyContent="space-between"
        alignItems="flex-start"
        sx={{ mb: 2 }}
      >
        <Box>
          <Typography variant="h5" sx={{ fontWeight: 600 }}>
            {vendor.name}
          </Typography>
          <Stack direction="row" spacing={1} alignItems="center" sx={{ mt: 1 }}>
            <Chip
              size="small"
              variant="outlined"
              color={vendor.is_active ? "success" : "error"}
              label={vendor.is_active ? "Active" : "Inactive"}
            />
            {vendor.registered && (
              <Chip size="small" variant="outlined" color="info" label="Registered" />
            )}
          </Stack>
        </Box>
        {!editing && (
          <Stack direction="row" spacing={1}>
            <Button variant="outlined" color="inherit" onClick={startEdit}>
              Edit details
            </Button>
            <Button
              variant="outlined"
              color="inherit"
              onClick={() => {
                // Reactivating is additive — only guard the removing direction.
                if (!vendor.is_active) {
                  toggleActive.mutate();
                  return;
                }
                confirmAction({
                  title: "Deactivate vendor",
                  message: (
                    <>
                      Deactivate <strong>{vendor.name}</strong>? They stop being offered when adding
                      quotations or contracts. Existing records keep the vendor, and you can
                      reactivate them later.
                    </>
                  ),
                  confirmLabel: "Deactivate",
                  onConfirm: () => toggleActive.mutate(),
                });
              }}
              disabled={toggleActive.isPending}
            >
              {vendor.is_active ? "Deactivate" : "Reactivate"}
            </Button>
          </Stack>
        )}
      </Stack>

      {actionError && (
        <Alert severity="error" sx={{ mb: 2 }}>
          {actionError}
        </Alert>
      )}

      <Card variant="outlined">
        <CardContent>
          {editing && draft ? (
            <Stack spacing={2}>
              <VendorFields value={draft} onChange={setDraft} />
              <Stack direction="row" spacing={1}>
                <Button
                  variant="contained"
                  onClick={() => save.mutate(draft)}
                  disabled={save.isPending || draft.name.trim() === ""}
                >
                  {save.isPending ? "Saving…" : "Save changes"}
                </Button>
                <Button
                  variant="outlined"
                  color="inherit"
                  onClick={() => {
                    setEditing(false);
                    setActionError(null);
                  }}
                >
                  Cancel
                </Button>
              </Stack>
            </Stack>
          ) : (
            <Box
              sx={{
                display: "grid",
                gridTemplateColumns: { xs: "1fr", sm: "1fr 1fr" },
                columnGap: 3,
                rowGap: 2,
              }}
            >
              <Field label="Contact name" value={vendor.contact_name} />
              <Field label="Tax / registration ID" value={vendor.tax_id} />
              <Field label="Email" value={vendor.email} />
              <Field label="Phone" value={vendor.phone} />
              <Field label="Website" value={vendor.website} />
              <Field label="Address" value={vendor.address_line} />
              <Field
                label="City / postal / country"
                value={[vendor.city, vendor.postal_code, vendor.country].filter(Boolean).join(", ")}
              />
              <Box sx={{ gridColumn: { sm: "1 / -1" } }}>
                <Typography variant="caption" color="text.secondary" sx={{ fontWeight: 600 }}>
                  Notes
                </Typography>
                <Typography variant="body2" sx={{ whiteSpace: "pre-wrap" }}>
                  {vendor.notes || "—"}
                </Typography>
              </Box>
            </Box>
          )}
        </CardContent>
      </Card>

      {/* Where used */}
      <Card variant="outlined" sx={{ mt: 3 }}>
        <CardContent>
          <Typography variant="subtitle1" sx={{ fontWeight: 600, mb: 2 }}>
            Where used
          </Typography>
          {!usage ? (
            <Typography variant="body2" color="text.secondary">
              Loading…
            </Typography>
          ) : (
            <Box
              sx={{
                display: "grid",
                gridTemplateColumns: "repeat(4, 1fr)",
                gap: 1.5,
              }}
            >
              <UsageStat to={`/quotations?vendor=${vendor.id}`} label="Quotations" count={usage.quotations} />
              <UsageStat to={`/contracts?vendor=${vendor.id}`} label="Contracts" count={usage.contracts} />
              <UsageStat to={`/grns?vendor=${vendor.id}`} label="GRNs" count={usage.grns} />
              <UsageStat to={`/invoices?vendor=${vendor.id}`} label="Invoices" count={usage.invoices} />
            </Box>
          )}
          <Typography variant="caption" color="text.secondary" sx={{ display: "block", mt: 2 }}>
            Vendors referenced by these records cannot be deleted — deactivate instead.
          </Typography>
        </CardContent>
      </Card>
      {confirmNode}
    </Box>
  );
}

function Field({ label, value }: { label: string; value: string }) {
  return (
    <Box>
      <Typography variant="caption" color="text.secondary" sx={{ fontWeight: 600 }}>
        {label}
      </Typography>
      <Typography variant="body2">{value || "—"}</Typography>
    </Box>
  );
}

function UsageStat({ to, label, count }: { to: string; label: string; count: number }) {
  return (
    <Box
      component={Link}
      to={to}
      sx={{
        textAlign: "center",
        p: 1.5,
        border: 1,
        borderColor: "divider",
        borderRadius: 2,
        textDecoration: "none",
        color: "inherit",
        "&:hover": { bgcolor: "action.hover" },
      }}
    >
      <Typography variant="h5" sx={{ fontWeight: 600 }}>
        {count}
      </Typography>
      <Typography variant="caption" color="text.secondary">
        {label}
      </Typography>
    </Box>
  );
}
