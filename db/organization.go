package db

import "fmt"

// Organization mirrors the organizations table.
type Organization struct {
	ID                   string  `db:"id"                      json:"id"`
	Code                 *string `db:"code"                    json:"code"`
	Name                 *string `db:"name"                    json:"name"`
	Country              *string `db:"country"                 json:"country"`
	Address              *string `db:"address"                 json:"address"`
	Email                *string `db:"email"                   json:"email"`
	Phone                *string `db:"phone"                   json:"phone"`
	Website              *string `db:"website"                 json:"website"`
	RegistrationNumber   *string `db:"registration_number"     json:"registration_number"`
	Vatin                *string `db:"vatin"                   json:"vatin"`
	BankName             *string `db:"bank_name"               json:"bank_name"`
	IBAN                 *string `db:"iban"                    json:"iban"`
	Currency             *string `db:"currency"                json:"currency"`
	MinimumFractionDigits *int64 `db:"minimum_fraction_digits" json:"minimum_fraction_digits"`
	DueDays              *int64  `db:"due_days"                json:"due_days"`
	OverdueCharge        *float64 `db:"overdueCharge"          json:"overdueCharge"`
	CustomerNotes        *string `db:"customerNotes"           json:"customerNotes"`
	CreatedAt            *string `db:"createdAt"               json:"createdAt"`
	Logo                 []byte  `db:"logo"                    json:"logo"`
	InvoiceNumberFormat  *string `db:"invoice_number_format"   json:"invoiceNumberFormat"`
	InvoiceNumberCounter *int64  `db:"invoice_number_counter"  json:"invoiceNumberCounter"`
	DateFormat           *string `db:"date_format"             json:"date_format"`
}

// CreateOrganizationRequest is the payload for creating an organization.
type CreateOrganizationRequest struct {
	ID                   string   `json:"id"`
	Code                 *string  `json:"code"`
	Name                 *string  `json:"name"`
	Country              *string  `json:"country"`
	Address              *string  `json:"address"`
	Email                *string  `json:"email"`
	Phone                *string  `json:"phone"`
	Website              *string  `json:"website"`
	RegistrationNumber   *string  `json:"registration_number"`
	Vatin                *string  `json:"vatin"`
	BankName             *string  `json:"bank_name"`
	IBAN                 *string  `json:"iban"`
	Currency             *string  `json:"currency"`
	MinimumFractionDigits *int64  `json:"minimum_fraction_digits"`
	DueDays              *int64   `json:"due_days"`
	OverdueCharge        *float64 `json:"overdueCharge"`
	CustomerNotes        *string  `json:"customerNotes"`
	Logo                 []byte   `json:"logo"`
	InvoiceNumberFormat  *string  `json:"invoiceNumberFormat"`
	DateFormat           *string  `json:"date_format"`
}

// UpdateOrganizationRequest is the payload for updating an organization.
type UpdateOrganizationRequest struct {
	Code                 *string  `json:"code"`
	Name                 *string  `json:"name"`
	Country              *string  `json:"country"`
	Address              *string  `json:"address"`
	Email                *string  `json:"email"`
	Phone                *string  `json:"phone"`
	Website              *string  `json:"website"`
	RegistrationNumber   *string  `json:"registration_number"`
	Vatin                *string  `json:"vatin"`
	BankName             *string  `json:"bank_name"`
	IBAN                 *string  `json:"iban"`
	Currency             *string  `json:"currency"`
	MinimumFractionDigits *int64  `json:"minimum_fraction_digits"`
	DueDays              *int64   `json:"due_days"`
	OverdueCharge        *float64 `json:"overdueCharge"`
	CustomerNotes        *string  `json:"customerNotes"`
	Logo                 []byte   `json:"logo"`
	InvoiceNumberFormat  *string  `json:"invoiceNumberFormat"`
	InvoiceNumberCounter *int64   `json:"invoiceNumberCounter"`
	DateFormat           *string  `json:"date_format"`
}

func (d *Database) GetOrganizations() ([]Organization, error) {
	orgs := []Organization{}
	err := d.DB.Select(&orgs, `SELECT * FROM organizations ORDER BY name ASC`)
	if err != nil {
		return nil, fmt.Errorf("get_organizations: %w", err)
	}
	return orgs, nil
}

func (d *Database) GetOrganization(organizationID string) (*Organization, error) {
	var org Organization
	err := d.DB.Get(&org,
		`SELECT * FROM organizations WHERE id = ? LIMIT 1`,
		organizationID,
	)
	if err != nil {
		return nil, fmt.Errorf("get_organization: %w", err)
	}
	return &org, nil
}

