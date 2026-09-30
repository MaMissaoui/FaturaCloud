ALTER TABLE clients DROP COLUMN importBatchId;
DROP INDEX IF EXISTS loan_import_batches_organizationId;
DROP TABLE IF EXISTS loan_import_batches;
