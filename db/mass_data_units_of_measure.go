package db

import "strings"

// unitsOfMeasureMassDataSpec is units of measure's massDataSpec
// (db/mass_data.go).
type unitsOfMeasureMassDataSpec struct{}

func (unitsOfMeasureMassDataSpec) Headers() []string {
	return []string{"ID", "Name", "Default"}
}

func (unitsOfMeasureMassDataSpec) ExportRows(d *Database, organizationID string) ([][]string, error) {
	units, err := d.GetUnitsOfMeasure(organizationID)
	if err != nil {
		return nil, err
	}
	rows := make([][]string, len(units))
	for i, u := range units {
		rows[i] = []string{u.ID, u.Name, int64BoolCell(u.IsDefault)}
	}
	return rows, nil
}

func (unitsOfMeasureMassDataSpec) NewImportContext(d *Database, organizationID string) (any, error) {
	return nil, nil
}

func (unitsOfMeasureMassDataSpec) ImportRow(d *Database, organizationID string, _ any, cells []string) (action, identifier string, err error) {
	id := strings.TrimSpace(cells[0])
	name := strings.TrimSpace(cells[1])
	identifier = name
	if name == "" {
		return "", identifier, newValidationError("name is required")
	}
	isDefault := cellToInt64BoolPtr(cells[2])

	if id == "" {
		req := CreateUnitOfMeasureRequest{OrganizationID: organizationID, Name: name, IsDefault: isDefault}
		if _, err := d.CreateUnitOfMeasure(req); err != nil {
			return "", identifier, err
		}
		return "created", identifier, nil
	}

	existing, err := d.GetUnitOfMeasure(id)
	if err != nil {
		return "", identifier, newValidationError("unit of measure id %q not found", id)
	}
	if existing.OrganizationID != organizationID {
		return "", identifier, newValidationError("unit of measure id %q belongs to a different organization", id)
	}
	update := UpdateUnitOfMeasureRequest{Name: &name, IsDefault: isDefault}
	if _, err := d.UpdateUnitOfMeasure(id, update); err != nil {
		return "", identifier, err
	}
	return "updated", identifier, nil
}

// ExportUnitsOfMeasureXLSX/ImportUnitsOfMeasureXLSX are the public entry
// points for units of measure's Excel mass-maintenance download/upload
// (api/mass_data.go).
func (d *Database) ExportUnitsOfMeasureXLSX(organizationID string) ([]byte, error) {
	return exportMassDataXLSX(unitsOfMeasureMassDataSpec{}, d, organizationID)
}

func (d *Database) ImportUnitsOfMeasureXLSX(organizationID string, content []byte) (*MassDataImportResult, error) {
	return importMassDataXLSX(unitsOfMeasureMassDataSpec{}, d, organizationID, content)
}
