-- A quotation is a vendor's response to an RFQ. quotation_items mirrors pr_items
-- so the repository's replace-items pattern applies verbatim.
CREATE TABLE quotations (
    id            BIGSERIAL     PRIMARY KEY,             -- displayed as QUO-{id:06d}
    rfq_id        BIGINT        NOT NULL REFERENCES rfqs(id) ON DELETE CASCADE,
    vendor_id     BIGINT        NOT NULL REFERENCES vendors(id) ON DELETE RESTRICT,
    total_amount  NUMERIC(16,2) NOT NULL DEFAULT 0,
    currency      TEXT          NOT NULL DEFAULT 'USD',   -- ISO 4217, free 3-letter for now
    valid_until   DATE,                                   -- nullable
    notes         TEXT          NOT NULL DEFAULT '',
    status        TEXT          NOT NULL DEFAULT 'received',
        -- received | under_evaluation | selected | rejected
    created_by    BIGINT        REFERENCES users(id),
    created_at    TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ   NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_quotations_rfq    ON quotations(rfq_id);
CREATE INDEX idx_quotations_vendor ON quotations(vendor_id);

CREATE TABLE quotation_items (
    id           BIGSERIAL     PRIMARY KEY,
    quotation_id BIGINT        NOT NULL REFERENCES quotations(id) ON DELETE CASCADE,
    description  TEXT          NOT NULL,
    quantity     NUMERIC(14,3) NOT NULL DEFAULT 1,
    unit_price   NUMERIC(16,2) NOT NULL DEFAULT 0,
    position     INT           NOT NULL DEFAULT 0
);
CREATE INDEX idx_quotation_items_q ON quotation_items(quotation_id);
