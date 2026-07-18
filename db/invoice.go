package db

import (
	"database/sql"
	"errors"
	"fmt"

	gonanoid "github.com/matoous/go-nanoid/v2"
)

// invoiceStates is the canonical set of invoice states. Unlike orders and
// deliveries, invoices may move freely between these (a bounced payment can
// legitimately send paid→sent), so there is no transition matrix here — only
// set membership is enforced, on create and on the PATCH state endpoint.
var invoiceStates = map[string]bool{
	"draft":     true,
	"sent":      true,
	"paid":      true,
	"cancelled": true,
}

// Invoice mirrors the invoices table (with an optional joined clientName).
type Invoice struct {
	ID             string   `db:"id"             json:"id"`
	OrganizationID string   `db:"organizationId" json:"organizationId"`
	Number         string   `db:"number"         json:"number"`
	State          string   `db:"state"          json:"state"`
	ClientID       string   `db:"clientId"       json:"clientId"`
	Date           int64    `db:"date"           json:"date"`
	DueDate        *int64   `db:"dueDate"        json:"dueDate"`
	Currency       string   `db:"currency"       json:"currency"`
	CustomerNotes  *string  `db:"customerNotes"  json:"customerNotes"`
	OverdueCharge  *float64 `db:"overdueCharge"  json:"overdueCharge"`
	Total          int64    `db:"total"          json:"total"`
	TaxTotal       int64    `db:"taxTotal"       json:"taxTotal"`
	SubTotal       int64    `db:"subTotal"       json:"subTotal"`
	CreatedAt      *string  `db:"createdAt"      json:"createdAt"`
	ClientName     *string  `db:"clientName"     json:"clientName"`
}

// InvoiceLineItem mirrors the invoiceLineItems table.
type InvoiceLineItem struct {
	ID          string  `db:"id"          json:"id"`
	InvoiceID   string  `db:"invoiceId"   json:"invoiceId"`
	Description *string `db:"description" json:"description"`
	Quantity    float64 `db:"quantity"    json:"quantity"`
	UnitPrice   int64   `db:"unitPrice"   json:"unitPrice"`
	TaxRate     *string `db:"taxRate"     json:"taxRate"`
	Position    int     `db:"position"    json:"position"`
	CreatedAt   *string `db:"createdAt"   json:"createdAt"`
}

// CreateInvoiceLineItemRequest is a single line item within a create/update request.
type CreateInvoiceLineItemRequest struct {
	Description *string `json:"description"`
	Quantity    float64 `json:"quantity"`
	UnitPrice   float64 `json:"unitPrice"`
	TaxRate     *string `json:"taxRate"`
}

// CreateInvoiceRequest is the payload for creating an invoice.
type CreateInvoiceRequest struct {
	ID             string                         `json:"id"`
	OrganizationID string                         `json:"organizationId"`
	Number         string                         `json:"number"`
	State          string                         `json:"state"`
	ClientID       string                         `json:"clientId"`
	Date           int64                          `json:"date"`
	DueDate        *int64                         `json:"dueDate"`
	Currency       string                         `json:"currency"`
	CustomerNotes  *string                        `json:"customerNotes"`
	OverdueCharge  *float64                       `json:"overdueCharge"`
	Total          int64                          `json:"total"`
	TaxTotal       int64                          `json:"taxTotal"`
	SubTotal       int64                          `json:"subTotal"`
	LineItems      []CreateInvoiceLineItemRequest `json:"lineItems"`
}

// UpdateInvoiceRequest is the payload for updating an invoice. State is
// deliberately absent — state changes go through PATCH /api/invoices/{id}/state
// only (which validates against invoiceStates), matching the orders/deliveries
// convention, so a PUT can't set an arbitrary state and bypass validation.
type UpdateInvoiceRequest struct {
	Number        *string                         `json:"number"`
	ClientID      *string                         `json:"clientId"`
	Date          *int64                          `json:"date"`
	DueDate       *int64                          `json:"dueDate"`
	Currency      *string                         `json:"currency"`
	CustomerNotes *string                         `json:"customerNotes"`
	OverdueCharge *float64                        `json:"overdueCharge"`
	Total         *int64                          `json:"total"`
	TaxTotal      *int64                          `json:"taxTotal"`
	SubTotal      *int64                          `json:"subTotal"`
	LineItems     *[]CreateInvoiceLineItemRequest `json:"lineItems"`
}

