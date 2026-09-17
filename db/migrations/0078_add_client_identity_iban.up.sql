-- Cash Book feature: the requested customer search (name, mobile, IBAN,
-- identity number) needs two fields clients didn't have yet. phone already
-- covers "mobile" (no separate column). identity_number is a national ID /
-- CIN card number (common Tunisian retail practice when extending informal
-- credit) — deliberately separate from vatin/tax_number, which are tax IDs,
-- not personal identity documents. iban is search/reference only; no
-- payment-rail behavior is attached to it.
ALTER TABLE clients ADD COLUMN identity_number TEXT;
ALTER TABLE clients ADD COLUMN iban TEXT;
