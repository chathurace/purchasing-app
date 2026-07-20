import { useQuery } from "@tanstack/react-query";
import {
  getPurchaseRequest,
  getRelatedDocuments,
  listPurchaseRequests,
} from "../api/purchaseRequests";
import { useProcurementAccess } from "./useProcurementAccess";

// usePurchaseRequests powers the Requests tab. Procurement/admin see the full
// procurement queue; everyone else sees only their own submissions (scope=mine).
export function usePurchaseRequests() {
  const procurement = useProcurementAccess();
  const scope = procurement ? undefined : ("mine" as const);
  return useQuery({
    queryKey: ["purchase-requests", scope ?? "all"],
    queryFn: () => listPurchaseRequests(scope),
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
