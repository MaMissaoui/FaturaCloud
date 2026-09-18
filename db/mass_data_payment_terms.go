package db

import "strings"

// paymentTermsMassDataSpec is payment terms' massDataSpec (db/mass_data.go).
type paymentTermsMassDataSpec struct{}

func (paymentTermsMassDataSpec) Headers() []string {
	return []string{"ID", "Name", "Default"}
}

func (paymentTermsMassDataSpec) ExportRows(d *Database, organizationID string) ([][]string, error) {
	terms, err := d.GetPaymentTerms(organizationID)
	if err != nil {
		return nil, err
	}
	rows := make([][]string, len(terms))
	for i, t := range terms {
		rows[i] = []string{t.ID, t.Name, int64BoolCell(t.IsDefault)}
	}
	return rows, nil
}

func (paymentTermsMassDataSpec) NewImportContext(d *Database, organizationID string) (any, error) {
	return nil, nil
}

func (paymentTermsMassDataSpec) ImportRow(d *Database, organizationID string, _ any, cells []string) (action, identifier string, err error) {
	id := strings.TrimSpace(cells[0])
	name := strings.TrimSpace(cells[1])
	identifier = name
	if name == "" {
		return "", identifier, newValidationError("name is required")
	}
	isDefault := cellToInt64BoolPtr(cells[2])

	if id == "" {
		req := CreatePaymentTermRequest{OrganizationID: organizationID, Name: name, IsDefault: isDefault}
		if _, err := d.CreatePaymentTerm(req); err != nil {
			return "", identifier, err
		}
		return "created", identifier, nil
	}

	existing, err := d.GetPaymentTerm(id)
	if err != nil {
		return "", identifier, newValidationError("payment term id %q not found", id)
	}
	if existing.OrganizationID != organizationID {
		return "", identifier, newValidationError("payment term id %q belongs to a different organization", id)
	}
	update := UpdatePaymentTermRequest{Name: &name, IsDefault: isDefault}
	if _, err := d.UpdatePaymentTerm(id, update); err != nil {
		return "", identifier, err
	}
	return "updated", identifier, nil
}

// ExportPaymentTermsXLSX/ImportPaymentTermsXLSX are the public entry points
// for payment terms' Excel mass-maintenance download/upload
// (api/mass_data.go).
func (d *Database) ExportPaymentTermsXLSX(organizationID string) ([]byte, error) {
	return exportMassDataXLSX(paymentTermsMassDataSpec{}, d, organizationID)
}

func (d *Database) ImportPaymentTermsXLSX(organizationID string, content []byte) (*MassDataImportResult, error) {
	return importMassDataXLSX(paymentTermsMassDataSpec{}, d, organizationID, content)
}
