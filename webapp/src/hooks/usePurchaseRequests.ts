import { useQuery } from "@tanstack/react-query";
import {
  getPurchaseRequest,
  getRelatedDocuments,
  listPurchaseRequests,
  type PRListFilters,
} from "../api/purchaseRequests";

// useMyRequests powers the "My requests" tab (visible to everyone): only the
// caller's own submissions (scope=mine).
export function useMyRequests() {
  return useQuery({
    queryKey: ["purchase-requests", "mine"],
    queryFn: () => listPurchaseRequests("mine"),
    refetchInterval: 5000,
  });
}

// useAllPurchaseRequests powers the "Purchase requests" tab (procurement/admin
// only): the full procurement queue (default role-based scope). Optional filters
// (status / business unit / recommended vendor / requester) are applied
// server-side and keyed into the cache so each filter combination is its own query.
export function useAllPurchaseRequests(filters?: PRListFilters) {
  return useQuery({
    queryKey: ["purchase-requests", "all", filters ?? {}],
    queryFn: () => listPurchaseRequests(undefined, filters),
    refetchInterval: 5000,
  });
}

// useApprovalRequests powers the Approvals tab: PRs awaiting the caller's
// decision across both approval systems (scope=approvals).
export function useApprovalRequests() {
  return useQuery({
    queryKey: ["purchase-requests", "approvals"],
    queryFn: () => listPurchaseRequests("approvals"),
    refetchInterval: 5000,
  });
}

export function usePurchaseRequest(id: number) {
  return useQuery({
    queryKey: ["purchase-requests", id],
    queryFn: () => getPurchaseRequest(id),
    enabled: Number.isFinite(id),
  });
}

// useRelatedDocuments loads the procurement case anchored on a PR. Pass enabled
// (e.g. the caller's procurement access) to skip the procurement-only fetch entirely.
export function useRelatedDocuments(prId: number, enabled = true) {
  return useQuery({
    queryKey: ["purchase-requests", prId, "related"],
    queryFn: () => getRelatedDocuments(prId),
    enabled: enabled && Number.isFinite(prId),
    refetchInterval: 5000,
  });
}
