import { useQuery } from "@tanstack/react-query";
import {
  getCostCenter,
  getCostCenterInvoices,
  getCostCenterUsage,
  listCostCenters,
  lookupCostCenters,
} from "../api/costCenters";

export function useCostCenters() {
  return useQuery({
    queryKey: ["cost_centers"],
    queryFn: listCostCenters,
    refetchInterval: 5000,
  });
}

export function useCostCenter(id: number) {
  return useQuery({
    queryKey: ["cost_centers", id],
    queryFn: () => getCostCenter(id),
    enabled: Number.isFinite(id) && id > 0,
  });
}

export function useCostCenterUsage(id: number) {
  return useQuery({
    queryKey: ["cost_centers", id, "usage"],
    queryFn: () => getCostCenterUsage(id),
    enabled: Number.isFinite(id) && id > 0,
  });
}

export function useCostCenterInvoices(id: number) {
  return useQuery({
    queryKey: ["cost_centers", id, "invoices"],
    queryFn: () => getCostCenterInvoices(id),
    enabled: Number.isFinite(id) && id > 0,
  });
}

// useCostCenterLookup loads the active cost centers for the PR dropdown. Any
// authenticated user may call it; results are id/code/name only.
export function useCostCenterLookup(enabled = true) {
  return useQuery({
    queryKey: ["cost_centers", "lookup"],
    queryFn: lookupCostCenters,
    enabled,
  });
}
