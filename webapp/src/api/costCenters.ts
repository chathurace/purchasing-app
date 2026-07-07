import { apiFetch } from "./client";
import type {
  CostCenter,
  CostCenterInput,
  CostCenterInvoiceSummary,
  CostCenterSummary,
  CostCenterUsage,
} from "../types/api";

export const listCostCenters = () => apiFetch<CostCenter[]>("/api/v1/cost-centers");

// lookupCostCenters returns active cost centers for the PR dropdown. Readable
// by any authenticated user.
export const lookupCostCenters = () =>
  apiFetch<CostCenterSummary[]>("/api/v1/cost-centers/lookup");

export const getCostCenter = (id: number) =>
  apiFetch<CostCenter>(`/api/v1/cost-centers/${id}`);

export const getCostCenterUsage = (id: number) =>
  apiFetch<CostCenterUsage>(`/api/v1/cost-centers/${id}/usage`);

export const getCostCenterInvoices = (id: number) =>
  apiFetch<CostCenterInvoiceSummary>(`/api/v1/cost-centers/${id}/invoices`);

export const createCostCenter = (input: CostCenterInput) =>
  apiFetch<CostCenter>("/api/v1/cost-centers", { method: "POST", body: input });

export const updateCostCenter = (id: number, input: CostCenterInput) =>
  apiFetch<CostCenter>(`/api/v1/cost-centers/${id}`, { method: "PUT", body: input });
