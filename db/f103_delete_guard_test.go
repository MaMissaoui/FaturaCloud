package db

import (
	"errors"
	"testing"
)

// F103: DeleteInvoice/DeleteIncomingInvoice/DeleteDelivery/DeleteOrder used to
// read the document's status (and, for the invoices, probe for a posted GL
// entry) before the delete and then run an unconditional DELETE — two of them
// with no transaction at all. A concurrent status PATCH could commit in the
// gap and post GL/stock, after which the delete would orphan a posted journal
// entry or stock movement. The deletes now share one transaction with their
// pre-checks and repeat the predicate in the DELETE itself.
//
// These tests pin the deterministic half of that contract: each document type
// still deletes in every state its previous rules allowed, and is refused with
// the same *ValidationError message once its status has advanced beyond that
// (or it carries a posted GL entry).

func assertDeleteRefusedWithMessage(t *testing.T, err error, want string) {
	t.Helper()
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected a *ValidationError, got %T: %v", err, err)
	}
	if verr.Error() != want {
		t.Fatalf("refusal message = %q, want %q", verr.Error(), want)
	}
}

func TestDeleteInvoiceGuardRespectsStateAndPostedEntry(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-f103-invoice")

	// A draft invoice carries no GL entry and still deletes.
	draft := fx.createInvoice(t, d, "f103-inv-draft", 1, 1000)
	if ok, err := d.DeleteInvoice(draft.ID); err != nil || !ok {
		t.Fatalf("DeleteInvoice(draft) = %v, %v; want true, nil", ok, err)
	}

	// Advancing to sent posts a GL entry; the delete must be refused and the
	// invoice must survive.
	sent := fx.createInvoice(t, d, "f103-inv-sent", 1, 1000)
	if _, err := d.UpdateInvoiceState(sent.ID, "sent"); err != nil {
		t.Fatalf("UpdateInvoiceState(sent): %v", err)
	}
	if _, err := d.DeleteInvoice(sent.ID); err == nil {
		t.Fatal("expected deleting a sent invoice with a posted GL entry to be rejected")
	} else {
		assertDeleteRefusedWithMessage(t, err, "cannot delete an invoice with a posted GL entry — cancel it instead")
	}
	if _, err := d.GetInvoice(sent.ID); err != nil {
		t.Fatalf("invoice was deleted despite the refusal: %v", err)
	}

	// Paid is refused by the state predicate itself.
	paid := fx.createInvoice(t, d, "f103-inv-paid", 1, 1000)
	if _, err := d.UpdateInvoiceState(paid.ID, "paid"); err != nil {
		t.Fatalf("UpdateInvoiceState(paid): %v", err)
	}
	if _, err := d.DeleteInvoice(paid.ID); err == nil {
		t.Fatal("expected deleting a paid invoice to be rejected")
	} else {
		assertDeleteRefusedWithMessage(t, err, "cannot delete a paid invoice — cancel it instead")
	}
}

func TestDeleteIncomingInvoiceGuardRespectsStateAndPostedEntry(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-f103-bill")

	draft := fx.createIncomingInvoice(t, d, "f103-bill-draft", 1, 1000)
	if ok, err := d.DeleteIncomingInvoice(draft.ID); err != nil || !ok {
		t.Fatalf("DeleteIncomingInvoice(draft) = %v, %v; want true, nil", ok, err)
	}

	approved := fx.createIncomingInvoice(t, d, "f103-bill-approved", 1, 1000)
	if _, err := d.UpdateIncomingInvoiceState(approved.ID, "approved"); err != nil {
		t.Fatalf("UpdateIncomingInvoiceState(approved): %v", err)
	}
	if _, err := d.DeleteIncomingInvoice(approved.ID); err == nil {
		t.Fatal("expected deleting an approved bill with a posted GL entry to be rejected")
	} else {
		assertDeleteRefusedWithMessage(t, err, "cannot delete a bill with a posted GL entry — cancel it instead")
	}
	if _, err := d.GetIncomingInvoice(approved.ID); err != nil {
		t.Fatalf("bill was deleted despite the refusal: %v", err)
	}

	paid := fx.createIncomingInvoice(t, d, "f103-bill-paid", 1, 1000)
	if _, err := d.UpdateIncomingInvoiceState(paid.ID, "paid"); err != nil {
		t.Fatalf("UpdateIncomingInvoiceState(paid): %v", err)
	}
	if _, err := d.DeleteIncomingInvoice(paid.ID); err == nil {
		t.Fatal("expected deleting a paid bill to be rejected")
	} else {
		assertDeleteRefusedWithMessage(t, err, "cannot delete a paid incoming invoice — cancel it instead")
	}
}

