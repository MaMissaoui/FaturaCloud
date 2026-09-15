package db

import (
	"errors"
	"testing"
)

// Regression tests for F70 / F93 (audit 2026-09-14): saving a document must
// not destroy the line-item ids other tables reference.
//
// The F93 test deliberately asserts the *guard*, not just the surviving id.
// A test that only checked purchaseOrderLineItemId was still populated would
// pass against a fix that restored the id but broke the cancel guard some
// other way — and the guard is the thing whose failure costs real money.

// TestUpdatePurchaseOrderPreservesLineItemIDsAndGRNIGuard is the F93
// regression. Before the fix, replacePurchaseOrderLineItemsTx deleted every
// line and reinserted with fresh nanoids, so editing an already-received,
// already-billed purchase order nulled
// inbound_delivery_line_items.purchaseOrderLineItemId. The billed-receipt
// cancel guard (db/inbound_delivery.go) skips any line whose id is nil, so
// it stopped running entirely and the receipt became cancellable — reversing
// its Dr GRNI / Cr Inventory entry while the approved bill's AP obligation
// still stood.
func TestUpdatePurchaseOrderPreservesLineItemIDsAndGRNIGuard(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	f := seedMatch(t, d, "org-f93-guard", 10, 250, 10)

	inv := createIncomingInvoice(t, d, f, "V-001", 10, 250)
	if _, err := d.UpdateIncomingInvoiceState(inv.ID, "approved"); err != nil {
		t.Fatalf("UpdateIncomingInvoiceState(approved): %v", err)
	}

	before, err := d.GetPurchaseOrderLineItems(f.OrderID)
	if err != nil || len(before) != 1 {
		t.Fatalf("GetPurchaseOrderLineItems: err=%v len=%d", err, len(before))
	}

	// The edit a user would actually make: change the description on an
	// already-received order, echoing the line's id back the way the client
	// now does.
	edited := []CreatePurchaseOrderLineItemRequest{{
		ID:          &before[0].ID,
		ProductID:   before[0].ProductID,
		Description: "Widget (revised description)",
		Quantity:    before[0].Quantity,
		UnitPrice:   float64(before[0].UnitPrice),
	}}
	if _, err := d.UpdatePurchaseOrder(f.OrderID, UpdatePurchaseOrderRequest{
		LineItems: &edited,
	}); err != nil {
		t.Fatalf("UpdatePurchaseOrder: %v", err)
	}

	after, err := d.GetPurchaseOrderLineItems(f.OrderID)
	if err != nil || len(after) != 1 {
		t.Fatalf("GetPurchaseOrderLineItems after: err=%v len=%d", err, len(after))
	}
	if after[0].ID != before[0].ID {
		t.Fatalf("line item id changed across an edit: before %q, after %q", before[0].ID, after[0].ID)
	}
	if after[0].Description != "Widget (revised description)" {
		t.Fatalf("edit did not apply: description = %q", after[0].Description)
	}

	// The receipt's link must have survived — that is what the guard reads.
	receipts, err := d.GetInboundDeliveries(f.OrgID)
	if err != nil || len(receipts) != 1 {
		t.Fatalf("GetInboundDeliveries: %v, %+v", err, receipts)
	}
	receiptLines, err := d.GetInboundDeliveryLineItems(receipts[0].ID)
	if err != nil || len(receiptLines) != 1 {
		t.Fatalf("GetInboundDeliveryLineItems: err=%v len=%d", err, len(receiptLines))
	}
	if receiptLines[0].PurchaseOrderLineItemID == nil {
		t.Fatal("receipt line lost its purchaseOrderLineItemId across a purchase order edit")
	}
	if *receiptLines[0].PurchaseOrderLineItemID != before[0].ID {
		t.Fatalf("receipt line now points at %q, want %q",
			*receiptLines[0].PurchaseOrderLineItemID, before[0].ID)
	}

	// The actual thing that matters: cancelling the billed receipt is still
	// refused.
	if _, err := d.UpdateInboundDeliveryStatus(receipts[0].ID, "cancelled", nil); err == nil {
		t.Fatal("cancelling a billed receipt was allowed after a purchase order edit — the GRNI guard did not run")
	} else {
		var verr *ValidationError
		if !errors.As(err, &verr) {
			t.Fatalf("expected a *ValidationError from the cancel guard, got %T: %v", err, err)
		}
	}
	current, err := d.GetInboundDelivery(receipts[0].ID)
	if err != nil {
		t.Fatalf("GetInboundDelivery: %v", err)
	}
	if current.Status != "received" {
		t.Fatalf("receipt status changed despite the rejection: %q", current.Status)
	}

	// And 3-way matching still sees the line as linked rather than unlinked.
	matchLines, err := d.GetIncomingInvoiceMatch(inv.ID)
	if err != nil {
		t.Fatalf("GetIncomingInvoiceMatch: %v", err)
	}
	if len(matchLines) != 1 {
		t.Fatalf("match lines = %d, want 1", len(matchLines))
	}
	if matchLines[0].Status == "unlinked" {
		t.Fatal("3-way match reports the line unlinked after a purchase order edit")
	}
}

