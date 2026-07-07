import { apiFetch } from "./client";
import type { Vendor, VendorInput, VendorLookup, VendorUsage } from "../types/api";

export const listVendors = () => apiFetch<Vendor[]>("/api/v1/vendors");

// listVendorLookup returns active vendor summaries; open to any authenticated
// user (used by the PR "proposed supplier" dropdown).
export const listVendorLookup = () => apiFetch<VendorLookup[]>("/api/v1/vendors/lookup");

export const getVendor = (id: number) => apiFetch<Vendor>(`/api/v1/vendors/${id}`);

export const getVendorUsage = (id: number) =>
  apiFetch<VendorUsage>(`/api/v1/vendors/${id}/usage`);

export const createVendor = (input: VendorInput) =>
  apiFetch<Vendor>("/api/v1/vendors", { method: "POST", body: input });

export const updateVendor = (id: number, input: VendorInput) =>
  apiFetch<Vendor>(`/api/v1/vendors/${id}`, { method: "PUT", body: input });
