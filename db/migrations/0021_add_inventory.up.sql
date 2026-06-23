ALTER TABLE products ADD COLUMN stockEnabled INTEGER NOT NULL DEFAULT 0;
ALTER TABLE products ADD COLUMN stockQuantity REAL NOT NULL DEFAULT 0;

CREATE TABLE stockMovements (
  id TEXT(21) PRIMARY KEY NOT NULL,
  organizationId TEXT NOT NULL,
  productId TEXT NOT NULL,
  type TEXT NOT NULL,
  quantity REAL NOT NULL,
  unitCost INTEGER,
  note TEXT,
  reference TEXT,
  createdAt TEXT DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY (organizationId) REFERENCES organizations(id) ON DELETE CASCADE,
  FOREIGN KEY (productId) REFERENCES products(id) ON DELETE CASCADE
);
CREATE INDEX idx_stockMovements_organizationId ON stockMovements(organizationId);
CREATE INDEX idx_stockMovements_productId ON stockMovements(productId);
