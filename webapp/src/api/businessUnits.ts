import { apiFetch } from "./client";
import type {
  BusinessUnit,
  BusinessUnitInput,
  BusinessUnitInvoiceSummary,
  BusinessUnitSummary,
  BusinessUnitUsage,
  UserSummary,
} from "../types/api";

export const listBusinessUnits = () => apiFetch<BusinessUnit[]>("/api/v1/business-units");

// lookupBusinessUnits returns active business units for the PR dropdown.
// Readable by any authenticated user.
export const lookupBusinessUnits = () =>
  apiFetch<BusinessUnitSummary[]>("/api/v1/business-units/lookup");

export const getBusinessUnit = (id: number) =>
  apiFetch<BusinessUnit>(`/api/v1/business-units/${id}`);

export const getBusinessUnitUsage = (id: number) =>
  apiFetch<BusinessUnitUsage>(`/api/v1/business-units/${id}/usage`);

export const getBusinessUnitInvoices = (id: number) =>
  apiFetch<BusinessUnitInvoiceSummary>(`/api/v1/business-units/${id}/invoices`);

// getBusinessUnitApprovers returns the flat approver list of a business unit —
// used to populate the budget-approver dropdown on the requisition form. Any
// authenticated user may call it.
export const getBusinessUnitApprovers = (id: number) =>
  apiFetch<UserSummary[]>(`/api/v1/business-units/${id}/approvers`);

export const createBusinessUnit = (input: BusinessUnitInput) =>
  apiFetch<BusinessUnit>("/api/v1/business-units", { method: "POST", body: input });

export const updateBusinessUnit = (id: number, input: BusinessUnitInput) =>
  apiFetch<BusinessUnit>(`/api/v1/business-units/${id}`, { method: "PUT", body: input });
