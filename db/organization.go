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

	// GL default accounts auto-posting resolves against when a document/tax
	// rate/product doesn't name a more specific account — seeded by
	// seedDefaultChartOfAccounts (db/account.go), editable afterward here.
	// Nullable: an org can use the chart of accounts + manual entries before
	// any of these are wired up; auto-posting is refused with a 409 (not a
	// 500) when a default it needs is unset.
	DefaultArAccountID        *string `db:"defaultArAccountId"        json:"defaultArAccountId"`
	DefaultApAccountID        *string `db:"defaultApAccountId"        json:"defaultApAccountId"`
	DefaultRevenueAccountID   *string `db:"defaultRevenueAccountId"   json:"defaultRevenueAccountId"`
	DefaultExpenseAccountID   *string `db:"defaultExpenseAccountId"   json:"defaultExpenseAccountId"`
	DefaultCashAccountID      *string `db:"defaultCashAccountId"      json:"defaultCashAccountId"`
	FxGainAccountID           *string `db:"fxGainAccountId"           json:"fxGainAccountId"`
	FxLossAccountID           *string `db:"fxLossAccountId"           json:"fxLossAccountId"`
	RetainedEarningsAccountID *string `db:"retainedEarningsAccountId" json:"retainedEarningsAccountId"`
	// DATEV export (Phase 5). datevClearingAccountId is the multi-line
	// Gegenkonto fallback for manual entries with more than one line per
	// side — see the design decision in the plan.
	DatevClearingAccountID *string `db:"datevClearingAccountId" json:"datevClearingAccountId"`
	DatevConsultantNumber  *string `db:"datev_consultant_number" json:"datev_consultant_number"`
	DatevClientNumber      *string `db:"datev_client_number"     json:"datev_client_number"`

	// Phase 7 (inventory/COGS GL integration). No per-product override —
	// one Inventory/GRNI/COGS/Adjustment account per organization.
	DefaultInventoryAccountID           *string `db:"defaultInventoryAccountId"           json:"defaultInventoryAccountId"`
	DefaultGRNIAccountID                *string `db:"defaultGRNIAccountId"                json:"defaultGRNIAccountId"`
	DefaultCOGSAccountID                *string `db:"defaultCOGSAccountId"                json:"defaultCOGSAccountId"`
	DefaultInventoryAdjustmentAccountID *string `db:"defaultInventoryAdjustmentAccountId" json:"defaultInventoryAdjustmentAccountId"`
}

// CreateOrganizationRequest is the payload for creating an organization.
type CreateOrganizationRequest struct {
	ID                    string   `json:"id"`
	Code                  *string  `json:"code"`
	Name                  *string  `json:"name"`
	Country               *string  `json:"country"`
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

	DefaultArAccountID        *string `json:"defaultArAccountId"`
	DefaultApAccountID        *string `json:"defaultApAccountId"`
	DefaultRevenueAccountID   *string `json:"defaultRevenueAccountId"`
	DefaultExpenseAccountID   *string `json:"defaultExpenseAccountId"`
	DefaultCashAccountID      *string `json:"defaultCashAccountId"`
	FxGainAccountID           *string `json:"fxGainAccountId"`
	FxLossAccountID           *string `json:"fxLossAccountId"`
	RetainedEarningsAccountID *string `json:"retainedEarningsAccountId"`
	DatevClearingAccountID    *string `json:"datevClearingAccountId"`
	DatevConsultantNumber     *string `json:"datev_consultant_number"`
	DatevClientNumber         *string `json:"datev_client_number"`

	DefaultInventoryAccountID           *string `json:"defaultInventoryAccountId"`
	DefaultGRNIAccountID                *string `json:"defaultGRNIAccountId"`
	DefaultCOGSAccountID                *string `json:"defaultCOGSAccountId"`
	DefaultInventoryAdjustmentAccountID *string `json:"defaultInventoryAdjustmentAccountId"`
}

