-- A finished product's recipe: which "component" products, and how many of
-- each, build one unit of it. Consumed by the production/assembly feature
-- (db/production_order.go) — a Production Order snapshots these rows into
-- its own line items at creation time rather than referencing this table
-- live, the same "every document owns its line items" convention every
-- other document type in this app follows.
CREATE TABLE IF NOT EXISTS bill_of_materials (
    id TEXT NOT NULL PRIMARY KEY,
    organizationId TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    finishedProductId TEXT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    componentProductId TEXT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    quantityPerUnit REAL NOT NULL,
    createdAt INTEGER NOT NULL DEFAULT (strftime('%s', 'now') * 1000)
);
CREATE UNIQUE INDEX IF NOT EXISTS bill_of_materials_finished_component
    ON bill_of_materials(finishedProductId, componentProductId);
CREATE INDEX IF NOT EXISTS bill_of_materials_finishedProductId
    ON bill_of_materials(finishedProductId);
