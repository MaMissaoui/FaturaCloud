DROP INDEX IF EXISTS product_families_org_name;
CREATE UNIQUE INDEX IF NOT EXISTS product_families_org_name
    ON product_families(organizationId, name);
