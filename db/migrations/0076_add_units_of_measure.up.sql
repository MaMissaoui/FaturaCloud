-- A maintained, per-organization list of base units of measure (e.g. "kg",
-- "piece", "hour") — replaces what used to be a frontend-only suggestion
-- list (src/utils/units.ts's UNIT_OPTIONS) with real, selectable data, the
-- same "maintained list" shape payment_terms (migration 0070) already
-- established. Org-scoped for consistency with every other reference table
-- in this app (taxRates, payment_terms, accounts, …), not because a unit
-- like "kg" is inherently an org-specific opinion.
CREATE TABLE IF NOT EXISTS units_of_measure (
    id TEXT NOT NULL PRIMARY KEY,
    organizationId TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    isDefault INTEGER NOT NULL DEFAULT 0 CHECK (isDefault IN (0, 1)),
    createdAt TEXT NOT NULL DEFAULT (strftime('%Y-%m-%d %H:%M:%S', 'now'))
);
CREATE UNIQUE INDEX IF NOT EXISTS units_of_measure_org_name
    ON units_of_measure(organizationId, name);

-- Nullable, ON DELETE SET NULL: deleting a unit of measure never orphans or
-- destroys a product, it just clears the structured link (the legacy
-- products.unit text column, kept in sync by db/product.go's
-- resolveProductUnit whenever a unitOfMeasureId is selected, still carries
-- the last-known display value). This is the deliberate, safer choice over
-- taxRates' CASCADE-plus-app-guard pattern: a tax rate's CASCADE only ever
-- destroys a line item, but CASCADE here would destroy the product itself.
ALTER TABLE products ADD COLUMN unitOfMeasureId TEXT REFERENCES units_of_measure(id) ON DELETE SET NULL;
