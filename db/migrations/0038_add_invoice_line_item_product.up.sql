-- Sales invoice line items never persisted a product link (unlike purchase
-- orders, sales orders, and both delivery types) — CreateInvoiceLineItemRequest's
-- ProductID field existed only for incoming (vendor) invoices and stayed nil
-- on the sales path. Adding it here so a product can now be required and its
-- selection actually round-trips through an edit/reload, matching every other
-- document.
ALTER TABLE invoiceLineItems ADD COLUMN productId TEXT REFERENCES products(id) ON DELETE SET NULL;
