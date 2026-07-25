-- Incoming invoices (vendor bills): the buy-side counterpart to invoices.
--
-- vendorId is NOT NULL so the list query can INNER JOIN vendors exactly as
-- GetInvoices does with clients — a nullable FK plus a copied INNER JOIN would
-- make vendor-less rows vanish from the list with no error. It carries no
-- ON DELETE clause: vendor integrity is enforced app-side by DeleteVendor's
-- guard, mirroring invoices.clientId, and a RESTRICT edge here could abort the
-- organization cascade depending on statement ordering.
CREATE TABLE IF NOT EXISTS incoming_invoices (
    id TEXT NOT NULL PRIMARY KEY,
    organizationId TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    vendorId TEXT NOT NULL REFERENCES vendors(id),
    purchaseOrderId TEXT REFERENCES purchase_orders(id) ON DELETE SET NULL,
    vendorInvoiceNumber TEXT NOT NULL,
    reference TEXT,
    state TEXT NOT NULL DEFAULT 'draft'
      CHECK (state IN ('draft', 'approved', 'paid', 'cancelled')),
    date INTEGER NOT NULL,
    dueDate INTEGER,
    currency TEXT NOT NULL,
    notes TEXT,
    total INTEGER NOT NULL DEFAULT 0,
    taxTotal INTEGER NOT NULL DEFAULT 0,
    subTotal INTEGER NOT NULL DEFAULT 0,
    matchOverride INTEGER NOT NULL DEFAULT 0,
    matchOverrideReason TEXT,
    createdAt INTEGER NOT NULL DEFAULT (strftime('%s', 'now') * 1000)
);
CREATE INDEX IF NOT EXISTS incoming_invoices_organizationId ON incoming_invoices(organizationId);
CREATE INDEX IF NOT EXISTS incoming_invoices_vendorId ON incoming_invoices(vendorId);
CREATE INDEX IF NOT EXISTS incoming_invoices_purchaseOrderId ON incoming_invoices(purchaseOrderId);

-- The same vendor must not be able to bill the same number twice.
CREATE UNIQUE INDEX IF NOT EXISTS incoming_invoices_vendor_number
  ON incoming_invoices(organizationId, vendorId, vendorInvoiceNumber);

CREATE TABLE IF NOT EXISTS incoming_invoice_line_items (
    id TEXT NOT NULL PRIMARY KEY,
    incomingInvoiceId TEXT NOT NULL REFERENCES incoming_invoices(id) ON DELETE CASCADE,
    purchaseOrderLineItemId TEXT REFERENCES purchase_order_line_items(id) ON DELETE SET NULL,
    productId TEXT REFERENCES products(id) ON DELETE SET NULL,
    description TEXT NOT NULL,
    quantity REAL NOT NULL DEFAULT 1,
    unitPrice INTEGER NOT NULL DEFAULT 0,
    taxRate TEXT REFERENCES taxRates(id) ON DELETE CASCADE,
    position INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS incoming_invoice_line_items_incomingInvoiceId
  ON incoming_invoice_line_items(incomingInvoiceId);
CREATE INDEX IF NOT EXISTS incoming_invoice_line_items_purchaseOrderLineItemId
  ON incoming_invoice_line_items(purchaseOrderLineItemId);
