-- Contract fulfillment: goods received notes (GRNs) and invoices. Both are
-- children of a signed contract. As with contracts, purchase_request_id and
-- vendor_id are denormalized (resolved from the contract at creation) so the
-- document storage path, the related-records bundle and list displays work
-- without extra joins and survive the origin contract row changing.

-- A GRN records receipt of goods/services against a contract. It has no status
-- workflow — it is simply a dated receipt record with line items.
CREATE TABLE grns (
    id                  BIGSERIAL     PRIMARY KEY,        -- displayed as GRN-{id:06d}
    contract_id         BIGINT        NOT NULL REFERENCES contracts(id) ON DELETE CASCADE,
    purchase_request_id BIGINT        NOT NULL REFERENCES purchase_requests(id) ON DELETE CASCADE,
    vendor_id           BIGINT        NOT NULL REFERENCES vendors(id) ON DELETE RESTRICT,
    received_date       DATE          NOT NULL,
    received_by         TEXT          NOT NULL DEFAULT '', -- who physically received (free text)
    note                TEXT          NOT NULL DEFAULT '',
    created_by          BIGINT        REFERENCES users(id),
    created_at          TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ   NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_grns_contract ON grns(contract_id);
CREATE INDEX idx_grns_pr       ON grns(purchase_request_id);

CREATE TABLE grn_items (
    id          BIGSERIAL     PRIMARY KEY,
    grn_id      BIGINT        NOT NULL REFERENCES grns(id) ON DELETE CASCADE,
    description TEXT          NOT NULL,
    quantity    NUMERIC(14,3) NOT NULL DEFAULT 1,         -- quantity received
    position    INT           NOT NULL DEFAULT 0
);
CREATE INDEX idx_grn_items_grn ON grn_items(grn_id);

-- A vendor invoice against a contract. total_amount is derived server-side from
-- the line items (sum of quantity * unit_price). status walks
-- received -> approved -> paid (revertible one step each way).
CREATE TABLE invoices (
    id                  BIGSERIAL     PRIMARY KEY,        -- displayed as INV-{id:06d}
    contract_id         BIGINT        NOT NULL REFERENCES contracts(id) ON DELETE CASCADE,
    purchase_request_id BIGINT        NOT NULL REFERENCES purchase_requests(id) ON DELETE CASCADE,
    vendor_id           BIGINT        NOT NULL REFERENCES vendors(id) ON DELETE RESTRICT,
    vendor_invoice_no   TEXT          NOT NULL DEFAULT '', -- the vendor's own invoice number
    invoice_date        DATE          NOT NULL,
    due_date            DATE,                              -- nullable
    total_amount        NUMERIC(16,2) NOT NULL DEFAULT 0,
    currency            TEXT          NOT NULL DEFAULT 'USD',
    status              TEXT          NOT NULL DEFAULT 'received', -- received | approved | paid
    note                TEXT          NOT NULL DEFAULT '',
    approved_by         BIGINT        REFERENCES users(id),
    approved_at         TIMESTAMPTZ,
    paid_date           DATE,
    created_by          BIGINT        REFERENCES users(id),
    created_at          TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ   NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_invoices_contract ON invoices(contract_id);
CREATE INDEX idx_invoices_pr       ON invoices(purchase_request_id);

CREATE TABLE invoice_items (
    id          BIGSERIAL     PRIMARY KEY,
    invoice_id  BIGINT        NOT NULL REFERENCES invoices(id) ON DELETE CASCADE,
    description TEXT          NOT NULL,
    quantity    NUMERIC(14,3) NOT NULL DEFAULT 1,
    unit_price  NUMERIC(16,2) NOT NULL DEFAULT 0,
    position    INT           NOT NULL DEFAULT 0
);
CREATE INDEX idx_invoice_items_invoice ON invoice_items(invoice_id);
