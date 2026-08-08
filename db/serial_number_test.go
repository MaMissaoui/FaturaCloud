package db

import (
	"errors"
	"testing"
)

// seedSerializedProduct creates an organization and a stock-enabled,
// serialized product with zero stock. Also creates a fiscal year covering
// 2023 (every fixture in this file dates its receipts/shipments
// 1700000000000) — Phase 7's GRNI/COGS auto-posting needs an open fiscal
// year covering that date, same as invoice/bill auto-posting already does.
func seedSerializedProduct(t *testing.T, orgID string) (*Database, *Product) {
	t.Helper()
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: orgID})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	if _, err := d.CreateFiscalYear(CreateFiscalYearRequest{
		OrganizationID: org.ID, Name: "2023", StartDate: 1672531200000, EndDate: 1704067199000,
	}); err != nil {
		t.Fatalf("CreateFiscalYear: %v", err)
	}
	product, err := d.CreateProduct(CreateProductRequest{
		ID: orgID + "-prod", OrganizationID: orgID, Name: "Serial Widget",
		Type: "product", StockEnabled: 1, Serialized: 1,
	})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	return d, product
}

func mustValidationError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected a *ValidationError (surfaced as 409), got %T: %v", err, err)
	}
}

func TestCreateProductSerializedRequiresStockEnabled(t *testing.T) {
	d, org := newTestDB(t), "org-ser-flag"
	if _, err := d.CreateOrganization(CreateOrganizationRequest{ID: org}); err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	product, err := d.CreateProduct(CreateProductRequest{
		ID: "prod-ser-flag", OrganizationID: org, Name: "Widget",
		Type: "product", StockEnabled: 0, Serialized: 1,
	})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	if product.Serialized != 0 {
		t.Fatalf("serialized should be coerced to 0 when stockEnabled is 0, got %d", product.Serialized)
	}
}

// Toggling serialized is blocked in both directions while stock is non-zero
// — turning it on would fabricate identity for untracked legacy stock,
// turning it off would strand the registry with nothing to reconcile against.
func TestUpdateProductSerializedToggleBlockedWhileStockNonZero(t *testing.T) {
	d, product := seedSerializedProduct(t, "org-ser-toggle")

	// Un-serialize, then try to re-serialize while stock is non-zero.
	unserialized, err := d.UpdateProduct(product.ID, UpdateProductRequest{
		Name: "Widget", Type: "product", StockEnabled: 1, Serialized: 0,
	})
	if err != nil {
		t.Fatalf("UpdateProduct (turn off, zero stock): %v", err)
	}
	if unserialized.Serialized != 0 {
		t.Fatalf("expected serialized=0, got %d", unserialized.Serialized)
	}

	if _, err := d.CreateStockMovement(CreateStockMovementRequest{
		OrganizationID: product.OrganizationID, ProductID: product.ID, Type: "in", Quantity: 3,
	}); err != nil {
		t.Fatalf("CreateStockMovement: %v", err)
	}

	_, err = d.UpdateProduct(product.ID, UpdateProductRequest{
		Name: "Widget", Type: "product", StockEnabled: 1, Serialized: 1,
	})
	mustValidationError(t, err)

	// Bring stock back to zero, then confirm turning it on now succeeds, and
	// immediately confirm turning it back off with non-zero stock is also
	// blocked.
	if _, err := d.CreateStockMovement(CreateStockMovementRequest{
		OrganizationID: product.OrganizationID, ProductID: product.ID, Type: "out", Quantity: -3,
	}); err != nil {
		t.Fatalf("CreateStockMovement (zero out): %v", err)
	}
	reserialized, err := d.UpdateProduct(product.ID, UpdateProductRequest{
		Name: "Widget", Type: "product", StockEnabled: 1, Serialized: 1,
	})
	if err != nil {
		t.Fatalf("UpdateProduct (turn on, zero stock): %v", err)
	}
	if reserialized.Serialized != 1 {
		t.Fatalf("expected serialized=1, got %d", reserialized.Serialized)
	}
}

