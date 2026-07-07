import { useQuery } from "@tanstack/react-query";
import { getQuotation, listQuotations, listQuotationsForPR } from "../api/quotations";

export function useQuotations() {
  return useQuery({
    queryKey: ["quotations"],
    queryFn: listQuotations,
    refetchInterval: 5000,
  });
}

export function useQuotationsForPR(prId: number, enabled = true) {
  return useQuery({
    queryKey: ["quotations", "pr", prId],
    queryFn: () => listQuotationsForPR(prId),
    enabled: enabled && Number.isFinite(prId),
    refetchInterval: 5000,
  });
}

export function useQuotation(id: number) {
  return useQuery({
    queryKey: ["quotations", id],
    queryFn: () => getQuotation(id),
    enabled: Number.isFinite(id),
    refetchInterval: 5000,
  });
}