func (d *Database) GetInvoices(organizationID string) ([]Invoice, error) {
	invoices := []Invoice{}
	err := d.DB.Select(&invoices, `
		SELECT invoices.*, clients.name AS clientName
		FROM invoices
		INNER JOIN clients ON invoices.clientId = clients.id
		WHERE invoices.organizationId = ?
		ORDER BY invoices.date DESC`,
		organizationID,
	)
	if err != nil {
		return nil, fmt.Errorf("get_invoices: %w", err)
	}
	return invoices, nil
}

func (d *Database) GetInvoice(invoiceID string) (*Invoice, error) {
	var invoice Invoice
	err := d.DB.Get(&invoice, `
		SELECT invoices.*, clients.name AS clientName
		FROM invoices
		INNER JOIN clients ON invoices.clientId = clients.id
		WHERE invoices.id = ?
		LIMIT 1`,
		invoiceID,
	)
	if err != nil {
		return nil, fmt.Errorf("get_invoice: %w", err)
	}
	return &invoice, nil
}

func (d *Database) GetInvoiceLineItems(invoiceID string) ([]InvoiceLineItem, error) {
	items := []InvoiceLineItem{}
	err := d.DB.Select(&items,
		`SELECT * FROM invoiceLineItems WHERE invoiceId = ? ORDER BY position ASC, createdAt ASC`,
		invoiceID,
	)
	if err != nil {
		return nil, fmt.Errorf("get_invoice_line_items: %w", err)
	}
	return items, nil
}

