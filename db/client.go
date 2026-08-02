package db

import (
	"fmt"

	gonanoid "github.com/matoous/go-nanoid/v2"
)

// Client mirrors the clients table.
type Client struct {
	ID                 string  `db:"id"                  json:"id"`
	OrganizationID     string  `db:"organizationId"      json:"organizationId"`
	Name               *string `db:"name"                json:"name"`
	Code               *string `db:"code"                json:"code"`
	Emails             *string `db:"emails"              json:"emails"`
	Phone              *string `db:"phone"               json:"phone"`
	Website            *string `db:"website"             json:"website"`
	RegistrationNumber *string `db:"registration_number" json:"registration_number"`
	Vatin              *string `db:"vatin"               json:"vatin"`
	// Mirrors vendors.defaultCurrency — the client's usual invoicing
	// currency, used only to prefill a new invoice's currency field.
	DefaultCurrency *string `db:"defaultCurrency"     json:"defaultCurrency"`
	CreatedAt       *string `db:"createdAt"           json:"createdAt"`

	// EN 16931 (XRechnung) buyer fields. Also the single source of truth for
	// display everywhere else (PDFs, lists) — clients no longer have a
	// separate free-text address blob. clients previously had no country at
	// all.
	Street                *string `db:"street"                  json:"street"`
	HouseNumber           *string `db:"house_number"             json:"house_number"`
	PostalCode            *string `db:"postal_code"              json:"postal_code"`
	City                  *string `db:"city"                     json:"city"`
	CountryCode           *string `db:"country_code"             json:"country_code"`
	TaxNumber             *string `db:"tax_number"               json:"tax_number"`
	DefaultBuyerReference *string `db:"default_buyer_reference"  json:"default_buyer_reference"`
}

// CreateClientRequest is the payload for creating a client.
type CreateClientRequest struct {
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

	Street                *string `json:"street"`
	HouseNumber           *string `json:"house_number"`
	PostalCode            *string `json:"postal_code"`
	City                  *string `json:"city"`
	CountryCode           *string `json:"country_code"`
	TaxNumber             *string `json:"tax_number"`
	DefaultBuyerReference *string `json:"default_buyer_reference"`
}

// UpdateClientRequest is the payload for updating a client.
type UpdateClientRequest struct {
	Name               *string `json:"name"`
	Code               *string `json:"code"`
	Emails             *string `json:"emails"`
	Phone              *string `json:"phone"`
	Website            *string `json:"website"`
	RegistrationNumber *string `json:"registration_number"`
	Vatin              *string `json:"vatin"`
	DefaultCurrency    *string `json:"defaultCurrency"`

	Street                *string `json:"street"`
	HouseNumber           *string `json:"house_number"`
	PostalCode            *string `json:"postal_code"`
	City                  *string `json:"city"`
	CountryCode           *string `json:"country_code"`
	TaxNumber             *string `json:"tax_number"`
	DefaultBuyerReference *string `json:"default_buyer_reference"`
}

func (d *Database) GetClients(organizationID string) ([]Client, error) {
	clients := []Client{}
	err := d.DB.Select(&clients,
		`SELECT * FROM clients WHERE organizationId = ? ORDER BY name ASC`,
		organizationID,
	)
	if err != nil {
		return nil, fmt.Errorf("get_clients: %w", err)
	}
	return clients, nil
}

func (d *Database) GetClient(clientID string) (*Client, error) {
	var client Client
	err := d.DB.Get(&client,
		`SELECT * FROM clients WHERE id = ? LIMIT 1`,
		clientID,
	)
	if err != nil {
		return nil, fmt.Errorf("get_client: %w", err)
	}
	return &client, nil
}

func (d *Database) CreateClient(req CreateClientRequest) (*Client, error) {
	if req.ID == "" {
		req.ID, _ = gonanoid.New()
	}
	_, err := d.DB.Exec(
		`INSERT INTO clients (
			id, organizationId, name, code, emails, phone, website,
			registration_number, vatin, defaultCurrency, street, house_number, postal_code, city,
			country_code, tax_number, default_buyer_reference
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		req.ID, req.OrganizationID, req.Name, req.Code,
		req.Emails, req.Phone, req.Website, req.RegistrationNumber, req.Vatin, req.DefaultCurrency,
		req.Street, req.HouseNumber, req.PostalCode, req.City,
		req.CountryCode, req.TaxNumber, req.DefaultBuyerReference,
	)
	if err != nil {
		return nil, fmt.Errorf("create_client: %w", err)
	}
	return d.GetClient(req.ID)
}

func (d *Database) UpdateClient(clientID string, updates UpdateClientRequest) (*Client, error) {
	_, err := d.DB.Exec(
		`UPDATE clients
		 SET name = ?, code = ?, emails = ?, phone = ?,
		     website = ?, registration_number = ?, vatin = ?, defaultCurrency = ?,
		     street = ?, house_number = ?, postal_code = ?, city = ?,
		     country_code = ?, tax_number = ?, default_buyer_reference = ?
		 WHERE id = ?`,
		updates.Name, updates.Code, updates.Emails, updates.Phone,
		updates.Website, updates.RegistrationNumber, updates.Vatin, updates.DefaultCurrency,
		updates.Street, updates.HouseNumber, updates.PostalCode, updates.City,
		updates.CountryCode, updates.TaxNumber, updates.DefaultBuyerReference,
		clientID,
	)
	if err != nil {
		return nil, fmt.Errorf("update_client: %w", err)
	}
	return d.GetClient(clientID)
}

func (d *Database) DeleteClient(clientID string) (bool, error) {
	res, err := d.DB.Exec(`DELETE FROM clients WHERE id = ?`, clientID)
	if err != nil {
		return false, fmt.Errorf("delete_client: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (d *Database) GetClientInvoiceCount(clientID string) (int64, error) {
	var count int64
	err := d.DB.Get(&count, `SELECT COUNT(*) FROM invoices WHERE clientId = ?`, clientID)
	if err != nil {
		return 0, fmt.Errorf("get_client_invoice_count: %w", err)
	}
	return count, nil
}
