-- Two accounts, not one: the same taxRates row is referenced by sales AND
-- purchase line items (see taxRateReferencingTables in db/tax_rate.go) —
-- output VAT is a liability owed to the state, input VAT is a reclaimable
-- asset, different account types.
ALTER TABLE taxRates ADD COLUMN outputTaxAccountId TEXT REFERENCES accounts(id);
ALTER TABLE taxRates ADD COLUMN inputTaxAccountId TEXT REFERENCES accounts(id);
-- DATEV BU-Schlüssel (tax key), a short code independent of the percentage.
ALTER TABLE taxRates ADD COLUMN datev_bu_key TEXT;
