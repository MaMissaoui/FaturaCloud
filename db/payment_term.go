package db

import (
	"fmt"
	"strings"

	gonanoid "github.com/matoous/go-nanoid/v2"
)

// PaymentTerm mirrors the payment_terms table — a maintained, per-organization
// list of selectable labels (e.g. "Net 30", "Due on receipt") for the
// invoice form's Payment terms field, replacing what used to be a free-typed
// string. There's no FK from invoices.paymentTerms to this table (see the
// migration 0070 comment), so unlike taxRates there's no usage-count guard
// on delete — removing a term never touches an invoice that already stored
// its name.
type PaymentTerm struct {
	ID             string `db:"id"             json:"id"`
	OrganizationID string `db:"organizationId" json:"organizationId"`
	Name           string `db:"name"           json:"name"`
	IsDefault      *int64 `db:"isDefault"      json:"isDefault"`
	CreatedAt      string `db:"createdAt"      json:"createdAt"`
}

// CreatePaymentTermRequest is the payload for creating a payment term.
type CreatePaymentTermRequest struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organizationId"`
	Name           string `json:"name"`
	IsDefault      *int64 `json:"isDefault"`
}

// UpdatePaymentTermRequest is the payload for updating a payment term.
type UpdatePaymentTermRequest struct {
	Name      *string `json:"name"`
	IsDefault *int64  `json:"isDefault"`
}

func (d *Database) GetPaymentTerms(organizationID string) ([]PaymentTerm, error) {
	terms := []PaymentTerm{}
	err := d.DB.Select(&terms,
		`SELECT * FROM payment_terms WHERE organizationId = ? ORDER BY name ASC`,
		organizationID,
	)
	if err != nil {
		return nil, fmt.Errorf("get_payment_terms: %w", err)
	}
	return terms, nil
}

func (d *Database) GetPaymentTerm(id string) (*PaymentTerm, error) {
	var term PaymentTerm
	err := d.DB.Get(&term, `SELECT * FROM payment_terms WHERE id = ? LIMIT 1`, id)
	if err != nil {
		return nil, fmt.Errorf("get_payment_term: %w", err)
	}
	return &term, nil
}

