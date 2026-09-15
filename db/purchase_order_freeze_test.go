package db

import (
	"errors"
	"strings"
	"sync"
	"testing"
)

// echoPurchaseOrderLines reads a purchase order's stored line items back in
// the exact shape the frontend echoes them to PUT /api/purchase-orders/{id}:
// every id preserved, unitPrice as float64 cents. A save built from this and
// nothing else must be accepted even while the order is frozen — the
// frontend always sends the full line-item array, so rejecting on the mere
// presence of a LineItems field would make a received order unsaveable
// rather than header-only-editable.
func echoPurchaseOrderLines(t *testing.T, d *Database, orderID string) []CreatePurchaseOrderLineItemRequest {
	t.Helper()
	stored, err := d.GetPurchaseOrderLineItems(orderID)
	if err != nil {
		t.Fatalf("GetPurchaseOrderLineItems: %v", err)
	}
	items := make([]CreatePurchaseOrderLineItemRequest, len(stored))
	for i, line := range stored {
		id := line.ID
		items[i] = CreatePurchaseOrderLineItemRequest{
			ID:          &id,
			ProductID:   line.ProductID,
			Description: line.Description,
			Quantity:    line.Quantity,
			UnitPrice:   float64(line.UnitPrice),
			Unit:        line.Unit,
			TaxRate:     line.TaxRate,
		}
	}
	return items
}

func assertFrozen(t *testing.T, err error, what string) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: expected the freeze to reject this edit, got nil", what)
	}
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("%s: expected *ValidationError (409), got %T: %v", what, err, err)
	}
	if !strings.Contains(verr.Error(), "GR-0001") {
		t.Errorf("%s: error should name the receipt that froze the order, got %q", what, verr.Error())
	}
}

// A header-only save must still work on a received purchase order. This is
// the case the whole diff-based design exists for: the frontend cannot send
// a request without line items, so "frozen" has to mean "unchanged line
// items", not "no line items in the payload".
func TestUpdateReceivedPurchaseOrderAllowsHeaderOnlyEdit(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGRNITestFixture(t, d, "org-po-freeze-header", 10, 250)
	fx.receive(t, d, "GR-0001", 10)

	notes := "Vendor confirmed the shipment left on Tuesday"
	items := echoPurchaseOrderLines(t, d, fx.poID)
	updated, err := d.UpdatePurchaseOrder(fx.poID, UpdatePurchaseOrderRequest{
		Notes: &notes, LineItems: &items,
	})
	if err != nil {
		t.Fatalf("header-only save of a received purchase order should succeed: %v", err)
	}
	if updated.Notes == nil || *updated.Notes != notes {
		t.Errorf("notes = %v, want %q", updated.Notes, notes)
	}
}

func TestUpdateReceivedPurchaseOrderRejectsLineItemChanges(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		mutate func(items []CreatePurchaseOrderLineItemRequest) []CreatePurchaseOrderLineItemRequest
	}{
		{
			name: "quantity",
			mutate: func(items []CreatePurchaseOrderLineItemRequest) []CreatePurchaseOrderLineItemRequest {
				items[0].Quantity = 99
				return items
			},
		},
		{
			name: "unit price",
			mutate: func(items []CreatePurchaseOrderLineItemRequest) []CreatePurchaseOrderLineItemRequest {
				items[0].UnitPrice = 999
				return items
			},
		},
		{
			name: "description",
			mutate: func(items []CreatePurchaseOrderLineItemRequest) []CreatePurchaseOrderLineItemRequest {
				items[0].Description = "Something else entirely"
				return items
			},
		},
		{
			name: "line added",
			mutate: func(items []CreatePurchaseOrderLineItemRequest) []CreatePurchaseOrderLineItemRequest {
				return append(items, CreatePurchaseOrderLineItemRequest{
					Description: "Bonus line", Quantity: 1, UnitPrice: 100,
				})
			},
		},
		{
			name: "line removed",
			mutate: func([]CreatePurchaseOrderLineItemRequest) []CreatePurchaseOrderLineItemRequest {
				return []CreatePurchaseOrderLineItemRequest{}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d := newTestDB(t)
			fx := newGRNITestFixture(t, d, "org-po-freeze-"+tc.name, 10, 250)
			fx.receive(t, d, "GR-0001", 10)

			items := tc.mutate(echoPurchaseOrderLines(t, d, fx.poID))
			_, err := d.UpdatePurchaseOrder(fx.poID, UpdatePurchaseOrderRequest{LineItems: &items})
			assertFrozen(t, err, tc.name)

			// The refusal must be total: nothing may have been written.
			after, getErr := d.GetPurchaseOrderLineItems(fx.poID)
			if getErr != nil {
				t.Fatalf("GetPurchaseOrderLineItems: %v", getErr)
			}
			if len(after) != 1 || after[0].Quantity != 10 || after[0].UnitPrice != 250 {
				t.Errorf("line items were modified despite the refusal: %+v", after)
			}
		})
	}
}

