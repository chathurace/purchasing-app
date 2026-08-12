-- Quotation comparison (one per purchase request). Procurement generates it once a
-- PR has two or more quotations; it is the WSO2 vendor-quote comparison sheet —
-- every vendor's initial quote beside its final (post-negotiation) quote, with the
-- variances and savings that follow. See docs/quotation-comparison.md.
--
-- Deliberately thin: NO figures are stored here. Every number on the card is
-- derived at read time from the PR's quotations and their per-document extractions,
-- so editing a quotation (or reading its final PDF) updates the comparison instead
-- of leaving a stale snapshot behind. What *is* stored is the part that cannot be
-- derived:
--   * that a comparison was generated at all, by whom and when;
--   * the approved budget the quotes are measured against (the sheet's header
--     field — the requisition form no longer collects an estimated value);
--   * currency — the one the comparison is stated in (quotes in another currency
--     are shown but excluded from the cross-vendor metrics);
--   * use_initial_for_final — the user's confirmed consent to stand a vendor's
--     initial figures in for a final quote it doesn't have yet.
CREATE TABLE IF NOT EXISTS quotation_comparisons (
    id                    BIGSERIAL PRIMARY KEY,
    purchase_request_id   BIGINT NOT NULL UNIQUE REFERENCES purchase_requests(id) ON DELETE CASCADE,
    approved_budget       NUMERIC(14,2),
    currency              TEXT NOT NULL DEFAULT '',
    use_initial_for_final BOOLEAN NOT NULL DEFAULT false,
    generated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    generated_by          BIGINT REFERENCES users(id) ON DELETE SET NULL,
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);
