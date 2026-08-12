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
import {
  useBusinessUnit,
  useBusinessUnitInvoices,
  useBusinessUnitUsage,
} from "../hooks/useBusinessUnits";
import { useCanManageBusinessUnits } from "../hooks/useCanManageBusinessUnits";
import { updateBusinessUnit } from "../api/businessUnits";
import { ApiError } from "../api/client";
import { BusinessUnitFields } from "../components/BusinessUnitFields";
import { useConfirmAction } from "../components/ConfirmDialog";
import {
  formatMoney,
  type BusinessUnit,
  type BusinessUnitInput,
  type BusinessUnitInvoiceCategory,
} from "../types/api";

function toInput(c: BusinessUnit): BusinessUnitInput {
  return {
    name: c.name,
    description: c.description,
    is_active: c.is_active,
    approver_ids: c.approvers.map((u) => u.id),
  };
}

export function BusinessUnitDetailPage() {
  const { id } = useParams();
  const businessUnitId = Number(id);
  const canManage = useCanManageBusinessUnits();
  const qc = useQueryClient();
  const { data: businessUnit, isLoading, error } = useBusinessUnit(businessUnitId);
  const { data: usage } = useBusinessUnitUsage(businessUnitId);
  const { data: invoices } = useBusinessUnitInvoices(businessUnitId);

  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState<BusinessUnitInput | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  // Deactivation is how a unit is retired (it can't be deleted) — confirm it.
  const [confirmNode, confirmAction] = useConfirmAction();

  const invalidate = () => {
    qc.invalidateQueries({ queryKey: ["business_units"] });
    qc.invalidateQueries({ queryKey: ["business_units", businessUnitId] });
  };

  const save = useMutation({
    mutationFn: (input: BusinessUnitInput) =>
      updateBusinessUnit(businessUnitId, { ...input, name: input.name.trim() }),
    onSuccess: () => {
      invalidate();
      setEditing(false);
      setActionError(null);
    },
    onError: (e) => setActionError(e instanceof ApiError ? e.message : "Failed to save"),
  });

  const toggleActive = useMutation({
    mutationFn: () =>
      updateBusinessUnit(businessUnitId, {
        ...toInput(businessUnit!),
        is_active: !businessUnit!.is_active,
      }),
    onSuccess: invalidate,
    onError: (e) => setActionError(e instanceof ApiError ? e.message : "Failed to update status"),
  });

  if (!canManage) {
    return (
      <Box sx={{ maxWidth: 1200, mx: "auto", p: { xs: 2, md: 4 } }}>
        <Card variant="outlined" sx={{ borderStyle: "dashed" }}>
          <CardContent sx={{ textAlign: "center", py: 6 }}>
            <Typography color="text.secondary">
              You need the admin or procurement_admin role to manage business units.
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
  if (error || !businessUnit)
    return (
      <Box sx={{ maxWidth: 1200, mx: "auto", p: { xs: 2, md: 4 } }}>
        <Alert severity="error">Failed to load business unit.</Alert>
      </Box>
    );

  const startEdit = () => {
    setDraft(toInput(businessUnit));
    setEditing(true);
    setActionError(null);
  };

  return (
    <Box sx={{ maxWidth: 1200, mx: "auto", p: { xs: 2, md: 4 } }}>
      <Stack direction="row" spacing={1} alignItems="center" sx={{ mb: 2 }}>
        <MuiLink component={Link} to="/business-units" variant="body2">
          Business units
        </MuiLink>
        <Typography variant="body2" color="text.secondary">
          /
        </Typography>
        <Typography variant="body2" color="text.secondary">
          {businessUnit.name}
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
            {businessUnit.name}
          </Typography>
          <Box sx={{ mt: 1 }}>
            <Chip
              size="small"
              variant="outlined"
              color={businessUnit.is_active ? "success" : "error"}
              label={businessUnit.is_active ? "Active" : "Inactive"}
            />
          </Box>
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
                if (!businessUnit.is_active) {
                  toggleActive.mutate();
                  return;
                }
                confirmAction({
                  title: "Deactivate business unit",
                  message: (
                    <>
                      Deactivate <strong>{businessUnit.name}</strong>? It stops being offered on the
                      requisition form, so no new request can pick it or its approvers. Existing
                      requests are unaffected, and you can reactivate it later.
                    </>
                  ),
                  confirmLabel: "Deactivate",
                  onConfirm: () => toggleActive.mutate(),
                });
              }}
              disabled={toggleActive.isPending}
            >
              {businessUnit.is_active ? "Deactivate" : "Reactivate"}
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
              <BusinessUnitFields value={draft} onChange={setDraft} />
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
            <>
              <Box sx={{ mb: 3 }}>
                <Typography variant="caption" color="text.secondary" sx={{ fontWeight: 600 }}>
                  Description
                </Typography>
                <Typography variant="body2" sx={{ whiteSpace: "pre-wrap" }}>
                  {businessUnit.description || "—"}
                </Typography>
              </Box>
              <Box>
                <Typography variant="subtitle2" sx={{ fontWeight: 600, mb: 1 }}>
                  Approvers
                </Typography>
                {businessUnit.approvers.length === 0 ? (
                  <Typography variant="body2" color="text.secondary">
                    No approvers configured.
                  </Typography>
                ) : (
                  <Stack spacing={0.5}>
                    {businessUnit.approvers.map((u) => (
                      <Typography key={u.id} variant="body2">
                        {u.name ? `${u.name} (${u.email})` : u.email}
                      </Typography>
                    ))}
                  </Stack>
                )}
              </Box>
            </>
          )}
        </CardContent>
      </Card>

      {/* Invoices allocated to this business unit, by status */}
      <Card variant="outlined" sx={{ mt: 3 }}>
        <CardContent>
          <Typography variant="subtitle1" sx={{ fontWeight: 600 }}>
            Invoices
          </Typography>
          <Typography variant="caption" color="text.secondary" sx={{ display: "block", mb: 2 }}>
            Each invoice's allocated share to this business unit, grouped by status.
          </Typography>
          {!invoices ? (
            <Typography variant="body2" color="text.secondary">
              Loading…
            </Typography>
          ) : (
            <Box
              sx={{
                display: "grid",
                gridTemplateColumns: "repeat(3, 1fr)",
                gap: 1.5,
              }}
            >
              <InvoiceCategory label="Pending" category={invoices.pending} />
              <InvoiceCategory label="Approved" category={invoices.approved} />
              <InvoiceCategory label="Paid" category={invoices.paid} />
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
                gridTemplateColumns: "repeat(2, 1fr)",
                gap: 1.5,
              }}
            >
              <Box
                component={Link}
                to={`/requests?business_unit=${businessUnit.id}`}
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
                  {usage.purchase_requests}
                </Typography>
                <Typography variant="caption" color="text.secondary">
                  Purchase requests
                </Typography>
              </Box>
            </Box>
          )}
          <Typography variant="caption" color="text.secondary" sx={{ display: "block", mt: 2 }}>
            Business units referenced by these records cannot be deleted — deactivate instead.
          </Typography>
        </CardContent>
      </Card>
      {confirmNode}
    </Box>
  );
}

function InvoiceCategory({
  label,
  category,
}: {
  label: string;
  category: BusinessUnitInvoiceCategory;
}) {
  return (
    <Box sx={{ p: 1.5, border: 1, borderColor: "divider", borderRadius: 2 }}>
      <Stack direction="row" justifyContent="space-between" alignItems="baseline">
        <Typography variant="body2" sx={{ fontWeight: 600 }}>
          {label}
        </Typography>
        <Typography variant="caption" color="text.secondary">
          {category.count} {category.count === 1 ? "invoice" : "invoices"}
        </Typography>
      </Stack>
      <Box sx={{ mt: 1 }}>
        {category.totals.length === 0 ? (
          <Typography variant="h6" color="text.secondary">
            —
          </Typography>
        ) : (
          category.totals.map((t) => (
            <Typography key={t.currency} variant="subtitle1" sx={{ fontWeight: 600 }}>
              {formatMoney(t.amount, t.currency)}
            </Typography>
          ))
        )}
      </Box>
    </Box>
  );
}
