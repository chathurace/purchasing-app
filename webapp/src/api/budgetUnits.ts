import { apiFetch } from "./client";
import type {
  BudgetUnit,
  BudgetUnitInput,
  BudgetUnitInvoiceSummary,
  BudgetUnitSummary,
  BudgetUnitUsage,
  UserSummary,
} from "../types/api";

export const listBudgetUnits = () => apiFetch<BudgetUnit[]>("/api/v1/budget-units");

// lookupBudgetUnits returns active budget units for the PR dropdown. Readable
// by any authenticated user.
export const lookupBudgetUnits = () =>
  apiFetch<BudgetUnitSummary[]>("/api/v1/budget-units/lookup");

export const getBudgetUnit = (id: number) =>
  apiFetch<BudgetUnit>(`/api/v1/budget-units/${id}`);

export const getBudgetUnitUsage = (id: number) =>
  apiFetch<BudgetUnitUsage>(`/api/v1/budget-units/${id}/usage`);

export const getBudgetUnitInvoices = (id: number) =>
  apiFetch<BudgetUnitInvoiceSummary>(`/api/v1/budget-units/${id}/invoices`);

// getBudgetUnitApprovers resolves the qualified budget approver(s) for a budget
// unit at a given estimated value + currency — the creation-phase preview. A
// null value resolves to the highest bracket. Any authenticated user may call.
export const getBudgetUnitApprovers = (id: number, value: number | null, currency: string) => {
  const q = new URLSearchParams();
  if (value != null) q.set("value", String(value));
  if (currency) q.set("currency", currency);
  const qs = q.toString();
  return apiFetch<UserSummary[]>(`/api/v1/budget-units/${id}/approvers${qs ? `?${qs}` : ""}`);
};

export const createBudgetUnit = (input: BudgetUnitInput) =>
  apiFetch<BudgetUnit>("/api/v1/budget-units", { method: "POST", body: input });

export const updateBudgetUnit = (id: number, input: BudgetUnitInput) =>
  apiFetch<BudgetUnit>(`/api/v1/budget-units/${id}`, { method: "PUT", body: input });
