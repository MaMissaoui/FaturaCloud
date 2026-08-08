package db

import (
	"errors"
	"fmt"
	"strings"
)

// ErrTaxRateInUse is returned by DeleteTaxRate when the rate is still
// referenced by invoice line items. invoiceLineItems.taxRate has an
// ON DELETE CASCADE foreign key, so deleting an in-use rate would silently
// strip line items (and corrupt totals) off existing invoices instead of
// just removing an unused lookup value.
var ErrTaxRateInUse = errors.New("tax rate is still used by one or more invoices")

// taxRateCategoryCodes is the UNTDID 5305 subset EN 16931 (XRechnung) allows
// for BT-118. Not the full UNTDID 5305 list — only the codes the EN 16931
// validation rules (BR-CO-*) actually accept.
var taxRateCategoryCodes = map[string]bool{
	"S":  true, // Standard rate
	"Z":  true, // Zero rated goods
	"E":  true, // Exempt from tax
	"AE": true, // VAT Reverse Charge
	"K":  true, // VAT exempt for EEA intra-community supply
	"G":  true, // Free export item, tax not charged
	"O":  true, // Services outside scope of tax
	"L":  true, // Canary Islands general indirect tax
	"M":  true, // Tax for production, services and importation in Ceuta and Melilla
}

// TaxRate mirrors the taxRates table.
type TaxRate struct {
	ID             string  `db:"id"             json:"id"`
	OrganizationID string  `db:"organizationId" json:"organizationId"`
	Name           string  `db:"name"           json:"name"`
	Description    *string `db:"description"    json:"description"`
	Percentage     float64 `db:"percentage"     json:"percentage"`
	IsDefault      *int64  `db:"isDefault"      json:"isDefault"`

	// BT-118 VAT category code (UNTDID 5305: S/Z/E/AE/...), defaults to "S"
	// (standard rate). BT-120 exemption reason, required by EN 16931 when
	// the category needs one (e.g. E, Z, O).
	CategoryCode    string  `db:"category_code"    json:"category_code"`
	ExemptionReason *string `db:"exemption_reason" json:"exemption_reason"`

	// Two accounts, not one: this same rate is referenced by both sales and
	// purchase line items (taxRateReferencingTables) — output VAT is a
	// liability owed to the state, input VAT is a reclaimable asset.
	OutputTaxAccountID *string `db:"outputTaxAccountId" json:"outputTaxAccountId"`
	InputTaxAccountID  *string `db:"inputTaxAccountId"  json:"inputTaxAccountId"`
	// DATEV BU-Schlüssel (tax key), independent of the percentage.
	DatevBuKey *string `db:"datev_bu_key" json:"datev_bu_key"`
}

// CreateTaxRateRequest is the payload for creating a tax rate.
type CreateTaxRateRequest struct {
	ID             string  `json:"id"`
	OrganizationID string  `json:"organizationId"`
	Name           string  `json:"name"`
	Description    *string `json:"description"`
	Percentage     float64 `json:"percentage"`
	IsDefault      *int64  `json:"isDefault"`

	CategoryCode    string  `json:"category_code"`
	ExemptionReason *string `json:"exemption_reason"`

	OutputTaxAccountID *string `json:"outputTaxAccountId"`
	InputTaxAccountID  *string `json:"inputTaxAccountId"`
	DatevBuKey         *string `json:"datev_bu_key"`
}

// UpdateTaxRateRequest is the payload for updating a tax rate.
type UpdateTaxRateRequest struct {
	Name        *string  `json:"name"`
	Description *string  `json:"description"`
	Percentage  *float64 `json:"percentage"`
	IsDefault   *int64   `json:"isDefault"`

	CategoryCode    *string `json:"category_code"`
	ExemptionReason *string `json:"exemption_reason"`

	OutputTaxAccountID *string `json:"outputTaxAccountId"`
	InputTaxAccountID  *string `json:"inputTaxAccountId"`
	DatevBuKey         *string `json:"datev_bu_key"`
}

func (d *Database) GetTaxRates(organizationID string) ([]TaxRate, error) {
	rates := []TaxRate{}
	err := d.DB.Select(&rates,
		`SELECT * FROM taxRates WHERE organizationId = ? ORDER BY name ASC`,
		organizationID,
	)
	if err != nil {
		return nil, fmt.Errorf("get_tax_rates: %w", err)
	}
	return rates, nil
}

func (d *Database) GetTaxRate(taxRateID string) (*TaxRate, error) {
	var rate TaxRate
	err := d.DB.Get(&rate,
		`SELECT * FROM taxRates WHERE id = ? LIMIT 1`,
		taxRateID,
	)
	if err != nil {
		return nil, fmt.Errorf("get_tax_rate: %w", err)
	}
	return &rate, nil
}