// TestUpdateOrderPreservesLineItemIDs is the F70 regression: an order edit
// must not null outbound_delivery_line_items.orderLineItemId, or
// GetOrderDeliveredQuantities returns nothing and the delivery prefill
// re-offers quantity that has already shipped.
func TestUpdateOrderPreservesLineItemIDs(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-f70"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	product, err := d.CreateProduct(CreateProductRequest{
		OrganizationID: org.ID, Name: "Widget", SKU: ptr("WID-1"), Type: "product",
	})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	order, err := d.CreateOrder(CreateOrderRequest{
		OrganizationID: org.ID, OrderNumber: "ORD-0001", Status: "confirmed",
		OrderDate: 1700000000000,
		LineItems: []CreateOrderLineItemRequest{
			{ProductID: &product.ID, Description: "Widget", Quantity: 10, UnitPrice: 1000},
		},
	})
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	before, err := d.GetOrderLineItems(order.ID)
	if err != nil || len(before) != 1 {
		t.Fatalf("GetOrderLineItems: err=%v len=%d", err, len(before))
	}

	if _, err := d.CreateDelivery(CreateDeliveryRequest{
		OrganizationID: org.ID, OrderID: &order.ID, DeliveryNumber: "DEL-0001",
		DeliveryDate: 1700000000000,
		LineItems: []CreateDeliveryLineItemRequest{
			{OrderLineItemID: &before[0].ID, Description: "Widget", Quantity: 4},
		},
	}); err != nil {
		t.Fatalf("CreateDelivery: %v", err)
	}

	edited := []CreateOrderLineItemRequest{{
		ID:          &before[0].ID,
		ProductID:   before[0].ProductID,
		Description: "Widget (revised)",
		Quantity:    before[0].Quantity,
		UnitPrice:   float64(before[0].UnitPrice),
	}}
	if _, err := d.UpdateOrder(order.ID, UpdateOrderRequest{LineItems: &edited}); err != nil {
		t.Fatalf("UpdateOrder: %v", err)
	}

	after, err := d.GetOrderLineItems(order.ID)
	if err != nil || len(after) != 1 {
		t.Fatalf("GetOrderLineItems after: err=%v len=%d", err, len(after))
	}
	if after[0].ID != before[0].ID {
		t.Fatalf("line item id changed across an edit: before %q, after %q", before[0].ID, after[0].ID)
	}

	delivered, err := d.GetOrderDeliveredQuantities(order.ID)
	if err != nil {
		t.Fatalf("GetOrderDeliveredQuantities: %v", err)
	}
	if got := delivered[before[0].ID]; got != 4 {
		t.Fatalf("delivered quantity after edit = %v, want 4 — the delivery prefill would re-offer shipped stock", got)
	}
}

