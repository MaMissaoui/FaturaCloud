-- F114: imports model a consolidated China shipment (a container carrying
-- several vendors' purchase orders) — customs and freight are assessed on
-- the shipment as a whole, and one exchange rate applies to it, not to each
-- vendor PO in isolation. See db/import.go / db/gl_posting.go's
-- applyLandedCost for how freight/customs get allocated into the received
-- components' landed cost.
--
-- currency/exchangeRate here are a *prefill default* for purchase orders
-- linked to this import, not something converted or posted anywhere
-- themselves — each linked PO/receipt still stores and freezes its own
-- currency/exchangeRate (db/exchange_rate.go), same "entered once per
-- shipment, applied to every document in it" shape the New PO form already
-- offers via prefillExchangeRate. freightCost/customsCost are cents in the
-- organization's own functional currency — they're typically paid to a
-- local carrier/customs authority, not the (foreign-currency) vendor, so
-- there's no natural foreign currency to convert them from.
CREATE TABLE IF NOT EXISTS imports (
    id TEXT NOT NULL PRIMARY KEY,
    organizationId TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    importNumber TEXT NOT NULL,
    date INTEGER NOT NULL,
    currency TEXT,
    exchangeRate TEXT,
    exchangeRateDate INTEGER,
    freightCost INTEGER NOT NULL DEFAULT 0,
    customsCost INTEGER NOT NULL DEFAULT 0,
    notes TEXT,
    createdAt INTEGER NOT NULL DEFAULT (strftime('%s', 'now') * 1000)
);
CREATE INDEX IF NOT EXISTS imports_organizationId ON imports(organizationId);

-- No ON DELETE clause, same precedent as purchase_orders.vendorId: import
-- referential integrity is enforced app-side by DeleteImport's guard
-- (db/import.go) rather than a cascade or SET NULL that would silently
-- detach a purchase order (and the landed cost already allocated against
-- it) from its shipment.
ALTER TABLE purchase_orders ADD COLUMN importId TEXT REFERENCES imports(id);
CREATE INDEX IF NOT EXISTS purchase_orders_importId ON purchase_orders(importId);
