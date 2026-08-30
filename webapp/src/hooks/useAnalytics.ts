import { useQuery } from "@tanstack/react-query";
import { getPRFlow, listAnalyticsPRs } from "../api/analytics";
import { useMe } from "./useMe";
import type { AnalyticsPRSort } from "../types/api";

// BPM analytics reads. `enabled` is passed the access flag so a user without it
// never fires the request (the server would 403).

// useCanViewAnalytics reports whether the signed-in user may open the Analytics
// section. Admin / procurement_admin only — the same audience as the audit log,
// since the flow view is a rendering of the same process events. Kept separate
// from useCanViewAuditLog so the two gates can diverge without a rename.
export function useCanViewAnalytics(): boolean {
  const { data: me } = useMe();
  return !!me?.roles.some((r) => r === "admin" || r === "procurement_admin");
}

// useAnalyticsPRs fetches the sortable PR list. The sort is part of the query key
// because it is applied server-side — clicking a header refetches.
export function useAnalyticsPRs(sort: AnalyticsPRSort, dir: "asc" | "desc", enabled = true) {
  return useQuery({
    queryKey: ["analytics", "purchase-requests", sort, dir],
    queryFn: () => listAnalyticsPRs(sort, dir),
    enabled,
    // Keep the previous page visible while a re-sort is in flight, so the table
    // doesn't blank out on every header click.
    placeholderData: (prev) => prev,
  });
}

export function usePRFlow(id: number | undefined, enabled = true) {
  return useQuery({
    queryKey: ["analytics", "pr-flow", id],
    queryFn: () => getPRFlow(id as number),
    enabled: enabled && id != null && Number.isFinite(id),
  });
}
