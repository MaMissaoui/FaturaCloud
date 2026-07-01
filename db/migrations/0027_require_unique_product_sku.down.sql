DROP INDEX IF EXISTS idx_products_org_sku;

CREATE INDEX idx_products_sku ON products(sku);
