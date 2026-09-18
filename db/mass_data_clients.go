package db

import "strings"

// clientsMassDataSpec is clients' massDataSpec (db/mass_data.go) — no
// foreign-key columns, so NewImportContext needs no lookup state.
type clientsMassDataSpec struct{}

func (clientsMassDataSpec) Headers() []string {
	return []string{
		"ID", "Name", "Code", "Emails", "Phone", "Website",
		"Registration Number", "VATIN", "Default Currency",
		"Street", "House Number", "Postal Code", "City", "Country Code",
		"Tax Number", "Default Buyer Reference", "Identity Number", "IBAN",
	}
}

func (clientsMassDataSpec) ExportRows(d *Database, organizationID string) ([][]string, error) {
	clients, err := d.GetClients(organizationID)
	if err != nil {
		return nil, err
	}
	rows := make([][]string, len(clients))
	for i, c := range clients {
		rows[i] = []string{
			c.ID, strOrEmpty(c.Name), strOrEmpty(c.Code), strOrEmpty(c.Emails), strOrEmpty(c.Phone), strOrEmpty(c.Website),
			strOrEmpty(c.RegistrationNumber), strOrEmpty(c.Vatin), strOrEmpty(c.DefaultCurrency),
			strOrEmpty(c.Street), strOrEmpty(c.HouseNumber), strOrEmpty(c.PostalCode), strOrEmpty(c.City), strOrEmpty(c.CountryCode),
			strOrEmpty(c.TaxNumber), strOrEmpty(c.DefaultBuyerReference), strOrEmpty(c.IdentityNumber), strOrEmpty(c.Iban),
		}
	}
	return rows, nil
}

func (clientsMassDataSpec) NewImportContext(d *Database, organizationID string) (any, error) {
	return nil, nil
}

func (clientsMassDataSpec) ImportRow(d *Database, organizationID string, _ any, cells []string) (action, identifier string, err error) {
	id := strings.TrimSpace(cells[0])
	name := strings.TrimSpace(cells[1])
	identifier = name
	if name == "" {
		return "", identifier, newValidationError("name is required")
	}

	fields := func(dst *CreateClientRequest) {
		dst.Name = &name
		dst.Code = cellStr(cells[2])
		dst.Emails = cellStr(cells[3])
		dst.Phone = cellStr(cells[4])
		dst.Website = cellStr(cells[5])
		dst.RegistrationNumber = cellStr(cells[6])
		dst.Vatin = cellStr(cells[7])
		dst.DefaultCurrency = cellStr(cells[8])
		dst.Street = cellStr(cells[9])
		dst.HouseNumber = cellStr(cells[10])
		dst.PostalCode = cellStr(cells[11])
		dst.City = cellStr(cells[12])
		dst.CountryCode = cellStr(cells[13])
		dst.TaxNumber = cellStr(cells[14])
		dst.DefaultBuyerReference = cellStr(cells[15])
		dst.IdentityNumber = cellStr(cells[16])
		dst.Iban = cellStr(cells[17])
	}

	if id == "" {
		req := CreateClientRequest{OrganizationID: organizationID}
		fields(&req)
		if _, err := d.CreateClient(req); err != nil {
			return "", identifier, err
		}
		return "created", identifier, nil
	}

	existing, err := d.GetClient(id)
	if err != nil {
		return "", identifier, newValidationError("client id %q not found", id)
	}
	if existing.OrganizationID != organizationID {
		return "", identifier, newValidationError("client id %q belongs to a different organization", id)
	}
	var create CreateClientRequest
	fields(&create)
	update := UpdateClientRequest{
		Name: create.Name, Code: create.Code, Emails: create.Emails, Phone: create.Phone, Website: create.Website,
		RegistrationNumber: create.RegistrationNumber, Vatin: create.Vatin, DefaultCurrency: create.DefaultCurrency,
		Street: create.Street, HouseNumber: create.HouseNumber, PostalCode: create.PostalCode, City: create.City,
		CountryCode: create.CountryCode, TaxNumber: create.TaxNumber, DefaultBuyerReference: create.DefaultBuyerReference,
		IdentityNumber: create.IdentityNumber, Iban: create.Iban,
	}
	if _, err := d.UpdateClient(id, update); err != nil {
		return "", identifier, err
	}
	return "updated", identifier, nil
}

// ExportClientsXLSX/ImportClientsXLSX are the public entry points for
// clients' Excel mass-maintenance download/upload (api/mass_data.go).
func (d *Database) ExportClientsXLSX(organizationID string) ([]byte, error) {
	return exportMassDataXLSX(clientsMassDataSpec{}, d, organizationID)
}

func (d *Database) ImportClientsXLSX(organizationID string, content []byte) (*MassDataImportResult, error) {
	return importMassDataXLSX(clientsMassDataSpec{}, d, organizationID, content)
}
