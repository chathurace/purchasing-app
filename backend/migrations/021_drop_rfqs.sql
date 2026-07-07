-- Flow change: RFQs are removed. Quotations now attach directly to a purchase
-- request (zero or more per PR), so a quotation carries purchase_request_id
-- instead of rfq_id. Documents that were owned by RFQs are dropped along with
-- the rfqs table.

-- 1. Add the direct PR link and backfill it from the quotation's old RFQ.
ALTER TABLE quotations
    ADD COLUMN purchase_request_id BIGINT REFERENCES purchase_requests(id) ON DELETE CASCADE;

UPDATE quotations q
SET purchase_request_id = r.purchase_request_id
FROM rfqs r
WHERE r.id = q.rfq_id;

ALTER TABLE quotations ALTER COLUMN purchase_request_id SET NOT NULL;

-- 2. Drop the old RFQ link and reindex by purchase request.
DROP INDEX IF EXISTS idx_quotations_rfq;
ALTER TABLE quotations DROP COLUMN rfq_id;
CREATE INDEX idx_quotations_pr ON quotations(purchase_request_id);

-- 3. Remove RFQ-owned documents and the rfqs table itself.
DELETE FROM documents WHERE owner_type = 'rfq';
DROP TABLE rfqs;
