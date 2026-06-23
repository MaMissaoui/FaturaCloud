CREATE TABLE products (
  id TEXT(21) PRIMARY KEY NOT NULL,
  organizationId TEXT NOT NULL,
  name TEXT NOT NULL,
  description TEXT,
  sku TEXT,
  price INTEGER NOT NULL DEFAULT 0,
  unitCost INTEGER,
  unit TEXT,
  type TEXT NOT NULL DEFAULT 'service',
  taxRateId TEXT,
  createdAt TEXT DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY (organizationId) REFERENCES organizations(id) ON DELETE CASCADE,
  FOREIGN KEY (taxRateId) REFERENCES taxRates(id) ON DELETE SET NULL
);
CREATE INDEX idx_products_organizationId ON products(organizationId);
CREATE INDEX idx_products_name ON products(name);
CREATE INDEX idx_products_sku ON products(sku);
