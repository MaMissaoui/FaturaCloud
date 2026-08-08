CREATE TABLE IF NOT EXISTS payments (
    id TEXT NOT NULL PRIMARY KEY,
    organizationId TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    direction TEXT NOT NULL CHECK (direction IN ('inbound', 'outbound')),
    clientId TEXT REFERENCES clients(id),
    vendorId TEXT REFERENCES vendors(id),
    bankAccountId TEXT NOT NULL REFERENCES accounts(id),
    amount INTEGER NOT NULL CHECK (amount > 0),
    currency TEXT NOT NULL,
    exchangeRate TEXT,
    exchangeRateDate INTEGER,
    date INTEGER NOT NULL,
    method TEXT NOT NULL CHECK (method IN ('bank_transfer', 'cash', 'card', 'direct_debit', 'check', 'other')),
    reference TEXT,
    notes TEXT,
    status TEXT NOT NULL DEFAULT 'posted' CHECK (status IN ('posted', 'voided')),
    journalEntryId TEXT REFERENCES journal_entries(id),
    voidingEntryId TEXT REFERENCES journal_entries(id),
    createdAt INTEGER NOT NULL DEFAULT (strftime('%s', 'now') * 1000),
    CHECK (
        (direction = 'inbound'  AND clientId IS NOT NULL AND vendorId IS NULL) OR
        (direction = 'outbound' AND vendorId IS NOT NULL AND clientId IS NULL)
    )
);
CREATE INDEX IF NOT EXISTS payments_organizationId ON payments(organizationId);
CREATE INDEX IF NOT EXISTS payments_clientId ON payments(clientId);
CREATE INDEX IF NOT EXISTS payments_vendorId ON payments(vendorId);
CREATE INDEX IF NOT EXISTS payments_journalEntryId ON payments(journalEntryId);

-- Supports partial payments and one payment settling multiple documents.
-- documentType/documentId mirrors journal_entries.sourceDocumentType/Id — a
-- polymorphic link since it targets two different tables by type.
CREATE TABLE IF NOT EXISTS payment_applications (
    id TEXT NOT NULL PRIMARY KEY,
    paymentId TEXT NOT NULL REFERENCES payments(id) ON DELETE CASCADE,
    documentType TEXT NOT NULL CHECK (documentType IN ('invoice', 'incoming_invoice')),
    documentId TEXT NOT NULL,
    amount INTEGER NOT NULL CHECK (amount > 0),
    createdAt INTEGER NOT NULL DEFAULT (strftime('%s', 'now') * 1000)
);
CREATE UNIQUE INDEX IF NOT EXISTS payment_applications_payment_document
  ON payment_applications(paymentId, documentType, documentId);
CREATE INDEX IF NOT EXISTS payment_applications_document ON payment_applications(documentType, documentId);
