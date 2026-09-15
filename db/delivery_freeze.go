package db

import "fmt"

// Outbound delivery line-item freeze — the "unchanged is not a change" half.
//
// db/delivery.go has rejected line-item edits on a shipped/delivered
// delivery since that guard was written, and CLAUDE.md records the intent
// as "line items are frozen once a delivery is shipped/delivered; PUT still
// accepts header-field-only edits (tracking number, notes, …)".
// src/routes/deliveries/details.tsx says the same in a comment and disables
// the line-item table accordingly.
//
// Both were wrong in practice. The guard fired on the mere *presence* of a
// LineItems field, and src/atoms/delivery.ts always sends the full
// line-item array — so saving a tracking number on a shipped delivery
// returned 409 "cannot edit line items of a shipped delivery", and the
// documented header-only edit was unreachable through the UI that documents
// it. Found while implementing the purchase-order freeze, which is the same
// mechanism (see db/purchase_order_freeze.go) and needed the same answer.
//
// The comparison differs from the purchase-order one in one respect:
// outbound_delivery_line_items has no inbound foreign key to its own ids, so
// CreateDeliveryLineItemRequest carries no ID field and replaceDeliveryLineItemsTx
// deliberately still deletes and reinserts (audit 2026-09-14's 1.1
// enumeration). Lines are therefore matched positionally, which is exactly
// how the write path assigns position anyway.

type deliveryLineSnapshot struct {
	OrderLineItemID string
	ProductID       string
	Description     string
	Quantity        float64
	Unit            string
}

// deliveryLineItemsUnchangedTx reports whether the incoming line items
// describe exactly what is already stored.
//
// productId gets one deliberate allowance: replaceDeliveryLineItemsTx
// resolves it from orderLineItemId when a line names an order line but no
// product, so a stored row can legitimately hold a productId the client
// never sent. An incoming line that omits productId while naming an order
// line is therefore treated as matching whatever is stored — it is not
// asking to change anything, and comparing the raw values would refuse a
// save that changes nothing, which is the exact failure this function
// exists to prevent.
func deliveryLineItemsUnchangedTx(
	exec sqlSelectExecer, deliveryID string, items []CreateDeliveryLineItemRequest,
) (bool, error) {
	stored := []OutboundDeliveryLineItem{}
	if err := exec.Select(&stored, `
		SELECT id, deliveryId, orderLineItemId, productId, description, quantity, unit, position
		FROM outbound_delivery_line_items
		WHERE deliveryId = ?
		ORDER BY position ASC, rowid ASC`, deliveryID); err != nil {
		return false, fmt.Errorf("delivery_line_items_unchanged select: %w", err)
	}
	if len(stored) != len(items) {
		return false, nil
	}

	for i, item := range items {
		incomingProduct := derefString(item.ProductID)
		storedProduct := derefString(stored[i].ProductID)
		if incomingProduct == "" && derefString(item.OrderLineItemID) != "" {
			incomingProduct = storedProduct
		}
		incoming := deliveryLineSnapshot{
			OrderLineItemID: derefString(item.OrderLineItemID),
			ProductID:       incomingProduct,
			Description:     item.Description,
			Quantity:        item.Quantity,
			Unit:            derefString(item.Unit),
		}
		current := deliveryLineSnapshot{
			OrderLineItemID: derefString(stored[i].OrderLineItemID),
			ProductID:       storedProduct,
			Description:     stored[i].Description,
			Quantity:        stored[i].Quantity,
			Unit:            derefString(stored[i].Unit),
		}
		if incoming != current {
			return false, nil
		}
	}
	return true, nil
}
