-- Quotation PDF extraction (Claude). A quotation PDF is run through the Anthropic
-- API to pull out vendor / currency / total / validity / line items; the result is
-- staged here rather than written straight onto the quotation, so a procurement
-- user reviews and applies it. See docs/plans/20-quotation-pdf-extraction.md.
--
-- A row exists in two shapes:
--   * pre-create  — quotation_id IS NULL. The PDF was uploaded on the PR page
--                   before the quotation existed; the create call adopts the
--                   document into the new quotation's initial-PDF slot.
--   * post-create — quotation_id set. Re-extraction of an already-attached
--                   initial/final PDF from the quotation card.
--
-- raw_json holds the model's validated output verbatim (the suggestion the UI
-- renders). token counts are kept for cost visibility.
CREATE TABLE IF NOT EXISTS quotation_extractions (
    id                  BIGSERIAL PRIMARY KEY,
    purchase_request_id BIGINT NOT NULL REFERENCES purchase_requests(id) ON DELETE CASCADE,
    document_id         BIGINT NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    quotation_id        BIGINT REFERENCES quotations(id) ON DELETE SET NULL,
    status              TEXT NOT NULL DEFAULT 'pending',
    model               TEXT NOT NULL DEFAULT '',
    raw_json            JSONB,
    error_message       TEXT NOT NULL DEFAULT '',
    input_tokens        INTEGER NOT NULL DEFAULT 0,
    output_tokens       INTEGER NOT NULL DEFAULT 0,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by          BIGINT REFERENCES users(id) ON DELETE SET NULL,
    applied_at          TIMESTAMPTZ,
    applied_by          BIGINT REFERENCES users(id) ON DELETE SET NULL,
    CONSTRAINT quotation_extractions_status_check
        CHECK (status IN ('pending', 'succeeded', 'failed'))
);

CREATE INDEX IF NOT EXISTS idx_quotation_extractions_pr
    ON quotation_extractions (purchase_request_id);

CREATE INDEX IF NOT EXISTS idx_quotation_extractions_quotation
    ON quotation_extractions (quotation_id) WHERE quotation_id IS NOT NULL;

-- One live extraction per document: re-running against the same PDF replaces the
-- previous attempt rather than accumulating rows. Failed attempts are exempt so a
-- retry after an API error doesn't collide with the failure it is retrying.
CREATE UNIQUE INDEX IF NOT EXISTS uq_quotation_extractions_document
    ON quotation_extractions (document_id) WHERE status <> 'failed';
