package db

import (
	"errors"
	"fmt"
	"strings"

	gonanoid "github.com/matoous/go-nanoid/v2"
)

// ErrJournalInUse is returned by DeleteJournal when journal entries still
// reference it.
var ErrJournalInUse = errors.New("journal is still used by one or more journal entries")

var journalTypes = map[string]bool{
	"sales": true, "purchases": true, "cash": true, "bank": true, "miscellaneous": true,
}

// Journal mirrors the journals table — sales/purchases/cash/bank/
// miscellaneous, a distinct concept from an account (maps directly onto
// DATEV/FEC's JournalCode/JournalLib).
type Journal struct {
	ID             string `db:"id"             json:"id"`
	OrganizationID string `db:"organizationId" json:"organizationId"`
	Code           string `db:"code"           json:"code"`
	Name           string `db:"name"           json:"name"`
	Type           string `db:"type"           json:"type"`
	IsSystem       int    `db:"isSystem"       json:"isSystem"`
	CreatedAt      int64  `db:"createdAt"      json:"createdAt"`
}

// CreateJournalRequest is the payload for creating a journal.
type CreateJournalRequest struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organizationId"`
	Code           string `json:"code"`
	Name           string `json:"name"`
	Type           string `json:"type"`
}

// UpdateJournalRequest is the payload for updating a journal. Code and Type
// are immutable after creation — DATEV/FEC exports and already-posted
// entries key on them, so silently changing either after entries exist would
// disagree with history rather than describe it going forward.
type UpdateJournalRequest struct {
	Name string `json:"name"`
}

func (d *Database) GetJournals(organizationID string) ([]Journal, error) {
	journals := []Journal{}
	err := d.DB.Select(&journals,
		`SELECT * FROM journals WHERE organizationId = ? ORDER BY code ASC`,
		organizationID,
	)
	if err != nil {
		return nil, fmt.Errorf("get_journals: %w", err)
	}
	return journals, nil
}

func (d *Database) GetJournal(journalID string) (*Journal, error) {
	var journal Journal
	err := d.DB.Get(&journal, `SELECT * FROM journals WHERE id = ? LIMIT 1`, journalID)
	if err != nil {
		return nil, fmt.Errorf("get_journal: %w", err)
	}
	return &journal, nil
}

func (d *Database) CreateJournal(req CreateJournalRequest) (*Journal, error) {
	if !journalTypes[req.Type] {
		return nil, newValidationError("invalid journal type %q", req.Type)
	}
	if req.ID == "" {
		req.ID, _ = gonanoid.New()
	}

	_, err := d.DB.Exec(
		`INSERT INTO journals (id, organizationId, code, name, type) VALUES (?, ?, ?, ?, ?)`,
		req.ID, req.OrganizationID, req.Code, req.Name, req.Type,
	)
	if err != nil {
		if isDuplicateJournalCode(err) {
			return nil, newValidationError("a journal with code %q already exists", req.Code)
		}
		return nil, fmt.Errorf("create_journal: %w", err)
	}
	return d.GetJournal(req.ID)
}

func (d *Database) UpdateJournal(journalID string, updates UpdateJournalRequest) (*Journal, error) {
	_, err := d.DB.Exec(`UPDATE journals SET name = ? WHERE id = ?`, updates.Name, journalID)
	if err != nil {
		return nil, fmt.Errorf("update_journal: %w", err)
	}
	return d.GetJournal(journalID)
}

// isDuplicateJournalCode recognizes the raw SQLite unique-index violation on
// (organizationId, code). Mirrors isDuplicateSKU (db/product.go).
func isDuplicateJournalCode(err error) bool {
	return strings.Contains(err.Error(), "UNIQUE constraint failed") && strings.Contains(err.Error(), "journals.code")
}

func (d *Database) GetJournalUsageCount(journalID string) (int64, error) {
	var count int64
	if err := d.DB.Get(&count, `SELECT COUNT(*) FROM journal_entries WHERE journalId = ?`, journalID); err != nil {
		return 0, fmt.Errorf("get_journal_usage_count: %w", err)
	}
	return count, nil
}

// DeleteJournal refuses to delete a journal still referenced by journal
// entries, or a seeded system journal auto-posting depends on existing.
func (d *Database) DeleteJournal(journalID string) (bool, error) {
	journal, err := d.GetJournal(journalID)
	if err != nil {
		return false, err
	}
	if journal.IsSystem == 1 {
		return false, newValidationError("%q is a built-in journal and cannot be deleted", journal.Name)
	}

	count, err := d.GetJournalUsageCount(journalID)
	if err != nil {
		return false, err
	}
	if count > 0 {
		return false, ErrJournalInUse
	}

	res, err := d.DB.Exec(`DELETE FROM journals WHERE id = ?`, journalID)
	if err != nil {
		return false, fmt.Errorf("delete_journal: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// defaultJournal is one row of the default journal set every organization
// gets on creation (seedDefaultJournals) — the minimum auto-posting
// (Phase 2) and payments (Phase 3) need to exist before either can post.
type defaultJournal struct {
	code, name, journalType string
}

var defaultJournals = []defaultJournal{
	{code: "VK", name: "Sales", journalType: "sales"},
	{code: "EK", name: "Purchases", journalType: "purchases"},
	{code: "BK", name: "Bank", journalType: "bank"},
	{code: "KA", name: "Cash", journalType: "cash"},
	{code: "OD", name: "Miscellaneous", journalType: "miscellaneous"},
}

// seedDefaultJournals inserts the default journal set for organizationID,
// marked isSystem=1 so DeleteJournal refuses to remove them. Runs on the
// given exec so callers can fold it into their own transaction
// (CreateOrganization) or run it standalone (the startup backfill in main.go).
func seedDefaultJournals(exec sqlExecer, organizationID string) error {
	for _, j := range defaultJournals {
		id, err := gonanoid.New()
		if err != nil {
			return fmt.Errorf("seed_default_journals new_id: %w", err)
		}
		if _, err := exec.Exec(
			`INSERT INTO journals (id, organizationId, code, name, type, isSystem) VALUES (?, ?, ?, ?, ?, 1)`,
			id, organizationID, j.code, j.name, j.journalType,
		); err != nil {
			return fmt.Errorf("seed_default_journals insert %s: %w", j.code, err)
		}
	}
	return nil
}