// CreateStockMovement's serialized branch: "in" registers new/returning
// serials, "out" requires each to already be in stock, and non-in/out types
// are rejected outright since they have no per-unit mapping.
func TestCreateStockMovementSerializedManualInOut(t *testing.T) {
	d, product := seedSerializedProduct(t, "org-ser-manual")

	result, err := d.CreateStockMovement(CreateStockMovementRequest{
		OrganizationID: product.OrganizationID, ProductID: product.ID, Type: "in",
		SerialNumbers: []string{"SN-1", "SN-2"}, UnitCost: ptr(int64(500)),
	})
	if err != nil {
		t.Fatalf("CreateStockMovement (in): %v", err)
	}
	if len(result.Movements) != 2 {
		t.Fatalf("expected 2 movement rows, got %d", len(result.Movements))
	}
	if result.Product.StockQuantity != 2 {
		t.Fatalf("stockQuantity: got %v, want 2", result.Product.StockQuantity)
	}

	serials, err := d.GetProductSerialNumbers(product.ID)
	if err != nil || len(serials) != 2 {
		t.Fatalf("GetProductSerialNumbers: err=%v len=%d", err, len(serials))
	}
	for _, s := range serials {
		if s.InStock != 1 {
			t.Fatalf("serial %q should be in stock, got InStock=%d", s.SerialNumber, s.InStock)
		}
	}

	// Registering the same serial again while it's still in stock is rejected.
	_, err = d.CreateStockMovement(CreateStockMovementRequest{
		OrganizationID: product.OrganizationID, ProductID: product.ID, Type: "in",
		SerialNumbers: []string{"SN-1"},
	})
	mustValidationError(t, err)

	// Removing a serial not in stock is rejected.
	_, err = d.CreateStockMovement(CreateStockMovementRequest{
		OrganizationID: product.OrganizationID, ProductID: product.ID, Type: "out",
		SerialNumbers: []string{"SN-NEVER-REGISTERED"},
	})
	mustValidationError(t, err)

	// Removing an in-stock serial succeeds and flips its status.
	result, err = d.CreateStockMovement(CreateStockMovementRequest{
		OrganizationID: product.OrganizationID, ProductID: product.ID, Type: "out",
		SerialNumbers: []string{"SN-1"},
	})
	if err != nil {
		t.Fatalf("CreateStockMovement (out): %v", err)
	}
	if result.Product.StockQuantity != 1 {
		t.Fatalf("stockQuantity after removing SN-1: got %v, want 1", result.Product.StockQuantity)
	}

	// adjustment/count types have no per-unit mapping and are rejected.
	_, err = d.CreateStockMovement(CreateStockMovementRequest{
		OrganizationID: product.OrganizationID, ProductID: product.ID, Type: "adjustment",
		SerialNumbers: []string{"SN-3"},
	})
	mustValidationError(t, err)
}

// Receiving a serialized line requires exactly as many serials as quantity,
// a whole-number quantity, and rejects duplicates within the request.
func TestSerializedInboundReceiveValidation(t *testing.T) {
	d, product := seedSerializedProduct(t, "org-ser-recv-val")

	receipt, err := d.CreateInboundDelivery(CreateInboundDeliveryRequest{
		OrganizationID: product.OrganizationID, DeliveryNumber: "GR-0001", DeliveryDate: 1700000000000,
		LineItems: []CreateInboundDeliveryLineItemRequest{
			{ProductID: &product.ID, Description: "Serial Widget", Quantity: 2.5},
		},
	})
	if err != nil {
		t.Fatalf("CreateInboundDelivery: %v", err)
	}
	items, _ := d.GetInboundDeliveryLineItems(receipt.ID)
	_, err = d.UpdateInboundDeliveryStatus(receipt.ID, "received", map[string][]string{
		items[0].ID: {"SN-A", "SN-B"},
	})
	mustValidationError(t, err) // fractional quantity

	receipt2, err := d.CreateInboundDelivery(CreateInboundDeliveryRequest{
		OrganizationID: product.OrganizationID, DeliveryNumber: "GR-0002", DeliveryDate: 1700000000000,
		LineItems: []CreateInboundDeliveryLineItemRequest{
			{ProductID: &product.ID, Description: "Serial Widget", Quantity: 2},
		},
	})
	if err != nil {
		t.Fatalf("CreateInboundDelivery: %v", err)
	}
	items2, _ := d.GetInboundDeliveryLineItems(receipt2.ID)

	// Wrong count.
	_, err = d.UpdateInboundDeliveryStatus(receipt2.ID, "received", map[string][]string{
		items2[0].ID: {"SN-A"},
	})
	mustValidationError(t, err)

	// Duplicate within the request.
	_, err = d.UpdateInboundDeliveryStatus(receipt2.ID, "received", map[string][]string{
		items2[0].ID: {"SN-A", "SN-A"},
	})
	mustValidationError(t, err)

	current, _ := d.GetInboundDelivery(receipt2.ID)
	if current.Status != "draft" {
		t.Fatalf("status changed despite rejected validation: %q", current.Status)
	}
}

