CREATE TABLE IF NOT EXISTS outbound_deliveries (
    id TEXT NOT NULL PRIMARY KEY,
    organizationId TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    orderId TEXT REFERENCES orders(id) ON DELETE SET NULL,
    deliveryNumber TEXT NOT NULL,
    deliveryDate INTEGER NOT NULL,
    shippingAddress TEXT,
    trackingNumber TEXT,
    notes TEXT,
    status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'shipped', 'delivered', 'cancelled')),
    createdAt INTEGER NOT NULL DEFAULT (strftime('%s', 'now') * 1000)
);
CREATE INDEX IF NOT EXISTS outbound_deliveries_organizationId ON outbound_deliveries(organizationId);
CREATE INDEX IF NOT EXISTS outbound_deliveries_orderId ON outbound_deliveries(orderId);

CREATE TABLE IF NOT EXISTS outbound_delivery_line_items (
    id TEXT NOT NULL PRIMARY KEY,
    deliveryId TEXT NOT NULL REFERENCES outbound_deliveries(id) ON DELETE CASCADE,
    orderLineItemId TEXT REFERENCES orderLineItems(id) ON DELETE SET NULL,
    description TEXT NOT NULL,
    quantity REAL NOT NULL DEFAULT 1,
    unit TEXT,
    position INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS outbound_delivery_line_items_deliveryId ON outbound_delivery_line_items(deliveryId);
