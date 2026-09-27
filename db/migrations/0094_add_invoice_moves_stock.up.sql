-- Set to 1 by db.CreateCashSale: this invoice is a Cash Book counter sale
-- that takes its own stock out (db/invoice_stock.go) — while its state is
-- sent/paid, one "out" movement per stock-enabled line stands against it
-- (stockMovements.sourceDocumentId = the invoice id), reversed when it's
-- cancelled/drafted and re-posted if it comes back. Every other invoice
-- (0) leaves stock to deliveries exactly as before, so an invoice plus its
-- delivery never deducts twice. No backfill: Cash Book sales recorded
-- before this migration never moved stock and keep not moving it.
ALTER TABLE invoices ADD COLUMN movesStock INTEGER NOT NULL DEFAULT 0;