func (d *Database) CreateOrganization(req CreateOrganizationRequest) (*Organization, error) {
	if req.Code == nil {
		empty := ""
		req.Code = &empty
	}
	_, err := d.DB.Exec(
		`INSERT INTO organizations (
			id, code, name, country, address, email, phone, website,
			registration_number, vatin, bank_name, iban, currency,
			minimum_fraction_digits, due_days, overdueCharge,
			customerNotes, logo, invoice_number_format, date_format
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		req.ID, req.Code, req.Name, req.Country, req.Address, req.Email, req.Phone, req.Website,
		req.RegistrationNumber, req.Vatin, req.BankName, req.IBAN, req.Currency,
		req.MinimumFractionDigits, req.DueDays, req.OverdueCharge,
		req.CustomerNotes, req.Logo, req.InvoiceNumberFormat, req.DateFormat,
	)
	if err != nil {
		return nil, fmt.Errorf("create_organization: %w", err)
	}
	return d.GetOrganization(req.ID)
}

func (d *Database) UpdateOrganization(organizationID string, updates UpdateOrganizationRequest) (*Organization, error) {
	_, err := d.DB.Exec(
		`UPDATE organizations
		 SET code                   = COALESCE(?, code),
		     name                   = COALESCE(?, name),
		     country                = COALESCE(?, country),
		     address                = COALESCE(?, address),
		     email                  = COALESCE(?, email),
		     phone                  = COALESCE(?, phone),
		     website                = COALESCE(?, website),
		     registration_number    = COALESCE(?, registration_number),
		     vatin                  = COALESCE(?, vatin),
		     bank_name              = COALESCE(?, bank_name),
		     iban                   = COALESCE(?, iban),
		     currency               = COALESCE(?, currency),
		     minimum_fraction_digits = COALESCE(?, minimum_fraction_digits),
		     due_days               = COALESCE(?, due_days),
		     overdueCharge          = COALESCE(?, overdueCharge),
		     customerNotes          = COALESCE(?, customerNotes),
		     logo                   = COALESCE(?, logo),
		     invoice_number_format  = COALESCE(?, invoice_number_format),
		     invoice_number_counter = COALESCE(?, invoice_number_counter),
		     date_format            = COALESCE(?, date_format)
		 WHERE id = ?`,
		updates.Code, updates.Name, updates.Country, updates.Address, updates.Email, updates.Phone,
		updates.Website, updates.RegistrationNumber, updates.Vatin, updates.BankName,
		updates.IBAN, updates.Currency, updates.MinimumFractionDigits, updates.DueDays,
		updates.OverdueCharge, updates.CustomerNotes, updates.Logo,
		updates.InvoiceNumberFormat, updates.InvoiceNumberCounter, updates.DateFormat,
		organizationID,
	)
	if err != nil {
		return nil, fmt.Errorf("update_organization: %w", err)
	}
	return d.GetOrganization(organizationID)
}

func (d *Database) DeleteOrganization(organizationID string) (bool, error) {
	res, err := d.DB.Exec(`DELETE FROM organizations WHERE id = ?`, organizationID)
	if err != nil {
		return false, fmt.Errorf("delete_organization: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// OrganizationUsageCount reports how many records under each domain would be
// cascade-deleted along with the organization, so the UI can warn about the
// blast radius before the user confirms.
type OrganizationUsageCount struct {
	Clients    int64 `db:"clients"    json:"clients"`
	Invoices   int64 `db:"invoices"   json:"invoices"`
	Products   int64 `db:"products"   json:"products"`
	Orders     int64 `db:"orders"     json:"orders"`
	Deliveries int64 `db:"deliveries" json:"deliveries"`
	TaxRates   int64 `db:"taxRates"   json:"taxRates"`
}

func (d *Database) GetOrganizationUsageCount(organizationID string) (*OrganizationUsageCount, error) {
	var counts OrganizationUsageCount
	err := d.DB.Get(&counts, `
		SELECT
			(SELECT COUNT(*) FROM clients WHERE organizationId = ?) AS clients,
			(SELECT COUNT(*) FROM invoices WHERE organizationId = ?) AS invoices,
			(SELECT COUNT(*) FROM products WHERE organizationId = ?) AS products,
			(SELECT COUNT(*) FROM orders WHERE organizationId = ?) AS orders,
			(SELECT COUNT(*) FROM outbound_deliveries WHERE organizationId = ?) AS deliveries,
			(SELECT COUNT(*) FROM taxRates WHERE organizationId = ?) AS taxRates`,
		organizationID, organizationID, organizationID, organizationID, organizationID, organizationID,
	)
	if err != nil {
		return nil, fmt.Errorf("get_organization_usage_count: %w", err)
	}
	return &counts, nil
}
