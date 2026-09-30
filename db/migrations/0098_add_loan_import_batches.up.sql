-- One row per paper loan register import (db/loan_import.go): the batch the
-- import's migrated loans (invoices.importBatchId, migration 0097) and the
-- customers it created (clients.importBatchId) belong to, so the whole
-- import can be undone while none of its loans has been collected in the
-- app. undoneAt is set when it is; the row itself stays as the record.
CREATE TABLE IF NOT EXISTS loan_import_batches (
    id TEXT NOT NULL PRIMARY KEY,
    organizationId TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    fileName TEXT,
    cutoverDate INTEGER NOT NULL,
    loanCount INTEGER NOT NULL DEFAULT 0,
    customersCreated INTEGER NOT NULL DEFAULT 0,
    total INTEGER NOT NULL DEFAULT 0,
    outstanding INTEGER NOT NULL DEFAULT 0,
    createdBy TEXT,
    createdAt INTEGER NOT NULL DEFAULT (strftime('%s', 'now') * 1000),
    undoneAt INTEGER
);
CREATE INDEX IF NOT EXISTS loan_import_batches_organizationId ON loan_import_batches(organizationId);

ALTER TABLE clients ADD COLUMN importBatchId TEXT;
