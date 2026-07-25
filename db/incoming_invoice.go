package db

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	gonanoid "github.com/matoous/go-nanoid/v2"
)

// incomingInvoiceStates is the canonical set. Like sales invoices — and unlike
// orders and receipts — there is no transition matrix: a bounced payment can
// legitimately send paid→approved, so only set membership is enforced.
var incomingInvoiceStates = map[string]bool{
	"draft":     true,
	"approved":  true,
	"paid":      true,
	"cancelled": true,
}

type IncomingInvoice struct {
	ID                  string  `db:"id"                  json:"id"`
	OrganizationID      string  `db:"organizationId"      json:"organizationId"`
	VendorID            string  `db:"vendorId"            json:"vendorId"`
	PurchaseOrderID     *string `db:"purchaseOrderId"     json:"purchaseOrderId"`
	VendorInvoiceNumber string  `db:"vendorInvoiceNumber" json:"vendorInvoiceNumber"`
	Reference           *string `db:"reference"           json:"reference"`
	State               string  `db:"state"               json:"state"`
	Date                int64   `db:"date"                json:"date"`
	DueDate             *int64  `db:"dueDate"             json:"dueDate"`
	Currency            string  `db:"currency"            json:"currency"`
	Notes               *string `db:"notes"               json:"notes"`
	Total               int64   `db:"total"               json:"total"`
	TaxTotal            int64   `db:"taxTotal"            json:"taxTotal"`
	SubTotal            int64   `db:"subTotal"            json:"subTotal"`
	MatchOverride       int     `db:"matchOverride"       json:"matchOverride"`
	MatchOverrideReason *string `db:"matchOverrideReason" json:"matchOverrideReason"`
	CreatedAt           int64   `db:"createdAt"           json:"createdAt"`
	// Joined
	VendorName  *string `db:"vendorName"  json:"vendorName"`
	OrderNumber *string `db:"orderNumber" json:"orderNumber"`
}

type IncomingInvoiceLineItem struct {
	ID                      string  `db:"id"                      json:"id"`
	IncomingInvoiceID       string  `db:"incomingInvoiceId"       json:"incomingInvoiceId"`
	PurchaseOrderLineItemID *string `db:"purchaseOrderLineItemId" json:"purchaseOrderLineItemId"`
	ProductID               *string `db:"productId"               json:"productId"`
	Description             string  `db:"description"             json:"description"`
	Quantity                float64 `db:"quantity"                json:"quantity"`
	UnitPrice               int64   `db:"unitPrice"               json:"unitPrice"`
	TaxRate                 *string `db:"taxRate"                 json:"taxRate"`
	Position                int     `db:"position"                json:"position"`
}

type CreateIncomingInvoiceRequest struct {
	ID                  string                         `json:"id"`
	OrganizationID      string                         `json:"organizationId"`
	VendorID            string                         `json:"vendorId"`
	PurchaseOrderID     *string                        `json:"purchaseOrderId"`
	VendorInvoiceNumber string                         `json:"vendorInvoiceNumber"`
	Reference           *string                        `json:"reference"`
	State               string                         `json:"state"`
	Date                int64                          `json:"date"`
	DueDate             *int64                         `json:"dueDate"`
	Currency            string                         `json:"currency"`
	Notes               *string                        `json:"notes"`
	Total               int64                          `json:"total"`
	TaxTotal            int64                          `json:"taxTotal"`
	SubTotal            int64                          `json:"subTotal"`
	MatchOverride       int                            `json:"matchOverride"`
	MatchOverrideReason *string                        `json:"matchOverrideReason"`
	LineItems           []CreateInvoiceLineItemRequest `json:"lineItems"`
}

// UpdateIncomingInvoiceRequest has no State field — state changes go through
// PATCH only, which is where the matching gate lives.
type UpdateIncomingInvoiceRequest struct {
	VendorID            *string                         `json:"vendorId"`
	PurchaseOrderID     *string                         `json:"purchaseOrderId"`
	VendorInvoiceNumber *string                         `json:"vendorInvoiceNumber"`
	Reference           *string                         `json:"reference"`
	Date                *int64                          `json:"date"`
	DueDate             *int64                          `json:"dueDate"`
	Currency            *string                         `json:"currency"`
	Notes               *string                         `json:"notes"`
	Total               *int64                          `json:"total"`
	TaxTotal            *int64                          `json:"taxTotal"`
	SubTotal            *int64                          `json:"subTotal"`
	MatchOverride       *int                            `json:"matchOverride"`
	MatchOverrideReason *string                         `json:"matchOverrideReason"`
	LineItems           *[]CreateInvoiceLineItemRequest `json:"lineItems"`
}

