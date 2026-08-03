ALTER TABLE products ADD COLUMN serialized INTEGER NOT NULL DEFAULT 0;

-- One row per physical unit ever registered, never deleted. Whether it's
-- currently in stock is computed on read from the sign of its most recent
-- linked stockMovements row — the same "computed on read, never stored"
-- rule products.stockQuantity/unitCost already follow, so a cancel or
-- delete elsewhere can never leave a stale stored flag here.
CREATE TABLE product_serial_numbers (
  id TEXT(21) PRIMARY KEY NOT NULL,
  organizationId TEXT NOT NULL,
  productId TEXT NOT NULL,
  serialNumber TEXT NOT NULL,
  createdAt TEXT DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY (organizationId) REFERENCES organizations(id) ON DELETE CASCADE,
  FOREIGN KEY (productId) REFERENCES products(id) ON DELETE CASCADE
);
-- Unique per product, not per organization: two different products may
-- legitimately reuse a manufacturer's serial string.
CREATE UNIQUE INDEX idx_product_serial_numbers_product_serial
  ON product_serial_numbers(organizationId, productId, serialNumber);
CREATE INDEX idx_product_serial_numbers_productId ON product_serial_numbers(productId);

-- Serialized products post one stockMovements row per physical unit
-- (quantity always exactly +1/-1) instead of one aggregate row per line;
-- NULL for every movement of a non-serialized product.
ALTER TABLE stockMovements ADD COLUMN serialNumberId TEXT REFERENCES product_serial_numbers(id) ON DELETE SET NULL;
CREATE INDEX idx_stockMovements_serialNumberId ON stockMovements(serialNumberId);

-- The receiving/shipping document's own row id — deliberately NOT
-- `reference`, which is free text a user types (e.g. a manual movement
-- could coincidentally reuse a receipt's display number). Cancel-reversal
-- for serialized lines looks up exactly the rows a document posted by this
-- column, never by `reference`.
ALTER TABLE stockMovements ADD COLUMN sourceDocumentId TEXT;
CREATE INDEX idx_stockMovements_sourceDocumentId ON stockMovements(sourceDocumentId);
