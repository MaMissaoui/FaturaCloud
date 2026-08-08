-- Journals (sales/purchases/cash/bank/miscellaneous) as a distinct concept
-- from accounts — maps directly to DATEV/FEC's JournalCode/JournalLib.
CREATE TABLE IF NOT EXISTS journals (
    id TEXT NOT NULL PRIMARY KEY,
    organizationId TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    type TEXT NOT NULL CHECK (type IN ('sales', 'purchases', 'cash', 'bank', 'miscellaneous')),
    -- Seeded default journals (see db/journal.go's seedDefaultJournals) that
    -- auto-posting depends on existing use the same "in-use" guard shape as
    -- taxRates/vendors, not a hardcoded ban on deletion.
    isSystem INTEGER NOT NULL DEFAULT 0,
    createdAt INTEGER NOT NULL DEFAULT (strftime('%s', 'now') * 1000)
);
CREATE UNIQUE INDEX IF NOT EXISTS journals_organizationId_code ON journals(organizationId, code);
