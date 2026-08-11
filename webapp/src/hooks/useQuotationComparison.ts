import { useQuery } from "@tanstack/react-query";
import { getQuotationComparison } from "../api/quotationComparison";

// The comparison's figures are derived server-side on every read, so polling on the
// same 5s cadence as the quotation queries is what keeps the card in step with a
// quotation that was just edited (or a final PDF that was just read).
export const comparisonKey = (prId: number) => ["quotation-comparison", prId];

export function useQuotationComparison(prId: number, enabled = true) {
  return useQuery({
    queryKey: comparisonKey(prId),
    queryFn: () => getQuotationComparison(prId),
    enabled: enabled && Number.isFinite(prId),
    refetchInterval: 5000,
    retry: false,
  });
}
