import { useQuery } from "@tanstack/react-query";
import { listConfigOptions, lookupConfigOptions } from "../api/config";
import {
  BUDGET_CATEGORIES,
  ENGAGEMENT_TYPES,
  IT_CATEGORIES,
  NONIT_CATEGORIES,
  PRODUCTS,
  REGIONS,
  REQ_CURRENCIES,
  WSO2_ENTITIES,
} from "../types/api";

// FALLBACK_OPTIONS mirrors the values seeded into config_options, used only while
// the lookup is loading or if a list has been emptied, so a dropdown is never
// blank during a transient state.
export const FALLBACK_OPTIONS: Record<string, string[]> = {
  entity: WSO2_ENTITIES,
  it_category: IT_CATEGORIES,
  nonit_category: NONIT_CATEGORIES,
  engagement_type: ENGAGEMENT_TYPES,
  currency: REQ_CURRENCIES,
  budget_category: BUDGET_CATEGORIES,
  product: PRODUCTS,
  region: REGIONS,
  engagement_code: [],
};

// useConfigLookup loads the active dropdown values grouped by list, for the
// requisition form. Any authenticated user may call it.
export function useConfigLookup(enabled = true) {
  return useQuery({
    queryKey: ["config_options", "lookup"],
    queryFn: lookupConfigOptions,
    enabled,
  });
}

// useConfigOptionsAdmin loads every option (active and inactive) for the Settings
// page. Restricted to admin/procurement_admin on the server.
export function useConfigOptionsAdmin() {
  return useQuery({
    queryKey: ["config_options", "all"],
    queryFn: listConfigOptions,
    refetchInterval: 5000,
  });
}

// optionsFor resolves the values for a list from the lookup, falling back to the
// seeded defaults, and guarantees `current` is present so an existing record's
// stored value still displays even after the option is deactivated/removed.
export function optionsFor(
  lists: Record<string, string[]> | undefined,
  key: string,
  current?: string,
): string[] {
  const base = lists?.[key]?.length ? lists[key] : FALLBACK_OPTIONS[key] ?? [];
  if (current && current.trim() && !base.includes(current)) {
    return [...base, current];
  }
  return base;
}