// The user's requirement, both halves: cancelling the receipt must lift the
// freeze. It needs no unfreeze code at all — the freeze is computed from
// receipt status, so a cancel lifts it by construction.
func TestCancellingTheReceiptUnfreezesThePurchaseOrder(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGRNITestFixture(t, d, "org-po-unfreeze-cancel", 10, 250)
	receipt := fx.receive(t, d, "GR-0001", 10)

	items := echoPurchaseOrderLines(t, d, fx.poID)
	items[0].Quantity = 12
	if _, err := d.UpdatePurchaseOrder(fx.poID, UpdatePurchaseOrderRequest{LineItems: &items}); err == nil {
		t.Fatal("expected the edit to be frozen before cancelling")
	}

	if _, err := d.UpdateInboundDeliveryStatus(receipt.ID, "cancelled", nil); err != nil {
		t.Fatalf("UpdateInboundDeliveryStatus(cancelled): %v", err)
	}

	items = echoPurchaseOrderLines(t, d, fx.poID)
	items[0].Quantity = 12
	if _, err := d.UpdatePurchaseOrder(fx.poID, UpdatePurchaseOrderRequest{LineItems: &items}); err != nil {
		t.Fatalf("cancelling the receipt should have unfrozen the order: %v", err)
	}
	after, err := d.GetPurchaseOrderLineItems(fx.poID)
	if err != nil {
		t.Fatalf("GetPurchaseOrderLineItems: %v", err)
	}
	if len(after) != 1 || after[0].Quantity != 12 {
		t.Errorf("quantity = %+v, want the edited 12", after)
	}
	// F93's id reuse must still hold across the now-permitted edit.
	if after[0].ID != *items[0].ID {
		t.Errorf("line id changed across the edit: %q -> %q", *items[0].ID, after[0].ID)
	}
}

// A draft receipt has moved no stock and posted no GRNI, and is itself
// freely editable and deletable — freezing the order for one would block a
// legitimate edit while protecting no invariant.
func TestDraftReceiptDoesNotFreezeThePurchaseOrder(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGRNITestFixture(t, d, "org-po-freeze-draft", 10, 250)

	if _, err := d.CreateInboundDelivery(CreateInboundDeliveryRequest{
		OrganizationID: fx.orgID, PurchaseOrderID: &fx.poID, VendorID: &fx.vendorID,
		DeliveryNumber: "GR-DRAFT", DeliveryDate: fx.date,
		LineItems: []CreateInboundDeliveryLineItemRequest{
			{PurchaseOrderLineItemID: &fx.poLineID, Description: "Widget", Quantity: 10},
		},
	}); err != nil {
		t.Fatalf("CreateInboundDelivery: %v", err)
	}

	items := echoPurchaseOrderLines(t, d, fx.poID)
	items[0].Quantity = 15
	if _, err := d.UpdatePurchaseOrder(fx.poID, UpdatePurchaseOrderRequest{LineItems: &items}); err != nil {
		t.Fatalf("a draft receipt must not freeze the order: %v", err)
	}
}

