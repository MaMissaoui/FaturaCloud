package db

import (
	"fmt"
	"strings"

	gonanoid "github.com/matoous/go-nanoid/v2"
)

// UnitOfMeasure mirrors the units_of_measure table — a maintained,
// per-organization list of selectable base units (e.g. "kg", "piece",
// "hour") for the product form's Unit field, replacing what used to be a
// frontend-only suggestion list (src/utils/units.ts's UNIT_OPTIONS) with
// real, selectable data. Same shape as PaymentTerm, but products.unitOfMeasureId
// is ON DELETE SET NULL rather than CASCADE-plus-usage-guard like taxRates —
// see migration 0076's comment for why (deleting a unit of measure must
// never delete a product).
type UnitOfMeasure struct {
	ID             string `db:"id"             json:"id"`
	OrganizationID string `db:"organizationId" json:"organizationId"`
	Name           string `db:"name"           json:"name"`
	IsDefault      *int64 `db:"isDefault"      json:"isDefault"`
	CreatedAt      string `db:"createdAt"      json:"createdAt"`
}

type CreateUnitOfMeasureRequest struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organizationId"`
	Name           string `json:"name"`
	IsDefault      *int64 `json:"isDefault"`
}

type UpdateUnitOfMeasureRequest struct {
	Name      *string `json:"name"`
	IsDefault *int64  `json:"isDefault"`
}

func (d *Database) GetUnitsOfMeasure(organizationID string) ([]UnitOfMeasure, error) {
	units := []UnitOfMeasure{}
	err := d.DB.Select(&units,
		`SELECT * FROM units_of_measure WHERE organizationId = ? ORDER BY name ASC`,
		organizationID,
	)
	if err != nil {
		return nil, fmt.Errorf("get_units_of_measure: %w", err)
	}
	return units, nil
}

func (d *Database) GetUnitOfMeasure(id string) (*UnitOfMeasure, error) {
	var unit UnitOfMeasure
	err := d.DB.Get(&unit, `SELECT * FROM units_of_measure WHERE id = ? LIMIT 1`, id)
	if err != nil {
		return nil, fmt.Errorf("get_unit_of_measure: %w", err)
	}
	return &unit, nil
}

