import { apiFetch } from "./client";
import type { AuditEvent, EventActions, EventFilters, ProcessEvent } from "../types/api";

// buildQuery drops empty filter values so the server treats them as "no filter".
function buildQuery(filters: EventFilters): string {
  const params = new URLSearchParams();
  for (const [key, value] of Object.entries(filters)) {
    if (value != null && String(value).trim() !== "") params.set(key, String(value).trim());
  }
  const qs = params.toString();
  return qs ? `?${qs}` : "";
}

// listProcessEvents / listAuditEvents return the two append-only logs newest-first,
// narrowed by the filters. Admin only (server-enforced).
export const listProcessEvents = (filters: EventFilters = {}) =>
  apiFetch<ProcessEvent[]>(`/api/v1/events/process${buildQuery(filters)}`);

export const listAuditEvents = (filters: EventFilters = {}) =>
  apiFetch<AuditEvent[]>(`/api/v1/events/audit${buildQuery({ ...filters, pr_id: undefined })}`);

// listEventActions returns the fixed action catalogs backing the filter dropdowns.
export const listEventActions = () => apiFetch<EventActions>("/api/v1/events/actions");
