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
	ID             string `db:"id"             json:"id"`
	OrganizationID string `db:"organizationId" json:"organizationId"`
	Number         string `db:"number"         json:"number"`
	State          string `db:"state"          json:"state"`
	ClientID       string `db:"clientId"       json:"clientId"`
	Date           int64  `db:"date"           json:"date"`
	DueDate        *int64 `db:"dueDate"        json:"dueDate"`
	Currency       string `db:"currency"       json:"currency"`
	// See db/exchange_rate.go for the rate direction convention. Nil when
	// Currency equals the organization's own currency.
	ExchangeRate     *string  `db:"exchangeRate"     json:"exchangeRate"`
	ExchangeRateDate *int64   `db:"exchangeRateDate" json:"exchangeRateDate"`
	CustomerNotes    *string  `db:"customerNotes"  json:"customerNotes"`
	OverdueCharge    *float64 `db:"overdueCharge"  json:"overdueCharge"`
	Total            int64    `db:"total"          json:"total"`
	TaxTotal         int64    `db:"taxTotal"       json:"taxTotal"`
	SubTotal         int64    `db:"subTotal"       json:"subTotal"`
	CreatedAt        *string  `db:"createdAt"      json:"createdAt"`
	ClientName       *string  `db:"clientName"     json:"clientName"`

	// BT-10 buyer reference (e.g. a German Leitweg-ID), mandatory for
	// XRechnung/B2G. BT-20 payment terms free text; BT-9 due date is dueDate.
	BuyerReference *string `db:"buyerReference" json:"buyerReference"`
	PaymentTerms   *string `db:"paymentTerms"   json:"paymentTerms"`
}

// InvoiceLineItem mirrors the invoiceLineItems table.
type InvoiceLineItem struct {
	ID          string  `db:"id"          json:"id"`
	InvoiceID   string  `db:"invoiceId"   json:"invoiceId"`
	Description *string `db:"description" json:"description"`
	Quantity    float64 `db:"quantity"    json:"quantity"`
	UnitPrice   int64   `db:"unitPrice"   json:"unitPrice"`
	TaxRate     *string `db:"taxRate"     json:"taxRate"`
	ProductID   *string `db:"productId"   json:"productId"`
	Position    int     `db:"position"    json:"position"`
	CreatedAt   *string `db:"createdAt"   json:"createdAt"`
}

// CreateInvoiceLineItemRequest is a single line item within a create/update
// request, shared by sales invoices and incoming (vendor) invoices so both go
// through the same validateInvoiceTotals check rather than duplicating its
// exact-rational arithmetic.
//
// PurchaseOrderLineItemID is used only by incoming invoices — it's what 3-way
// matching links against — and stays nil on the sales path. ProductID is
// persisted on both paths.
type CreateInvoiceLineItemRequest struct {
	Description             *string `json:"description"`
	Quantity                float64 `json:"quantity"`
	UnitPrice               float64 `json:"unitPrice"`
	TaxRate                 *string `json:"taxRate"`
	PurchaseOrderLineItemID *string `json:"purchaseOrderLineItemId"`
	ProductID               *string `json:"productId"`
}

// CreateInvoiceRequest is the payload for creating an invoice.
type CreateInvoiceRequest struct {
	ID               string                         `json:"id"`
	OrganizationID   string                         `json:"organizationId"`
	Number           string                         `json:"number"`
	State            string                         `json:"state"`
	ClientID         string                         `json:"clientId"`
	Date             int64                          `json:"date"`
	DueDate          *int64                         `json:"dueDate"`
	Currency         string                         `json:"currency"`
	ExchangeRate     *float64                       `json:"exchangeRate"`
	ExchangeRateDate *int64                         `json:"exchangeRateDate"`
	CustomerNotes    *string                        `json:"customerNotes"`
	OverdueCharge    *float64                       `json:"overdueCharge"`
	Total            int64                          `json:"total"`
	TaxTotal         int64                          `json:"taxTotal"`
	SubTotal         int64                          `json:"subTotal"`
	LineItems        []CreateInvoiceLineItemRequest `json:"lineItems"`
	BuyerReference   *string                        `json:"buyerReference"`
	PaymentTerms     *string                        `json:"paymentTerms"`
}

