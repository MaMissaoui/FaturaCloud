package db

import (
	"bytes"
	"encoding/base64"
	"fmt"
)

// Organization mirrors the organizations table.
type Organization struct {
	ID                    string   `db:"id"                      json:"id"`
	Code                  *string  `db:"code"                    json:"code"`
	Name                  *string  `db:"name"                    json:"name"`
	Country               *string  `db:"country"                 json:"country"`
	Address               *string  `db:"address"                 json:"address"`
	Email                 *string  `db:"email"                   json:"email"`
	Phone                 *string  `db:"phone"                   json:"phone"`
	Website               *string  `db:"website"                 json:"website"`
	RegistrationNumber    *string  `db:"registration_number"     json:"registration_number"`
	Vatin                 *string  `db:"vatin"                   json:"vatin"`
	BankName              *string  `db:"bank_name"               json:"bank_name"`
	IBAN                  *string  `db:"iban"                    json:"iban"`
	Currency              *string  `db:"currency"                json:"currency"`
	MinimumFractionDigits *int64   `db:"minimum_fraction_digits" json:"minimum_fraction_digits"`
	DueDays               *int64   `db:"due_days"                json:"due_days"`
	OverdueCharge         *float64 `db:"overdueCharge"          json:"overdueCharge"`
	CustomerNotes         *string  `db:"customerNotes"           json:"customerNotes"`
	CreatedAt             *string  `db:"createdAt"               json:"createdAt"`
	InvoiceNumberFormat   *string  `db:"invoice_number_format"   json:"invoiceNumberFormat"`
	InvoiceNumberCounter  *int64   `db:"invoice_number_counter"  json:"invoiceNumberCounter"`
	DateFormat            *string  `db:"date_format"             json:"date_format"`

	// 3-way matching tolerance policy (percent). Zero means any variance is flagged.
	MatchPriceTolerancePercent    *float64 `db:"match_price_tolerance_percent"    json:"match_price_tolerance_percent"`
	MatchQuantityTolerancePercent *float64 `db:"match_quantity_tolerance_percent" json:"match_quantity_tolerance_percent"`

	// EN 16931 (XRechnung/ZUGFeRD) seller fields. address/country above are
	// kept as free-text legacy display fields; these structured columns are
	// what e-invoice export reads and validates.
	BIC         *string `db:"bic"           json:"bic"`
	TaxNumber   *string `db:"tax_number"    json:"tax_number"`
	Street      *string `db:"street"        json:"street"`
	HouseNumber *string `db:"house_number"  json:"house_number"`
	PostalCode  *string `db:"postal_code"   json:"postal_code"`
	City        *string `db:"city"          json:"city"`
	CountryCode *string `db:"country_code"  json:"country_code"`
}

// CreateOrganizationRequest is the payload for creating an organization.
type CreateOrganizationRequest struct {
	ID                    string   `json:"id"`
	Code                  *string  `json:"code"`
	Name                  *string  `json:"name"`
	Country               *string  `json:"country"`
	Address               *string  `json:"address"`
	Email                 *string  `json:"email"`
	Phone                 *string  `json:"phone"`
	Website               *string  `json:"website"`
	RegistrationNumber    *string  `json:"registration_number"`
	Vatin                 *string  `json:"vatin"`
	BankName              *string  `json:"bank_name"`
	IBAN                  *string  `json:"iban"`
	Currency              *string  `json:"currency"`
	MinimumFractionDigits *int64   `json:"minimum_fraction_digits"`
	DueDays               *int64   `json:"due_days"`
	OverdueCharge         *float64 `json:"overdueCharge"`
	CustomerNotes         *string  `json:"customerNotes"`
	InvoiceNumberFormat   *string  `json:"invoiceNumberFormat"`
	DateFormat            *string  `json:"date_format"`

	MatchPriceTolerancePercent    *float64 `json:"match_price_tolerance_percent"`
	MatchQuantityTolerancePercent *float64 `json:"match_quantity_tolerance_percent"`

	BIC         *string `json:"bic"`
	TaxNumber   *string `json:"tax_number"`
	Street      *string `json:"street"`
	HouseNumber *string `json:"house_number"`
	PostalCode  *string `json:"postal_code"`
	City        *string `json:"city"`
	CountryCode *string `json:"country_code"`
}

