-- Restores 0059's global index. It fails once two organizations each have a
-- live posted entry naming the same document; reverse one of them first.
DROP INDEX idx_journal_entries_posted_source_document;
CREATE UNIQUE INDEX idx_journal_entries_posted_source_document
	ON journal_entries(sourceDocumentType, sourceDocumentId)
	WHERE status = 'posted' AND reversalOfEntryId IS NULL AND sourceDocumentId IS NOT NULL;