// organizationColumns is every organizations column except logo, shared by
// GetOrganizations and GetOrganization. The logo BLOB is never loaded as part
// of the Organization struct — GetOrganizationLogo reads it directly, and the
// only way to read or write it over HTTP is the dedicated /logo endpoints.
const organizationColumns = `id, code, name, country, email, phone, website,
	       registration_number, vatin, bank_name, iban, currency,
	       minimum_fraction_digits, due_days, overdueCharge, customerNotes,
	       createdAt, invoice_number_format, invoice_number_counter, date_format,
	       match_price_tolerance_percent, match_quantity_tolerance_percent,
	       bic, tax_number, street, house_number, postal_code, city, country_code,
	       defaultArAccountId, defaultApAccountId, defaultRevenueAccountId,
	       defaultExpenseAccountId, defaultCashAccountId, fxGainAccountId, fxLossAccountId,
	       retainedEarningsAccountId, datevClearingAccountId,
	       datev_consultant_number, datev_client_number,
	       defaultInventoryAccountId, defaultGRNIAccountId,
	       defaultCOGSAccountId, defaultInventoryAdjustmentAccountId`

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

	tx, err := d.DB.Beginx()
	if err != nil {
		return nil, fmt.Errorf("create_organization begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.Exec(
		`INSERT INTO organizations (
			id, code, name, country, email, phone, website,
			registration_number, vatin, bank_name, iban, currency,
			minimum_fraction_digits, due_days, overdueCharge,
			customerNotes, invoice_number_format, date_format,
			bic, tax_number, street, house_number, postal_code, city, country_code
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		req.ID, req.Code, req.Name, req.Country, req.Email, req.Phone, req.Website,
		req.RegistrationNumber, req.Vatin, req.BankName, req.IBAN, req.Currency,
		req.MinimumFractionDigits, req.DueDays, req.OverdueCharge,
		req.CustomerNotes, req.InvoiceNumberFormat, req.DateFormat,
		req.BIC, req.TaxNumber, req.Street, req.HouseNumber, req.PostalCode, req.City, req.CountryCode,
	); err != nil {
		return nil, fmt.Errorf("create_organization: %w", err)
	}

	// Every organization gets a starter chart of accounts + default journals
	// immediately, so the GL module (manual journal entries at minimum) is
	// usable right away rather than requiring a separate setup step. See
	// seedDefaultChartOfAccounts (db/account.go): req.Country (the New
	// Organization form's free-text country name — org creation collects no
	// ISO country_code) selects a curated SKR04/PCG import when it matches
	// chartTemplates, otherwise the generic starter chart.
	if err := seedDefaultChartOfAccounts(tx, req.ID, req.Country); err != nil {
		return nil, fmt.Errorf("create_organization seed_accounts: %w", err)
	}
	if err := seedDefaultJournals(tx, req.ID); err != nil {
		return nil, fmt.Errorf("create_organization seed_journals: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("create_organization commit: %w", err)
	}
	return d.GetOrganization(req.ID)
}

func (d *Database) UpdateOrganization(organizationID string, updates UpdateOrganizationRequest) (*Organization, error) {
	_, err := d.DB.Exec(
		`UPDATE organizations
		 SET code                   = COALESCE(?, code),
		     name                   = COALESCE(?, name),
		     country                = COALESCE(?, country),
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
		     country_code           = COALESCE(?, country_code),
		     defaultArAccountId        = COALESCE(?, defaultArAccountId),
		     defaultApAccountId        = COALESCE(?, defaultApAccountId),
		     defaultRevenueAccountId   = COALESCE(?, defaultRevenueAccountId),
		     defaultExpenseAccountId   = COALESCE(?, defaultExpenseAccountId),
		     defaultCashAccountId      = COALESCE(?, defaultCashAccountId),
		     fxGainAccountId           = COALESCE(?, fxGainAccountId),
		     fxLossAccountId           = COALESCE(?, fxLossAccountId),
		     retainedEarningsAccountId = COALESCE(?, retainedEarningsAccountId),
		     datevClearingAccountId    = COALESCE(?, datevClearingAccountId),
		     datev_consultant_number   = COALESCE(?, datev_consultant_number),
		     datev_client_number       = COALESCE(?, datev_client_number),
		     defaultInventoryAccountId           = COALESCE(?, defaultInventoryAccountId),
		     defaultGRNIAccountId                = COALESCE(?, defaultGRNIAccountId),
		     defaultCOGSAccountId                = COALESCE(?, defaultCOGSAccountId),
		     defaultInventoryAdjustmentAccountId = COALESCE(?, defaultInventoryAdjustmentAccountId)
		 WHERE id = ?`,
		updates.Code, updates.Name, updates.Country, updates.Email, updates.Phone,
		updates.Website, updates.RegistrationNumber, updates.Vatin, updates.BankName,
		updates.IBAN, updates.Currency, updates.MinimumFractionDigits, updates.DueDays,
		updates.OverdueCharge, updates.CustomerNotes,
		updates.InvoiceNumberFormat, updates.InvoiceNumberCounter, updates.DateFormat,
		updates.MatchPriceTolerancePercent, updates.MatchQuantityTolerancePercent,
		updates.BIC, updates.TaxNumber, updates.Street, updates.HouseNumber,
		updates.PostalCode, updates.City, updates.CountryCode,
		updates.DefaultArAccountID, updates.DefaultApAccountID, updates.DefaultRevenueAccountID,
		updates.DefaultExpenseAccountID, updates.DefaultCashAccountID,
		updates.FxGainAccountID, updates.FxLossAccountID, updates.RetainedEarningsAccountID,
		updates.DatevClearingAccountID, updates.DatevConsultantNumber, updates.DatevClientNumber,
		updates.DefaultInventoryAccountID, updates.DefaultGRNIAccountID,
		updates.DefaultCOGSAccountID, updates.DefaultInventoryAdjustmentAccountID,
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
// blast radius before the user confirms. Reused by ResetOrganizationData for
// the same reason, minus the organization itself.
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
	StockMovements    int64 `db:"stockMovements"    json:"stockMovements"`

	Accounts       int64 `db:"accounts"      json:"accounts"`
	Journals       int64 `db:"journals"      json:"journals"`
	JournalEntries int64 `db:"journalEntries" json:"journalEntries"`
	Payments       int64 `db:"payments"      json:"payments"`
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
			(SELECT COUNT(*) FROM incoming_invoices WHERE organizationId = ?) AS incomingInvoices,
			(SELECT COUNT(*) FROM stockMovements WHERE organizationId = ?) AS stockMovements,
			(SELECT COUNT(*) FROM accounts WHERE organizationId = ?) AS accounts,
			(SELECT COUNT(*) FROM journals WHERE organizationId = ?) AS journals,
			(SELECT COUNT(*) FROM journal_entries WHERE organizationId = ?) AS journalEntries,
			(SELECT COUNT(*) FROM payments WHERE organizationId = ?) AS payments`,
		organizationID, organizationID, organizationID, organizationID, organizationID,
		organizationID, organizationID, organizationID, organizationID, organizationID,
		organizationID, organizationID, organizationID, organizationID, organizationID,
	)
	if err != nil {
		return nil, fmt.Errorf("get_organization_usage_count: %w", err)
	}
	return &counts, nil
}
