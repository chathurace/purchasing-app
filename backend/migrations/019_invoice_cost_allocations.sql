-- An invoice's cost is allocated across one or more cost centers. The whole
-- invoice is split either by percentage or by absolute amount (allocation_mode);
-- each allocation row carries the raw entered value, interpreted per the mode.
ALTER TABLE invoices
    ADD COLUMN allocation_mode TEXT NOT NULL DEFAULT 'percentage';  -- percentage | amount

CREATE TABLE invoice_cost_allocations (
    id             BIGSERIAL     PRIMARY KEY,
    invoice_id     BIGINT        NOT NULL REFERENCES invoices(id) ON DELETE CASCADE,
    cost_center_id BIGINT        NOT NULL REFERENCES cost_centers(id),
    value          NUMERIC(16,4) NOT NULL DEFAULT 0,  -- percentage (0-100) or amount, per invoices.allocation_mode
    position       INT           NOT NULL DEFAULT 0,
    UNIQUE (invoice_id, cost_center_id)
);
CREATE INDEX idx_invoice_cost_allocations_invoice ON invoice_cost_allocations(invoice_id);
CREATE INDEX idx_invoice_cost_allocations_cost_center ON invoice_cost_allocations(cost_center_id);
