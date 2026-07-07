-- An invoice's value can be entered directly instead of (or in addition to)
-- being derived from line items. entered_total holds the directly-entered value
-- when present; NULL means "derive from line items". The stored total_amount is
-- always the effective value (the entered total when given, else the line-items
-- sum), so existing consumers (over-billing checks, allocation validation) are
-- unchanged. The entered total takes priority when both are present.
ALTER TABLE invoices
    ADD COLUMN entered_total NUMERIC(16,2);  -- NULL = derive total from line items