// Adding, removing and reordering must still work: only the lines the
// request no longer carries are deleted, and a line with no id is inserted
// fresh.
func TestUpdateOrderAddsAndRemovesLineItems(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-f70-addremove"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	order, err := d.CreateOrder(CreateOrderRequest{
		OrganizationID: org.ID, OrderNumber: "ORD-0002", OrderDate: 1700000000000,
		LineItems: []CreateOrderLineItemRequest{
			{Description: "First", Quantity: 1, UnitPrice: 100},
			{Description: "Second", Quantity: 2, UnitPrice: 200},
		},
	})
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	before, err := d.GetOrderLineItems(order.ID)
	if err != nil || len(before) != 2 {
		t.Fatalf("GetOrderLineItems: err=%v len=%d", err, len(before))
	}

	// Drop "First", keep "Second" (now at position 0), add a third.
	edited := []CreateOrderLineItemRequest{
		{ID: &before[1].ID, Description: "Second", Quantity: 2, UnitPrice: 200},
		{Description: "Third", Quantity: 3, UnitPrice: 300},
	}
	if _, err := d.UpdateOrder(order.ID, UpdateOrderRequest{LineItems: &edited}); err != nil {
		t.Fatalf("UpdateOrder: %v", err)
	}

	after, err := d.GetOrderLineItems(order.ID)
	if err != nil || len(after) != 2 {
		t.Fatalf("GetOrderLineItems after: err=%v len=%d", err, len(after))
	}
	if after[0].ID != before[1].ID {
		t.Fatalf("kept line lost its id: got %q, want %q", after[0].ID, before[1].ID)
	}
	if after[0].Description != "Second" || after[1].Description != "Third" {
		t.Fatalf("unexpected lines after edit: %q, %q", after[0].Description, after[1].Description)
	}
	if after[1].ID == before[0].ID {
		t.Fatal("the new line reused the removed line's id")
	}
	for _, l := range after {
		if l.ID == before[0].ID {
			t.Fatalf("removed line %q still present", before[0].ID)
		}
	}
}

// An id that does not belong to this document is never trusted as a primary
// key — it falls back to a fresh nanoid instead. Without this, a client
// could point one document's line at another document's row, or collide with
// an existing primary key.
func TestUpdateOrderIgnoresForeignLineItemID(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-f70-foreign"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	mk := func(number, desc string) *Order {
		o, err := d.CreateOrder(CreateOrderRequest{
			OrganizationID: org.ID, OrderNumber: number, OrderDate: 1700000000000,
			LineItems: []CreateOrderLineItemRequest{{Description: desc, Quantity: 1, UnitPrice: 100}},
		})
		if err != nil {
			t.Fatalf("CreateOrder(%s): %v", number, err)
		}
		return o
	}
	victim, thief := mk("ORD-A", "Victim line"), mk("ORD-B", "Thief line")

	victimLines, _ := d.GetOrderLineItems(victim.ID)
	if len(victimLines) != 1 {
		t.Fatalf("victim lines = %d", len(victimLines))
	}

	// ORD-B claims ORD-A's line-item id.
	edited := []CreateOrderLineItemRequest{
		{ID: &victimLines[0].ID, Description: "Hijacked", Quantity: 9, UnitPrice: 900},
	}
	if _, err := d.UpdateOrder(thief.ID, UpdateOrderRequest{LineItems: &edited}); err != nil {
		t.Fatalf("UpdateOrder: %v", err)
	}

	thiefLines, _ := d.GetOrderLineItems(thief.ID)
	if len(thiefLines) != 1 {
		t.Fatalf("thief lines = %d, want 1", len(thiefLines))
	}
	if thiefLines[0].ID == victimLines[0].ID {
		t.Fatal("a foreign line-item id was accepted as this order's own primary key")
	}

	stillVictim, _ := d.GetOrderLineItems(victim.ID)
	if len(stillVictim) != 1 || stillVictim[0].Description != "Victim line" {
		t.Fatalf("the other order's line was modified: %+v", stillVictim)
	}
}

// A request that sends no ids at all (an older client, a direct API caller)
// still saves correctly — it just does not get the id preservation, which is
// the documented contract and no worse than the pre-fix behaviour.
func TestUpdateOrderWithoutLineItemIDsStillSaves(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-f70-noids"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	order, err := d.CreateOrder(CreateOrderRequest{
		OrganizationID: org.ID, OrderNumber: "ORD-0003", OrderDate: 1700000000000,
		LineItems: []CreateOrderLineItemRequest{{Description: "Only", Quantity: 1, UnitPrice: 100}},
	})
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}

	edited := []CreateOrderLineItemRequest{{Description: "Only, edited", Quantity: 2, UnitPrice: 250}}
	if _, err := d.UpdateOrder(order.ID, UpdateOrderRequest{LineItems: &edited}); err != nil {
		t.Fatalf("UpdateOrder: %v", err)
	}
	after, err := d.GetOrderLineItems(order.ID)
	if err != nil || len(after) != 1 {
		t.Fatalf("GetOrderLineItems: err=%v len=%d", err, len(after))
	}
	if after[0].Description != "Only, edited" || after[0].Quantity != 2 {
		t.Fatalf("edit did not apply: %+v", after[0])
	}
}
