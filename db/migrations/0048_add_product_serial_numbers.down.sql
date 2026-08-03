DROP INDEX IF EXISTS idx_stockMovements_sourceDocumentId;
ALTER TABLE stockMovements DROP COLUMN sourceDocumentId;

DROP INDEX IF EXISTS idx_stockMovements_serialNumberId;
ALTER TABLE stockMovements DROP COLUMN serialNumberId;

DROP TABLE IF EXISTS product_serial_numbers;

ALTER TABLE products DROP COLUMN serialized;
