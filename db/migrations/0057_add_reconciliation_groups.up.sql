-- Lettrage (FEC's EcritureLet/DateLet): links an AR/AP line to the payment
-- line that settled it. `code` (AAA, AAB, ...) is allocated the same
-- MAX-style way as journal_entries.entryNumber, safe for the same
-- single-writer reason (db.SetMaxOpenConns(1)).
CREATE TABLE IF NOT EXISTS reconciliation_groups (
    id TEXT NOT NULL PRIMARY KEY,
    organizationId TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    code TEXT NOT NULL,
    settledAt INTEGER,
    createdAt INTEGER NOT NULL DEFAULT (strftime('%s', 'now') * 1000)
);
CREATE UNIQUE INDEX IF NOT EXISTS reconciliation_groups_organizationId_code ON reconciliation_groups(organizationId, code);

-- journal_lines.reconciliationGroupId was added as a plain column (no FK)
-- back in 0052, since reconciliation_groups didn't exist yet. Only the
-- index is added here.
CREATE INDEX IF NOT EXISTS journal_lines_reconciliationGroupId ON journal_lines(reconciliationGroupId);
