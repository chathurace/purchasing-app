import { useQuery } from "@tanstack/react-query";
import { getGRN, listGRNs, listGRNsForContract } from "../api/grns";

export function useGRNs() {
  return useQuery({
    queryKey: ["grns"],
    queryFn: listGRNs,
    refetchInterval: 5000,
  });
}

export function useGRNsForContract(contractId: number, enabled = true) {
  return useQuery({
    queryKey: ["grns", "contract", contractId],
    queryFn: () => listGRNsForContract(contractId),
    enabled: enabled && Number.isFinite(contractId),
    refetchInterval: 5000,
  });
}

export function useGRN(id: number) {
  return useQuery({
    queryKey: ["grns", id],
    queryFn: () => getGRN(id),
    enabled: Number.isFinite(id),
    refetchInterval: 5000,
  });
}
