-- F191: one live posted entry per source document *within an organization*.
-- 0059's index left organizationId out, so an entry another organization had
-- tagged with a document's id blocked that document's own posting. The new
-- index is strictly looser than 0059's, so existing rows always satisfy it.
DROP INDEX idx_journal_entries_posted_source_document;
CREATE UNIQUE INDEX idx_journal_entries_posted_source_document
	ON journal_entries(organizationId, sourceDocumentType, sourceDocumentId)
	WHERE status = 'posted' AND reversalOfEntryId IS NULL AND sourceDocumentId IS NOT NULL;
