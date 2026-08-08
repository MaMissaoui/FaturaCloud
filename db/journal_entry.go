package db

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	gonanoid "github.com/matoous/go-nanoid/v2"
)

// JournalEntry mirrors the journal_entries table.
type JournalEntry struct {
	ID                 string  `db:"id"                 json:"id"`
	OrganizationID     string  `db:"organizationId"     json:"organizationId"`
	JournalID          string  `db:"journalId"          json:"journalId"`
	FiscalYearID       string  `db:"fiscalYearId"       json:"fiscalYearId"`
	FiscalPeriodID     *string `db:"fiscalPeriodId"     json:"fiscalPeriodId"`
	EntryNumber        *int64  `db:"entryNumber"        json:"entryNumber"`
	Date               int64   `db:"date"               json:"date"`
	Reference          *string `db:"reference"          json:"reference"`
	Description        string  `db:"description"        json:"description"`
	SourceDocumentType *string `db:"sourceDocumentType" json:"sourceDocumentType"`
	SourceDocumentID   *string `db:"sourceDocumentId"   json:"sourceDocumentId"`
	Status             string  `db:"status"              json:"status"`
	ReversalOfEntryID  *string `db:"reversalOfEntryId"  json:"reversalOfEntryId"`
	ReversalReason     *string `db:"reversalReason"     json:"reversalReason"`
	PostedAt           *int64  `db:"postedAt"           json:"postedAt"`
	CreatedBy          *string `db:"createdBy"          json:"createdBy"`
	CreatedAt          int64   `db:"createdAt"          json:"createdAt"`
}

// JournalLine mirrors the journal_lines table.
type JournalLine struct {
	ID                    string  `db:"id"                    json:"id"`
	JournalEntryID        string  `db:"journalEntryId"        json:"journalEntryId"`
	AccountID             string  `db:"accountId"             json:"accountId"`
	Description           *string `db:"description"           json:"description"`
	Debit                 int64   `db:"debit"                 json:"debit"`
	Credit                int64   `db:"credit"                json:"credit"`
	Currency              *string `db:"currency"              json:"currency"`
	ForeignAmount         *int64  `db:"foreignAmount"         json:"foreignAmount"`
	ExchangeRate          *string `db:"exchangeRate"          json:"exchangeRate"`
	ClientID              *string `db:"clientId"              json:"clientId"`
	VendorID              *string `db:"vendorId"              json:"vendorId"`
	TaxRateID             *string `db:"taxRateId"             json:"taxRateId"`
	ReconciliationGroupID *string `db:"reconciliationGroupId" json:"reconciliationGroupId"`
	Position              int     `db:"position"              json:"position"`
	CreatedAt             int64   `db:"createdAt"             json:"createdAt"`
}

// CreateJournalLineRequest is one line of a manual journal entry.
type CreateJournalLineRequest struct {
	AccountID     string  `json:"accountId"`
	Description   *string `json:"description"`
	Debit         int64   `json:"debit"`
	Credit        int64   `json:"credit"`
	Currency      *string `json:"currency"`
	ForeignAmount *int64  `json:"foreignAmount"`
	ExchangeRate  *string `json:"exchangeRate"`
	ClientID      *string `json:"clientId"`
	VendorID      *string `json:"vendorId"`
	TaxRateID     *string `json:"taxRateId"`
}

// CreateJournalEntryRequest is the payload for creating a manual journal
// entry — always starts as a draft (status='draft', no entryNumber). Posting
// is a separate step (PostJournalEntry).
type CreateJournalEntryRequest struct {
	ID                 string                     `json:"id"`
	OrganizationID     string                     `json:"organizationId"`
	JournalID          string                     `json:"journalId"`
	Date               int64                      `json:"date"`
	Reference          *string                    `json:"reference"`
	Description        string                     `json:"description"`
	SourceDocumentType *string                    `json:"sourceDocumentType"`
	SourceDocumentID   *string                    `json:"sourceDocumentId"`
	CreatedBy          *string                    `json:"createdBy"`
	Lines              []CreateJournalLineRequest `json:"lines"`
}

func (d *Database) GetJournalEntries(organizationID, journalID, status string) ([]JournalEntry, error) {
	where := "WHERE organizationId = ?"
	args := []any{organizationID}
	if journalID != "" {
		where += " AND journalId = ?"
		args = append(args, journalID)
	}
	if status != "" {
		where += " AND status = ?"
		args = append(args, status)
	}

	entries := []JournalEntry{}
	err := d.DB.Select(&entries,
		"SELECT * FROM journal_entries "+where+" ORDER BY date DESC, createdAt DESC",
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("get_journal_entries: %w", err)
	}
	return entries, nil
}

func (d *Database) GetJournalEntry(entryID string) (*JournalEntry, error) {
	var entry JournalEntry
	err := d.DB.Get(&entry, `SELECT * FROM journal_entries WHERE id = ? LIMIT 1`, entryID)
	if err != nil {
		return nil, fmt.Errorf("get_journal_entry: %w", err)
	}
	return &entry, nil
}