func (d *Database) CreatePaymentTerm(req CreatePaymentTermRequest) (*PaymentTerm, error) {
	if req.ID == "" {
		req.ID, _ = gonanoid.New()
	}
	if strings.TrimSpace(req.Name) == "" {
		return nil, newValidationError("name is required")
	}

	tx, err := d.DB.Beginx()
	if err != nil {
		return nil, fmt.Errorf("create_payment_term begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	isOne := req.IsDefault != nil && *req.IsDefault == 1
	if isOne {
		if _, err = tx.Exec(
			`UPDATE payment_terms SET isDefault = 0 WHERE organizationId = ? AND isDefault = 1`,
			req.OrganizationID,
		); err != nil {
			return nil, fmt.Errorf("create_payment_term unset_default: %w", err)
		}
	}

	if _, err = tx.Exec(
		`INSERT INTO payment_terms (id, organizationId, name, isDefault) VALUES (?, ?, ?, ?)`,
		req.ID, req.OrganizationID, req.Name, req.IsDefault,
	); err != nil {
		if isDuplicatePaymentTermName(err) {
			return nil, newValidationError("a payment term named %q already exists", req.Name)
		}
		return nil, fmt.Errorf("create_payment_term insert: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("create_payment_term commit: %w", err)
	}
	return d.GetPaymentTerm(req.ID)
}

func (d *Database) UpdatePaymentTerm(id string, updates UpdatePaymentTermRequest) (*PaymentTerm, error) {
	if updates.Name != nil && strings.TrimSpace(*updates.Name) == "" {
		return nil, newValidationError("name is required")
	}

	tx, err := d.DB.Beginx()
	if err != nil {
		return nil, fmt.Errorf("update_payment_term begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	isOne := updates.IsDefault != nil && *updates.IsDefault == 1
	if isOne {
		var existing PaymentTerm
		if err = tx.Get(&existing, `SELECT * FROM payment_terms WHERE id = ? LIMIT 1`, id); err != nil {
			return nil, fmt.Errorf("update_payment_term fetch_existing: %w", err)
		}
		if _, err = tx.Exec(
			`UPDATE payment_terms SET isDefault = 0 WHERE organizationId = ? AND id != ? AND isDefault = 1`,
			existing.OrganizationID, id,
		); err != nil {
			return nil, fmt.Errorf("update_payment_term unset_default: %w", err)
		}
	}

	if _, err = tx.Exec(`
		UPDATE payment_terms
		SET name      = COALESCE(?, name),
		    isDefault = COALESCE(?, isDefault)
		WHERE id = ?`,
		updates.Name, updates.IsDefault, id,
	); err != nil {
		if isDuplicatePaymentTermName(err) {
			return nil, newValidationError("a payment term named %q already exists", *updates.Name)
		}
		return nil, fmt.Errorf("update_payment_term exec: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("update_payment_term commit: %w", err)
	}
	return d.GetPaymentTerm(id)
}

// isDuplicatePaymentTermName recognizes the raw SQLite unique-index
// violation on (organizationId, name). Mirrors isDuplicateJournalCode
// (db/journal.go).
func isDuplicatePaymentTermName(err error) bool {
	return strings.Contains(err.Error(), "UNIQUE constraint failed") &&
		strings.Contains(err.Error(), "payment_terms")
}

func (d *Database) DeletePaymentTerm(id string) (bool, error) {
	res, err := d.DB.Exec(`DELETE FROM payment_terms WHERE id = ?`, id)
	if err != nil {
		return false, fmt.Errorf("delete_payment_term: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// defaultPaymentTerms is the starter set every organization gets on
// creation, editable/removable afterward like any other row — "Net 30" is
// marked default so a new invoice prefills it the same way taxRates.isDefault
// prefills a new line item's tax rate.
var defaultPaymentTerms = []struct {
	name      string
	isDefault bool
}{
	{name: "Due on receipt"},
	{name: "Net 15"},
	{name: "Net 30", isDefault: true},
	{name: "Net 60"},
}

// SeedDefaultPaymentTermsForAllOrganizations backfills the starter set for
// any organization with zero rows — covers both a pre-migration organization
// and one seeded before this feature existed. Idempotent every startup, same
// shape as SeedAccountingDefaultsForAllOrganizations (db/account.go).
func (d *Database) SeedDefaultPaymentTermsForAllOrganizations() error {
	var orgIDs []string
	if err := d.DB.Select(&orgIDs, `SELECT id FROM organizations`); err != nil {
		return fmt.Errorf("seed_default_payment_terms_for_all_organizations list_organizations: %w", err)
	}

	for _, orgID := range orgIDs {
		var count int
		if err := d.DB.Get(&count, `SELECT COUNT(*) FROM payment_terms WHERE organizationId = ?`, orgID); err != nil {
			return fmt.Errorf("seed_default_payment_terms_for_all_organizations count %s: %w", orgID, err)
		}
		if count > 0 {
			continue
		}
		if err := seedDefaultPaymentTerms(d.DB, orgID); err != nil {
			return fmt.Errorf("seed_default_payment_terms_for_all_organizations seed %s: %w", orgID, err)
		}
	}
	return nil
}

func seedDefaultPaymentTerms(exec sqlExecer, organizationID string) error {
	for _, term := range defaultPaymentTerms {
		id, err := gonanoid.New()
		if err != nil {
			return fmt.Errorf("seed_default_payment_terms new_id: %w", err)
		}
		var isDefault int64
		if term.isDefault {
			isDefault = 1
		}
		if _, err := exec.Exec(
			`INSERT INTO payment_terms (id, organizationId, name, isDefault) VALUES (?, ?, ?, ?)`,
			id, organizationID, term.name, isDefault,
		); err != nil {
			return fmt.Errorf("seed_default_payment_terms insert %s: %w", term.name, err)
		}
	}
	return nil
}
