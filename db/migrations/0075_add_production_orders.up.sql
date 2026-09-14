-- Production Order: consumes a finished product's Bill of Materials
-- components and produces finished units — the actual assembly/production
-- feature the BOM (migration 0072) and Import serial-number range
-- (migration 0073) were both built to support (db/production_order.go).
--
-- finishedProductId is nullable with ON DELETE SET NULL, and
-- finishedProductName is denormalized at creation — the same convention
-- every other document type's productId column already follows
-- (invoiceLineItems, purchase_order_line_items, outbound/inbound delivery
-- line items all SET NULL rather than restrict deleting a product; see
-- CLAUDE.md's Database section). DeleteProduct has no usage guard today and
-- this doesn't add one — it matches the existing app-wide behavior instead
-- of inventing a stricter rule found nowhere else.
CREATE TABLE IF NOT EXISTS production_orders (
    id TEXT NOT NULL PRIMARY KEY,
    organizationId TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    orderNumber TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'completed', 'cancelled')),
    finishedProductId TEXT REFERENCES products(id) ON DELETE SET NULL,
    finishedProductName TEXT NOT NULL,
    quantity REAL NOT NULL,
    date INTEGER NOT NULL,
    -- No ON DELETE clause, same precedent as purchase_orders.importId —
    -- guarded app-side (db/import.go's DeleteImport, extended alongside
    -- this migration) rather than a cascade/SET NULL that would silently
    -- detach a completed order from the shipment its serial numbers were
    -- validated against.
    importId TEXT REFERENCES imports(id),
    notes TEXT,
    createdAt INTEGER NOT NULL DEFAULT (strftime('%s', 'now') * 1000)
);
CREATE INDEX IF NOT EXISTS production_orders_organizationId ON production_orders(organizationId);
CREATE INDEX IF NOT EXISTS production_orders_importId ON production_orders(importId);

-- Snapshotted from the BOM at creation time (see db/product_bom.go's
-- ReplaceBillOfMaterials comment) — a Production Order owns its own line
-- items, the same "every document owns its line items" convention every
-- other document type in this app follows, so a later BOM edit never
-- retroactively changes an existing order. componentProductId/componentName
-- follow the identical nullable-SET-NULL + denormalized-name shape as
-- finishedProductId above, and the same shape bill_of_materials_version_lines
-- already uses for the same reason.
CREATE TABLE IF NOT EXISTS production_order_component_lines (
    id TEXT NOT NULL PRIMARY KEY,
    productionOrderId TEXT NOT NULL REFERENCES production_orders(id) ON DELETE CASCADE,
    componentProductId TEXT REFERENCES products(id) ON DELETE SET NULL,
    componentName TEXT NOT NULL,
    quantityPerUnit REAL NOT NULL,
    totalQuantity REAL NOT NULL,
    createdAt INTEGER NOT NULL DEFAULT (strftime('%s', 'now') * 1000)
);
CREATE INDEX IF NOT EXISTS production_order_component_lines_productionOrderId
    ON production_order_component_lines(productionOrderId);