func (d *Database) CreateInvoice(req CreateInvoiceRequest) (*Invoice, error) {
	if req.State == "" {
		req.State = "draft"
	}
	if !invoiceStates[req.State] {
		return nil, newValidationError("invalid invoice state %q", req.State)
	}
	if err := d.validateInvoiceTotals(req.LineItems, req.SubTotal, req.TaxTotal, req.Total); err != nil {
		return nil, err
	}

	tx, err := d.DB.Beginx()
	if err != nil {
		return nil, fmt.Errorf("create_invoice begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	_, err = tx.Exec(`
		INSERT INTO invoices (
			id, organizationId, number, state, clientId, date, dueDate,
			currency, customerNotes, overdueCharge, total, taxTotal, subTotal
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		req.ID, req.OrganizationID, req.Number, req.State, req.ClientID,
		req.Date, req.DueDate, req.Currency, req.CustomerNotes, req.OverdueCharge,
		req.Total, req.TaxTotal, req.SubTotal,
	)
	if err != nil {
		return nil, fmt.Errorf("create_invoice insert: %w", err)
	}

	for i, item := range req.LineItems {
		itemID, _ := gonanoid.New()
		_, err = tx.Exec(`
			INSERT INTO invoiceLineItems (id, invoiceId, description, quantity, unitPrice, taxRate, position)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			itemID, req.ID, item.Description, item.Quantity, item.UnitPrice, item.TaxRate, i,
		)
		if err != nil {
			return nil, fmt.Errorf("create_invoice line_item: %w", err)
		}
	}

	_, err = tx.Exec(
		`UPDATE organizations SET invoice_number_counter = invoice_number_counter + 1 WHERE id = ?`,
		req.OrganizationID,
	)
	if err != nil {
		return nil, fmt.Errorf("create_invoice counter: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("create_invoice commit: %w", err)
	}

	return d.GetInvoice(req.ID)
}

func (d *Database) UpdateInvoice(invoiceID string, updates UpdateInvoiceRequest) (*Invoice, error) {
	// If any financial field is being touched, validate the *effective*
	// resulting state — falling back to what's already stored for whichever
	// of lineItems/total/taxTotal/subTotal isn't part of this request. A
	// partial update (e.g. totals only, or line items only) must not be able
	// to bypass validation by omitting the field that would catch it.
	if updates.LineItems != nil || updates.SubTotal != nil || updates.TaxTotal != nil || updates.Total != nil {
		lineItems := updates.LineItems
		if lineItems == nil {
			stored, err := d.GetInvoiceLineItems(invoiceID)
			if err != nil {
				return nil, fmt.Errorf("update_invoice fetch line items: %w", err)
			}
			converted := make([]CreateInvoiceLineItemRequest, len(stored))
			for i, item := range stored {
				converted[i] = CreateInvoiceLineItemRequest{
					Description: item.Description,
					Quantity:    item.Quantity,
					UnitPrice:   float64(item.UnitPrice),
					TaxRate:     item.TaxRate,
				}
			}
			lineItems = &converted
		}

		subTotal, taxTotal, total := updates.SubTotal, updates.TaxTotal, updates.Total
		if subTotal == nil || taxTotal == nil || total == nil {
			current, err := d.GetInvoice(invoiceID)
			if err != nil {
				return nil, fmt.Errorf("update_invoice fetch current: %w", err)
			}
			if subTotal == nil {
				subTotal = &current.SubTotal
			}
			if taxTotal == nil {
				taxTotal = &current.TaxTotal
			}
			if total == nil {
				total = &current.Total
			}
		}

		if err := d.validateInvoiceTotals(*lineItems, *subTotal, *taxTotal, *total); err != nil {
			return nil, err
		}
	}

	tx, err := d.DB.Beginx()
	if err != nil {
		return nil, fmt.Errorf("update_invoice begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Required fields use COALESCE (kept when nil); nullable fields are set
	// directly so users can clear them by passing null.
	_, err = tx.Exec(`
		UPDATE invoices
		SET number        = COALESCE(?, number),
		    clientId      = COALESCE(?, clientId),
		    date          = COALESCE(?, date),
		    dueDate       = ?,
		    currency      = COALESCE(?, currency),
		    customerNotes = ?,
		    overdueCharge = ?,
		    total         = COALESCE(?, total),
		    taxTotal      = COALESCE(?, taxTotal),
		    subTotal      = COALESCE(?, subTotal)
		WHERE id = ?`,
		updates.Number, updates.ClientID,
		updates.Date, updates.DueDate, updates.Currency,
		updates.CustomerNotes, updates.OverdueCharge,
		updates.Total, updates.TaxTotal, updates.SubTotal,
		invoiceID,
	)
	if err != nil {
		return nil, fmt.Errorf("update_invoice exec: %w", err)
	}

	if updates.LineItems != nil {
		if _, err = tx.Exec(`DELETE FROM invoiceLineItems WHERE invoiceId = ?`, invoiceID); err != nil {
			return nil, fmt.Errorf("update_invoice delete_items: %w", err)
		}
		for i, item := range *updates.LineItems {
			itemID, _ := gonanoid.New()
			_, err = tx.Exec(`
				INSERT INTO invoiceLineItems (id, invoiceId, description, quantity, unitPrice, taxRate, position)
				VALUES (?, ?, ?, ?, ?, ?, ?)`,
				itemID, invoiceID, item.Description, item.Quantity, item.UnitPrice, item.TaxRate, i,
			)
			if err != nil {
				return nil, fmt.Errorf("update_invoice line_item: %w", err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("update_invoice commit: %w", err)
	}

	return d.GetInvoice(invoiceID)
}

func (d *Database) UpdateInvoiceState(invoiceID string, state string) (*Invoice, error) {
	if !invoiceStates[state] {
		return nil, newValidationError("invalid invoice state %q", state)
	}
	_, err := d.DB.Exec(`UPDATE invoices SET state = ? WHERE id = ?`, state, invoiceID)
	if err != nil {
		return nil, fmt.Errorf("update_invoice_state: %w", err)
	}
	return d.GetInvoice(invoiceID)
}

func (d *Database) DeleteInvoice(invoiceID string) (bool, error) {
	current, err := d.GetInvoice(invoiceID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	if current.State == "paid" {
		return false, newValidationError("cannot delete a paid invoice — cancel it instead")
	}

	tx, err := d.DB.Beginx()
	if err != nil {
		return false, fmt.Errorf("delete_invoice begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err = tx.Exec(`DELETE FROM invoiceLineItems WHERE invoiceId = ?`, invoiceID); err != nil {
		return false, fmt.Errorf("delete_invoice items: %w", err)
	}

	res, err := tx.Exec(`DELETE FROM invoices WHERE id = ?`, invoiceID)
	if err != nil {
		return false, fmt.Errorf("delete_invoice: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("delete_invoice commit: %w", err)
	}

	n, _ := res.RowsAffected()
	return n > 0, nil
}
