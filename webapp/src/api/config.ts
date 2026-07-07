import { apiFetch } from "./client";

// ConfigOption is one value within a configurable dropdown list.
export interface ConfigOption {
  id: number;
  list_key: string;
  value: string;
  sort_order: number;
  is_active: boolean;
  created_at: string;
  updated_at: string;
}

export interface ConfigList {
  key: string;
  label: string;
}

// ConfigLookup is the active values grouped by list, plus the registry of known
// lists. Returned by /config/options/lookup (any authenticated user).
export interface ConfigLookup {
  lists: Record<string, string[]>;
  keys: ConfigList[];
}

export interface ConfigOptionInput {
  list_key: string;
  value: string;
  sort_order: number;
  is_active: boolean;
}

export const lookupConfigOptions = () =>
  apiFetch<ConfigLookup>("/api/v1/config/options/lookup");

export const listConfigOptions = () =>
  apiFetch<ConfigOption[]>("/api/v1/config/options");

export const createConfigOption = (input: ConfigOptionInput) =>
  apiFetch<ConfigOption>("/api/v1/config/options", { method: "POST", body: input });

export const updateConfigOption = (id: number, input: Omit<ConfigOptionInput, "list_key">) =>
  apiFetch<ConfigOption>(`/api/v1/config/options/${id}`, { method: "PUT", body: input });

export const deleteConfigOption = (id: number) =>
  apiFetch<void>(`/api/v1/config/options/${id}`, { method: "DELETE" });
