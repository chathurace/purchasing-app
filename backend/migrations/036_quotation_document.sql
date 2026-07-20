-- A quotation has a single primary "quotation PDF" plus zero or more other
-- supporting documents. The primary is tracked by an FK to documents; the rest
-- stay as plain owner_type='quotation' rows. Mirrors contracts.signed_document_id.
ALTER TABLE quotations
    ADD COLUMN quotation_document_id BIGINT REFERENCES documents(id) ON DELETE SET NULL;
