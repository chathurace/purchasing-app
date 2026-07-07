CREATE TABLE documents (
    id                  BIGSERIAL   PRIMARY KEY,
    purchase_request_id BIGINT      NOT NULL REFERENCES purchase_requests(id) ON DELETE CASCADE,
    filename            TEXT        NOT NULL,        -- original name, preserved
    stored_path         TEXT        NOT NULL,        -- relative to files root
    content_type        TEXT        NOT NULL DEFAULT 'application/octet-stream',
    size_bytes          BIGINT      NOT NULL DEFAULT 0,
    uploaded_by         BIGINT      REFERENCES users(id),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_documents_pr ON documents(purchase_request_id);
