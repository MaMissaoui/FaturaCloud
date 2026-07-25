-- Inbound deliveries (goods receipts): the buy-side counterpart to
-- outbound_deliveries, with the stock arrow inverted.
--
-- vendorId carries no ON DELETE clause for the same reason purchase_orders
-- does not: vendor integrity is enforced app-side by DeleteVendor's guard.
CREATE TABLE IF NOT EXISTS inbound_deliveries (
    id TEXT NOT NULL PRIMARY KEY,
    organizationId TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    purchaseOrderId TEXT REFERENCES purchase_orders(id) ON DELETE SET NULL,
    vendorId TEXT REFERENCES vendors(id),
    deliveryNumber TEXT NOT NULL,
    deliveryDate INTEGER NOT NULL,
    vendorDeliveryNote TEXT,
    trackingNumber TEXT,
    notes TEXT,
    status TEXT NOT NULL DEFAULT 'draft'
      CHECK (status IN ('draft', 'received', 'cancelled')),
    createdAt INTEGER NOT NULL DEFAULT (strftime('%s', 'now') * 1000)
);
CREATE INDEX IF NOT EXISTS inbound_deliveries_organizationId ON inbound_deliveries(organizationId);
CREATE INDEX IF NOT EXISTS inbound_deliveries_purchaseOrderId ON inbound_deliveries(purchaseOrderId);
CREATE INDEX IF NOT EXISTS inbound_deliveries_vendorId ON inbound_deliveries(vendorId);

-- Unlike outbound delivery lines (which deliberately carry no prices, since a
-- delivery note never shows them), a goods receipt records unitCost: it is what
-- feeds stockMovements.unitCost and, through it, the product's average cost.
CREATE TABLE IF NOT EXISTS inbound_delivery_line_items (
    id TEXT NOT NULL PRIMARY KEY,
    deliveryId TEXT NOT NULL REFERENCES inbound_deliveries(id) ON DELETE CASCADE,
    purchaseOrderLineItemId TEXT REFERENCES purchase_order_line_items(id) ON DELETE SET NULL,
    productId TEXT REFERENCES products(id) ON DELETE SET NULL,
    description TEXT NOT NULL,
    quantity REAL NOT NULL DEFAULT 1,
    unitCost INTEGER,
    unit TEXT,
    position INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS inbound_delivery_line_items_deliveryId
  ON inbound_delivery_line_items(deliveryId);
CREATE INDEX IF NOT EXISTS inbound_delivery_line_items_purchaseOrderLineItemId
  ON inbound_delivery_line_items(purchaseOrderLineItemId);