// UpdateInvoiceRequest is the payload for updating an invoice. State is
// deliberately absent — state changes go through PATCH /api/invoices/{id}/state
// only (which validates against invoiceStates), matching the orders/deliveries
// convention, so a PUT can't set an arbitrary state and bypass validation.
type UpdateInvoiceRequest struct {
	Number           *string                         `json:"number"`
	ClientID         *string                         `json:"clientId"`
	Date             *int64                          `json:"date"`
	DueDate          *int64                          `json:"dueDate"`
	Currency         *string                         `json:"currency"`
	ExchangeRate     *float64                        `json:"exchangeRate"`
	ExchangeRateDate *int64                          `json:"exchangeRateDate"`
	CustomerNotes    *string                         `json:"customerNotes"`
	OverdueCharge    *float64                        `json:"overdueCharge"`
	Total            *int64                          `json:"total"`
	TaxTotal         *int64                          `json:"taxTotal"`
	SubTotal         *int64                          `json:"subTotal"`
	LineItems        *[]CreateInvoiceLineItemRequest `json:"lineItems"`
	BuyerReference   *string                         `json:"buyerReference"`
	PaymentTerms     *string                         `json:"paymentTerms"`
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
	org, err := d.GetOrganization(req.OrganizationID)
	if err != nil {
		return nil, fmt.Errorf("create_invoice organization: %w", err)
	}
	exchangeRate, err := resolveExchangeRateForSave(
		orgCurrencyOrDefault(org), "", nil, &req.Currency, req.ExchangeRate,
	)
	if err != nil {
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
			currency, exchangeRate, exchangeRateDate, customerNotes, overdueCharge, total, taxTotal, subTotal,
			buyerReference, paymentTerms
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		req.ID, req.OrganizationID, req.Number, req.State, req.ClientID,
		req.Date, req.DueDate, req.Currency, exchangeRate, req.ExchangeRateDate, req.CustomerNotes, req.OverdueCharge,
		req.Total, req.TaxTotal, req.SubTotal, req.BuyerReference, req.PaymentTerms,
	)
	if err != nil {
		return nil, fmt.Errorf("create_invoice insert: %w", err)
	}

	for i, item := range req.LineItems {
		itemID, _ := gonanoid.New()
		_, err = tx.Exec(`
			INSERT INTO invoiceLineItems (id, invoiceId, description, quantity, unitPrice, taxRate, productId, position)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			itemID, req.ID, item.Description, item.Quantity, roundCents(item.UnitPrice), item.TaxRate, item.ProductID, i,
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

// invoiceUpdateTouchesGLFields reports whether updates would change anything
// a posted GL entry was built from — line items, totals, currency/rate, or
// the client the AR line is tagged with. A header-only edit (notes, due
// date, buyer reference, ...) touches none of these and is always allowed.
func invoiceUpdateTouchesGLFields(updates UpdateInvoiceRequest) bool {
	return updates.LineItems != nil || updates.SubTotal != nil || updates.TaxTotal != nil ||
		updates.Total != nil || updates.Currency != nil || updates.ExchangeRate != nil ||
		updates.ClientID != nil
}

func (d *Database) UpdateInvoice(invoiceID string, updates UpdateInvoiceRequest) (*Invoice, error) {
	// A posted GL entry was built from the invoice's line items, totals,
	// currency, and client — editing any of those out from under it would
	// leave the entry silently wrong (the trial balance would keep the old
	// numbers forever, since nothing re-triggers a re-post on PUT). Reject
	// rather than let it drift; the invoice must be cancelled (which
	// reverses the entry) before a financial edit, then re-sent to re-post.
	if invoiceUpdateTouchesGLFields(updates) {
		entry, err := d.FindPostedEntryForSourceDocument("invoice", invoiceID)
		if err != nil {
			return nil, err
		}
		if entry != nil {
			return nil, newValidationError("cannot change line items, totals, currency, or client on an invoice with a posted GL entry — cancel it instead")
		}
	}

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

	// Same "only when touched" rule as totals above: resolving/validating the
	// exchange rate needs the invoice's current currency+rate only when this
	// request is actually changing one of them.
	var exchangeRate *string
	exchangeRateSet := 0
	if updates.Currency != nil || updates.ExchangeRate != nil {
		current, err := d.GetInvoice(invoiceID)
		if err != nil {
			return nil, fmt.Errorf("update_invoice fetch current: %w", err)
		}
		org, err := d.GetOrganization(current.OrganizationID)
		if err != nil {
			return nil, fmt.Errorf("update_invoice organization: %w", err)
		}
		rate, err := resolveExchangeRateForSave(
			orgCurrencyOrDefault(org), current.Currency, current.ExchangeRate,
			updates.Currency, updates.ExchangeRate,
		)
		if err != nil {
			return nil, err
		}
		exchangeRate, exchangeRateSet = rate, 1
	}

	tx, err := d.DB.Beginx()
	if err != nil {
		return nil, fmt.Errorf("update_invoice begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Required fields use COALESCE (kept when nil); nullable fields are set
	// directly so users can clear them by passing null. exchangeRate/
	// exchangeRateDate are only written when this request touches
	// currency/exchangeRate — otherwise COALESCE keeps what's stored.
	_, err = tx.Exec(`
		UPDATE invoices
		SET number        = COALESCE(?, number),
		    clientId      = COALESCE(?, clientId),
		    date          = COALESCE(?, date),
		    dueDate       = ?,
		    currency      = COALESCE(?, currency),
		    exchangeRate     = CASE WHEN ? THEN ? ELSE exchangeRate END,
		    exchangeRateDate = CASE WHEN ? THEN ? ELSE exchangeRateDate END,
		    customerNotes = ?,
		    overdueCharge = ?,
		    total         = COALESCE(?, total),
		    taxTotal      = COALESCE(?, taxTotal),
		    subTotal      = COALESCE(?, subTotal),
		    buyerReference = ?,
		    paymentTerms   = ?
		WHERE id = ?`,
		updates.Number, updates.ClientID,
		updates.Date, updates.DueDate, updates.Currency,
		exchangeRateSet, exchangeRate,
		exchangeRateSet, updates.ExchangeRateDate,
		updates.CustomerNotes, updates.OverdueCharge,
		updates.Total, updates.TaxTotal, updates.SubTotal,
		updates.BuyerReference, updates.PaymentTerms,
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
				INSERT INTO invoiceLineItems (id, invoiceId, description, quantity, unitPrice, taxRate, productId, position)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
				itemID, invoiceID, item.Description, item.Quantity, roundCents(item.UnitPrice), item.TaxRate, item.ProductID, i,
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

// needsInvoiceGLPresence reports whether state requires a posted GL entry to
// exist. Accrual revenue recognition happens at "sent" (the invoice has been
// issued to the client) and also holds at "paid" — reachable directly from
// "draft" since invoiceStates has no transition matrix, so a jump straight
// to "paid" must post exactly as if it had passed through "sent" first. A
// zero-total invoice has no economic event to record — skipping it also
// avoids handing buildInvoiceGLLines a line set with nothing to post, which
// would otherwise produce a debit=0/credit=0 line that fails journal_lines'
// CHECK constraint.
func needsInvoiceGLPresence(state string, total int64) bool {
	return (state == "sent" || state == "paid") && total != 0
}

// UpdateInvoiceState changes state and, in the same transaction, brings the
// invoice's GL presence in line with it: posts a new entry if the new state
// needs one and none exists yet, reverses the existing one if the new state
// no longer needs it. The check is always "does a posted entry currently
// exist for this document" — never bound to a specific from/to edge — so it
// stays correct for every legal transition, including ones that skip states
// (draft→paid) or bounce backward (paid→sent on a returned payment).
// Building the GL lines (which reads tax rates/products/the organization)
// happens before the transaction opens — db.SetMaxOpenConns(1) means a
// second connection request while a tx holds the only one blocks forever
// rather than erroring, so no d.DB read may happen between Beginx and Commit.
func (d *Database) UpdateInvoiceState(invoiceID string, state string) (*Invoice, error) {
	if !invoiceStates[state] {
		return nil, newValidationError("invalid invoice state %q", state)
	}

	invoice, err := d.GetInvoice(invoiceID)
	if err != nil {
		return nil, fmt.Errorf("update_invoice_state lookup: %w", err)
	}
	existingEntry, err := d.FindPostedEntryForSourceDocument("invoice", invoiceID)
	if err != nil {
		return nil, err
	}
	needsGL := needsInvoiceGLPresence(state, invoice.Total)

	// Reversing the invoice's posted entry while a payment has already
	// settled against it would leave that payment's AR clearing line with no
	// matching original debit — void the payment(s) first.
	if !needsGL && existingEntry != nil {
		paid, err := d.GetInvoiceAmountPaid(invoiceID)
		if err != nil {
			return nil, err
		}
		if paid > 0 {
			return nil, newValidationError(
				"cannot change invoice state: payments totaling %d are still applied to this invoice — void them first", paid,
			)
		}
	}

	var glLines []CreateJournalLineRequest
	var journal *Journal
	if needsGL && existingEntry == nil {
		lineItems, err := d.GetInvoiceLineItems(invoiceID)
		if err != nil {
			return nil, fmt.Errorf("update_invoice_state line_items: %w", err)
		}
		glLines, journal, err = d.buildInvoiceGLLines(invoice, lineItems)
		if err != nil {
			return nil, err
		}
	}

	tx, err := d.DB.Beginx()
	if err != nil {
		return nil, fmt.Errorf("update_invoice_state begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.Exec(`UPDATE invoices SET state = ? WHERE id = ?`, state, invoiceID); err != nil {
		return nil, fmt.Errorf("update_invoice_state exec: %w", err)
	}

	switch {
	case needsGL && existingEntry == nil:
		reference := invoice.Number
		description := "Invoice " + invoice.Number
		if _, err := postAutoEntryTx(
			tx, invoice.OrganizationID, journal.ID, "invoice", invoice.ID, invoice.Date, reference, description, glLines,
		); err != nil {
			return nil, err
		}
	case !needsGL && existingEntry != nil:
		if _, err := reverseEntryTx(tx, existingEntry, "invoice state changed to "+state, invoice.Date); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("update_invoice_state commit: %w", err)
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
	if entry, err := d.FindPostedEntryForSourceDocument("invoice", invoiceID); err != nil {
		return false, err
	} else if entry != nil {
		return false, newValidationError("cannot delete an invoice with a posted GL entry — cancel it instead")
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
