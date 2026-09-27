package db

import (
	"errors"
	"fmt"
	"math"
	"sort"

	"github.com/jmoiron/sqlx"
	gonanoid "github.com/matoous/go-nanoid/v2"
)

// A Cash Book sale (invoices.movesStock = 1, migration 0094) takes its own
// stock out: at a counter there is no delivery document to do it. Stock
// presence follows the invoice state exactly like its GL presence does
// (needsInvoiceGLPresence / UpdateInvoiceState): while the invoice is sent or
// paid, one "out" movement per stock-enabled line stands against it; moving
// it to draft/cancelled reverses whatever was posted, and moving it back
// posts again. Every movement carries sourceDocumentId = the invoice id —
// the only reliable way back to exactly what this sale moved.
//
// Decided with the owner (2026-09-27): a sale is never blocked for lack of
// recorded STOCK — the counter must keep selling when the records are off,
// and negative stock is the visible signal that a count is due. Serialized
// products are refused at the counter until it has a serial picker; sell
// them through a delivery. In perpetual valuation the stock-out also posts
// COGS (source type "invoice_cogs", since "invoice" is the sale's own
// revenue entry), so an uncosted stock product is refused with the same
// no-cost-basis 409 as a delivery — the message names both ways out: give
// the product a unit cost, or switch the organization to Quantities only
// (allowed while nothing is posted to its Inventory account, see
// checkInventoryValuationSwitch). In quantity-only valuation it posts
// nothing and needs no cost.

// invoiceCOGSSourceType is the journal_entries.sourceDocumentType of a Cash Book
// sale's COGS entry — distinct from the sale's own "invoice" entry, which
// the partial unique index on (sourceDocumentType, sourceDocumentId) would
// otherwise collide with.
const invoiceCOGSSourceType = "invoice_cogs"

// invoiceStockQuantityEpsilon: stockQuantity and movement quantities are
// REAL, so "nothing posted" is a tolerance, the same one UpdateProduct uses.
const invoiceStockQuantityEpsilon = 1e-9

// saleLabel names the sale in movement notes/references. An organization
// with no invoice number format gets an empty number from CreateCashSale
// (a separate, pre-existing gap), so fall back to no reference rather than
// writing an empty one.
func saleLabel(invoice *Invoice) (label string, reference *string) {
	if invoice.Number == "" {
		return "Cash sale", nil
	}
	return "Cash sale " + invoice.Number, &invoice.Number
}

func needsInvoiceStockPresence(invoice *Invoice, state string) bool {
	return invoice.MovesStock == 1 && (state == "sent" || state == "paid")
}

// invoiceStockItem is the product-bearing part of an invoice line, whether
// it comes from a CreateCashSale request (not yet stored) or from stored
// invoiceLineItems.
type invoiceStockItem struct {
	ProductID *string
	Quantity  float64
}

// resolveInvoiceStockLines turns an invoice's lines into the stock lines its
// stock-out moves — only lines linked to a stock-enabled product. A
// serialized product or a non-positive quantity on such a line is refused
// with a 409. Reads only, so it runs before any transaction opens.
func (d *Database) resolveInvoiceStockLines(items []invoiceStockItem) ([]deliveryStockLine, error) {
	lines := []deliveryStockLine{}
	for _, item := range items {
		if item.ProductID == nil || *item.ProductID == "" {
			continue
		}
		product, err := d.GetProduct(*item.ProductID)
		if err != nil {
			return nil, newValidationError("product not found")
		}
		if product.StockEnabled != 1 {
			continue
		}
		if product.Serialized == 1 {
			return nil, newValidationError(
				"%q is serialized — the Cash Book can't pick serial numbers yet; sell it through a delivery instead", product.Name,
			)
		}
		if item.Quantity <= 0 {
			return nil, newValidationError("%q: a stock-tracked line needs a quantity above zero", product.Name)
		}
		lines = append(lines, deliveryStockLine{
			ProductID: product.ID, ProductName: product.Name, Quantity: item.Quantity,
		})
	}
	return lines, nil
}

// invoiceStockPlan is everything posting an invoice's stock-out needs,
// built before the transaction opens (db.SetMaxOpenConns(1): no d.DB read
// may happen between Beginx and Commit).
type invoiceStockPlan struct {
	lines       []deliveryStockLine
	cogsLines   []CreateJournalLineRequest
	cogsJournal *Journal
}

func (d *Database) planInvoiceStockOut(organizationID string, lines []deliveryStockLine) (*invoiceStockPlan, error) {
	plan := &invoiceStockPlan{lines: lines}
	if len(lines) == 0 {
		return plan, nil
	}
	// buildCOGSGLLines returns nothing in quantity-only valuation, and a
	// 409 in perpetual valuation when a line has no cost basis — before any
	// stock is touched.
	cogsLines, journal, err := buildCOGSGLLines(d, organizationID, lines, nil)
	if err != nil {
		var verr *ValidationError
		if errors.As(err, &verr) {
			return nil, newValidationError(
				"%s — give the product a unit cost, or switch the organization's inventory valuation to Quantities only",
				verr.Error(),
			)
		}
		return nil, err
	}
	plan.cogsLines, plan.cogsJournal = cogsLines, journal
	return plan, nil
}

