import { useQuery } from "@tanstack/react-query";
import {
  getBudgetUnit,
  getBudgetUnitApprovers,
  getBudgetUnitInvoices,
  getBudgetUnitUsage,
  listBudgetUnits,
  lookupBudgetUnits,
} from "../api/budgetUnits";

export function useBudgetUnits() {
  return useQuery({
    queryKey: ["budget_units"],
    queryFn: listBudgetUnits,
    refetchInterval: 5000,
  });
}

export function useBudgetUnit(id: number) {
  return useQuery({
    queryKey: ["budget_units", id],
    queryFn: () => getBudgetUnit(id),
    enabled: Number.isFinite(id) && id > 0,
  });
}

export function useBudgetUnitUsage(id: number) {
  return useQuery({
    queryKey: ["budget_units", id, "usage"],
    queryFn: () => getBudgetUnitUsage(id),
    enabled: Number.isFinite(id) && id > 0,
  });
}

export function useBudgetUnitInvoices(id: number) {
  return useQuery({
    queryKey: ["budget_units", id, "invoices"],
    queryFn: () => getBudgetUnitInvoices(id),
    enabled: Number.isFinite(id) && id > 0,
  });
}

// useBudgetUnitLookup loads the active budget units for the PR dropdown. Any
// authenticated user may call it; results are id/code/name/currency only.
export function useBudgetUnitLookup(enabled = true) {
  return useQuery({
    queryKey: ["budget_units", "lookup"],
    queryFn: lookupBudgetUnits,
    enabled,
  });
}

// useBudgetUnitApprovers resolves the budget approver(s) for a budget unit at a
// given estimated value + currency — the creation-phase preview. Disabled until
// a budget unit is chosen.
export function useBudgetUnitApprovers(
  budgetUnitId: number | null,
  value: number | null,
  currency: string,
) {
  return useQuery({
    queryKey: ["budget_units", budgetUnitId, "approvers", value, currency],
    queryFn: () => getBudgetUnitApprovers(budgetUnitId as number, value, currency),
    enabled: budgetUnitId != null && budgetUnitId > 0,
  });
}
