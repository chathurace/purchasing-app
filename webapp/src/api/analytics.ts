import { apiFetch } from "./client";
import type { AnalyticsPR, AnalyticsPRSort, PRFlow } from "../types/api";

// BPM analytics reads (docs/bpm-analytics.md). Admin / procurement_admin only —
// server-enforced, like the audit log this shares its event rows with.

// listAnalyticsPRs returns every purchase request, sorted server-side by one of
// the whitelisted columns and capped at a page. `dir` is optional: the server
// defaults each column to the direction its header reads as on first click
// (newest-first for created_at, A→Z for a person).
export const listAnalyticsPRs = (sort: AnalyticsPRSort, dir: "asc" | "desc") =>
  apiFetch<AnalyticsPR[]>(
    `/api/v1/analytics/purchase-requests?sort=${sort}&dir=${dir}`,
  );

// getPRFlow returns one request's header plus its whole process-event timeline,
// oldest-first, in a single round trip.
export const getPRFlow = (id: number) =>
  apiFetch<PRFlow>(`/api/v1/analytics/purchase-requests/${id}`);
