-- entryNumber is NULL until POST time — a draft that's never posted must not
-- consume a number, or FEC's gapless-numbering requirement breaks. It's
-- allocated inside the same transaction as the row that consumes it via
-- MAX+1, which is race-free only because db.SetMaxOpenConns(1) serializes
-- every write through one connection (the same reasoning that already
-- backs NextInboundDeliveryNumber's MAX+1 scheme).
--
-- CRITICAL for whoever writes the allocator: the MAX+1 query must scan
-- `WHERE entryNumber IS NOT NULL`, not `WHERE status = 'posted'` — a
-- reversed entry keeps its entryNumber (status flips to 'reversed', the row
-- is never deleted), so scoping to status='posted' would re-issue an
-- already-used number the moment anything is reversed and violate the
-- unique index below.
CREATE TABLE IF NOT EXISTS journal_entries (
    id TEXT NOT NULL PRIMARY KEY,
    organizationId TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    journalId TEXT NOT NULL REFERENCES journals(id),
    fiscalYearId TEXT NOT NULL REFERENCES fiscal_years(id),
    fiscalPeriodId TEXT REFERENCES fiscal_periods(id),
    entryNumber INTEGER,
    date INTEGER NOT NULL,
    reference TEXT,
    description TEXT NOT NULL,
    -- Polymorphic source-document link, same idiom as
    -- stockMovements.sourceDocumentId: sourceDocumentType is one of
    -- 'invoice' | 'incoming_invoice' | 'payment' | 'manual' | 'closing' |
    -- 'fx_revaluation'; sourceDocumentId is NOT a DB foreign key since it
    -- targets different tables depending on the type.
    sourceDocumentType TEXT,
    sourceDocumentId TEXT,
    status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'posted', 'reversed')),
    reversalOfEntryId TEXT REFERENCES journal_entries(id),
    reversalReason TEXT,
    postedAt INTEGER,
    createdBy TEXT REFERENCES users(id) ON DELETE SET NULL,
    createdAt INTEGER NOT NULL DEFAULT (strftime('%s', 'now') * 1000)
);
CREATE UNIQUE INDEX IF NOT EXISTS journal_entries_org_year_journal_number
  ON journal_entries(organizationId, fiscalYearId, journalId, entryNumber);
CREATE INDEX IF NOT EXISTS journal_entries_organizationId ON journal_entries(organizationId);
CREATE INDEX IF NOT EXISTS journal_entries_sourceDocument ON journal_entries(sourceDocumentType, sourceDocumentId);
CREATE INDEX IF NOT EXISTS journal_entries_status ON journal_entries(organizationId, status);

-- Balance (sum debit = sum credit across a header's lines) can't be a
-- declarative cross-row SQLite constraint — enforced per-line by the CHECK
-- below plus an app-level assertion in allocateAndFinalizeEntryTx, the one
-- function every posting path is required to call (mirrors
-- insertStockMovementTx being the sole place stock actually moves).
--
-- clientId/vendorId are real typed FKs, not a polymorphic partnerId — this
-- app already has that precedent (invoices.clientId, purchase_orders.vendorId).
CREATE TABLE IF NOT EXISTS journal_lines (
    id TEXT NOT NULL PRIMARY KEY,
    journalEntryId TEXT NOT NULL REFERENCES journal_entries(id) ON DELETE CASCADE,
    accountId TEXT NOT NULL REFERENCES accounts(id),
    description TEXT,
    debit INTEGER NOT NULL DEFAULT 0,
    credit INTEGER NOT NULL DEFAULT 0,
    -- Foreign-currency shadow of debit/credit, same frozen-rate convention
    -- as every document's exchangeRate (see db/exchange_rate.go): debit and
    -- credit above are always in the organization's functional currency;
    -- these three populate only when the line's originating document was
    -- in a foreign currency.
    currency TEXT,
    foreignAmount INTEGER,
    exchangeRate TEXT,
    clientId TEXT REFERENCES clients(id),
    vendorId TEXT REFERENCES vendors(id),
    taxRateId TEXT REFERENCES taxRates(id) ON DELETE SET NULL,
    -- No FK yet — reconciliation_groups doesn't exist until migration 0057
    -- (SQLite can't ALTER TABLE ADD a FK to a not-yet-existing table). 0057
    -- adds the index once the referenced table exists.
    reconciliationGroupId TEXT,
    position INTEGER NOT NULL DEFAULT 0,
    createdAt INTEGER NOT NULL DEFAULT (strftime('%s', 'now') * 1000),
    CHECK (debit >= 0 AND credit >= 0),
    CHECK ((debit = 0) <> (credit = 0)),
    CHECK (clientId IS NULL OR vendorId IS NULL)
);
CREATE INDEX IF NOT EXISTS journal_lines_journalEntryId ON journal_lines(journalEntryId);
CREATE INDEX IF NOT EXISTS journal_lines_accountId ON journal_lines(accountId);
CREATE INDEX IF NOT EXISTS journal_lines_clientId ON journal_lines(clientId);
CREATE INDEX IF NOT EXISTS journal_lines_vendorId ON journal_lines(vendorId);
