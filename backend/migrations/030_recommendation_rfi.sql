-- A procurement recommendation may optionally carry an RFI (request for
-- information) raised with the selected vendor: a free-text description plus PDF
-- attachments. The description lives here; the PDFs attach via the shared
-- documents table (owner_type 'pr_recommendation_rfi', owner_id = recommendation
-- id). An RFI is considered present when the description is non-empty or at least
-- one attachment exists.
ALTER TABLE pr_recommendations
    ADD COLUMN rfi_description TEXT NOT NULL DEFAULT '';
