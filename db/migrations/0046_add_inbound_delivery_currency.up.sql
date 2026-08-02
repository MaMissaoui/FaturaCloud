-- Goods receipts had no currency at all — unitCost on each line was
-- implicitly assumed to be in the organization's own currency. A receipt
-- against a foreign-currency purchase order needs its own currency +
-- exchangeRate so db.UpdateInboundDeliveryStatus can convert unitCost to
-- organization-currency terms before it feeds stockMovements/average cost
-- (see db/product_cost.go — that replay has no way to know about, or
-- reconcile, more than one currency). Direction and storage convention: see
-- db/exchange_rate.go.
ALTER TABLE inbound_deliveries ADD COLUMN currency TEXT;
ALTER TABLE inbound_deliveries ADD COLUMN exchangeRate TEXT;
ALTER TABLE inbound_deliveries ADD COLUMN exchangeRateDate INTEGER;
