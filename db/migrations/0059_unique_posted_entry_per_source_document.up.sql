-- F48: journal_entries had no uniqueness guard on
-- (sourceDocumentType, sourceDocumentId), so two concurrent identical
-- requests to the same state-change path (e.g. draft->sent on the same
-- invoice) could each pass a pre-transaction "does a posted entry already
-- exist" check and each post a full entry, double-counting AR/revenue.
--
-- This partial unique index is a faithful encoding of exactly what
-- FindPostedEntryForSourceDocument (db/journal_entry.go) already treats as
-- "the live entry for a document": status='posted' AND reversalOfEntryId IS
-- NULL. A reversed entry keeps its sourceDocumentId for traceability but
-- flips to status='reversed', so it never collides with the index — the
-- document can be re-posted after a reversal, same as today.
--
-- sourceDocumentId IS NOT NULL is explicit (not just implied by the other
-- predicates) because manual journal entries leave both source columns NULL
-- and SQLite treats NULLs as distinct in a unique index anyway — this just
-- makes the intent readable rather than relying on that behavior silently.
--
-- This turns the double-post race from silent ledger corruption into a 500
-- on the losing request. It is the durable backstop; the actual fix is
-- moving each path's existing-entry check inside its transaction (see
-- db/invoice.go, db/incoming_invoice.go, db/payment.go) so the race window
-- closes under SetMaxOpenConns(1) — the index alone would still let two
-- concurrent requests both attempt an insert and one 500, which is correct
-- but noisier than necessary without the in-tx re-check.
CREATE UNIQUE INDEX idx_journal_entries_posted_source_document
	ON journal_entries(sourceDocumentType, sourceDocumentId)
	WHERE status = 'posted' AND reversalOfEntryId IS NULL AND sourceDocumentId IS NOT NULL;