// Receiving with valid serials registers each unit, raises stock, and stamps
// sourceDocumentId on every movement row.
func TestSerializedInboundReceiveRegistersSerialsAndRaisesStock(t *testing.T) {
	d, product := seedSerializedProduct(t, "org-ser-recv-ok")

	receipt, err := d.CreateInboundDelivery(CreateInboundDeliveryRequest{
		OrganizationID: product.OrganizationID, DeliveryNumber: "GR-0001", DeliveryDate: 1700000000000,
		LineItems: []CreateInboundDeliveryLineItemRequest{
			{ProductID: &product.ID, Description: "Serial Widget", Quantity: 3, UnitCost: ptr(float64(700))},
		},
	})
	if err != nil {
		t.Fatalf("CreateInboundDelivery: %v", err)
	}
	items, _ := d.GetInboundDeliveryLineItems(receipt.ID)

	if _, err := d.UpdateInboundDeliveryStatus(receipt.ID, "received", map[string][]string{
		items[0].ID: {"SN-1", "SN-2", "SN-3"},
	}); err != nil {
		t.Fatalf("UpdateInboundDeliveryStatus(received): %v", err)
	}

	p, _ := d.GetProduct(product.ID)
	if p.StockQuantity != 3 {
		t.Fatalf("stockQuantity: got %v, want 3", p.StockQuantity)
	}
	if p.UnitCost == nil || *p.UnitCost != 700 {
		t.Fatalf("unitCost: got %v, want 700", p.UnitCost)
	}

	serials, err := d.GetProductSerialNumbers(product.ID)
	if err != nil || len(serials) != 3 {
		t.Fatalf("GetProductSerialNumbers: err=%v len=%d", err, len(serials))
	}
	for _, s := range serials {
		if s.InStock != 1 {
			t.Fatalf("serial %q should be in stock", s.SerialNumber)
		}
	}

	movements, _ := d.GetProductStockMovements(product.ID)
	if len(movements) != 3 {
		t.Fatalf("expected 3 movement rows (one per unit), got %d", len(movements))
	}
	for _, m := range movements {
		if m.SerialNumberID == nil {
			t.Fatalf("movement %q missing serialNumberId", m.ID)
		}
		if m.SourceDocumentID == nil || *m.SourceDocumentID != receipt.ID {
			t.Fatalf("movement %q sourceDocumentId: got %v, want %q", m.ID, m.SourceDocumentID, receipt.ID)
		}
	}

	// A serial already in stock can't be received again on a second receipt.
	receipt2, err := d.CreateInboundDelivery(CreateInboundDeliveryRequest{
		OrganizationID: product.OrganizationID, DeliveryNumber: "GR-0002", DeliveryDate: 1700000000000,
		LineItems: []CreateInboundDeliveryLineItemRequest{
			{ProductID: &product.ID, Description: "Serial Widget", Quantity: 1},
		},
	})
	if err != nil {
		t.Fatalf("CreateInboundDelivery: %v", err)
	}
	items2, _ := d.GetInboundDeliveryLineItems(receipt2.ID)
	_, err = d.UpdateInboundDeliveryStatus(receipt2.ID, "received", map[string][]string{
		items2[0].ID: {"SN-1"},
	})
	mustValidationError(t, err)
}

// Cancelling a received receipt reverses exactly the serials it posted, and
// is rejected if any of them has since been shipped out.
func TestSerializedInboundCancelReversesExactSerials(t *testing.T) {
	d, product := seedSerializedProduct(t, "org-ser-recv-cancel")

	receipt, err := d.CreateInboundDelivery(CreateInboundDeliveryRequest{
		OrganizationID: product.OrganizationID, DeliveryNumber: "GR-0001", DeliveryDate: 1700000000000,
		LineItems: []CreateInboundDeliveryLineItemRequest{
			{ProductID: &product.ID, Description: "Serial Widget", Quantity: 2},
		},
	})
	if err != nil {
		t.Fatalf("CreateInboundDelivery: %v", err)
	}
	items, _ := d.GetInboundDeliveryLineItems(receipt.ID)
	if _, err := d.UpdateInboundDeliveryStatus(receipt.ID, "received", map[string][]string{
		items[0].ID: {"SN-1", "SN-2"},
	}); err != nil {
		t.Fatalf("receive: %v", err)
	}

	if _, err := d.UpdateInboundDeliveryStatus(receipt.ID, "cancelled", nil); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	p, _ := d.GetProduct(product.ID)
	if p.StockQuantity != 0 {
		t.Fatalf("stockQuantity after cancel: got %v, want 0", p.StockQuantity)
	}
	serials, _ := d.GetProductSerialNumbers(product.ID)
	for _, s := range serials {
		if s.InStock != 0 {
			t.Fatalf("serial %q should no longer be in stock after cancel", s.SerialNumber)
		}
	}
}

