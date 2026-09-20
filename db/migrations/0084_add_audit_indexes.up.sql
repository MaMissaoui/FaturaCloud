-- F126: three unindexed hot predicates, each a full scan of a table that
-- grows without bound. Same rationale as 0060 (F58) — an index-only
-- migration with no schema change.

-- 1. GetJournalEntryReversal (db/journal_entry.go) runs
--    SELECT * FROM journal_entries WHERE reversalOfEntryId = ? LIMIT 1.
--    Every journal-entry detail view calls it, and journal_entries is the
--    largest append-mostly, never-compacted table in the database, so this
--    was a full scan on each detail load.
CREATE INDEX IF NOT EXISTS journal_entries_reversalOfEntryId
    ON journal_entries(reversalOfEntryId);

-- 2. GetCashMovementDetails (db/cash_movement_details.go) filters payments
--    by organizationId (inside the ranked CTE), then bankAccountId and a
--    date range in the outer query. Migration 0056 indexed organizationId,
--    clientId, vendorId and journalEntryId only. A composite index on the
--    actual filter tuple supports the leading organizationId, the
--    bankAccountId equality probe, and the date range in one index rather
--    than combining two separate single-column indexes.
CREATE INDEX IF NOT EXISTS payments_organizationId_bankAccountId_date
    ON payments(organizationId, bankAccountId, date);

-- 3. GetAccountUsageCount (db/account.go) checks
--    accountId = ? OR counterAccountId = ? against cash_movements.
--    Migration 0080 indexed accountId but not counterAccountId, so the
--    second half of the OR was an unindexed full scan.
CREATE INDEX IF NOT EXISTS cash_movements_counterAccountId
    ON cash_movements(counterAccountId);
