-- The free-text `address` blob on clients/organizations/vendors duplicated
-- the structured street/house_number/postal_code/city/country_code columns
-- added for e-invoicing (migrations 0034/0035/0039). Keeping both meant every
-- edit form asked for the same address twice. The structured fields are now
-- the single source of truth everywhere (forms, lists, PDFs).
--
-- Best-effort backfill: for any row that never got structured data, dump the
-- old free-text value into `street` (as a single line) rather than silently
-- discarding it — a one-time migration, not a permanent parsing feature.
UPDATE clients SET street = address WHERE street IS NULL AND address IS NOT NULL AND address != '';
UPDATE organizations SET street = address WHERE street IS NULL AND address IS NOT NULL AND address != '';
UPDATE vendors SET street = address WHERE street IS NULL AND address IS NOT NULL AND address != '';

ALTER TABLE clients DROP COLUMN address;
ALTER TABLE organizations DROP COLUMN address;
ALTER TABLE vendors DROP COLUMN address;
