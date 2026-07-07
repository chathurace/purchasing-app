-- Adds a "registered vendor" flag to the vendor master. Defaults to FALSE so
-- existing and newly-created vendors start unregistered; only admins /
-- finance_admins flip it (enforced via middleware.HasVendorAdmin on the
-- vendor Update endpoint).
ALTER TABLE vendors ADD COLUMN registered BOOLEAN NOT NULL DEFAULT FALSE;
