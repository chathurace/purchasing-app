-- A quotation now carries TWO primary PDFs instead of one: the **initial**
-- quotation received from the vendor and the **final** (post-negotiation)
-- quotation. The pre-existing single "quotation PDF" becomes the initial one, so
-- the column is renamed rather than replaced; the final PDF is optional and is
-- normally attached later from the quotation card on the PR page (a quotation may
-- be selected for the procurement recommendation without it).
--
-- Both are tracked by an FK into documents, mirroring contracts.signed_document_id.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'quotations' AND column_name = 'quotation_document_id'
    ) THEN
        ALTER TABLE quotations RENAME COLUMN quotation_document_id TO initial_quotation_document_id;
    END IF;
END $$;

ALTER TABLE quotations
    ADD COLUMN IF NOT EXISTS final_quotation_document_id BIGINT REFERENCES documents(id) ON DELETE SET NULL;
