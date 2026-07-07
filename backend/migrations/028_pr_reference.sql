-- Human-readable purchase-request reference, e.g. PR-2026-0000001, generated
-- at submission with a per-year sequence that resets each calendar year.
-- Nullable: purchase requests created before this feature keep NULL and fall
-- back to the legacy PR-NNNNNN display on the client.
ALTER TABLE purchase_requests ADD COLUMN reference TEXT;
CREATE UNIQUE INDEX idx_purchase_requests_reference ON purchase_requests(reference);

-- Per-year counter backing the reference sequence. One row per year; the
-- INSERT ... ON CONFLICT DO UPDATE in CreatePurchaseRequest bumps it atomically
-- under the row lock, so concurrent submissions never collide.
CREATE TABLE pr_reference_sequences (
    year     INT    PRIMARY KEY,
    last_seq BIGINT NOT NULL DEFAULT 0
);
