-- Adds a website to the vendor master so the purchase-request "proposed
-- supplier" section can auto-fill the supplier website when a requester picks
-- an existing vendor from the editable dropdown.
ALTER TABLE vendors ADD COLUMN website TEXT NOT NULL DEFAULT '';
