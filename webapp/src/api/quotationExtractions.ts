import { apiFetch } from "./client";
import type { ExtractionResponse, ExtractionStatus } from "../types/api";

// Quotation PDF extraction (Claude). The result is always a *suggestion* — the
// user reviews and edits it before anything is written to a quotation.
// See docs/quotation-extraction.md.

// getExtractionStatus reports whether extraction is configured, so the UI can hide
// the feature rather than offer a control that 503s.
export const getExtractionStatus = () =>
  apiFetch<ExtractionStatus>("/api/v1/quotation-extractions/status");

// extractForPR stages a PDF against a purchase request and reads it *before* any
// quotation exists. Pass the returned extraction id to createQuotation to adopt the
// stored PDF as the new quotation's initial document.
export const extractForPR = (prId: number, file: File) => {
  const fd = new FormData();
  fd.append("file", file);
  return apiFetch<ExtractionResponse>(
    `/api/v1/purchase-requests/${prId}/quotation-extractions`,
    {
      method: "POST",
      body: fd,
    },
  );
};

// extractForQuotation re-reads one of an existing quotation's two primary PDFs.
export const extractForQuotation = (
  quotationId: number,
  slot: "initial" | "final",
) =>
  apiFetch<ExtractionResponse>(
    `/api/v1/quotations/${quotationId}/extract?slot=${slot}`,
    {
      method: "POST",
    },
  );

export const getExtraction = (id: number) =>
  apiFetch<ExtractionResponse>(`/api/v1/quotation-extractions/${id}`);

// listExtractionsForPR returns what each of a PR's quotation PDFs said, one entry
// per document. Fetched once for the page rather than per card: the initial and final
// PDF of a quotation are separate documents with separate — legitimately different —
// figures, and each card looks up its two slots by document id.
export const listExtractionsForPR = (prId: number) =>
  apiFetch<ExtractionResponse[]>(
    `/api/v1/purchase-requests/${prId}/quotation-extractions`,
  );
