CREATE TABLE pr_items (
    id                  BIGSERIAL PRIMARY KEY,
    purchase_request_id BIGINT NOT NULL REFERENCES purchase_requests(id) ON DELETE CASCADE,
    description         TEXT   NOT NULL,
    quantity            NUMERIC(14,3) NOT NULL DEFAULT 1,
    position            INT    NOT NULL DEFAULT 0
);
CREATE INDEX idx_pr_items_pr ON pr_items(purchase_request_id);

CREATE TABLE pr_links (
    id                  BIGSERIAL PRIMARY KEY,
    purchase_request_id BIGINT NOT NULL REFERENCES purchase_requests(id) ON DELETE CASCADE,
    url                 TEXT   NOT NULL,
    label               TEXT   NOT NULL DEFAULT '',
    position            INT    NOT NULL DEFAULT 0
);
CREATE INDEX idx_pr_links_pr ON pr_links(purchase_request_id);
