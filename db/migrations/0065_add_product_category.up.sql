-- Distinguishes a purchasable component/intermediate from a sellable
-- finished good — orthogonal to `type` (product|service), since "component
-- vs finished" only ever applies to a physical product, never a service.
-- NULL (the default for every existing row) means "unclassified" — every
-- picker treats NULL as eligible on both the purchasing and sales side, so
-- this is opt-in and fully backward compatible.
ALTER TABLE products ADD COLUMN category TEXT
  CHECK (category IS NULL OR category IN ('finished', 'component'));
