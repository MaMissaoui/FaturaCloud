package db

import (
	"errors"
	"fmt"
	"strings"

	gonanoid "github.com/matoous/go-nanoid/v2"
)

// ErrVendorInUse is returned by DeleteVendor when the vendor is still
// referenced by purchasing documents. Deleting it anyway would either fail on
// a foreign key or orphan those documents, so callers get a 409 instead.
var ErrVendorInUse = errors.New("vendor is still used by one or more purchasing documents")

// Vendor mirrors the vendors table.
type Vendor struct {
	ID                 string  `db:"id"                  json:"id"`
	OrganizationID     string  `db:"organizationId"      json:"organizationId"`
	Name               *string `db:"name"                json:"name"`
	Code               *string `db:"code"                json:"code"`
	Emails             *string `db:"emails"              json:"emails"`
	Phone              *string `db:"phone"               json:"phone"`
	Website            *string `db:"website"             json:"website"`
	RegistrationNumber *string `db:"registration_number" json:"registration_number"`
	Vatin              *string `db:"vatin"               json:"vatin"`
	DefaultCurrency    *string `db:"defaultCurrency"     json:"defaultCurrency"`
	PaymentTermsDays   *int64  `db:"paymentTermsDays"    json:"paymentTermsDays"`
	CreatedAt          *string `db:"createdAt"           json:"createdAt"`

	// Structured address, matching clients/organizations — the single
	// source of truth for display (purchase order PDFs, lists); there is no
	// separate free-text address field.
	Street      *string `db:"street"       json:"street"`
	HouseNumber *string `db:"house_number" json:"house_number"`
	PostalCode  *string `db:"postal_code"  json:"postal_code"`
	City        *string `db:"city"         json:"city"`
	CountryCode *string `db:"country_code" json:"country_code"`
}

// CreateVendorRequest is the payload for creating a vendor.
type CreateVendorRequest struct {
	ID                 string  `json:"id"`
	OrganizationID     string  `json:"organizationId"`
	Name               *string `json:"name"`
	Code               *string `json:"code"`
	Emails             *string `json:"emails"`
	Phone              *string `json:"phone"`
	Website            *string `json:"website"`
	RegistrationNumber *string `json:"registration_number"`
	Vatin              *string `json:"vatin"`
	DefaultCurrency    *string `json:"defaultCurrency"`
	PaymentTermsDays   *int64  `json:"paymentTermsDays"`

	Street      *string `json:"street"`
	HouseNumber *string `json:"house_number"`
	PostalCode  *string `json:"postal_code"`
	City        *string `json:"city"`
	CountryCode *string `json:"country_code"`
}

// UpdateVendorRequest is the payload for updating a vendor.
type UpdateVendorRequest struct {
	Name               *string `json:"name"`
	Code               *string `json:"code"`
	Emails             *string `json:"emails"`
	Phone              *string `json:"phone"`
	Website            *string `json:"website"`
	RegistrationNumber *string `json:"registration_number"`
	Vatin              *string `json:"vatin"`
	DefaultCurrency    *string `json:"defaultCurrency"`
	PaymentTermsDays   *int64  `json:"paymentTermsDays"`

	Street      *string `json:"street"`
	HouseNumber *string `json:"house_number"`
	PostalCode  *string `json:"postal_code"`
	City        *string `json:"city"`
	CountryCode *string `json:"country_code"`
}

func (d *Database) GetVendors(organizationID string) ([]Vendor, error) {
	vendors := []Vendor{}
	err := d.DB.Select(&vendors,
		`SELECT * FROM vendors WHERE organizationId = ? ORDER BY name ASC`,
		organizationID,
	)
	if err != nil {
		return nil, fmt.Errorf("get_vendors: %w", err)
	}
	return vendors, nil
}

func (d *Database) GetVendor(vendorID string) (*Vendor, error) {
	var vendor Vendor
	err := d.DB.Get(&vendor,
		`SELECT * FROM vendors WHERE id = ? LIMIT 1`,
		vendorID,
	)
	if err != nil {
		return nil, fmt.Errorf("get_vendor: %w", err)
	}
	return &vendor, nil
}

