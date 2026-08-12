import { useQuery } from "@tanstack/react-query";
import {
  getExtractionStatus,
  listExtractionsForPR,
} from "../api/quotationExtractions";

// useExtractionStatus reports whether Claude-backed quotation PDF extraction is
// configured on the server. Cached for the session — it only changes on a restart —
// so every card can ask without a request each time.
export function useExtractionStatus() {
  return useQuery({
    queryKey: ["quotation-extraction-status"],
    queryFn: getExtractionStatus,
    staleTime: Infinity,
    retry: false,
  });
}

// useCanExtractQuotations is the boolean the UI gates on. Defaults to false while
// loading or on error, so the feature is hidden rather than half-shown.
export function useCanExtractQuotations(): boolean {
  const { data } = useExtractionStatus();
  return data?.enabled ?? false;
}

// usePRExtractions loads what each of a PR's quotation PDFs said, keyed by document
// id so a quotation card can show its initial and final PDF separately. Skipped
// entirely when extraction isn't configured or the caller can't use it.
export function usePRExtractions(prId: number, enabled: boolean) {
  return useQuery({
    queryKey: ["quotation-extractions", "pr", prId],
    queryFn: () => listExtractionsForPR(prId),
    enabled,
    retry: false,
  });
}
