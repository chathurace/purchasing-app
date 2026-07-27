import { useQuery } from "@tanstack/react-query";
import { listAuditEvents, listEventActions, listProcessEvents } from "../api/events";
import type { EventFilters } from "../types/api";

// Admin-only reads over the two append-only event logs. `enabled` is passed the
// admin flag so non-admins never fire the request. Filters are part of the query
// key so the list refetches when they change.

export function useProcessEvents(filters: EventFilters, enabled = true) {
  return useQuery({
    queryKey: ["events", "process", filters],
    queryFn: () => listProcessEvents(filters),
    enabled,
  });
}

export function useAuditEvents(filters: EventFilters, enabled = true) {
  return useQuery({
    queryKey: ["events", "audit", filters],
    queryFn: () => listAuditEvents(filters),
    enabled,
  });
}

export function useEventActions(enabled = true) {
  return useQuery({
    queryKey: ["events", "actions"],
    queryFn: listEventActions,
    enabled,
    staleTime: Infinity, // the catalog is fixed at build time
  });
}