func (d *Database) CreateVendor(req CreateVendorRequest) (*Vendor, error) {
	if req.ID == "" {
		req.ID, _ = gonanoid.New()
	}
	_, err := d.DB.Exec(
		`INSERT INTO vendors (id, organizationId, name, code, emails, phone, website,
		                      registration_number, vatin, defaultCurrency, paymentTermsDays,
		                      street, house_number, postal_code, city, country_code)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		req.ID, req.OrganizationID, req.Name, req.Code,
		req.Emails, req.Phone, req.Website, req.RegistrationNumber, req.Vatin,
		req.DefaultCurrency, req.PaymentTermsDays,
		req.Street, req.HouseNumber, req.PostalCode, req.City, req.CountryCode,
	)
	if err != nil {
		return nil, fmt.Errorf("create_vendor: %w", err)
	}
	return d.GetVendor(req.ID)
}

func (d *Database) UpdateVendor(vendorID string, updates UpdateVendorRequest) (*Vendor, error) {
	_, err := d.DB.Exec(
		`UPDATE vendors
		 SET name = ?, code = ?, emails = ?, phone = ?,
		     website = ?, registration_number = ?, vatin = ?,
		     defaultCurrency = ?, paymentTermsDays = ?,
		     street = ?, house_number = ?, postal_code = ?, city = ?, country_code = ?
		 WHERE id = ?`,
		updates.Name, updates.Code, updates.Emails, updates.Phone,
		updates.Website, updates.RegistrationNumber, updates.Vatin,
		updates.DefaultCurrency, updates.PaymentTermsDays,
		updates.Street, updates.HouseNumber, updates.PostalCode, updates.City, updates.CountryCode,
		vendorID,
	)
	if err != nil {
		return nil, fmt.Errorf("update_vendor: %w", err)
	}
	return d.GetVendor(vendorID)
}

// GetVendorDocumentCount returns how many purchasing documents reference this
// vendor, so callers can decide whether it's safe to delete.
//
// Each purchasing phase adds its table here as it introduces it (purchase
// orders, inbound deliveries, incoming invoices); until then nothing can
// reference a vendor and the count is always zero. TestVendorDocumentCountCoversEveryReference
// fails if a table gains a vendorId column without being listed, so the guard
// can't silently rot into a no-op.
var vendorReferencingTables = []string{
	"purchase_orders",
	"inbound_deliveries",
	"incoming_invoices",
	"payments",
	"journal_lines",
}

func (d *Database) GetVendorDocumentCount(vendorID string) (int64, error) {
	if len(vendorReferencingTables) == 0 {
		return 0, nil
	}

	// Table names come from the package-level slice above, never from input.
	subqueries := make([]string, len(vendorReferencingTables))
	args := make([]any, len(vendorReferencingTables))
	for i, table := range vendorReferencingTables {
		subqueries[i] = fmt.Sprintf("(SELECT COUNT(*) FROM %s WHERE vendorId = ?)", table)
		args[i] = vendorID
	}

	var count int64
	if err := d.DB.Get(&count, "SELECT "+strings.Join(subqueries, " + "), args...); err != nil {
		return 0, fmt.Errorf("get_vendor_document_count: %w", err)
	}
	return count, nil
}

// DeleteVendor refuses to delete a vendor that still has purchasing documents.
// Unlike DeleteClient (whose invoices cascade by design), purchasing documents
// reference vendors without a cascade, so an unguarded delete would surface a
// raw driver foreign-key error as an opaque 500.
func (d *Database) DeleteVendor(vendorID string) (bool, error) {
	count, err := d.GetVendorDocumentCount(vendorID)
	if err != nil {
		return false, err
	}
	if count > 0 {
		return false, ErrVendorInUse
	}

	res, err := d.DB.Exec(`DELETE FROM vendors WHERE id = ?`, vendorID)
	if err != nil {
		return false, fmt.Errorf("delete_vendor: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}