func (d *Database) CreateUnitOfMeasure(req CreateUnitOfMeasureRequest) (*UnitOfMeasure, error) {
	if req.ID == "" {
		req.ID, _ = gonanoid.New()
	}
	if strings.TrimSpace(req.Name) == "" {
		return nil, newValidationError("name is required")
	}
	// isDefault is NOT NULL at the schema level — a caller that omits it
	// entirely (a minimal request body, or a direct API call) must not hit a
	// raw constraint violation; "not sent" means "not the default", same as
	// an explicit 0.
	if req.IsDefault == nil {
		var zero int64
		req.IsDefault = &zero
	}

	tx, err := d.DB.Beginx()
	if err != nil {
		return nil, fmt.Errorf("create_unit_of_measure begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	isOne := req.IsDefault != nil && *req.IsDefault == 1
	if isOne {
		if _, err = tx.Exec(
			`UPDATE units_of_measure SET isDefault = 0 WHERE organizationId = ? AND isDefault = 1`,
			req.OrganizationID,
		); err != nil {
			return nil, fmt.Errorf("create_unit_of_measure unset_default: %w", err)
		}
	}

	if _, err = tx.Exec(
		`INSERT INTO units_of_measure (id, organizationId, name, isDefault) VALUES (?, ?, ?, ?)`,
		req.ID, req.OrganizationID, req.Name, req.IsDefault,
	); err != nil {
		if isDuplicateUnitOfMeasureName(err) {
			return nil, newValidationError("a unit of measure named %q already exists", req.Name)
		}
		return nil, fmt.Errorf("create_unit_of_measure insert: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("create_unit_of_measure commit: %w", err)
	}
	return d.GetUnitOfMeasure(req.ID)
}

func (d *Database) UpdateUnitOfMeasure(id string, updates UpdateUnitOfMeasureRequest) (*UnitOfMeasure, error) {
	if updates.Name != nil && strings.TrimSpace(*updates.Name) == "" {
		return nil, newValidationError("name is required")
	}

	tx, err := d.DB.Beginx()
	if err != nil {
		return nil, fmt.Errorf("update_unit_of_measure begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	isOne := updates.IsDefault != nil && *updates.IsDefault == 1
	if isOne {
		var existing UnitOfMeasure
		if err = tx.Get(&existing, `SELECT * FROM units_of_measure WHERE id = ? LIMIT 1`, id); err != nil {
			return nil, fmt.Errorf("update_unit_of_measure fetch_existing: %w", err)
		}
		if _, err = tx.Exec(
			`UPDATE units_of_measure SET isDefault = 0 WHERE organizationId = ? AND id != ? AND isDefault = 1`,
			existing.OrganizationID, id,
		); err != nil {
			return nil, fmt.Errorf("update_unit_of_measure unset_default: %w", err)
		}
	}

	if _, err = tx.Exec(`
		UPDATE units_of_measure
		SET name      = COALESCE(?, name),
		    isDefault = COALESCE(?, isDefault)
		WHERE id = ?`,
		updates.Name, updates.IsDefault, id,
	); err != nil {
		if isDuplicateUnitOfMeasureName(err) {
			return nil, newValidationError("a unit of measure named %q already exists", *updates.Name)
		}
		return nil, fmt.Errorf("update_unit_of_measure exec: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("update_unit_of_measure commit: %w", err)
	}
	return d.GetUnitOfMeasure(id)
}

// isDuplicateUnitOfMeasureName recognizes the raw SQLite unique-index
// violation on (organizationId, name). Mirrors isDuplicatePaymentTermName.
func isDuplicateUnitOfMeasureName(err error) bool {
	return strings.Contains(err.Error(), "UNIQUE constraint failed") &&
		strings.Contains(err.Error(), "units_of_measure")
}

// DeleteUnitOfMeasure is unconditional, like DeletePaymentTerm — safe
// because products.unitOfMeasureId is ON DELETE SET NULL, not CASCADE (see
// migration 0076), so deleting an in-use unit of measure only clears the
// structured link on any product that had it selected. Those products keep
// their last-resolved legacy `unit` text (set by resolveProductUnit) as a
// display fallback.
func (d *Database) DeleteUnitOfMeasure(id string) (bool, error) {
	res, err := d.DB.Exec(`DELETE FROM units_of_measure WHERE id = ?`, id)
	if err != nil {
		return false, fmt.Errorf("delete_unit_of_measure: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// defaultUnitsOfMeasure is the starter set every organization gets on
// creation — the exact same 13 codes src/utils/units.ts's now-removed
// UNIT_OPTIONS offered, so an organization's out-of-the-box options (and
// unitLabel's existing translations for the five spelled-out ones) don't
// change from what the frontend already showed. "piece" is marked default
// so a new product prefills it the same way taxRates.isDefault prefills a
// new invoice line's tax rate.
var defaultUnitsOfMeasure = []struct {
	name      string
	isDefault bool
}{
	{name: "hour"}, {name: "day"}, {name: "week"}, {name: "month"},
	{name: "piece", isDefault: true},
	{name: "kg"}, {name: "g"}, {name: "lb"}, {name: "oz"},
	{name: "l"}, {name: "ml"}, {name: "m"}, {name: "km"},
}

// SeedDefaultUnitsOfMeasureForAllOrganizations backfills the starter set for
// any organization with zero rows — covers both a pre-migration organization
// and one seeded before this feature existed. Idempotent every startup, same
// shape as SeedDefaultPaymentTermsForAllOrganizations.
//
// For an organization that's actually being backfilled (not one that
// already has units of measure), it also does a one-time, best-effort link:
// any existing product whose free-text `unit` column case-insensitively
// matches one of the seeded names gets unitOfMeasureId set to that row,
// leaving it NULL rather than guessing on anything that doesn't match. This
// runs only once per organization, in the same pass as the seed itself —
// products saved afterward go through resolveProductUnit instead.
func (d *Database) SeedDefaultUnitsOfMeasureForAllOrganizations() error {
	var orgIDs []string
	if err := d.DB.Select(&orgIDs, `SELECT id FROM organizations`); err != nil {
		return fmt.Errorf("seed_default_units_of_measure_for_all_organizations list_organizations: %w", err)
	}

	for _, orgID := range orgIDs {
		var count int
		if err := d.DB.Get(&count, `SELECT COUNT(*) FROM units_of_measure WHERE organizationId = ?`, orgID); err != nil {
			return fmt.Errorf("seed_default_units_of_measure_for_all_organizations count %s: %w", orgID, err)
		}
		if count > 0 {
			continue
		}
		if err := seedDefaultUnitsOfMeasure(d.DB, orgID); err != nil {
			return fmt.Errorf("seed_default_units_of_measure_for_all_organizations seed %s: %w", orgID, err)
		}
		if _, err := d.DB.Exec(`
			UPDATE products SET unitOfMeasureId = (
				SELECT id FROM units_of_measure
				WHERE organizationId = products.organizationId AND name = products.unit COLLATE NOCASE
				LIMIT 1
			)
			WHERE organizationId = ? AND unitOfMeasureId IS NULL AND unit IS NOT NULL AND unit != ''`,
			orgID,
		); err != nil {
			return fmt.Errorf("seed_default_units_of_measure_for_all_organizations backfill %s: %w", orgID, err)
		}
	}
	return nil
}

func seedDefaultUnitsOfMeasure(exec sqlExecer, organizationID string) error {
	for _, unit := range defaultUnitsOfMeasure {
		id, err := gonanoid.New()
		if err != nil {
			return fmt.Errorf("seed_default_units_of_measure new_id: %w", err)
		}
		var isDefault int64
		if unit.isDefault {
			isDefault = 1
		}
		if _, err := exec.Exec(
			`INSERT INTO units_of_measure (id, organizationId, name, isDefault) VALUES (?, ?, ?, ?)`,
			id, organizationID, unit.name, isDefault,
		); err != nil {
			return fmt.Errorf("seed_default_units_of_measure insert %s: %w", unit.name, err)
		}
	}
	return nil
}