func TestSerializedInboundCancelRejectedWhenSerialAlreadyShipped(t *testing.T) {
	d, product := seedSerializedProduct(t, "org-ser-recv-shipped")

	receipt, err := d.CreateInboundDelivery(CreateInboundDeliveryRequest{
		OrganizationID: product.OrganizationID, DeliveryNumber: "GR-0001", DeliveryDate: 1700000000000,
		LineItems: []CreateInboundDeliveryLineItemRequest{
			{ProductID: &product.ID, Description: "Serial Widget", Quantity: 1},
		},
	})
	if err != nil {
		t.Fatalf("CreateInboundDelivery: %v", err)
	}
	items, _ := d.GetInboundDeliveryLineItems(receipt.ID)
	if _, err := d.UpdateInboundDeliveryStatus(receipt.ID, "received", map[string][]string{
		items[0].ID: {"SN-1"},
	}); err != nil {
		t.Fatalf("receive: %v", err)
	}

	// Ship the unit out via a manual movement (simplest way to move it along
	// without a full outbound delivery).
	if _, err := d.CreateStockMovement(CreateStockMovementRequest{
		OrganizationID: product.OrganizationID, ProductID: product.ID, Type: "out",
		SerialNumbers: []string{"SN-1"},
	}); err != nil {
		t.Fatalf("CreateStockMovement (out): %v", err)
	}

	_, err = d.UpdateInboundDeliveryStatus(receipt.ID, "cancelled", nil)
	mustValidationError(t, err)
}

// Shipping a serialized line requires each serial to already be registered
// and in stock; cancelling restores exactly the shipped serials.
func TestSerializedOutboundShipAndCancel(t *testing.T) {
	d, product := seedSerializedProduct(t, "org-ser-ship")

	// Shipping an unregistered serial is rejected outright.
	delivery, err := d.CreateDelivery(CreateDeliveryRequest{
		OrganizationID: product.OrganizationID, DeliveryNumber: "DEL-0001", DeliveryDate: 1700000000000,
		LineItems: []CreateDeliveryLineItemRequest{
			{ProductID: &product.ID, Description: "Serial Widget", Quantity: 2},
		},
	})
	if err != nil {
		t.Fatalf("CreateDelivery: %v", err)
	}
	dItems, _ := d.GetDeliveryLineItems(delivery.ID)
	_, err = d.UpdateDeliveryStatus(delivery.ID, "shipped", map[string][]string{
		dItems[0].ID: {"SN-1", "SN-2"},
	})
	mustValidationError(t, err)

	// Receive stock first.
	receipt, err := d.CreateInboundDelivery(CreateInboundDeliveryRequest{
		OrganizationID: product.OrganizationID, DeliveryNumber: "GR-0001", DeliveryDate: 1700000000000,
		LineItems: []CreateInboundDeliveryLineItemRequest{
			{ProductID: &product.ID, Description: "Serial Widget", Quantity: 2},
		},
	})
	if err != nil {
		t.Fatalf("CreateInboundDelivery: %v", err)
	}
	rItems, _ := d.GetInboundDeliveryLineItems(receipt.ID)
	if _, err := d.UpdateInboundDeliveryStatus(receipt.ID, "received", map[string][]string{
		rItems[0].ID: {"SN-1", "SN-2"},
	}); err != nil {
		t.Fatalf("receive: %v", err)
	}

	// Now shipping SN-1/SN-2 succeeds.
	if _, err := d.UpdateDeliveryStatus(delivery.ID, "shipped", map[string][]string{
		dItems[0].ID: {"SN-1", "SN-2"},
	}); err != nil {
		t.Fatalf("UpdateDeliveryStatus(shipped): %v", err)
	}
	p, _ := d.GetProduct(product.ID)
	if p.StockQuantity != 0 {
		t.Fatalf("stockQuantity after ship: got %v, want 0", p.StockQuantity)
	}
	serials, _ := d.GetProductSerialNumbers(product.ID)
	for _, s := range serials {
		if s.InStock != 0 {
			t.Fatalf("serial %q should be shipped out", s.SerialNumber)
		}
	}

	// Cancelling restores both units.
	if _, err := d.UpdateDeliveryStatus(delivery.ID, "cancelled", nil); err != nil {
		t.Fatalf("UpdateDeliveryStatus(cancelled): %v", err)
	}
	p, _ = d.GetProduct(product.ID)
	if p.StockQuantity != 2 {
		t.Fatalf("stockQuantity after cancel: got %v, want 2", p.StockQuantity)
	}
	serials, _ = d.GetProductSerialNumbers(product.ID)
	for _, s := range serials {
		if s.InStock != 1 {
			t.Fatalf("serial %q should be back in stock after cancel", s.SerialNumber)
		}
	}
}