func (d *Database) CreateTaxRate(req CreateTaxRateRequest) (*TaxRate, error) {
	if req.CategoryCode == "" {
		req.CategoryCode = "S"
	}
	if !taxRateCategoryCodes[req.CategoryCode] {
		return nil, newValidationError("invalid tax rate category code %q", req.CategoryCode)
	}

	tx, err := d.DB.Beginx()
	if err != nil {
		return nil, fmt.Errorf("create_tax_rate begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	isOne := req.IsDefault != nil && *req.IsDefault == 1
	if isOne {
		if _, err = tx.Exec(
			`UPDATE taxRates SET isDefault = 0 WHERE organizationId = ? AND isDefault = 1`,
			req.OrganizationID,
		); err != nil {
			return nil, fmt.Errorf("create_tax_rate unset_default: %w", err)
		}
	}

	if _, err = tx.Exec(
		`INSERT INTO taxRates (id, organizationId, name, description, percentage, isDefault, category_code, exemption_reason, outputTaxAccountId, inputTaxAccountId, datev_bu_key)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		req.ID, req.OrganizationID, req.Name, req.Description, req.Percentage, req.IsDefault,
		req.CategoryCode, req.ExemptionReason, req.OutputTaxAccountID, req.InputTaxAccountID, req.DatevBuKey,
	); err != nil {
		return nil, fmt.Errorf("create_tax_rate insert: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("create_tax_rate commit: %w", err)
	}
	return d.GetTaxRate(req.ID)
}

func (d *Database) UpdateTaxRate(taxRateID string, updates UpdateTaxRateRequest) (*TaxRate, error) {
	if updates.CategoryCode != nil && !taxRateCategoryCodes[*updates.CategoryCode] {
		return nil, newValidationError("invalid tax rate category code %q", *updates.CategoryCode)
	}

	tx, err := d.DB.Beginx()
	if err != nil {
		return nil, fmt.Errorf("update_tax_rate begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	isOne := updates.IsDefault != nil && *updates.IsDefault == 1
	if isOne {
		var existing TaxRate
		if err = tx.Get(&existing, `SELECT * FROM taxRates WHERE id = ? LIMIT 1`, taxRateID); err != nil {
			return nil, fmt.Errorf("update_tax_rate fetch_existing: %w", err)
		}
		if _, err = tx.Exec(
			`UPDATE taxRates SET isDefault = 0 WHERE organizationId = ? AND id != ? AND isDefault = 1`,
			existing.OrganizationID, taxRateID,
		); err != nil {
			return nil, fmt.Errorf("update_tax_rate unset_default: %w", err)
		}
	}

	if _, err = tx.Exec(`
		UPDATE taxRates
		SET name                = COALESCE(?, name),
		    description         = COALESCE(?, description),
		    percentage          = COALESCE(?, percentage),
		    isDefault           = COALESCE(?, isDefault),
		    category_code       = COALESCE(?, category_code),
		    exemption_reason    = ?,
		    outputTaxAccountId  = COALESCE(?, outputTaxAccountId),
		    inputTaxAccountId   = COALESCE(?, inputTaxAccountId),
		    datev_bu_key        = COALESCE(?, datev_bu_key)
		WHERE id = ?`,
		updates.Name, updates.Description, updates.Percentage, updates.IsDefault,
		updates.CategoryCode, updates.ExemptionReason,
		updates.OutputTaxAccountID, updates.InputTaxAccountID, updates.DatevBuKey,
		taxRateID,
	); err != nil {
		return nil, fmt.Errorf("update_tax_rate exec: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("update_tax_rate commit: %w", err)
	}
	return d.GetTaxRate(taxRateID)
}

// GetTaxRateUsageCount returns how many invoice line items reference this
// tax rate, so callers can decide whether it's safe to delete.
// Every line-item table that references taxRates must be counted here.
// invoiceLineItems and incoming_invoice_line_items cascade on delete, so a rate
// missing from this count could be deleted and silently strip line items off
// existing invoices; purchase_order_line_items merely null out, but a rate
// vanishing from a live order is just as unwanted.
// TestTaxRateUsageCountCoversEveryReference reads the live schema and fails if
// a table gains a taxRate column without being listed.
var taxRateReferencingTables = []string{
	"invoiceLineItems",
	"incoming_invoice_line_items",
	"purchase_order_line_items",
}

func (d *Database) GetTaxRateUsageCount(taxRateID string) (int64, error) {
	subqueries := make([]string, len(taxRateReferencingTables))
	args := make([]any, len(taxRateReferencingTables))
	for i, table := range taxRateReferencingTables {
		// Table names come from the package-level slice above, never from input.
		subqueries[i] = fmt.Sprintf("(SELECT COUNT(*) FROM %s WHERE taxRate = ?)", table)
		args[i] = taxRateID
	}

	var count int64
	if err := d.DB.Get(&count, "SELECT "+strings.Join(subqueries, " + "), args...); err != nil {
		return 0, fmt.Errorf("get_tax_rate_usage_count: %w", err)
	}
	return count, nil
}

func (d *Database) DeleteTaxRate(taxRateID string) (bool, error) {
	count, err := d.GetTaxRateUsageCount(taxRateID)
	if err != nil {
		return false, err
	}
	if count > 0 {
		return false, ErrTaxRateInUse
	}

	res, err := d.DB.Exec(`DELETE FROM taxRates WHERE id = ?`, taxRateID)
	if err != nil {
		return false, fmt.Errorf("delete_tax_rate: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}
