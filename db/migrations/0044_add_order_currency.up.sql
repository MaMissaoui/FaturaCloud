-- Orders were the one gap in the sales chain: invoices and purchase_orders
-- both carry their own currency, orders didn't. Nullable like
-- purchase_orders.currency — a null order currency is displayed/interpreted
-- as the organization's own currency.
ALTER TABLE orders ADD COLUMN currency TEXT;
