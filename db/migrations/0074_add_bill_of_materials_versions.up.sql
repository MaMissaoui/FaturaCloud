-- An append-only audit trail of a finished product's recipe over time.
-- Every successful ReplaceBillOfMaterials call (db/product_bom.go) snapshots
-- the resulting NEW current state as a new version here — version 1 is the
-- very first recipe ever saved, not "the state before the first edit" — so
-- history is complete from the start rather than only starting once
-- something has already changed. The live `bill_of_materials` table stays
-- exactly as-is (current truth, every existing call site unchanged); this
-- is purely additive.
CREATE TABLE IF NOT EXISTS bill_of_materials_versions (
    id TEXT NOT NULL PRIMARY KEY,
    organizationId TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    finishedProductId TEXT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    versionNumber INTEGER NOT NULL,
    -- The "batch size" the editor was showing quantities against when this
    -- version was saved (default 1 — plain per-unit entry) — purely a UI
    -- entry-helper memory, replayed back so reopening the editor shows the
    -- same batch view rather than resetting to 1. Every quantityPerUnit in
    -- this version's lines is still always the canonical per-single-unit
    -- value regardless of this field.
    batchSize INTEGER NOT NULL DEFAULT 1,
    createdAt INTEGER NOT NULL DEFAULT (strftime('%s', 'now') * 1000)
);
CREATE UNIQUE INDEX IF NOT EXISTS bill_of_materials_versions_product_number
    ON bill_of_materials_versions(finishedProductId, versionNumber);
CREATE INDEX IF NOT EXISTS bill_of_materials_versions_finishedProductId
    ON bill_of_materials_versions(finishedProductId);

-- Denormalized (componentName/Sku/Unit captured at snapshot time, not
-- live-joined) — unlike the live bill_of_materials table, a historical
-- version must keep reading correctly even after the component product is
-- later renamed or deleted. componentProductId is ON DELETE SET NULL for
-- exactly that reason: deleting a component must not delete history, only
-- detach the (still-displayable-by-name) link to it.
CREATE TABLE IF NOT EXISTS bill_of_materials_version_lines (
    id TEXT NOT NULL PRIMARY KEY,
    versionId TEXT NOT NULL REFERENCES bill_of_materials_versions(id) ON DELETE CASCADE,
    componentProductId TEXT REFERENCES products(id) ON DELETE SET NULL,
    componentName TEXT NOT NULL,
    componentSku TEXT,
    componentUnit TEXT,
    quantityPerUnit REAL NOT NULL,
    createdAt INTEGER NOT NULL DEFAULT (strftime('%s', 'now') * 1000)
);
CREATE INDEX IF NOT EXISTS bill_of_materials_version_lines_versionId
    ON bill_of_materials_version_lines(versionId);
