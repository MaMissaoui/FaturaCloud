package db

import "fmt"

// TaxRate mirrors the taxRates table.
type TaxRate struct {
	ID             string   `db:"id"             json:"id"`
	OrganizationID string   `db:"organizationId" json:"organizationId"`
	Name           string   `db:"name"           json:"name"`
	Description    *string  `db:"description"    json:"description"`
	Percentage     float64  `db:"percentage"     json:"percentage"`
	IsDefault      *int64   `db:"isDefault"      json:"isDefault"`
}

// CreateTaxRateRequest is the payload for creating a tax rate.
type CreateTaxRateRequest struct {
	ID             string   `json:"id"`
	OrganizationID string   `json:"organizationId"`
	Name           string   `json:"name"`
	Description    *string  `json:"description"`
	Percentage     float64  `json:"percentage"`
	IsDefault      *int64   `json:"isDefault"`
}

// UpdateTaxRateRequest is the payload for updating a tax rate.
type UpdateTaxRateRequest struct {
	Name        *string  `json:"name"`
	Description *string  `json:"description"`
	Percentage  *float64 `json:"percentage"`
	IsDefault   *int64   `json:"isDefault"`
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
		`INSERT INTO taxRates (id, organizationId, name, description, percentage, isDefault)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		req.ID, req.OrganizationID, req.Name, req.Description, req.Percentage, req.IsDefault,
	); err != nil {
		return nil, fmt.Errorf("create_tax_rate insert: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("create_tax_rate commit: %w", err)
	}
	return d.GetTaxRate(req.ID)
}

func (d *Database) UpdateTaxRate(taxRateID string, updates UpdateTaxRateRequest) (*TaxRate, error) {
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
		SET name        = COALESCE(?, name),
		    description = COALESCE(?, description),
		    percentage  = COALESCE(?, percentage),
		    isDefault   = COALESCE(?, isDefault)
		WHERE id = ?`,
		updates.Name, updates.Description, updates.Percentage, updates.IsDefault, taxRateID,
	); err != nil {
		return nil, fmt.Errorf("update_tax_rate exec: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("update_tax_rate commit: %w", err)
	}
	return d.GetTaxRate(taxRateID)
}

func (d *Database) DeleteTaxRate(taxRateID string) (bool, error) {
	res, err := d.DB.Exec(`DELETE FROM taxRates WHERE id = ?`, taxRateID)
	if err != nil {
		return false, fmt.Errorf("delete_tax_rate: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}
