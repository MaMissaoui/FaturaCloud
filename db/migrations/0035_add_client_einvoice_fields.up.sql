-- Structured buyer data required by EN 16931 (XRechnung). clients previously
-- had no country column at all — only the free-text `address` blob, kept
-- as-is for legacy display.
ALTER TABLE clients ADD COLUMN street TEXT;
ALTER TABLE clients ADD COLUMN house_number TEXT;
ALTER TABLE clients ADD COLUMN postal_code TEXT;
ALTER TABLE clients ADD COLUMN city TEXT;
ALTER TABLE clients ADD COLUMN country_code TEXT;
ALTER TABLE clients ADD COLUMN tax_number TEXT;
-- Convenience default (e.g. a Leitweg-ID) copied into Invoice.buyerReference
-- at invoice-creation time in the frontend; not re-derived on every read.
ALTER TABLE clients ADD COLUMN default_buyer_reference TEXT;
