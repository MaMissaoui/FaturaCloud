CREATE TABLE IF NOT EXISTS orders (
    id TEXT NOT NULL PRIMARY KEY,
    organizationId TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    clientId TEXT REFERENCES clients(id) ON DELETE SET NULL,
    orderNumber TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'draft',
    orderDate INTEGER NOT NULL,
    deliveryDate INTEGER,
    shippingAddress TEXT,
    trackingNumber TEXT,
    notes TEXT,
    createdAt INTEGER NOT NULL DEFAULT (strftime('%s', 'now') * 1000)
);
CREATE INDEX IF NOT EXISTS orders_organizationId ON orders(organizationId);

CREATE TABLE IF NOT EXISTS orderLineItems (
    id TEXT NOT NULL PRIMARY KEY,
    orderId TEXT NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    productId TEXT REFERENCES products(id) ON DELETE SET NULL,
    description TEXT NOT NULL,
    quantity REAL NOT NULL DEFAULT 1,
    unitPrice INTEGER NOT NULL DEFAULT 0,
    position INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS orderLineItems_orderId ON orderLineItems(orderId);
