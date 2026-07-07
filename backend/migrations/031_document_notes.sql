-- Documents can carry a free-text note (used by the contract card: each draft
-- contract PDF and the signed contract PDF has an associated notes field).
ALTER TABLE documents
    ADD COLUMN notes TEXT NOT NULL DEFAULT '';