func (d *Database) GetJournalEntryLines(entryID string) ([]JournalLine, error) {
	lines := []JournalLine{}
	err := d.DB.Select(&lines,
		`SELECT * FROM journal_lines WHERE journalEntryId = ? ORDER BY position ASC`,
		entryID,
	)
	if err != nil {
		return nil, fmt.Errorf("get_journal_entry_lines: %w", err)
	}
	return lines, nil
}

// FindPostedEntryForSourceDocument returns the live posted entry for a given
// source document, or nil if none exists — the same idiom as looking up
// stockMovements rows by sourceDocumentId. Reversal entries are excluded via
// reversalOfEntryId IS NOT NULL even though they carry the same
// sourceDocumentType/sourceDocumentId for traceability: after a reversal,
// the original flips to status='reversed' and the reversal entry itself
// must not read back as "the current posted entry for this document", or
// idempotent posting (design decision #8) would wrongly skip re-posting the
// next time the document's state changes.
func (d *Database) FindPostedEntryForSourceDocument(sourceDocumentType, sourceDocumentID string) (*JournalEntry, error) {
	var entry JournalEntry
	err := d.DB.Get(&entry,
		`SELECT * FROM journal_entries
		 WHERE sourceDocumentType = ? AND sourceDocumentId = ? AND status = 'posted' AND reversalOfEntryId IS NULL
		 LIMIT 1`,
		sourceDocumentType, sourceDocumentID,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("find_posted_entry_for_source_document: %w", err)
	}
	return &entry, nil
}

