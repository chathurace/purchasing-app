import { useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  Alert,
  Box,
  Button,
  Chip,
  CircularProgress,
  Link as MuiLink,
  MenuItem,
  Paper,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  TextField,
  Typography,
} from "@wso2/oxygen-ui";
import { useVendors } from "../hooks/useVendors";
import { useCanManageVendors } from "../hooks/useCanManageVendors";
import { createVendor } from "../api/vendors";
import { ApiError } from "../api/client";
import { VendorFields, emptyVendor } from "../components/VendorFields";
import { vendorRef, type VendorInput } from "../types/api";

type StatusFilter = "active" | "inactive" | "all";

export function VendorListPage() {
  const canManage = useCanManageVendors();
  const { data: vendors, isLoading, error } = useVendors();
  const qc = useQueryClient();

  const [search, setSearch] = useState("");
  const [statusFilter, setStatusFilter] = useState<StatusFilter>("active");
  const [adding, setAdding] = useState(false);
  const [draft, setDraft] = useState<VendorInput>(emptyVendor);
  const [formError, setFormError] = useState<string | null>(null);

  const create = useMutation({
    mutationFn: () => createVendor({ ...draft, name: draft.name.trim() }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["vendors"] });
      setAdding(false);
      setDraft(emptyVendor);
      setFormError(null);
    },
    onError: (e) => setFormError(e instanceof ApiError ? e.message : "Failed to create vendor"),
  });

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    return (vendors ?? [])
      .filter((v) =>
        statusFilter === "all" ? true : statusFilter === "active" ? v.is_active : !v.is_active,
      )
      .filter((v) =>
        q === ""
          ? true
          : v.name.toLowerCase().includes(q) ||
            v.contact_name.toLowerCase().includes(q) ||
            v.email.toLowerCase().includes(q),
      );
  }, [vendors, search, statusFilter]);

  if (!canManage) {
    return (
      <Box sx={{ maxWidth: 1200, mx: "auto", p: { xs: 2, md: 4 } }}>
        <Alert severity="info">You need the admin or procurement_admin role to manage vendors.</Alert>
      </Box>
    );
  }

  return (
    <Box sx={{ maxWidth: 1200, mx: "auto", p: { xs: 2, md: 4 } }}>
      <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
        <Typography variant="h5" sx={{ fontWeight: 600 }}>
          Vendors
        </Typography>
        <Button
          variant="contained"
          onClick={() => {
            setAdding((a) => !a);
            setFormError(null);
            setDraft(emptyVendor);
          }}
        >
          {adding ? "Cancel" : "New vendor"}
        </Button>
      </Box>
      <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5, mb: 3 }}>
        Manage the vendor master used across quotations, contracts, GRNs and invoices. Deactivate a
        vendor to retire it without losing history — inactive vendors can no longer be selected on
        new records.
      </Typography>

      {adding && (
        <Paper
          component="form"
          variant="outlined"
          sx={{ mb: 3, p: 2 }}
          onSubmit={(e) => {
            e.preventDefault();
            create.mutate();
          }}
        >
          <VendorFields value={draft} onChange={setDraft} />
          {formError && (
            <Alert severity="error" sx={{ mt: 2 }}>
              {formError}
            </Alert>
          )}
          <Box sx={{ mt: 2 }}>
            <Button
              type="submit"
              variant="contained"
              disabled={create.isPending || draft.name.trim() === ""}
            >
              {create.isPending ? "Adding…" : "Add vendor"}
            </Button>
          </Box>
        </Paper>
      )}

      <Box sx={{ mb: 2, display: "flex", flexWrap: "wrap", alignItems: "center", gap: 1.5 }}>
        <TextField
          size="small"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="Search by name, contact or email…"
          sx={{ width: 288 }}
        />
        <TextField
          select
          size="small"
          value={statusFilter}
          onChange={(e) => setStatusFilter(e.target.value as StatusFilter)}
          sx={{ minWidth: 120 }}
        >
          <MenuItem value="active">Active</MenuItem>
          <MenuItem value="inactive">Inactive</MenuItem>
          <MenuItem value="all">All</MenuItem>
        </TextField>
      </Box>

      {isLoading && <CircularProgress size={24} />}
      {error && <Alert severity="error">Failed to load vendors.</Alert>}

      {vendors && (
        <TableContainer component={Paper} variant="outlined">
          <Table size="small">
            <TableHead>
              <TableRow>
                <TableCell>Vendor</TableCell>
                <TableCell>Contact</TableCell>
                <TableCell>Status</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {filtered.length === 0 && (
                <TableRow>
                  <TableCell colSpan={3} align="center" sx={{ py: 4, color: "text.disabled" }}>
                    No vendors match.
                  </TableCell>
                </TableRow>
              )}
              {filtered.map((v) => (
                <TableRow key={v.id} sx={v.is_active ? undefined : { bgcolor: "action.hover" }}>
                  <TableCell sx={{ verticalAlign: "top" }}>
                    <MuiLink component={Link} to={`/vendors/${v.id}`} sx={{ fontWeight: 500 }}>
                      {v.name}
                    </MuiLink>
                    <Typography variant="caption" color="text.disabled" display="block">
                      {vendorRef(v.id)}
                    </Typography>
                  </TableCell>
                  <TableCell sx={{ verticalAlign: "top" }}>
                    <Typography variant="body2">{v.contact_name || "—"}</Typography>
                    {v.email && (
                      <Typography variant="body2" color="text.secondary">
                        {v.email}
                      </Typography>
                    )}
                  </TableCell>
                  <TableCell sx={{ verticalAlign: "top" }}>
                    <Stack direction="row" spacing={0.75} flexWrap="wrap" alignItems="center">
                      <Chip
                        size="small"
                        variant="outlined"
                        color={v.is_active ? "success" : "error"}
                        label={v.is_active ? "Active" : "Inactive"}
                      />
                      {v.registered && (
                        <Chip size="small" variant="outlined" color="primary" label="Registered" />
                      )}
                    </Stack>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableContainer>
      )}
    </Box>
  );
}
