-- Configurable dropdown lists. A single generic key/value store backs every
-- runtime-editable dropdown in the requisition form (WSO2 entity, IT/Non-IT
-- category, currency, engagement type, budget category, product, region, and the
-- newly-configurable engagement code). Each list is identified by list_key; the
-- set of known keys lives in the Go model registry (model.ConfigLists).
CREATE TABLE config_options (
    id         BIGSERIAL PRIMARY KEY,
    list_key   TEXT        NOT NULL,
    value      TEXT        NOT NULL,
    sort_order INT         NOT NULL DEFAULT 0,
    is_active  BOOLEAN     NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (list_key, value)
);

CREATE INDEX idx_config_options_key ON config_options(list_key, sort_order);

-- Seed with the values that were previously hardcoded in the frontend
-- (frontend/src/types/api.ts). sort_order preserves the original ordering.
INSERT INTO config_options (list_key, value, sort_order) VALUES
    -- WSO2 entity
    ('entity', 'WSO2 Lanka Private Limited', 0),
    ('entity', 'WSO2 LLC (USA)', 1),
    ('entity', 'WSO2 UK Limited', 2),
    ('entity', 'WSO2 Australia', 3),
    ('entity', 'WSO2 India', 4),
    ('entity', 'Other', 5),

    -- IT category
    ('it_category', 'Software licences & subscriptions (incl. renewals)', 0),
    ('it_category', 'SaaS platform / cloud service', 1),
    ('it_category', 'IT support services', 2),
    ('it_category', 'Hosting services & domain names', 3),
    ('it_category', 'IT hardware & equipment', 4),
    ('it_category', 'Free / open-source tool (approval required)', 5),
    ('it_category', 'Other IT solution', 6),

    -- Non-IT category
    ('nonit_category', 'Fixed assets & hardware equipment', 0),
    ('nonit_category', 'Insurance (Medical, Life, Public Liability)', 1),
    ('nonit_category', 'Seasonal gifts, hampers, employee welcome packs', 2),
    ('nonit_category', 'Corporate merchandise & bulk printing', 3),
    ('nonit_category', 'Facilities — Soft services (security, janitorial)', 4),
    ('nonit_category', 'Facilities — Hard services (engineering, maintenance)', 5),
    ('nonit_category', 'Travel & accommodation', 6),
    ('nonit_category', 'Event management', 7),
    ('nonit_category', 'Other non-IT solution', 8),

    -- Engagement type
    ('engagement_type', 'One-time purchase', 0),
    ('engagement_type', 'Monthly subscription', 1),
    ('engagement_type', 'Annual subscription', 2),
    ('engagement_type', 'Multi-year agreement', 3),
    ('engagement_type', 'Other', 4),

    -- Currency
    ('currency', 'USD', 0),
    ('currency', 'LKR', 1),
    ('currency', 'GBP', 2),
    ('currency', 'AUD', 3),
    ('currency', 'INR', 4),
    ('currency', 'EUR', 5),

    -- Budget category
    ('budget_category', 'Software & Subscriptions', 0),
    ('budget_category', 'Hardware & Equipment', 1),
    ('budget_category', 'Cloud & Hosting', 2),
    ('budget_category', 'Professional Services', 3),
    ('budget_category', 'Marketing & Events', 4),
    ('budget_category', 'Facilities & Office', 5),
    ('budget_category', 'HR & People', 6),
    ('budget_category', 'Travel & Accommodation', 7),
    ('budget_category', 'Insurance', 8),
    ('budget_category', 'Other', 9),

    -- Product
    ('product', 'WSO2 API Manager', 0),
    ('product', 'WSO2 Identity Server', 1),
    ('product', 'WSO2 MI / Integration', 2),
    ('product', 'Choreo', 3),
    ('product', 'Asgardeo', 4),
    ('product', 'Ballerina', 5),
    ('product', 'Cross-product / Platform', 6),
    ('product', 'Not product-specific', 7),

    -- Region
    ('region', 'APAC — Sri Lanka', 0),
    ('region', 'APAC — India', 1),
    ('region', 'APAC — Australia', 2),
    ('region', 'APAC — Other', 3),
    ('region', 'Americas — USA', 4),
    ('region', 'Americas — Other', 5),
    ('region', 'EMEA — UK', 6),
    ('region', 'EMEA — Europe', 7),
    ('region', 'EMEA — Other', 8),
    ('region', 'Global / Multi-region', 9);

-- engagement_code starts empty (it was a free-text field); admins add codes from
-- the Settings page.