// The freeze must see a receipt linked only at line level, not just one
// carrying inbound_deliveries.purchaseOrderId. A standalone receipt whose
// lines point at this order's lines accrues exactly the same GRNI.
func TestLineLevelOnlyReceiptLinkFreezesThePurchaseOrder(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGRNITestFixture(t, d, "org-po-freeze-linelink", 10, 250)

	receipt, err := d.CreateInboundDelivery(CreateInboundDeliveryRequest{
		OrganizationID: fx.orgID, VendorID: &fx.vendorID, // deliberately no PurchaseOrderID
		DeliveryNumber: "GR-0001", DeliveryDate: fx.date,
		LineItems: []CreateInboundDeliveryLineItemRequest{
			{PurchaseOrderLineItemID: &fx.poLineID, Description: "Widget", Quantity: 10},
		},
	})
	if err != nil {
		t.Fatalf("CreateInboundDelivery: %v", err)
	}
	if receipt.PurchaseOrderID != nil {
		t.Fatalf("fixture error: this receipt was meant to carry no header link, got %v", *receipt.PurchaseOrderID)
	}
	if _, err := d.UpdateInboundDeliveryStatus(receipt.ID, "received", nil); err != nil {
		t.Fatalf("UpdateInboundDeliveryStatus(received): %v", err)
	}

	items := echoPurchaseOrderLines(t, d, fx.poID)
	items[0].Quantity = 15
	_, err = d.UpdatePurchaseOrder(fx.poID, UpdatePurchaseOrderRequest{LineItems: &items})
	assertFrozen(t, err, "line-level-only link")
}

// The standing requirement this repo puts on every new status-dependent
// guard (F69, re-flagged as F80): a concurrency test proving the guard
// can't be raced. Receiving and editing at the same time must resolve to
// exactly one of "the edit landed first" or "the edit was refused" — never
// an edit that silently rewrites the basis of a GRNI accrual that has
// already posted.
func TestConcurrentPurchaseOrderEditAndReceiptCannotBypassTheFreeze(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGRNITestFixture(t, d, "org-po-freeze-race", 10, 250)

	receipt, err := d.CreateInboundDelivery(CreateInboundDeliveryRequest{
		OrganizationID: fx.orgID, PurchaseOrderID: &fx.poID, VendorID: &fx.vendorID,
		DeliveryNumber: "GR-0001", DeliveryDate: fx.date,
		LineItems: []CreateInboundDeliveryLineItemRequest{
			{PurchaseOrderLineItemID: &fx.poLineID, Description: "Widget", Quantity: 10},
		},
	})
	if err != nil {
		t.Fatalf("CreateInboundDelivery: %v", err)
	}

	items := echoPurchaseOrderLines(t, d, fx.poID)
	items[0].Quantity = 15

	var wg sync.WaitGroup
	var editErr, receiveErr error
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, editErr = d.UpdatePurchaseOrder(fx.poID, UpdatePurchaseOrderRequest{LineItems: &items})
	}()
	go func() {
		defer wg.Done()
		_, receiveErr = d.UpdateInboundDeliveryStatus(receipt.ID, "received", nil)
	}()
	wg.Wait()

	if receiveErr != nil {
		t.Fatalf("receiving should always succeed here: %v", receiveErr)
	}

	after, err := d.GetPurchaseOrderLineItems(fx.poID)
	if err != nil {
		t.Fatalf("GetPurchaseOrderLineItems: %v", err)
	}
	if len(after) != 1 {
		t.Fatalf("line items = %+v, want exactly 1", after)
	}
	if editErr == nil {
		// The edit won the race: it must have landed entirely, before the
		// receipt was ever received.
		if after[0].Quantity != 15 {
			t.Errorf("the edit reported success but quantity = %v, want 15", after[0].Quantity)
		}
		return
	}
	// The receipt won: the edit must have been refused by the freeze and
	// must have written nothing.
	var verr *ValidationError
	if !errors.As(editErr, &verr) {
		t.Fatalf("the losing edit should fail as *ValidationError (409), got %T: %v", editErr, editErr)
	}
	if after[0].Quantity != 10 {
		t.Errorf("the edit was refused but quantity = %v, want the original 10", after[0].Quantity)
	}
}
