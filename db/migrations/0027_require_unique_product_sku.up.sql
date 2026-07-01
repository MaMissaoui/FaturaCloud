-- Backfill any missing/blank product codes with something derived from the
-- product's own id (already unique) so the new unique index below can apply
-- cleanly to existing data.
UPDATE products
SET sku = 'PRD-' || upper(substr(id, 1, 8))
WHERE sku IS NULL OR trim(sku) = '';

DROP INDEX IF EXISTS idx_products_sku;

CREATE UNIQUE INDEX idx_products_org_sku ON products(organizationId, sku);
