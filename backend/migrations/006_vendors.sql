-- Vendors are reusable suppliers referenced by quotations and contracts.
-- They are never cascade-deleted by purchase-request activity.
CREATE TABLE vendors (
    id           BIGSERIAL   PRIMARY KEY,           -- displayed as VEN-{id:06d}
    name         TEXT        NOT NULL,
    contact_name TEXT        NOT NULL DEFAULT '',
    email        TEXT        NOT NULL DEFAULT '',
    phone        TEXT        NOT NULL DEFAULT '',
    notes        TEXT        NOT NULL DEFAULT '',
    created_by   BIGINT      REFERENCES users(id),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_vendors_name ON vendors(name);