// UpdateOrganizationRequest is the payload for updating an organization.
type UpdateOrganizationRequest struct {
	Code                  *string  `json:"code"`
	Name                  *string  `json:"name"`
	Country               *string  `json:"country"`
	Address               *string  `json:"address"`
	Email                 *string  `json:"email"`
	Phone                 *string  `json:"phone"`
	Website               *string  `json:"website"`
	RegistrationNumber    *string  `json:"registration_number"`
	Vatin                 *string  `json:"vatin"`
	BankName              *string  `json:"bank_name"`
	IBAN                  *string  `json:"iban"`
	Currency              *string  `json:"currency"`
	MinimumFractionDigits *int64   `json:"minimum_fraction_digits"`
	DueDays               *int64   `json:"due_days"`
	OverdueCharge         *float64 `json:"overdueCharge"`
	CustomerNotes         *string  `json:"customerNotes"`
	InvoiceNumberFormat   *string  `json:"invoiceNumberFormat"`
	InvoiceNumberCounter  *int64   `json:"invoiceNumberCounter"`
	DateFormat            *string  `json:"date_format"`

	MatchPriceTolerancePercent    *float64 `json:"match_price_tolerance_percent"`
	MatchQuantityTolerancePercent *float64 `json:"match_quantity_tolerance_percent"`

	BIC         *string `json:"bic"`
	TaxNumber   *string `json:"tax_number"`
	Street      *string `json:"street"`
	HouseNumber *string `json:"house_number"`
	PostalCode  *string `json:"postal_code"`
	City        *string `json:"city"`
	CountryCode *string `json:"country_code"`
}

// organizationColumns is every organizations column except logo, shared by
// GetOrganizations and GetOrganization. The logo BLOB is never loaded as part
// of the Organization struct — GetOrganizationLogo reads it directly, and the
// only way to read or write it over HTTP is the dedicated /logo endpoints.
const organizationColumns = `id, code, name, country, address, email, phone, website,
	       registration_number, vatin, bank_name, iban, currency,
	       minimum_fraction_digits, due_days, overdueCharge, customerNotes,
	       createdAt, invoice_number_format, invoice_number_counter, date_format,
	       match_price_tolerance_percent, match_quantity_tolerance_percent,
	       bic, tax_number, street, house_number, postal_code, city, country_code`

func (d *Database) GetOrganizations() ([]Organization, error) {
	orgs := []Organization{}
	// The list is re-fetched on every auth change and can hold many orgs, so
	// shipping each org's (potentially multi-MB) logo BLOB here is pure waste.
	err := d.DB.Select(&orgs, `SELECT `+organizationColumns+` FROM organizations ORDER BY name ASC`)
	if err != nil {
		return nil, fmt.Errorf("get_organizations: %w", err)
	}
	return orgs, nil
}

func (d *Database) GetOrganization(organizationID string) (*Organization, error) {
	var org Organization
	err := d.DB.Get(&org,
		`SELECT `+organizationColumns+` FROM organizations WHERE id = ? LIMIT 1`,
		organizationID,
	)
	if err != nil {
		return nil, fmt.Errorf("get_organization: %w", err)
	}
	return &org, nil
}

// GetOrganizationLogo returns the raw logo bytes for GET /organizations/{id}/logo.
// Organizations created before this endpoint existed (and the desktop-Fatura
// import that predates migration 0005) may still hold the browser's full
// "data:image/png;base64,..." string stored verbatim as text, from when the
// only write path was JSON-encoding a data URL into the logo column. Detect
// and decode that legacy format on read rather than leaving it to render as
// broken images forever.
func (d *Database) GetOrganizationLogo(organizationID string) ([]byte, error) {
	var logo []byte
	err := d.DB.Get(&logo, `SELECT logo FROM organizations WHERE id = ?`, organizationID)
	if err != nil {
		return nil, fmt.Errorf("get_organization_logo: %w", err)
	}
	if idx := bytes.IndexByte(logo, ','); bytes.HasPrefix(logo, []byte("data:")) && idx != -1 {
		if decoded, err := base64.StdEncoding.DecodeString(string(logo[idx+1:])); err == nil {
			return decoded, nil
		}
	}
	return logo, nil
}

