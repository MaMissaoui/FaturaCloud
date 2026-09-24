DROP INDEX IF EXISTS payment_applications_invoice_line_item;
ALTER TABLE payment_applications DROP COLUMN invoiceLineItemId;
