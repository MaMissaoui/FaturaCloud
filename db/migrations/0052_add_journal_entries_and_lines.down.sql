DROP INDEX IF EXISTS journal_lines_vendorId;
DROP INDEX IF EXISTS journal_lines_clientId;
DROP INDEX IF EXISTS journal_lines_accountId;
DROP INDEX IF EXISTS journal_lines_journalEntryId;
DROP TABLE IF EXISTS journal_lines;

DROP INDEX IF EXISTS journal_entries_status;
DROP INDEX IF EXISTS journal_entries_sourceDocument;
DROP INDEX IF EXISTS journal_entries_organizationId;
DROP INDEX IF EXISTS journal_entries_org_year_journal_number;
DROP TABLE IF EXISTS journal_entries;
