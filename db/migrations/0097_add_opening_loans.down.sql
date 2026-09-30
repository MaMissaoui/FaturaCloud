ALTER TABLE payments DROP COLUMN origin;
DROP INDEX IF EXISTS invoices_importBatchId;
ALTER TABLE invoices DROP COLUMN importBatchId;
ALTER TABLE invoices DROP COLUMN origin;
