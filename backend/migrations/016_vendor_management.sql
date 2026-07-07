-- Vendor management: extend the vendor master with a status flag, tax /
-- registration id, and a basic structured address. Existing rows default to
-- active with empty strings.
ALTER TABLE vendors
    ADD COLUMN is_active     BOOLEAN NOT NULL DEFAULT TRUE,
    ADD COLUMN tax_id        TEXT    NOT NULL DEFAULT '',
    ADD COLUMN address_line  TEXT    NOT NULL DEFAULT '',
    ADD COLUMN city          TEXT    NOT NULL DEFAULT '',
    ADD COLUMN postal_code   TEXT    NOT NULL DEFAULT '',
    ADD COLUMN country       TEXT    NOT NULL DEFAULT '';

CREATE INDEX idx_vendors_active ON vendors(is_active);
