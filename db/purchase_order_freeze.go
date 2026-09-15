package db

import (
	"database/sql"
	"errors"
	"fmt"
)

// Purchase-order line-item freeze.
//
// Once goods have actually been received against a purchase order, that
// order's line items are the basis of postings that already exist: the
// receipt's GRNI accrual (db/gl_posting.go's buildReceiptGRNILines) is
// valued line by line, grniAccrualForPOLine/grniClearedQtyForPOLine net a
// bill against the PO line's own quantity and price, and 3-way matching
// (db/incoming_invoice_match.go) compares each bill line back to it.
// Moving a quantity or a price underneath all of that silently rewrites
// what a posted entry was computed from, with nothing to reconcile against.
//
// F93 (audit 2026-09-14) fixed the worse half of this — an edit used to
// regenerate every line-item id and sever the receipt's link entirely. Id
// reuse keeps the link, but a link to a line whose quantity just changed
// is not the same thing as an unchanged line. This is the other half.
//
// Two deliberate shapes here:
//
//   - The freeze is **computed, never stored** — the same philosophy as
//     products.stockQuantity and the 3-way match report. There is no
//     "frozen" column to unset and no unfreeze path to write: a receipt
//     stops freezing the moment it stops being `received`, which is
//     exactly what cancelling it does. Deleting a receipt needs no
//     handling either, because DeleteInboundDelivery already refuses to
//     delete a received one (cancel it instead) — so every deletable
//     receipt is one that never froze anything.
//
//   - Only `received` receipts freeze, not merely non-cancelled ones. A
//     draft receipt has moved no stock and posted no GRNI; it is itself
//     freely editable and deletable, so freezing a purchase order because
//     someone started drafting a receipt against it would block a
//     legitimate edit while protecting no invariant.
//
// purchase_orders.status is deliberately NOT the signal: it is never
// advanced automatically from received quantities (see
// purchaseOrderStatusTransitions), so a user can set it to "received" with
// no receipt behind it, and can leave it at "confirmed" with one.

// freezingReceiptForPurchaseOrderTx returns the delivery number of a
// received goods receipt against this purchase order, or "" if none exists.
//
// Both link shapes count. A receipt created from an order carries
// inbound_deliveries.purchaseOrderId, but a standalone receipt whose lines
// were pointed at this order's lines carries only the line-level link — and
// a header-linked receipt can have manually added lines that carry no line
// link at all. Checking one would miss the other.
func freezingReceiptForPurchaseOrderTx(exec sqlGetExecer, orderID string) (string, error) {
	var number string
	err := exec.Get(&number, `
		SELECT d.deliveryNumber
		FROM inbound_deliveries d
		WHERE d.status = 'received'
		  AND (
		    d.purchaseOrderId = ?
		    OR EXISTS (
		      SELECT 1
		      FROM inbound_delivery_line_items li
		      JOIN purchase_order_line_items poli ON poli.id = li.purchaseOrderLineItemId
		      WHERE li.deliveryId = d.id AND poli.purchaseOrderId = ?
		    )
		  )
		ORDER BY d.createdAt ASC, d.rowid ASC
		LIMIT 1`, orderID, orderID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("freezing_receipt_for_purchase_order: %w", err)
	}
	return number, nil
}

// purchaseOrderLineSnapshot is the normalized projection two sets of line
// items are compared on: everything replacePurchaseOrderLineItemsTx would
// actually write, and nothing else.
//
// Normalizing matters more than it looks. unitPrice arrives from JSON as
// float64 cents and is stored through roundCents, so the incoming value has
// to go through the identical rounding before it can be compared to the
// stored int64. The three nullable text columns arrive as nil from a client
// that omits them and as "" from one that sends them empty, while the
// column may hold either — all three spellings mean the same thing and must
// compare equal, or a save that changed nothing would be refused.
type purchaseOrderLineSnapshot struct {
	ID          string
	ProductID   string
	Description string
	Quantity    float64
	UnitPrice   int64
	Unit        string
	TaxRate     string
	Position    int
}

// purchaseOrderLineItemsUnchangedTx reports whether the incoming line items
// describe exactly what is already stored.
//
// This exists because the frozen check cannot simply reject any request
// carrying a LineItems field: src/atoms/purchase-order.ts always sends the
// full line-item array, even for a save that only touched the notes or the
// tracking-adjacent header fields. Rejecting on presence would make a
// received purchase order completely unsaveable rather than
// header-only-editable, which is the opposite of the intent.
func purchaseOrderLineItemsUnchangedTx(
	exec sqlSelectExecer, orderID string, items []CreatePurchaseOrderLineItemRequest,
) (bool, error) {
	stored := []PurchaseOrderLineItem{}
	if err := exec.Select(&stored, `
		SELECT id, purchaseOrderId, productId, description, quantity, unitPrice,
		       unit, taxRate, position
		FROM purchase_order_line_items
		WHERE purchaseOrderId = ?
		ORDER BY position ASC, rowid ASC`, orderID); err != nil {
		return false, fmt.Errorf("purchase_order_line_items_unchanged select: %w", err)
	}
	if len(stored) != len(items) {
		return false, nil
	}

	// The incoming array's own order is the position each line would be
	// written at, matching replacePurchaseOrderLineItemsTx's `position = i`.
	for i, item := range items {
		incoming := purchaseOrderLineSnapshot{
			ID:          derefString(item.ID),
			ProductID:   derefString(item.ProductID),
			Description: item.Description,
			Quantity:    item.Quantity,
			UnitPrice:   roundCents(item.UnitPrice),
			Unit:        derefString(item.Unit),
			TaxRate:     derefString(item.TaxRate),
			Position:    i,
		}
		current := purchaseOrderLineSnapshot{
			ID:          stored[i].ID,
			ProductID:   derefString(stored[i].ProductID),
			Description: stored[i].Description,
			Quantity:    stored[i].Quantity,
			UnitPrice:   stored[i].UnitPrice,
			Unit:        derefString(stored[i].Unit),
			TaxRate:     derefString(stored[i].TaxRate),
			Position:    stored[i].Position,
		}
		if incoming != current {
			return false, nil
		}
	}
	return true, nil
}

// checkPurchaseOrderLineItemFreezeTx rejects an edit that would change a
// received purchase order's line items.
//
// Called inside UpdatePurchaseOrder's own transaction rather than before
// it: db.SetMaxOpenConns(1) means holding the transaction holds the only
// connection, so a concurrent PATCH .../status cannot mark a receipt
// received between this check and the write it guards. That is the same
// race F48 closed elsewhere with a re-read under tx — here the check simply
// never runs outside one.
func checkPurchaseOrderLineItemFreezeTx(
	exec interface {
		sqlGetExecer
		sqlSelectExecer
	},
	orderID string, items []CreatePurchaseOrderLineItemRequest,
) error {
	receipt, err := freezingReceiptForPurchaseOrderTx(exec, orderID)
	if err != nil {
		return err
	}
	if receipt == "" {
		return nil
	}
	unchanged, err := purchaseOrderLineItemsUnchangedTx(exec, orderID, items)
	if err != nil {
		return err
	}
	if unchanged {
		return nil
	}
	return newValidationError(
		"cannot change the line items of a purchase order that goods receipt %s has already received — "+
			"cancel that receipt first; header fields can still be edited",
		receipt,
	)
}