// ErrDuplicateVendorInvoiceNumber is returned when a vendor bills the same
// number twice, which the unique index catches.
var ErrDuplicateVendorInvoiceNumber = errors.New("this vendor invoice number already exists")

const incomingInvoiceSelect = `
	SELECT ii.*, v.name AS vendorName, po.orderNumber AS orderNumber
	FROM incoming_invoices ii
	INNER JOIN vendors v ON ii.vendorId = v.id
	LEFT JOIN purchase_orders po ON ii.purchaseOrderId = po.id`

func (d *Database) GetIncomingInvoices(organizationID string) ([]IncomingInvoice, error) {
	rows := []IncomingInvoice{}
	err := d.DB.Select(&rows, incomingInvoiceSelect+`
		WHERE ii.organizationId = ?
		ORDER BY ii.date DESC, ii.createdAt DESC`,
		organizationID,
	)
	if err != nil {
		return nil, fmt.Errorf("get_incoming_invoices: %w", err)
	}
	return rows, nil
}

func (d *Database) GetIncomingInvoice(id string) (*IncomingInvoice, error) {
	var row IncomingInvoice
	err := d.DB.Get(&row, incomingInvoiceSelect+` WHERE ii.id = ? LIMIT 1`, id)
	if err != nil {
		return nil, fmt.Errorf("get_incoming_invoice: %w", err)
	}
	return &row, nil
}

func (d *Database) GetIncomingInvoiceLineItems(invoiceID string) ([]IncomingInvoiceLineItem, error) {
	items := []IncomingInvoiceLineItem{}
	err := d.DB.Select(&items,
		`SELECT * FROM incoming_invoice_line_items WHERE incomingInvoiceId = ? ORDER BY position ASC`,
		invoiceID,
	)
	if err != nil {
		return nil, fmt.Errorf("get_incoming_invoice_line_items: %w", err)
	}
	return items, nil
}

// validateOverride rejects an override with no reason: the reason is the entire
// audit value of the escape hatch, so allowing a blank one would make it a
// silent bypass.
func validateOverride(override int, reason *string) error {
	if override != 1 {
		return nil
	}
	if reason == nil || strings.TrimSpace(*reason) == "" {
		return newValidationError("a reason is required when overriding a matching variance")
	}
	return nil
}

