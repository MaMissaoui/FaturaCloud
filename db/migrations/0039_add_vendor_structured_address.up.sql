-- Give vendors the same structured address columns clients/organizations
-- already have (migrations 0034/0035), so purchasing documents can drop the
-- free-text address blob too instead of keeping vendors as the odd one out.
ALTER TABLE vendors ADD COLUMN street TEXT;
ALTER TABLE vendors ADD COLUMN house_number TEXT;
ALTER TABLE vendors ADD COLUMN postal_code TEXT;
ALTER TABLE vendors ADD COLUMN city TEXT;
ALTER TABLE vendors ADD COLUMN country_code TEXT;
