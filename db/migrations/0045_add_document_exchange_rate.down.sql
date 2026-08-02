ALTER TABLE invoices DROP COLUMN exchangeRate;
ALTER TABLE invoices DROP COLUMN exchangeRateDate;

ALTER TABLE purchase_orders DROP COLUMN exchangeRate;
ALTER TABLE purchase_orders DROP COLUMN exchangeRateDate;

ALTER TABLE incoming_invoices DROP COLUMN exchangeRate;
ALTER TABLE incoming_invoices DROP COLUMN exchangeRateDate;
