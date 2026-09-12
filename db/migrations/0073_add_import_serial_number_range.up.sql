-- An optional serial-number range an Import (F114, consolidated shipment)
-- reserves for whatever gets produced from the components it brought in —
-- the production/assembly feature (db/production_order.go) validates a
-- linked Production Order's serial numbers against this range when set.
-- All three nullable: an import never used for production leaves them
-- unset, the same "absence is legal" convention as every other optional
-- Import field (Currency/ExchangeRate/Notes).
ALTER TABLE imports ADD COLUMN serialNumberPrefix TEXT;
ALTER TABLE imports ADD COLUMN serialNumberRangeStart INTEGER;
ALTER TABLE imports ADD COLUMN serialNumberRangeEnd INTEGER;