func (d *Database) CreateIncomingInvoice(req CreateIncomingInvoiceRequest) (*IncomingInvoice, error) {
	if req.ID == "" {
		req.ID, _ = gonanoid.New()
	}
	if req.State == "" {
		req.State = "draft"
	}
	if !incomingInvoiceStates[req.State] {
		return nil, newValidationError("invalid incoming invoice state %q", req.State)
	}
	if err := validateOverride(req.MatchOverride, req.MatchOverrideReason); err != nil {
		return nil, err
	}
	// Reuses the sales-invoice check verbatim: exact rational arithmetic,
	// tax rounded once per tax-rate group.
	if err := d.validateInvoiceTotals(req.LineItems, req.SubTotal, req.TaxTotal, req.Total); err != nil {
		return nil, err
	}

	tx, err := d.DB.Beginx()
	if err != nil {
		return nil, fmt.Errorf("create_incoming_invoice begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	_, err = tx.Exec(`
		INSERT INTO incoming_invoices (
			id, organizationId, vendorId, purchaseOrderId, vendorInvoiceNumber, reference,
			state, date, dueDate, currency, notes, total, taxTotal, subTotal,
			matchOverride, matchOverrideReason
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		req.ID, req.OrganizationID, req.VendorID, req.PurchaseOrderID,
		req.VendorInvoiceNumber, req.Reference, req.State, req.Date, req.DueDate,
		req.Currency, req.Notes, req.Total, req.TaxTotal, req.SubTotal,
		req.MatchOverride, req.MatchOverrideReason,
	)
	if err != nil {
		if isUniqueConstraintError(err) {
			return nil, ErrDuplicateVendorInvoiceNumber
		}
		return nil, fmt.Errorf("create_incoming_invoice insert: %w", err)
	}
	if err := replaceIncomingInvoiceLineItemsTx(tx, req.ID, req.LineItems); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("create_incoming_invoice commit: %w", err)
	}
	return d.GetIncomingInvoice(req.ID)
}

func (d *Database) UpdateIncomingInvoice(id string, updates UpdateIncomingInvoiceRequest) (*IncomingInvoice, error) {
	// Validate the *effective* resulting state whenever any financial field is
	// touched, filling in whichever of lineItems/total/taxTotal/subTotal this
	// request omits from what's stored — so a partial update can't bypass the
	// check by leaving out the field that would catch it. Mirrors UpdateInvoice.
	if updates.LineItems != nil || updates.SubTotal != nil || updates.TaxTotal != nil || updates.Total != nil {
		lineItems := updates.LineItems
		if lineItems == nil {
			stored, err := d.GetIncomingInvoiceLineItems(id)
			if err != nil {
				return nil, fmt.Errorf("update_incoming_invoice fetch line items: %w", err)
			}
			converted := make([]CreateInvoiceLineItemRequest, len(stored))
			for i, item := range stored {
				description := item.Description
				converted[i] = CreateInvoiceLineItemRequest{
					Description: &description,
					Quantity:    item.Quantity,
					UnitPrice:   float64(item.UnitPrice),
					TaxRate:     item.TaxRate,
				}
			}
			lineItems = &converted
		}

		subTotal, taxTotal, total := updates.SubTotal, updates.TaxTotal, updates.Total
		if subTotal == nil || taxTotal == nil || total == nil {
			current, err := d.GetIncomingInvoice(id)
			if err != nil {
				return nil, fmt.Errorf("update_incoming_invoice fetch current: %w", err)
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

	// An override justifies one specific variance. Editing the financials can
	// turn it into a different one, so anything that changes what would be
	// matched clears the flag unless this same request re-states it — otherwise
	// an invoice could be edited after approval and re-approved against a
	// reason that no longer describes it.
	financialsChanged := updates.LineItems != nil || updates.SubTotal != nil ||
		updates.TaxTotal != nil || updates.Total != nil || updates.PurchaseOrderID != nil
	clearOverride := financialsChanged && updates.MatchOverride == nil

	// The override flag and its reason must be judged together, using whichever
	// side this request doesn't supply.
	if updates.MatchOverride != nil || updates.MatchOverrideReason != nil {
		current, err := d.GetIncomingInvoice(id)
		if err != nil {
			return nil, fmt.Errorf("update_incoming_invoice fetch current: %w", err)
		}
		override := current.MatchOverride
		if updates.MatchOverride != nil {
			override = *updates.MatchOverride
		}
		reason := current.MatchOverrideReason
		if updates.MatchOverrideReason != nil {
			reason = updates.MatchOverrideReason
		}
		if err := validateOverride(override, reason); err != nil {
			return nil, err
		}
	}

	tx, err := d.DB.Beginx()
	if err != nil {
		return nil, fmt.Errorf("update_incoming_invoice begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	_, err = tx.Exec(`
		UPDATE incoming_invoices SET
		  vendorId            = COALESCE(?, vendorId),
		  purchaseOrderId     = COALESCE(?, purchaseOrderId),
		  vendorInvoiceNumber = COALESCE(?, vendorInvoiceNumber),
		  reference           = COALESCE(?, reference),
		  date                = COALESCE(?, date),
		  dueDate             = COALESCE(?, dueDate),
		  currency            = COALESCE(?, currency),
		  notes               = COALESCE(?, notes),
		  total               = COALESCE(?, total),
		  taxTotal            = COALESCE(?, taxTotal),
		  subTotal            = COALESCE(?, subTotal),
		  matchOverride       = COALESCE(?, matchOverride),
		  matchOverrideReason = COALESCE(?, matchOverrideReason)
		WHERE id = ?`,
		updates.VendorID, updates.PurchaseOrderID, updates.VendorInvoiceNumber, updates.Reference,
		updates.Date, updates.DueDate, updates.Currency, updates.Notes,
		updates.Total, updates.TaxTotal, updates.SubTotal,
		updates.MatchOverride, updates.MatchOverrideReason,
		id,
	)
	if err != nil {
		if isUniqueConstraintError(err) {
			return nil, ErrDuplicateVendorInvoiceNumber
		}
		return nil, fmt.Errorf("update_incoming_invoice: %w", err)
	}
	// Cleared after the COALESCE'd update above, which by design can't null a
	// column out.
	if clearOverride {
		if _, err := tx.Exec(
			`UPDATE incoming_invoices SET matchOverride = 0, matchOverrideReason = NULL WHERE id = ?`,
			id,
		); err != nil {
			return nil, fmt.Errorf("update_incoming_invoice clear_override: %w", err)
		}
	}

	if updates.LineItems != nil {
		if err := replaceIncomingInvoiceLineItemsTx(tx, id, *updates.LineItems); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("update_incoming_invoice commit: %w", err)
	}
	return d.GetIncomingInvoice(id)
}

// UpdateIncomingInvoiceState is where 3-way matching is enforced. Saving a
// draft with variances is deliberately allowed so they can be investigated;
// approving (or paying without having approved) is not, unless the invoice
// carries an explicit override with a reason.
func (d *Database) UpdateIncomingInvoiceState(id, state string) (*IncomingInvoice, error) {
	if !incomingInvoiceStates[state] {
		return nil, newValidationError("invalid incoming invoice state %q", state)
	}

	current, err := d.GetIncomingInvoice(id)
	if err != nil {
		return nil, fmt.Errorf("update_incoming_invoice_state lookup: %w", err)
	}

	gated := state == "approved" || (state == "paid" && current.State != "approved")
	if gated && current.MatchOverride != 1 {
		lines, err := d.GetIncomingInvoiceMatch(id)
		if err != nil {
			return nil, err
		}
		if offenders := unmatchedLines(lines); len(offenders) > 0 {
			return nil, newValidationError(
				"cannot approve: %s. Resolve the variance or record an override with a reason.",
				strings.Join(offenders, "; "),
			)
		}
	}

	if _, err := d.DB.Exec(`UPDATE incoming_invoices SET state = ? WHERE id = ?`, state, id); err != nil {
		return nil, fmt.Errorf("update_incoming_invoice_state: %w", err)
	}
	return d.GetIncomingInvoice(id)
}

func (d *Database) DeleteIncomingInvoice(id string) (bool, error) {
	res, err := d.DB.Exec(`DELETE FROM incoming_invoices WHERE id = ?`, id)
	if err != nil {
		return false, fmt.Errorf("delete_incoming_invoice: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func replaceIncomingInvoiceLineItemsTx(
	exec sqlGetExecer, invoiceID string, items []CreateInvoiceLineItemRequest,
) error {
	if _, err := exec.Exec(
		`DELETE FROM incoming_invoice_line_items WHERE incomingInvoiceId = ?`, invoiceID,
	); err != nil {
		return fmt.Errorf("delete_incoming_invoice_line_items: %w", err)
	}

	for i, item := range items {
		itemID, _ := gonanoid.New()
		description := ""
		if item.Description != nil {
			description = *item.Description
		}
		productID := item.ProductID
		// A line linked to a purchase order inherits that line's product when
		// it doesn't name one, so matching and stock reporting agree.
		if productID == nil && item.PurchaseOrderLineItemID != nil {
			var resolved sql.NullString
			if err := exec.Get(&resolved,
				`SELECT productId FROM purchase_order_line_items WHERE id = ?`,
				*item.PurchaseOrderLineItemID,
			); err == nil && resolved.Valid {
				productID = &resolved.String
			}
		}

		if _, err := exec.Exec(`
			INSERT INTO incoming_invoice_line_items
			  (id, incomingInvoiceId, purchaseOrderLineItemId, productId, description,
			   quantity, unitPrice, taxRate, position)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			itemID, invoiceID, item.PurchaseOrderLineItemID, productID, description,
			item.Quantity, int64(item.UnitPrice), item.TaxRate, i,
		); err != nil {
			return fmt.Errorf("insert_incoming_invoice_line_item: %w", err)
		}
	}
	return nil
}

func isUniqueConstraintError(err error) bool {
	return err != nil && strings.Contains(strings.ToUpper(err.Error()), "UNIQUE CONSTRAINT FAILED")
}
