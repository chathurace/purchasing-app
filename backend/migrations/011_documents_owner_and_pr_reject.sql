-- Generalize the documents table so RFQs, quotations and contracts can own
-- documents too, without three near-identical tables. purchase_request_id stays
-- NOT NULL so every file keeps living under that PR's storage directory.
ALTER TABLE documents
    ADD COLUMN owner_type TEXT NOT NULL DEFAULT 'purchase_request',
        -- purchase_request | rfq | quotation | contract
    ADD COLUMN owner_id   BIGINT;

-- Existing rows are purchase-request documents; mirror their id into owner_id
-- so owner-scoped queries are uniform.
UPDATE documents SET owner_id = purchase_request_id WHERE owner_id IS NULL;

CREATE INDEX idx_documents_owner ON documents(owner_type, owner_id);

-- Store the PR rejection reason separately so it never clobbers the requester's
-- own comments.
ALTER TABLE purchase_requests
    ADD COLUMN rejection_reason TEXT NOT NULL DEFAULT '';
