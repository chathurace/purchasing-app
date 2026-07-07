import { useQuery } from "@tanstack/react-query";
import { getVendor, getVendorUsage, listVendors, listVendorLookup } from "../api/vendors";

export function useVendors() {
  return useQuery({
    queryKey: ["vendors"],
    queryFn: listVendors,
    refetchInterval: 5000,
  });
}

// useVendorLookup powers the PR "proposed supplier" dropdown for any user.
export function useVendorLookup() {
  return useQuery({
    queryKey: ["vendors", "lookup"],
    queryFn: listVendorLookup,
    staleTime: 60_000,
  });
}

export function useVendor(id: number) {
  return useQuery({
    queryKey: ["vendors", id],
    queryFn: () => getVendor(id),
    enabled: Number.isFinite(id) && id > 0,
  });
}

export function useVendorUsage(id: number) {
  return useQuery({
    queryKey: ["vendors", id, "usage"],
    queryFn: () => getVendorUsage(id),
    enabled: Number.isFinite(id) && id > 0,
  });
}