// postedInvoiceStockTx returns the net quantity this invoice's movements
// currently stand at, per product (negative while its stock-out stands),
// dropping products that net to zero. Read inside the caller's transaction,
// so the post/reverse decision is made from the same connection that writes.
func postedInvoiceStockTx(exec sqlSelectExecer, invoiceID string) (map[string]float64, error) {
	var rows []struct {
		ProductID string  `db:"productId"`
		Net       float64 `db:"net"`
	}
	if err := exec.Select(&rows, `
		SELECT productId, SUM(quantity) AS net FROM stockMovements
		WHERE sourceDocumentId = ? GROUP BY productId`, invoiceID,
	); err != nil {
		return nil, fmt.Errorf("posted_invoice_stock: %w", err)
	}
	posted := map[string]float64{}
	for _, r := range rows {
		if math.Abs(r.Net) > invoiceStockQuantityEpsilon {
			posted[r.ProductID] = r.Net
		}
	}
	return posted, nil
}

// applyInvoiceStockOutTx posts the stock-out (and its COGS entry, if any).
// No availability check, by decision — see this file's header.
func applyInvoiceStockOutTx(tx *sqlx.Tx, invoice *Invoice, plan *invoiceStockPlan) error {
	label, reference := saleLabel(invoice)
	touched := map[string]bool{}
	for _, line := range plan.lines {
		movementID, _ := gonanoid.New()
		if err := insertStockMovementTx(tx, CreateStockMovementRequest{
			ID:               movementID,
			OrganizationID:   invoice.OrganizationID,
			ProductID:        line.ProductID,
			Type:             "out",
			Quantity:         -line.Quantity,
			Note:             ptrStr(label),
			Reference:        reference,
			SourceDocumentID: &invoice.ID,
		}); err != nil {
			return fmt.Errorf("invoice_stock_out: %w", err)
		}
		touched[line.ProductID] = true
	}
	if err := recomputeAverageCostForTx(tx, touched); err != nil {
		return err
	}
	if plan.cogsLines != nil {
		if _, err := postAutoEntryTx(
			tx, invoice.OrganizationID, plan.cogsJournal.ID, invoiceCOGSSourceType, invoice.ID,
			invoice.Date, invoice.Number, label, plan.cogsLines,
		); err != nil {
			return err
		}
	}
	return nil
}

// reverseInvoiceStockOutTx puts back exactly what this invoice's movements
// still net to — never re-derived from its current line items — and
// reverses its COGS entry, if one is posted. Restored units come back
// uncosted, at the running average, like a cancelled delivery's.
func reverseInvoiceStockOutTx(tx *sqlx.Tx, invoice *Invoice, posted map[string]float64, reason string) error {
	productIDs := make([]string, 0, len(posted))
	for id := range posted {
		productIDs = append(productIDs, id)
	}
	sort.Strings(productIDs) // deterministic insertion order
	label, reference := saleLabel(invoice)
	touched := map[string]bool{}
	for _, productID := range productIDs {
		movementID, _ := gonanoid.New()
		net := posted[productID]
		movementType := "in"
		if net > 0 {
			movementType = "out"
		}
		if err := insertStockMovementTx(tx, CreateStockMovementRequest{
			ID:               movementID,
			OrganizationID:   invoice.OrganizationID,
			ProductID:        productID,
			Type:             movementType,
			Quantity:         -net,
			Note:             ptrStr(label + " " + reason),
			Reference:        reference,
			SourceDocumentID: &invoice.ID,
		}); err != nil {
			return fmt.Errorf("invoice_stock_restore: %w", err)
		}
		touched[productID] = true
	}
	if err := recomputeAverageCostForTx(tx, touched); err != nil {
		return err
	}
	entry, err := findPostedEntryForSourceDocumentTx(tx, invoiceCOGSSourceType, invoice.ID)
	if err != nil {
		return err
	}
	if entry != nil {
		if _, err := reverseEntryTx(tx, entry, "cash sale "+reason, invoice.Date); err != nil {
			return err
		}
	}
	return nil
}

func recomputeAverageCostForTx(tx sqlSelectExecer, products map[string]bool) error {
	ids := make([]string, 0, len(products))
	for id := range products {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if err := recomputeAverageCostTx(tx, id); err != nil {
			return fmt.Errorf("invoice_stock recompute_cost: %w", err)
		}
	}
	return nil
}

// invoiceHasPostedStock is the pre-tx check DeleteInvoice/UpdateInvoice use
// to refuse removing or re-lining a sale whose stock-out still stands — the
// GL guard already covers most of those, but not a zero-total sale, which
// has no GL entry yet can still move stock.
func (d *Database) invoiceHasPostedStock(invoiceID string) (bool, error) {
	posted, err := postedInvoiceStockTx(d.DB, invoiceID)
	if err != nil {
		return false, err
	}
	return len(posted) > 0, nil
}
