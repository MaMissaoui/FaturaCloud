package db

import (
	"database/sql"
	"errors"
	"fmt"
)

// postedEntryForSourceDocument is the tests' unscoped lookup: a test that
// already knows which document it posted needn't name the organization too.
// Production code uses the organization-scoped FindPostedEntryForSourceDocument
// (F191).
func (d *Database) postedEntryForSourceDocument(sourceDocumentType, sourceDocumentID string) (*JournalEntry, error) {
	var entry JournalEntry
	err := d.DB.Get(&entry,
		`SELECT * FROM journal_entries
		 WHERE `+postedEntrySourceDocumentPredicate+`
		 LIMIT 1`,
		sourceDocumentType, sourceDocumentID,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("posted_entry_for_source_document: %w", err)
	}
	return &entry, nil
}