// SetOrganizationLogo overwrites (or, with data == nil, clears) an
// organization's logo. Used by both the upload and delete /logo handlers.
func (d *Database) SetOrganizationLogo(organizationID string, data []byte) (bool, error) {
	res, err := d.DB.Exec(`UPDATE organizations SET logo = ? WHERE id = ?`, data, organizationID)
	if err != nil {
		return false, fmt.Errorf("set_organization_logo: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
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
			customerNotes, invoice_number_format, date_format,
			bic, tax_number, street, house_number, postal_code, city, country_code
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		req.ID, req.Code, req.Name, req.Country, req.Address, req.Email, req.Phone, req.Website,
		req.RegistrationNumber, req.Vatin, req.BankName, req.IBAN, req.Currency,
		req.MinimumFractionDigits, req.DueDays, req.OverdueCharge,
		req.CustomerNotes, req.InvoiceNumberFormat, req.DateFormat,
		req.BIC, req.TaxNumber, req.Street, req.HouseNumber, req.PostalCode, req.City, req.CountryCode,
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
		     invoice_number_format  = COALESCE(?, invoice_number_format),
		     invoice_number_counter = COALESCE(?, invoice_number_counter),
		     date_format            = COALESCE(?, date_format),
		     match_price_tolerance_percent    = COALESCE(?, match_price_tolerance_percent),
		     match_quantity_tolerance_percent = COALESCE(?, match_quantity_tolerance_percent),
		     bic                    = COALESCE(?, bic),
		     tax_number             = COALESCE(?, tax_number),
		     street                 = COALESCE(?, street),
		     house_number           = COALESCE(?, house_number),
		     postal_code            = COALESCE(?, postal_code),
		     city                   = COALESCE(?, city),
		     country_code           = COALESCE(?, country_code)
		 WHERE id = ?`,
		updates.Code, updates.Name, updates.Country, updates.Address, updates.Email, updates.Phone,
		updates.Website, updates.RegistrationNumber, updates.Vatin, updates.BankName,
		updates.IBAN, updates.Currency, updates.MinimumFractionDigits, updates.DueDays,
		updates.OverdueCharge, updates.CustomerNotes,
		updates.InvoiceNumberFormat, updates.InvoiceNumberCounter, updates.DateFormat,
		updates.MatchPriceTolerancePercent, updates.MatchQuantityTolerancePercent,
		updates.BIC, updates.TaxNumber, updates.Street, updates.HouseNumber,
		updates.PostalCode, updates.City, updates.CountryCode,
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
	Vendors    int64 `db:"vendors"    json:"vendors"`
	Invoices   int64 `db:"invoices"   json:"invoices"`
	Products   int64 `db:"products"   json:"products"`
	Orders     int64 `db:"orders"     json:"orders"`
	Deliveries int64 `db:"deliveries" json:"deliveries"`
	TaxRates   int64 `db:"taxRates"   json:"taxRates"`

	PurchaseOrders    int64 `db:"purchaseOrders"    json:"purchaseOrders"`
	InboundDeliveries int64 `db:"inboundDeliveries" json:"inboundDeliveries"`
	IncomingInvoices  int64 `db:"incomingInvoices"  json:"incomingInvoices"`
}

func (d *Database) GetOrganizationUsageCount(organizationID string) (*OrganizationUsageCount, error) {
	var counts OrganizationUsageCount
	err := d.DB.Get(&counts, `
		SELECT
			(SELECT COUNT(*) FROM clients WHERE organizationId = ?) AS clients,
			(SELECT COUNT(*) FROM vendors WHERE organizationId = ?) AS vendors,
			(SELECT COUNT(*) FROM invoices WHERE organizationId = ?) AS invoices,
			(SELECT COUNT(*) FROM products WHERE organizationId = ?) AS products,
			(SELECT COUNT(*) FROM orders WHERE organizationId = ?) AS orders,
			(SELECT COUNT(*) FROM outbound_deliveries WHERE organizationId = ?) AS deliveries,
			(SELECT COUNT(*) FROM taxRates WHERE organizationId = ?) AS taxRates,
			(SELECT COUNT(*) FROM purchase_orders WHERE organizationId = ?) AS purchaseOrders,
			(SELECT COUNT(*) FROM inbound_deliveries WHERE organizationId = ?) AS inboundDeliveries,
			(SELECT COUNT(*) FROM incoming_invoices WHERE organizationId = ?) AS incomingInvoices`,
		organizationID, organizationID, organizationID, organizationID, organizationID,
		organizationID, organizationID, organizationID, organizationID, organizationID,
	)
	if err != nil {
		return nil, fmt.Errorf("get_organization_usage_count: %w", err)
	}
	return &counts, nil
}
