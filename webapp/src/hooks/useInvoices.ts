import { useQuery } from "@tanstack/react-query";
import { getInvoice, listInvoices, listInvoicesForContract } from "../api/invoices";

export function useInvoices() {
  return useQuery({
    queryKey: ["invoices"],
    queryFn: listInvoices,
    refetchInterval: 5000,
  });
}

export function useInvoicesForContract(contractId: number, enabled = true) {
  return useQuery({
    queryKey: ["invoices", "contract", contractId],
    queryFn: () => listInvoicesForContract(contractId),
    enabled: enabled && Number.isFinite(contractId),
    refetchInterval: 5000,
  });
}

export function useInvoice(id: number) {
  return useQuery({
    queryKey: ["invoices", id],
    queryFn: () => getInvoice(id),
    enabled: Number.isFinite(id),
    refetchInterval: 5000,
  });
}
