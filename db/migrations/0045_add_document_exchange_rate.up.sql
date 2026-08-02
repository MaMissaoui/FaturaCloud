-- Direction: 1 unit of the document's own currency = exchangeRate units of
-- the organization's currency. Captured once at save time and frozen — see
-- db/exchange_rate.go. Stored as TEXT (a decimal string), not a float
-- column, matching the exact-rational arithmetic used elsewhere for money.
ALTER TABLE invoices ADD COLUMN exchangeRate TEXT;
ALTER TABLE invoices ADD COLUMN exchangeRateDate INTEGER;

ALTER TABLE purchase_orders ADD COLUMN exchangeRate TEXT;
ALTER TABLE purchase_orders ADD COLUMN exchangeRateDate INTEGER;

ALTER TABLE incoming_invoices ADD COLUMN exchangeRate TEXT;
ALTER TABLE incoming_invoices ADD COLUMN exchangeRateDate INTEGER;
