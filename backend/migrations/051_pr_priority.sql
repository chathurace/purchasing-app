-- PR priority. Procurement triages a request as P1 (highest) / P2 / P3 and the
-- rest of the app reads it — the assignment card sets it, the list view shows it.
-- Every PR has one from creation (P3, the default); only procurement sets it, so
-- the requisition form does not collect it and existing rows land on P3.

ALTER TABLE purchase_requests
    ADD COLUMN priority TEXT NOT NULL DEFAULT 'P3'
        CHECK (priority IN ('P1', 'P2', 'P3'));
