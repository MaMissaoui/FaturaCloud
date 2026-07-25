-- Structured seller data required by EN 16931 (XRechnung). The existing
-- free-text `address`/`country` columns are kept as-is for legacy display;
-- these new columns are what the e-invoice export validates and reads from.
ALTER TABLE organizations ADD COLUMN bic TEXT;
ALTER TABLE organizations ADD COLUMN tax_number TEXT;
ALTER TABLE organizations ADD COLUMN street TEXT;
ALTER TABLE organizations ADD COLUMN house_number TEXT;
ALTER TABLE organizations ADD COLUMN postal_code TEXT;
ALTER TABLE organizations ADD COLUMN city TEXT;
ALTER TABLE organizations ADD COLUMN country_code TEXT;
