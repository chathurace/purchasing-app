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
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  TextField,
  Typography,
} from "@wso2/oxygen-ui";
import { useBusinessUnits } from "../hooks/useBusinessUnits";
import { useCanManageBusinessUnits } from "../hooks/useCanManageBusinessUnits";
import { createBusinessUnit } from "../api/businessUnits";
import { ApiError } from "../api/client";
import { BusinessUnitFields } from "../components/BusinessUnitFields";
import { emptyBusinessUnit, type BusinessUnitInput } from "../types/api";

type StatusFilter = "active" | "inactive" | "all";

export function BusinessUnitListPage() {
  const canManage = useCanManageBusinessUnits();
  const { data: businessUnits, isLoading, error } = useBusinessUnits();
  const qc = useQueryClient();

  const [search, setSearch] = useState("");
  const [statusFilter, setStatusFilter] = useState<StatusFilter>("active");
  const [adding, setAdding] = useState(false);
  const [draft, setDraft] = useState<BusinessUnitInput>(emptyBusinessUnit);
  const [formError, setFormError] = useState<string | null>(null);

  const create = useMutation({
    mutationFn: () => createBusinessUnit({ ...draft, name: draft.name.trim() }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["business_units"] });
      setAdding(false);
      setDraft(emptyBusinessUnit);
      setFormError(null);
    },
    onError: (e) =>
      setFormError(e instanceof ApiError ? e.message : "Failed to create business unit"),
  });

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    return (businessUnits ?? [])
      .filter((c) =>
        statusFilter === "all" ? true : statusFilter === "active" ? c.is_active : !c.is_active,
      )
      .filter((c) => (q === "" ? true : c.name.toLowerCase().includes(q)));
  }, [businessUnits, search, statusFilter]);

  if (!canManage) {
    return (
      <Box sx={{ maxWidth: 1200, mx: "auto", p: { xs: 2, md: 4 } }}>
        <Alert severity="info">
          You need the admin or procurement_admin role to manage business units.
        </Alert>
      </Box>
    );
  }

  return (
    <Box sx={{ maxWidth: 1200, mx: "auto", p: { xs: 2, md: 4 } }}>
      <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
        <Typography variant="h5" sx={{ fontWeight: 600 }}>
          Business units
        </Typography>
        <Button
          variant="contained"
          onClick={() => {
            setAdding((a) => !a);
            setFormError(null);
            setDraft(emptyBusinessUnit);
          }}
        >
          {adding ? "Cancel" : "New business unit"}
        </Button>
      </Box>
      <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5, mb: 3 }}>
        Manage the business units selectable on purchase requests. Each unit has a list of budget
        approvers; the requester picks one when they select the unit. Deactivate a unit to retire it
        without losing history — inactive units can no longer be selected on new requests.
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
          <BusinessUnitFields value={draft} onChange={setDraft} />
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
              {create.isPending ? "Adding…" : "Add business unit"}
            </Button>
          </Box>
        </Paper>
      )}

      <Box sx={{ mb: 2, display: "flex", flexWrap: "wrap", alignItems: "center", gap: 1.5 }}>
        <TextField
          size="small"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="Search by name…"
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
      {error && <Alert severity="error">Failed to load business units.</Alert>}

      {businessUnits && (
        <TableContainer component={Paper} variant="outlined">
          <Table size="small">
            <TableHead>
              <TableRow>
                <TableCell>Business unit</TableCell>
                <TableCell>Approvers</TableCell>
                <TableCell>Status</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {filtered.length === 0 && (
                <TableRow>
                  <TableCell colSpan={3} align="center" sx={{ py: 4, color: "text.disabled" }}>
                    No business units match.
                  </TableCell>
                </TableRow>
              )}
              {filtered.map((c) => (
                <TableRow key={c.id} sx={c.is_active ? undefined : { bgcolor: "action.hover" }}>
                  <TableCell sx={{ verticalAlign: "top" }}>
                    <MuiLink component={Link} to={`/business-units/${c.id}`} sx={{ fontWeight: 500 }}>
                      {c.name}
                    </MuiLink>
                    {c.description && (
                      <Typography variant="caption" color="text.disabled" display="block">
                        {c.description}
                      </Typography>
                    )}
                  </TableCell>
                  <TableCell sx={{ verticalAlign: "top" }}>
                    {c.approvers.length} {c.approvers.length === 1 ? "approver" : "approvers"}
                  </TableCell>
                  <TableCell sx={{ verticalAlign: "top" }}>
                    <Chip
                      size="small"
                      variant="outlined"
                      color={c.is_active ? "success" : "error"}
                      label={c.is_active ? "Active" : "Inactive"}
                    />
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
