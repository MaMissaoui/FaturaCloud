-- Purchase orders: the buy-side counterpart to `orders`.
--
-- vendorId deliberately carries no ON DELETE clause. Vendor referential
-- integrity is enforced app-side by DeleteVendor's guard (see db/vendor.go);
-- ON DELETE SET NULL would silently orphan a purchase order from its vendor
-- and make that guard's rationale false.
CREATE TABLE IF NOT EXISTS purchase_orders (
    id TEXT NOT NULL PRIMARY KEY,
    organizationId TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    vendorId TEXT REFERENCES vendors(id),
    orderNumber TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'draft'
      CHECK (status IN ('draft', 'confirmed', 'received', 'cancelled')),
    orderDate INTEGER NOT NULL,
    expectedDate INTEGER,
    currency TEXT,
    deliveryAddress TEXT,
    notes TEXT,
    createdAt INTEGER NOT NULL DEFAULT (strftime('%s', 'now') * 1000)
);
CREATE INDEX IF NOT EXISTS purchase_orders_organizationId ON purchase_orders(organizationId);
CREATE INDEX IF NOT EXISTS purchase_orders_vendorId ON purchase_orders(vendorId);

CREATE TABLE IF NOT EXISTS purchase_order_line_items (
    id TEXT NOT NULL PRIMARY KEY,
    purchaseOrderId TEXT NOT NULL REFERENCES purchase_orders(id) ON DELETE CASCADE,
    productId TEXT REFERENCES products(id) ON DELETE SET NULL,
    description TEXT NOT NULL,
    quantity REAL NOT NULL DEFAULT 1,
    unitPrice INTEGER NOT NULL DEFAULT 0,
    unit TEXT,
    taxRate TEXT REFERENCES taxRates(id) ON DELETE SET NULL,
    position INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS purchase_order_line_items_purchaseOrderId
  ON purchase_order_line_items(purchaseOrderId);
