-- Orders got a currency column in 0044 but were missed when 0045 added
-- exchangeRate/exchangeRateDate to every other document — a foreign-currency
-- order had no way to record its conversion to the organization's currency.
-- Direction and storage convention: see db/exchange_rate.go.
ALTER TABLE orders ADD COLUMN exchangeRate TEXT;
ALTER TABLE orders ADD COLUMN exchangeRateDate INTEGER;
