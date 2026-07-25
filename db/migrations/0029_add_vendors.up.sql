-- Vendors: the purchasing-side counterpart to clients. Deliberately a separate
-- table rather than a flag on clients — invoices INNER JOIN clients and every
-- sales-side picker assumes each client row is a customer.
CREATE TABLE IF NOT EXISTS vendors (
  id TEXT(21) PRIMARY KEY NOT NULL,
  organizationId TEXT NOT NULL,
  name TEXT,
  code TEXT,
  address TEXT,
  emails TEXT DEFAULT '[]',
  phone TEXT,
  website TEXT,
  registration_number TEXT,
  vatin TEXT,
  defaultCurrency TEXT,
  paymentTermsDays INTEGER,
  createdAt TEXT DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY (organizationId) REFERENCES organizations(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_vendors_organizationId ON vendors(organizationId);
CREATE INDEX IF NOT EXISTS idx_vendors_name ON vendors(name);
