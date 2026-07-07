import { useQuery } from "@tanstack/react-query";
import { getContract, listContracts } from "../api/contracts";

export function useContracts() {
  return useQuery({
    queryKey: ["contracts"],
    queryFn: listContracts,
    refetchInterval: 5000,
  });
}

export function useContract(id: number) {
  return useQuery({
    queryKey: ["contracts", id],
    queryFn: () => getContract(id),
    enabled: Number.isFinite(id),
    refetchInterval: 5000,
  });
}
