-- A contract is created from a selected quotation. purchase_request_id is
-- denormalized (quotation -> rfq -> pr) so the document path and PR auto-advance
-- work without a 3-way join and survive the origin quotation being deleted.
CREATE TABLE contracts (
    id                  BIGSERIAL     PRIMARY KEY,         -- displayed as CON-{id:06d}
    purchase_request_id BIGINT        NOT NULL REFERENCES purchase_requests(id) ON DELETE CASCADE,
    quotation_id        BIGINT        REFERENCES quotations(id) ON DELETE SET NULL, -- origin quotation
    vendor_id           BIGINT        NOT NULL REFERENCES vendors(id) ON DELETE RESTRICT,
    title               TEXT          NOT NULL DEFAULT '',
    total_amount        NUMERIC(16,2) NOT NULL DEFAULT 0,
    currency            TEXT          NOT NULL DEFAULT 'USD',
    terms               TEXT          NOT NULL DEFAULT '',
    status              TEXT          NOT NULL DEFAULT 'draft',
        -- draft | pending_approval | approved | rejected | signed
    signed_document_id  BIGINT        REFERENCES documents(id) ON DELETE SET NULL,
    created_by          BIGINT        REFERENCES users(id),
    created_at          TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ   NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_contracts_pr        ON contracts(purchase_request_id);
CREATE INDEX idx_contracts_quotation ON contracts(quotation_id);