func (d *Database) CreateJournalEntry(req CreateJournalEntryRequest) (*JournalEntry, error) {
	if len(req.Lines) == 0 {
		return nil, newValidationError("a journal entry needs at least one line")
	}
	for i, line := range req.Lines {
		if line.AccountID == "" {
			return nil, newValidationError("line %d: an account is required", i+1)
		}
		if line.Debit < 0 || line.Credit < 0 {
			return nil, newValidationError("line %d: amounts cannot be negative", i+1)
		}
		if (line.Debit == 0) == (line.Credit == 0) {
			return nil, newValidationError("line %d: exactly one of debit or credit must be set", i+1)
		}
	}
	if req.ID == "" {
		req.ID, _ = gonanoid.New()
	}

	tx, err := d.DB.Beginx()
	if err != nil {
		return nil, fmt.Errorf("create_journal_entry begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	fiscalYearID, fiscalPeriodID, err := resolveFiscalPeriodForDate(tx, req.OrganizationID, req.Date)
	if err != nil {
		return nil, err
	}

	if _, err := tx.Exec(
		`INSERT INTO journal_entries
		   (id, organizationId, journalId, fiscalYearId, fiscalPeriodId, date, reference, description, sourceDocumentType, sourceDocumentId, createdBy)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		req.ID, req.OrganizationID, req.JournalID, fiscalYearID, fiscalPeriodID, req.Date, req.Reference, req.Description,
		req.SourceDocumentType, req.SourceDocumentID, req.CreatedBy,
	); err != nil {
		return nil, fmt.Errorf("create_journal_entry insert: %w", err)
	}

	if err := insertJournalLinesTx(tx, req.ID, req.Lines); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("create_journal_entry commit: %w", err)
	}
	return d.GetJournalEntry(req.ID)
}

func insertJournalLinesTx(exec sqlExecer, journalEntryID string, lines []CreateJournalLineRequest) error {
	for i, line := range lines {
		id, err := gonanoid.New()
		if err != nil {
			return fmt.Errorf("insert_journal_lines new_id: %w", err)
		}
		if _, err := exec.Exec(
			`INSERT INTO journal_lines
			   (id, journalEntryId, accountId, description, debit, credit, currency, foreignAmount, exchangeRate, clientId, vendorId, taxRateId, position)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, journalEntryID, line.AccountID, line.Description, line.Debit, line.Credit,
			line.Currency, line.ForeignAmount, line.ExchangeRate, line.ClientID, line.VendorID, line.TaxRateID, i,
		); err != nil {
			return fmt.Errorf("insert_journal_lines insert: %w", err)
		}
	}
	return nil
}

// PostJournalEntry moves a draft entry to posted via allocateAndFinalizeEntryTx.
func (d *Database) PostJournalEntry(entryID string) (*JournalEntry, error) {
	tx, err := d.DB.Beginx()
	if err != nil {
		return nil, fmt.Errorf("post_journal_entry begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	var entry JournalEntry
	if err := tx.Get(&entry, `SELECT * FROM journal_entries WHERE id = ? LIMIT 1`, entryID); err != nil {
		return nil, fmt.Errorf("post_journal_entry lookup: %w", err)
	}
	if entry.Status != "draft" {
		return nil, newValidationError("only a draft journal entry can be posted")
	}

	if err := allocateAndFinalizeEntryTx(tx, entryID, entry.OrganizationID, entry.FiscalYearID, entry.JournalID); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("post_journal_entry commit: %w", err)
	}
	return d.GetJournalEntry(entryID)
}

// allocateAndFinalizeEntryTx is the ONLY place a journal entry becomes
// posted — every posting path (manual post, invoice/bill auto-post in
// Phase 2, payments in Phase 3, reversal, closing, revaluation) must funnel
// through it. Mirrors insertStockMovementTx being the sole place stock
// actually moves.
func allocateAndFinalizeEntryTx(tx *sqlx.Tx, entryID, organizationID, fiscalYearID, journalID string) error {
	// isGroup=1 accounts are headers only, never postable — this is the one
	// place that's enforced, not a DB constraint.
	var groupLineCount int64
	if err := tx.Get(&groupLineCount, `
		SELECT COUNT(*) FROM journal_lines jl
		JOIN accounts a ON a.id = jl.accountId
		WHERE jl.journalEntryId = ? AND a.isGroup = 1`,
		entryID,
	); err != nil {
		return fmt.Errorf("allocate_and_finalize_entry group_check: %w", err)
	}
	if groupLineCount > 0 {
		return newValidationError("cannot post to a group account — group accounts are headers only")
	}

	// The per-line CHECK (debit=0)<>(credit=0) only guarantees one side per
	// row, not that the header balances — that's asserted here.
	var sums struct {
		Debit  int64 `db:"debit"`
		Credit int64 `db:"credit"`
	}
	if err := tx.Get(&sums,
		`SELECT COALESCE(SUM(debit), 0) AS debit, COALESCE(SUM(credit), 0) AS credit
		 FROM journal_lines WHERE journalEntryId = ?`,
		entryID,
	); err != nil {
		return fmt.Errorf("allocate_and_finalize_entry sums: %w", err)
	}
	if sums.Debit == 0 && sums.Credit == 0 {
		return newValidationError("a journal entry needs at least one line")
	}
	if sums.Debit != sums.Credit {
		return newValidationError("journal entry is not balanced: debit %d does not equal credit %d", sums.Debit, sums.Credit)
	}

	// CRITICAL: scoped to entryNumber IS NOT NULL, not status = 'posted' — a
	// reversed entry keeps its entryNumber (status flips to 'reversed', the
	// row is never deleted), so scoping to status='posted' would re-issue an
	// already-used number the moment anything is reversed and violate the
	// unique index on (organizationId, fiscalYearId, journalId, entryNumber).
	// Race-free only because db.SetMaxOpenConns(1) serializes every write
	// through one connection — the same reasoning that already backs
	// NextInboundDeliveryNumber's MAX+1 scheme.
	var entryNumber int64
	if err := tx.Get(&entryNumber,
		`SELECT COALESCE(MAX(entryNumber), 0) + 1 FROM journal_entries
		 WHERE organizationId = ? AND fiscalYearId = ? AND journalId = ? AND entryNumber IS NOT NULL`,
		organizationID, fiscalYearID, journalID,
	); err != nil {
		return fmt.Errorf("allocate_and_finalize_entry entry_number: %w", err)
	}

	if _, err := tx.Exec(
		`UPDATE journal_entries SET entryNumber = ?, status = 'posted', postedAt = ? WHERE id = ?`,
		entryNumber, time.Now().UnixMilli(), entryID,
	); err != nil {
		return fmt.Errorf("allocate_and_finalize_entry update: %w", err)
	}
	return nil
}

// DeleteJournalEntry only allows deleting a draft — mirrors "cannot delete a
// paid invoice, cancel it instead". A posted entry must be reversed.
func (d *Database) DeleteJournalEntry(entryID string) (bool, error) {
	entry, err := d.GetJournalEntry(entryID)
	if err != nil {
		return false, err
	}
	if entry.Status != "draft" {
		return false, newValidationError("only a draft journal entry can be deleted — reverse a posted one instead")
	}

	res, err := d.DB.Exec(`DELETE FROM journal_entries WHERE id = ?`, entryID)
	if err != nil {
		return false, fmt.Errorf("delete_journal_entry: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// ReverseJournalEntry posts a new entry with every line's debit/credit
// flipped against the original, links it back via reversalOfEntryId, and
// flips the original to status='reversed'. Posted entries are never
// updated or deleted — this is the only way to undo one, and it's what
// makes FEC's gapless entryNumber requirement hold (the original's number
// is never freed up for reuse).
func (d *Database) ReverseJournalEntry(entryID, reason string, reversalDate int64) (*JournalEntry, error) {
	if reason == "" {
		return nil, newValidationError("a reversal reason is required")
	}

	tx, err := d.DB.Beginx()
	if err != nil {
		return nil, fmt.Errorf("reverse_journal_entry begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	var original JournalEntry
	if err := tx.Get(&original, `SELECT * FROM journal_entries WHERE id = ? LIMIT 1`, entryID); err != nil {
		return nil, fmt.Errorf("reverse_journal_entry lookup: %w", err)
	}
	if original.Status != "posted" {
		return nil, newValidationError("only a posted journal entry can be reversed")
	}

	reversalID, err := reverseEntryTx(tx, &original, reason, reversalDate)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("reverse_journal_entry commit: %w", err)
	}
	return d.GetJournalEntry(reversalID)
}
