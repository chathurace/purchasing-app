import { useState } from "react";
import { Navigate } from "react-router-dom";
import { Box, Button, MenuItem, TextField } from "@wso2/oxygen-ui";
import { useAllPurchaseRequests } from "../hooks/usePurchaseRequests";
import { useProcurementAccess } from "../hooks/useProcurementAccess";
import { useMe } from "../hooks/useMe";
import { useBusinessUnitLookup } from "../hooks/useBusinessUnits";
import { useVendorLookup } from "../hooks/useVendors";
import { useUserLookup } from "../hooks/useUserLookup";
import { PurchaseRequestsList } from "../components/PurchaseRequestsList";
import { UserComboBox } from "../components/UserComboBox";
import { PR_STATUS_LABELS, PR_STATUSES, type PRStatus } from "../types/api";
import type { PRListFilters } from "../api/purchaseRequests";

// A blank value in a <select> maps to "no filter" for that dimension.
type NumOrBlank = number | "";

// PurchaseRequestListPage is the full procurement queue ("Purchase requests"
// tab). Only procurement/procurement_admin/admin may see it; everyone else is
// redirected to their own "My requests" list. Procurement can narrow the queue
// by status, business unit, recommended vendor, and requester (server-side).
export function PurchaseRequestListPage() {
  const procurement = useProcurementAccess();
  const { data: me } = useMe();

  const [status, setStatus] = useState<PRStatus | "">("");
  const [businessUnitId, setBusinessUnitId] = useState<NumOrBlank>("");
  const [vendorId, setVendorId] = useState<NumOrBlank>("");
  const [requesterId, setRequesterId] = useState<NumOrBlank>("");
  const [assigneeId, setAssigneeId] = useState<NumOrBlank>("");
  // Bumped by "Clear filters" to remount the comboboxes so they wipe their text.
  const [resetKey, setResetKey] = useState(0);

  const filters: PRListFilters = {
    status: status || undefined,
    businessUnitId: businessUnitId || undefined,
    vendorId: vendorId || undefined,
    requesterId: requesterId || undefined,
    assigneeId: assigneeId || undefined,
  };
  const filtersActive = !!(status || businessUnitId || vendorId || requesterId || assigneeId);

  const { data, isLoading, error } = useAllPurchaseRequests(filters);
  const { data: businessUnits } = useBusinessUnitLookup(procurement);
  const { data: vendors } = useVendorLookup();
  const { data: users } = useUserLookup(procurement);

  if (me && !procurement) {
    return <Navigate to="/my-requests" replace />;
  }

  const toolbar = (
    <Box sx={{ mb: 2, display: "flex", flexWrap: "wrap", alignItems: "center", gap: 1.5 }}>
      <TextField
        select
        size="small"
        label="Status"
        value={status}
        onChange={(e) => setStatus(e.target.value as PRStatus | "")}
        aria-label="Filter by status"
        sx={{ minWidth: 160 }}
      >
        <MenuItem value="">All statuses</MenuItem>
        {PR_STATUSES.map((s) => (
          <MenuItem key={s} value={s}>
            {PR_STATUS_LABELS[s]}
          </MenuItem>
        ))}
      </TextField>

      <TextField
        select
        size="small"
        label="Business unit"
        value={businessUnitId}
        onChange={(e) => setBusinessUnitId(e.target.value ? Number(e.target.value) : "")}
        aria-label="Filter by business unit"
        sx={{ minWidth: 180 }}
      >
        <MenuItem value="">All business units</MenuItem>
        {(businessUnits ?? []).map((bu) => (
          <MenuItem key={bu.id} value={bu.id}>
            {bu.name}
          </MenuItem>
        ))}
      </TextField>

      <TextField
        select
        size="small"
        label="Vendor"
        value={vendorId}
        onChange={(e) => setVendorId(e.target.value ? Number(e.target.value) : "")}
        aria-label="Filter by recommended vendor"
        sx={{ minWidth: 160 }}
      >
        <MenuItem value="">All vendors</MenuItem>
        {(vendors ?? []).map((v) => (
          <MenuItem key={v.id} value={v.id}>
            {v.name}
          </MenuItem>
        ))}
      </TextField>

      <UserComboBox
        key={`requester-${resetKey}`}
        users={users ?? []}
        value={requesterId}
        onChange={setRequesterId}
        label="Requester"
        placeholder="Search…"
        ariaLabel="Filter by requester"
      />

      <UserComboBox
        key={`assignee-${resetKey}`}
        users={users ?? []}
        value={assigneeId}
        onChange={setAssigneeId}
        label="Assignee"
        placeholder="Search…"
        ariaLabel="Filter by assignee"
      />

      {filtersActive && (
        <Button
          variant="text"
          onClick={() => {
            setStatus("");
            setBusinessUnitId("");
            setVendorId("");
            setRequesterId("");
            setAssigneeId("");
            setResetKey((k) => k + 1);
          }}
        >
          Clear filters
        </Button>
      )}
    </Box>
  );

  return (
    <PurchaseRequestsList
      title="Purchase requests"
      subtitle="All purchasing requests ready for procurement."
      data={data}
      isLoading={isLoading}
      error={error}
      me={me}
      toolbar={toolbar}
      filtersActive={filtersActive}
    />
  );
}
