-- A cash movement records money leaving a cash register with no underlying
-- vendor bill or client document — deposited to the bank, or spent as an
-- undocumented (petty-cash) expense. The `payments` table cannot represent
-- either case: payment_applications.documentType is CHECK-constrained to
-- ('invoice', 'incoming_invoice'), and payments' own CHECK requires a
-- client (inbound) or vendor (outbound) partner — neither exists here.
-- Paying an EXISTING vendor bill out of the till already works via the
-- ordinary payments flow and is untouched by this table.
--
-- accountId is the register account being credited; counterAccountId is
-- either a bank account (a deposit) or an expense account (an undocumented
-- spend) per counterAccountType. journalEntryId anchors the posted,
-- two-line (Dr counterAccount / Cr accountId) entry CreateCashMovement
-- posts atomically — this row is what gives that entry a stable
-- sourceDocumentId (journal_entries' partial unique index on
-- (sourceDocumentType, sourceDocumentId) needs one) and what a future
-- report/void path can address by id, the same "every document owns its
-- own row" convention every other GL-posting feature in this app follows.
CREATE TABLE IF NOT EXISTS cash_movements (
    id TEXT NOT NULL PRIMARY KEY,
    organizationId TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    accountId TEXT NOT NULL REFERENCES accounts(id),
    date INTEGER NOT NULL,
    counterAccountType TEXT NOT NULL CHECK (counterAccountType IN ('bank', 'expense')),
    counterAccountId TEXT NOT NULL REFERENCES accounts(id),
    amount INTEGER NOT NULL CHECK (amount > 0),
    note TEXT,
    journalEntryId TEXT REFERENCES journal_entries(id),
    createdAt INTEGER NOT NULL DEFAULT (strftime('%s', 'now') * 1000)
);
CREATE INDEX IF NOT EXISTS cash_movements_organizationId ON cash_movements(organizationId);
CREATE INDEX IF NOT EXISTS cash_movements_accountId ON cash_movements(accountId);
CREATE INDEX IF NOT EXISTS cash_movements_journalEntryId ON cash_movements(journalEntryId);