func TestDeleteDeliveryGuardRespectsStatus(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGRNITestFixture(t, d, "org-f103-delivery", 10, 250)

	// A draft delivery has moved no stock and still deletes.
	draft, err := d.CreateDelivery(CreateDeliveryRequest{
		OrganizationID: fx.orgID, DeliveryNumber: "DN-F103-DRAFT", DeliveryDate: fx.date,
		LineItems: []CreateDeliveryLineItemRequest{
			{ProductID: &fx.productID, Description: "Widget", Quantity: 1},
		},
	})
	if err != nil {
		t.Fatalf("CreateDelivery(draft): %v", err)
	}
	if ok, err := d.DeleteDelivery(draft.ID); err != nil || !ok {
		t.Fatalf("DeleteDelivery(draft) = %v, %v; want true, nil", ok, err)
	}

	// A cancelled draft also remains deletable — the guard only refuses
	// shipped/delivered, and must not invent a new restriction.
	cancellable, err := d.CreateDelivery(CreateDeliveryRequest{
		OrganizationID: fx.orgID, DeliveryNumber: "DN-F103-CANCELLED", DeliveryDate: fx.date,
		LineItems: []CreateDeliveryLineItemRequest{
			{ProductID: &fx.productID, Description: "Widget", Quantity: 1},
		},
	})
	if err != nil {
		t.Fatalf("CreateDelivery(cancellable): %v", err)
	}
	if _, err := d.UpdateDeliveryStatus(cancellable.ID, "cancelled", nil); err != nil {
		t.Fatalf("UpdateDeliveryStatus(cancelled): %v", err)
	}
	if ok, err := d.DeleteDelivery(cancellable.ID); err != nil || !ok {
		t.Fatalf("DeleteDelivery(cancelled) = %v, %v; want true, nil", ok, err)
	}

	// A shipped delivery has moved stock and posted COGS; the delete must be
	// refused and the delivery must survive.
	fx.receive(t, d, "GR-F103-DELIVERY", 10)
	shipped := fx.ship(t, d, "DN-F103-SHIPPED", 1)
	if _, err := d.DeleteDelivery(shipped.ID); err == nil {
		t.Fatal("expected deleting a shipped delivery to be rejected")
	} else {
		assertDeleteRefusedWithMessage(t, err, "cannot delete a shipped delivery — cancel it instead")
	}
	if _, err := d.GetDelivery(shipped.ID); err != nil {
		t.Fatalf("delivery was deleted despite the refusal: %v", err)
	}

	// Delivered is refused the same way.
	delivered, err := d.UpdateDeliveryStatus(shipped.ID, "delivered", nil)
	if err != nil {
		t.Fatalf("UpdateDeliveryStatus(delivered): %v", err)
	}
	if _, err := d.DeleteDelivery(delivered.ID); err == nil {
		t.Fatal("expected deleting a delivered delivery to be rejected")
	} else {
		assertDeleteRefusedWithMessage(t, err, "cannot delete a delivered delivery — cancel it instead")
	}
}

func TestDeleteOrderGuardRespectsStatus(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-f103-order"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}

	draft, err := d.CreateOrder(CreateOrderRequest{
		ID: "f103-order-draft", OrganizationID: org.ID, OrderNumber: "ORD-F103-DRAFT",
		OrderDate: 1700000000000,
	})
	if err != nil {
		t.Fatalf("CreateOrder(draft): %v", err)
	}
	if ok, err := d.DeleteOrder(draft.ID); err != nil || !ok {
		t.Fatalf("DeleteOrder(draft) = %v, %v; want true, nil", ok, err)
	}

	// Confirmed is still deletable — the existing rule only refuses
	// shipped/delivered.
	confirmed, err := d.CreateOrder(CreateOrderRequest{
		ID: "f103-order-confirmed", OrganizationID: org.ID, OrderNumber: "ORD-F103-CONFIRMED",
		OrderDate: 1700000000000, Status: "confirmed",
	})
	if err != nil {
		t.Fatalf("CreateOrder(confirmed): %v", err)
	}
	if ok, err := d.DeleteOrder(confirmed.ID); err != nil || !ok {
		t.Fatalf("DeleteOrder(confirmed) = %v, %v; want true, nil", ok, err)
	}

	// Shipped is refused.
	shipped, err := d.CreateOrder(CreateOrderRequest{
		ID: "f103-order-shipped", OrganizationID: org.ID, OrderNumber: "ORD-F103-SHIPPED",
		OrderDate: 1700000000000, Status: "confirmed",
	})
	if err != nil {
		t.Fatalf("CreateOrder(shipped): %v", err)
	}
	if _, err := d.UpdateOrderStatus(shipped.ID, "shipped"); err != nil {
		t.Fatalf("UpdateOrderStatus(shipped): %v", err)
	}
	if _, err := d.DeleteOrder(shipped.ID); err == nil {
		t.Fatal("expected deleting a shipped order to be rejected")
	} else {
		assertDeleteRefusedWithMessage(t, err, "cannot delete a shipped order — cancel it instead")
	}
	if _, err := d.GetOrder(shipped.ID); err != nil {
		t.Fatalf("order was deleted despite the refusal: %v", err)
	}

	// Delivered is refused too.
	if _, err := d.UpdateOrderStatus(shipped.ID, "delivered"); err != nil {
		t.Fatalf("UpdateOrderStatus(delivered): %v", err)
	}
	if _, err := d.DeleteOrder(shipped.ID); err == nil {
		t.Fatal("expected deleting a delivered order to be rejected")
	} else {
		assertDeleteRefusedWithMessage(t, err, "cannot delete a delivered order — cancel it instead")
	}
}
