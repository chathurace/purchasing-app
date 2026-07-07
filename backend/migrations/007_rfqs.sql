-- An RFQ (request for quotation) is raised from a purchase request. A PR can
-- have many RFQs; each RFQ collects many quotations.
CREATE TABLE rfqs (
    id                  BIGSERIAL   PRIMARY KEY,           -- displayed as RFQ-{id:06d}
    purchase_request_id BIGINT      NOT NULL REFERENCES purchase_requests(id) ON DELETE CASCADE,
    title               TEXT        NOT NULL DEFAULT '',
    details             TEXT        NOT NULL DEFAULT '',    -- scope / instructions to vendors
    status              TEXT        NOT NULL DEFAULT 'draft',
        -- draft | sent | responses_received | closed | cancelled
    created_by          BIGINT      REFERENCES users(id),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_rfqs_pr     ON rfqs(purchase_request_id);
CREATE INDEX idx_rfqs_status ON rfqs(status);
