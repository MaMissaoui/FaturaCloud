DROP INDEX IF EXISTS products_familyId;
ALTER TABLE products DROP COLUMN familyId;
DROP TABLE IF EXISTS product_families;
