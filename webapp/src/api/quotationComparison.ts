import { apiFetch } from "./client";
import type { QuotationComparison, QuotationComparisonInput } from "../types/api";

// Quotation comparison (one per purchase request). The server assembles every
// figure on each read from the PR's quotations and what was read out of their PDFs,
// so a quotation edit shows up here without regenerating anything — and approvers,
// who cannot read the quotation endpoints, still get the numbers.
// See docs/quotation-comparison.md.

// getQuotationComparison is readable by anyone who may view the PR. Before a
// comparison has been generated it answers `exists: false`; for procurement it also
// carries the computed figures and `missing_final`, which is what the confirmation
// modal needs *before* generating.
export const getQuotationComparison = (prId: number) =>
  apiFetch<QuotationComparison>(`/api/v1/purchase-requests/${prId}/quotation-comparison`);

// generateQuotationComparison creates (or regenerates) the comparison. Needs two or
// more quotations, and `use_initial_for_final` when any vendor has no final quote.
export const generateQuotationComparison = (prId: number, input: QuotationComparisonInput) =>
  apiFetch<QuotationComparison>(`/api/v1/purchase-requests/${prId}/quotation-comparison`, {
    method: "POST",
    body: input,
  });

// updateQuotationComparison edits the sheet's own inputs (approved budget, currency,
// the missing-final consent) without re-stamping who generated it.
export const updateQuotationComparison = (prId: number, input: QuotationComparisonInput) =>
  apiFetch<QuotationComparison>(`/api/v1/purchase-requests/${prId}/quotation-comparison`, {
    method: "PUT",
    body: input,
  });

export const deleteQuotationComparison = (prId: number) =>
  apiFetch<void>(`/api/v1/purchase-requests/${prId}/quotation-comparison`, { method: "DELETE" });
