package db

import (
	"strconv"
	"strings"
)

// vendorsMassDataSpec is vendors' massDataSpec (db/mass_data.go) — no
// foreign-key columns, so NewImportContext needs no lookup state.
type vendorsMassDataSpec struct{}

func (vendorsMassDataSpec) Headers() []string {
	return []string{
		"ID", "Name", "Code", "Emails", "Phone", "Website",
		"Registration Number", "VATIN", "Default Currency", "Payment Terms (Days)",
		"Street", "House Number", "Postal Code", "City", "Country Code",
	}
}

func (vendorsMassDataSpec) ExportRows(d *Database, organizationID string) ([][]string, error) {
	vendors, err := d.GetVendors(organizationID)
	if err != nil {
		return nil, err
	}
	rows := make([][]string, len(vendors))
	for i, v := range vendors {
		paymentTermsDays := ""
		if v.PaymentTermsDays != nil {
			paymentTermsDays = strconv.FormatInt(*v.PaymentTermsDays, 10)
		}
		rows[i] = []string{
			v.ID, strOrEmpty(v.Name), strOrEmpty(v.Code), strOrEmpty(v.Emails), strOrEmpty(v.Phone), strOrEmpty(v.Website),
			strOrEmpty(v.RegistrationNumber), strOrEmpty(v.Vatin), strOrEmpty(v.DefaultCurrency), paymentTermsDays,
			strOrEmpty(v.Street), strOrEmpty(v.HouseNumber), strOrEmpty(v.PostalCode), strOrEmpty(v.City), strOrEmpty(v.CountryCode),
		}
	}
	return rows, nil
}

func (vendorsMassDataSpec) NewImportContext(d *Database, organizationID string) (any, error) {
	return nil, nil
}

func (vendorsMassDataSpec) ImportRow(d *Database, organizationID string, _ any, cells []string) (action, identifier string, err error) {
	id := strings.TrimSpace(cells[0])
	name := strings.TrimSpace(cells[1])
	identifier = name
	if name == "" {
		return "", identifier, newValidationError("name is required")
	}
	var paymentTermsDays *int64
	if v := strings.TrimSpace(cells[9]); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return "", identifier, newValidationError("payment terms (days) %q is not a whole number", v)
		}
		paymentTermsDays = &n
	}

	fields := func(dst *CreateVendorRequest) {
		dst.Name = &name
		dst.Code = cellStr(cells[2])
		dst.Emails = cellStr(cells[3])
		dst.Phone = cellStr(cells[4])
		dst.Website = cellStr(cells[5])
		dst.RegistrationNumber = cellStr(cells[6])
		dst.Vatin = cellStr(cells[7])
		dst.DefaultCurrency = cellStr(cells[8])
		dst.PaymentTermsDays = paymentTermsDays
		dst.Street = cellStr(cells[10])
		dst.HouseNumber = cellStr(cells[11])
		dst.PostalCode = cellStr(cells[12])
		dst.City = cellStr(cells[13])
		dst.CountryCode = cellStr(cells[14])
	}

	if id == "" {
		req := CreateVendorRequest{OrganizationID: organizationID}
		fields(&req)
		if _, err := d.CreateVendor(req); err != nil {
			return "", identifier, err
		}
		return "created", identifier, nil
	}

	existing, err := d.GetVendor(id)
	if err != nil {
		return "", identifier, newValidationError("vendor id %q not found", id)
	}
	if existing.OrganizationID != organizationID {
		return "", identifier, newValidationError("vendor id %q belongs to a different organization", id)
	}
	var create CreateVendorRequest
	fields(&create)
	update := UpdateVendorRequest{
		Name: create.Name, Code: create.Code, Emails: create.Emails, Phone: create.Phone, Website: create.Website,
		RegistrationNumber: create.RegistrationNumber, Vatin: create.Vatin, DefaultCurrency: create.DefaultCurrency,
		PaymentTermsDays: create.PaymentTermsDays,
		Street:           create.Street, HouseNumber: create.HouseNumber, PostalCode: create.PostalCode,
		City: create.City, CountryCode: create.CountryCode,
	}
	if _, err := d.UpdateVendor(id, update); err != nil {
		return "", identifier, err
	}
	return "updated", identifier, nil
}

// ExportVendorsXLSX/ImportVendorsXLSX are the public entry points for
// vendors' Excel mass-maintenance download/upload (api/mass_data.go).
func (d *Database) ExportVendorsXLSX(organizationID string) ([]byte, error) {
	return exportMassDataXLSX(vendorsMassDataSpec{}, d, organizationID)
}

func (d *Database) ImportVendorsXLSX(organizationID string, content []byte) (*MassDataImportResult, error) {
	return importMassDataXLSX(vendorsMassDataSpec{}, d, organizationID, content)
}
