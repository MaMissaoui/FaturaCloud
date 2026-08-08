-- Chart of accounts: hierarchical (parent/child groups + postable leaf
-- accounts), typed, scoped per organization like every other master-data
-- table. isGroup=1 rows are headers only — never postable; posting code
-- must reject a line whose account has isGroup=1. normalBalance is
-- deliberately NOT stored: it's a pure function of type (asset/expense are
-- debit-normal, liability/equity/revenue are credit-normal), so storing it
-- would be a second source of truth that could drift — the same "compute,
-- don't cache" rule products.stockQuantity/unitCost already follow, applied
-- here to a structural constant rather than a derived value.
CREATE TABLE IF NOT EXISTS accounts (
    id TEXT NOT NULL PRIMARY KEY,
    organizationId TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    parentId TEXT REFERENCES accounts(id),
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    type TEXT NOT NULL CHECK (type IN ('asset', 'liability', 'equity', 'revenue', 'expense')),
    isGroup INTEGER NOT NULL DEFAULT 0,
    isActive INTEGER NOT NULL DEFAULT 1,
    -- SKR03/SKR04-style numeric code for the DATEV Buchungsstapel "Konto"
    -- column. Added now even though the DATEV exporter itself ships later —
    -- retrofitting it once accounts are already referenced by immutable
    -- journal_lines rows would be a data migration, not a column add.
    datevAccountNumber TEXT,
    description TEXT,
    createdAt INTEGER NOT NULL DEFAULT (strftime('%s', 'now') * 1000)
);
CREATE UNIQUE INDEX IF NOT EXISTS accounts_organizationId_code ON accounts(organizationId, code);
CREATE INDEX IF NOT EXISTS accounts_organizationId ON accounts(organizationId);
CREATE INDEX IF NOT EXISTS accounts_parentId ON accounts(parentId);
CREATE INDEX IF NOT EXISTS accounts_type ON accounts(organizationId, type);
