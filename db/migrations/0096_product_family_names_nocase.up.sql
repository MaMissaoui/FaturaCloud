-- Product family names are unique per organization regardless of case:
-- "Four" and "four" are the same family, and the products Excel import
-- matches a Family cell case-insensitively. NOCASE only folds ASCII, so
-- db/product_family.go also checks with Unicode case folding
-- ("Électroménager" vs "électroménager") before insert/update; this index is
-- the database-level backstop for the ASCII cases.
DROP INDEX IF EXISTS product_families_org_name;
CREATE UNIQUE INDEX IF NOT EXISTS product_families_org_name
    ON product_families(organizationId, name COLLATE NOCASE);
