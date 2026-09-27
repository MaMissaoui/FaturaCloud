-- Product families (e.g. "Machine à laver", "Réfrigérateur"): a maintained,
-- per-organization list products are grouped by — deliberately separate
-- from products.category, which is the finished/component classification
-- the BOM/production and sales/purchasing pickers depend on. Same shape as
-- units_of_measure (0076): unique name per organization, and
-- products.familyId is ON DELETE SET NULL so deleting a family only
-- ungroups its products, never deletes one.
CREATE TABLE IF NOT EXISTS product_families (
    id TEXT NOT NULL PRIMARY KEY,
    organizationId TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    createdAt TEXT NOT NULL DEFAULT (strftime('%Y-%m-%d %H:%M:%S', 'now'))
);
CREATE UNIQUE INDEX IF NOT EXISTS product_families_org_name
    ON product_families(organizationId, name);

ALTER TABLE products ADD COLUMN familyId TEXT REFERENCES product_families(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS products_familyId ON products(familyId);
