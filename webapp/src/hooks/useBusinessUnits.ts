import { useQuery } from "@tanstack/react-query";
import {
  getBusinessUnit,
  getBusinessUnitApprovers,
  getBusinessUnitInvoices,
  getBusinessUnitUsage,
  listBusinessUnits,
  lookupBusinessUnits,
} from "../api/businessUnits";

export function useBusinessUnits() {
  return useQuery({
    queryKey: ["business_units"],
    queryFn: listBusinessUnits,
    refetchInterval: 5000,
  });
}

export function useBusinessUnit(id: number) {
  return useQuery({
    queryKey: ["business_units", id],
    queryFn: () => getBusinessUnit(id),
    enabled: Number.isFinite(id) && id > 0,
  });
}

export function useBusinessUnitUsage(id: number) {
  return useQuery({
    queryKey: ["business_units", id, "usage"],
    queryFn: () => getBusinessUnitUsage(id),
    enabled: Number.isFinite(id) && id > 0,
  });
}

export function useBusinessUnitInvoices(id: number) {
  return useQuery({
    queryKey: ["business_units", id, "invoices"],
    queryFn: () => getBusinessUnitInvoices(id),
    enabled: Number.isFinite(id) && id > 0,
  });
}

// useBusinessUnitLookup loads the active business units for the PR dropdown. Any
// authenticated user may call it; results are id/name only.
export function useBusinessUnitLookup(enabled = true) {
  return useQuery({
    queryKey: ["business_units", "lookup"],
    queryFn: lookupBusinessUnits,
    enabled,
  });
}

// useBusinessUnitApprovers loads the flat approver list of a business unit —
// used to populate the budget-approver dropdown on the requisition form.
// Disabled until a business unit is chosen.
export function useBusinessUnitApprovers(businessUnitId: number | null) {
  return useQuery({
    queryKey: ["business_units", businessUnitId, "approvers"],
    queryFn: () => getBusinessUnitApprovers(businessUnitId as number),
    enabled: businessUnitId != null && businessUnitId > 0,
  });
}
