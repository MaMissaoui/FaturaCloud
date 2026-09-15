package db

import (
	"errors"
	"testing"
)

// echoDeliveryLines reads a delivery's stored line items back in the shape
// src/atoms/delivery.ts echoes them to PUT /api/deliveries/{id} — the full
// array, unchanged. A header-only save sends exactly this.
func echoDeliveryLines(t *testing.T, d *Database, deliveryID string) []CreateDeliveryLineItemRequest {
	t.Helper()
	stored, err := d.GetDeliveryLineItems(deliveryID)
	if err != nil {
		t.Fatalf("GetDeliveryLineItems: %v", err)
	}
	items := make([]CreateDeliveryLineItemRequest, len(stored))
	for i, line := range stored {
		items[i] = CreateDeliveryLineItemRequest{
			OrderLineItemID: line.OrderLineItemID,
			ProductID:       line.ProductID,
			Description:     line.Description,
			Quantity:        line.Quantity,
			Unit:            line.Unit,
		}
	}
	return items
}

// CLAUDE.md and src/routes/deliveries/details.tsx both state that a shipped
// delivery still accepts header-field-only edits. Before this fix neither
// was true through the UI: the guard fired on the presence of a LineItems
// field and the frontend always sends one, so saving a tracking number
// returned 409 "cannot edit line items of a shipped delivery".
func TestUpdateShippedDeliveryAllowsHeaderOnlyEdit(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGRNITestFixture(t, d, "org-deliv-freeze-header", 10, 250)
	fx.receive(t, d, "GR-0001", 10)
	delivery := fx.ship(t, d, "DN-0001", 5)

	tracking := "TRACK-123"
	items := echoDeliveryLines(t, d, delivery.ID)
	updated, err := d.UpdateDelivery(delivery.ID, UpdateDeliveryRequest{
		TrackingNumber: &tracking, LineItems: &items,
	})
	if err != nil {
		t.Fatalf("header-only save of a shipped delivery should succeed: %v", err)
	}
	if updated.TrackingNumber == nil || *updated.TrackingNumber != tracking {
		t.Errorf("trackingNumber = %v, want %q", updated.TrackingNumber, tracking)
	}
}

// The freeze itself must still hold: an actual line-item change on a
// shipped delivery is still refused, because those lines have already moved
// stock and posted COGS.
func TestUpdateShippedDeliveryStillRejectsLineItemChanges(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGRNITestFixture(t, d, "org-deliv-freeze-reject", 10, 250)
	fx.receive(t, d, "GR-0001", 10)
	delivery := fx.ship(t, d, "DN-0001", 5)

	items := echoDeliveryLines(t, d, delivery.ID)
	items[0].Quantity = 8
	_, err := d.UpdateDelivery(delivery.ID, UpdateDeliveryRequest{LineItems: &items})
	if err == nil {
		t.Fatal("expected a changed line item on a shipped delivery to be refused")
	}
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected *ValidationError (409), got %T: %v", err, err)
	}

	after, err := d.GetDeliveryLineItems(delivery.ID)
	if err != nil {
		t.Fatalf("GetDeliveryLineItems: %v", err)
	}
	if len(after) != 1 || after[0].Quantity != 5 {
		t.Errorf("line items were modified despite the refusal: %+v", after)
	}
}

// A draft delivery is not frozen at all — line items stay fully editable
// until it ships.
func TestUpdateDraftDeliveryStillAllowsLineItemChanges(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGRNITestFixture(t, d, "org-deliv-freeze-draft", 10, 250)
	fx.receive(t, d, "GR-0001", 10)

	delivery, err := d.CreateDelivery(CreateDeliveryRequest{
		OrganizationID: fx.orgID, DeliveryNumber: "DN-0002", DeliveryDate: fx.date,
		LineItems: []CreateDeliveryLineItemRequest{
			{ProductID: &fx.productID, Description: "Widget", Quantity: 2},
		},
	})
	if err != nil {
		t.Fatalf("CreateDelivery: %v", err)
	}

	items := echoDeliveryLines(t, d, delivery.ID)
	items[0].Quantity = 4
	if _, err := d.UpdateDelivery(delivery.ID, UpdateDeliveryRequest{LineItems: &items}); err != nil {
		t.Fatalf("a draft delivery's line items must stay editable: %v", err)
	}
	after, err := d.GetDeliveryLineItems(delivery.ID)
	if err != nil {
		t.Fatalf("GetDeliveryLineItems: %v", err)
	}
	if len(after) != 1 || after[0].Quantity != 4 {
		t.Errorf("quantity = %+v, want the edited 4", after)
	}
}
