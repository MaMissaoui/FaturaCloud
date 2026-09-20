-- Cash Book walk-in customer details: an extra free-text address, two more
-- phone numbers and a guarantor (French: garant). The structured address
-- columns (street/house_number/postal_code/city) stay for the full client
-- form; `address` is a single-line convenience field for the counter's quick
-- "New customer" modal, shown alongside the structured one.
ALTER TABLE clients ADD COLUMN phone2 TEXT;
ALTER TABLE clients ADD COLUMN phone3 TEXT;
ALTER TABLE clients ADD COLUMN guarantor TEXT;
ALTER TABLE clients ADD COLUMN address TEXT;
