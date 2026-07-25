package db

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newTestDB returns a fully migrated database in a temp directory.
func newTestDB(t *testing.T) *Database {
	t.Helper()
	d, err := NewDatabase(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("NewDatabase: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func TestMigrations(t *testing.T) {
	d := newTestDB(t)
	// Verify the schema by checking a known table exists.
	var count int
	if err := d.DB.Get(&count,
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='invoices'`); err != nil {
		t.Fatalf("query schema: %v", err)
	}
	if count != 1 {
		t.Fatal("invoices table not found after migrations")
	}
}

func TestForeignKeysEnabled(t *testing.T) {
	d := newTestDB(t)
	var fk int
	if err := d.DB.Get(&fk, `PRAGMA foreign_keys`); err != nil {
		t.Fatalf("PRAGMA foreign_keys: %v", err)
	}
	if fk != 1 {
		t.Fatal("foreign_keys PRAGMA is off — referential integrity not enforced")
	}
}

func TestOrganizationCRUD(t *testing.T) {
	d := newTestDB(t)

	req := CreateOrganizationRequest{
		ID:   "test-org-001",
		Name: ptr("ACME Corp"),
	}
	org, err := d.CreateOrganization(req)
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	if org.ID != req.ID {
		t.Fatalf("got id %q, want %q", org.ID, req.ID)
	}

	orgs, err := d.GetOrganizations()
	if err != nil || len(orgs) != 1 {
		t.Fatalf("GetOrganizations: err=%v, len=%d", err, len(orgs))
	}

	updated, err := d.UpdateOrganization(org.ID, UpdateOrganizationRequest{Name: ptr("Updated Corp")})
	if err != nil || *updated.Name != "Updated Corp" {
		t.Fatalf("UpdateOrganization: err=%v, name=%v", err, updated.Name)
	}

	ok, err := d.DeleteOrganization(org.ID)
	if err != nil || !ok {
		t.Fatalf("DeleteOrganization: err=%v, ok=%v", err, ok)
	}

	orgs, _ = d.GetOrganizations()
	if len(orgs) != 0 {
		t.Fatal("organization not deleted")
	}
}

func TestClientCRUD(t *testing.T) {
	d := newTestDB(t)

	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-1"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}

	client, err := d.CreateClient(CreateClientRequest{
		ID:             "client-1",
		OrganizationID: org.ID,
		Name:           ptr("Test Client"),
	})
	if err != nil {
		t.Fatalf("CreateClient: %v", err)
	}

	fetched, err := d.GetClient(client.ID)
	if err != nil || *fetched.Name != "Test Client" {
		t.Fatalf("GetClient: err=%v, name=%v", err, fetched.Name)
	}

	updated, err := d.UpdateClient(client.ID, UpdateClientRequest{Name: ptr("Updated Client")})
	if err != nil || *updated.Name != "Updated Client" {
		t.Fatalf("UpdateClient: err=%v", err)
	}

	ok, err := d.DeleteClient(client.ID)
	if err != nil || !ok {
		t.Fatalf("DeleteClient: err=%v, ok=%v", err, ok)
	}
}

func TestInvoiceCRUD(t *testing.T) {
	d := newTestDB(t)

	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-1"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	client, _ := d.CreateClient(CreateClientRequest{
		ID: "client-1", OrganizationID: org.ID, Name: ptr("Client"),
	})

	taxRate, _ := d.CreateTaxRate(CreateTaxRateRequest{
		ID: "tax-1", OrganizationID: org.ID, Name: "VAT 20%", Percentage: 20,
	})

	createReq := CreateInvoiceRequest{
		ID:             "inv-1",
		OrganizationID: org.ID,
		Number:         "INV-001",
		State:          "draft",
		ClientID:       client.ID,
		Date:           1700000000000,
		Currency:       "EUR",
		// 2*5000 + 1*2000 = 12000 subtotal; 20% tax on the 10000 taxed portion = 2000.
		Total:    14000,
		TaxTotal: 2000,
		SubTotal: 12000,
		LineItems: []CreateInvoiceLineItemRequest{
			{Quantity: 2, UnitPrice: 5000, TaxRate: &taxRate.ID},
			{Quantity: 1, UnitPrice: 2000},
		},
	}

	inv, err := d.CreateInvoice(createReq)
	if err != nil {
		t.Fatalf("CreateInvoice: %v", err)
	}
	if inv.Number != "INV-001" {
		t.Fatalf("got number %q", inv.Number)
	}

	items, err := d.GetInvoiceLineItems(inv.ID)
	if err != nil || len(items) != 2 {
		t.Fatalf("GetInvoiceLineItems: err=%v, len=%d", err, len(items))
	}
	// Verify position ordering — first item should have position 0.
	if items[0].Position != 0 || items[1].Position != 1 {
		t.Fatalf("unexpected positions: %d, %d", items[0].Position, items[1].Position)
	}

	// Update — clear dueDate (was nil, stays nil, just confirms the query works).
	dueDate := int64(1700100000000)
	notes := "Pay promptly"
	_, err = d.UpdateInvoice(inv.ID, UpdateInvoiceRequest{
		DueDate:       &dueDate,
		CustomerNotes: &notes,
	})
	if err != nil {
		t.Fatalf("UpdateInvoice (set fields): %v", err)
	}

	// Clear nullable fields by passing nil.
	_, err = d.UpdateInvoice(inv.ID, UpdateInvoiceRequest{
		DueDate:       nil,
		CustomerNotes: nil,
	})
	if err != nil {
		t.Fatalf("UpdateInvoice (clear fields): %v", err)
	}
	cleared, _ := d.GetInvoice(inv.ID)
	if cleared.DueDate != nil || cleared.CustomerNotes != nil {
		t.Fatalf("nullable fields not cleared: dueDate=%v, notes=%v", cleared.DueDate, cleared.CustomerNotes)
	}

	ok, err := d.DeleteInvoice(inv.ID)
	if err != nil || !ok {
		t.Fatalf("DeleteInvoice: err=%v, ok=%v", err, ok)
	}
	// Line items should be gone (FK CASCADE).
	items, _ = d.GetInvoiceLineItems(inv.ID)
	if len(items) != 0 {
		t.Fatal("line items not deleted with invoice")
	}
}

func TestOrganizationCascadeDeletesClients(t *testing.T) {
	d := newTestDB(t)

	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-1"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	_, _ = d.CreateClient(CreateClientRequest{
		ID: "client-1", OrganizationID: org.ID, Name: ptr("Client A"),
	})

	_, _ = d.DeleteOrganization(org.ID)

	clients, err := d.GetClients(org.ID)
	if err != nil {
		t.Fatalf("GetClients: %v", err)
	}
	if len(clients) != 0 {
		t.Fatal("clients not cascade-deleted with organization")
	}
}

func TestDeliveryShipReducesStockAndCancelRestores(t *testing.T) {
	d := newTestDB(t)

	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-1"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}

	product, err := d.CreateProduct(CreateProductRequest{
		ID: "prod-1", OrganizationID: org.ID, Name: "Widget", Type: "product", StockEnabled: 1,
	})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	if _, err := d.CreateStockMovement(CreateStockMovementRequest{
		OrganizationID: org.ID, ProductID: product.ID, Type: "in", Quantity: 10,
	}); err != nil {
		t.Fatalf("CreateStockMovement (initial stock): %v", err)
	}

	order, err := d.CreateOrder(CreateOrderRequest{
		ID: "order-1", OrganizationID: org.ID, OrderNumber: "ORD-0001", Status: "confirmed",
		OrderDate: 1700000000000,
		LineItems: []CreateOrderLineItemRequest{
			{ProductID: &product.ID, Description: "Widget", Quantity: 5, UnitPrice: 1000},
		},
	})
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	orderLineItems, err := d.GetOrderLineItems(order.ID)
	if err != nil || len(orderLineItems) != 1 {
		t.Fatalf("GetOrderLineItems: err=%v, len=%d", err, len(orderLineItems))
	}
	orderLineItemID := orderLineItems[0].ID

	delivery, err := d.CreateDelivery(CreateDeliveryRequest{
		ID: "del-1", OrganizationID: org.ID, OrderID: &order.ID, DeliveryNumber: "DEL-0001",
		DeliveryDate: 1700000000000,
		LineItems: []CreateDeliveryLineItemRequest{
			{OrderLineItemID: &orderLineItemID, Description: "Widget", Quantity: 5},
		},
	})
	if err != nil {
		t.Fatalf("CreateDelivery: %v", err)
	}

	// Ship — should reduce stock and record a referenced "out" movement.
	if _, err := d.UpdateDeliveryStatus(delivery.ID, "shipped"); err != nil {
		t.Fatalf("UpdateDeliveryStatus(shipped): %v", err)
	}
	shipped, err := d.GetProduct(product.ID)
	if err != nil || shipped.StockQuantity != 5 {
		t.Fatalf("after ship: err=%v, stockQuantity=%v, want 5", err, shipped.StockQuantity)
	}
	movements, err := d.GetProductStockMovements(product.ID)
	if err != nil || len(movements) != 2 {
		t.Fatalf("GetProductStockMovements after ship: err=%v, len=%d, want 2", err, len(movements))
	}
	outMovement := findMovementByReference(movements, "DEL-0001", "out")
	if outMovement == nil || outMovement.Quantity != -5 {
		t.Fatalf("unexpected out movement: %+v", outMovement)
	}

	// Cancel the shipped delivery — should restore stock via a reversing "in" movement.
	if _, err := d.UpdateDeliveryStatus(delivery.ID, "cancelled"); err != nil {
		t.Fatalf("UpdateDeliveryStatus(cancelled): %v", err)
	}
	restored, err := d.GetProduct(product.ID)
	if err != nil || restored.StockQuantity != 10 {
		t.Fatalf("after cancel: err=%v, stockQuantity=%v, want 10", err, restored.StockQuantity)
	}
	movements, err = d.GetProductStockMovements(product.ID)
	if err != nil || len(movements) != 3 {
		t.Fatalf("GetProductStockMovements after cancel: err=%v, len=%d, want 3", err, len(movements))
	}
	reversal := findMovementByReference(movements, "DEL-0001", "in")
	if reversal == nil || reversal.Quantity != 5 {
		t.Fatalf("unexpected reversing in movement: %+v", reversal)
	}
}

func findMovementByReference(movements []StockMovement, reference, movementType string) *StockMovement {
	for i := range movements {
		if movements[i].Type == movementType && movements[i].Reference != nil && *movements[i].Reference == reference {
			return &movements[i]
		}
	}
	return nil
}

func TestDeliveryShipInsufficientStockBlocked(t *testing.T) {
	d := newTestDB(t)

	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-1"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}

	product, err := d.CreateProduct(CreateProductRequest{
		ID: "prod-1", OrganizationID: org.ID, Name: "Widget", Type: "product", StockEnabled: 1,
	})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	if _, err := d.CreateStockMovement(CreateStockMovementRequest{
		OrganizationID: org.ID, ProductID: product.ID, Type: "in", Quantity: 2,
	}); err != nil {
		t.Fatalf("CreateStockMovement (initial stock): %v", err)
	}

	order, err := d.CreateOrder(CreateOrderRequest{
		ID: "order-1", OrganizationID: org.ID, OrderNumber: "ORD-0001", Status: "confirmed",
		OrderDate: 1700000000000,
		LineItems: []CreateOrderLineItemRequest{
			{ProductID: &product.ID, Description: "Widget", Quantity: 5, UnitPrice: 1000},
		},
	})
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	orderLineItems, err := d.GetOrderLineItems(order.ID)
	if err != nil || len(orderLineItems) != 1 {
		t.Fatalf("GetOrderLineItems: err=%v, len=%d", err, len(orderLineItems))
	}
	orderLineItemID := orderLineItems[0].ID

	delivery, err := d.CreateDelivery(CreateDeliveryRequest{
		ID: "del-1", OrganizationID: org.ID, OrderID: &order.ID, DeliveryNumber: "DEL-0001",
		DeliveryDate: 1700000000000,
		LineItems: []CreateDeliveryLineItemRequest{
			{OrderLineItemID: &orderLineItemID, Description: "Widget", Quantity: 5},
		},
	})
	if err != nil {
		t.Fatalf("CreateDelivery: %v", err)
	}

	if _, err := d.UpdateDeliveryStatus(delivery.ID, "shipped"); err == nil {
		t.Fatal("expected shipping to be blocked by insufficient stock, got nil error")
	}

	unchanged, err := d.GetProduct(product.ID)
	if err != nil || unchanged.StockQuantity != 2 {
		t.Fatalf("stock should be untouched: err=%v, stockQuantity=%v, want 2", err, unchanged.StockQuantity)
	}
	stillDraft, err := d.GetDelivery(delivery.ID)
	if err != nil || stillDraft.Status != "draft" {
		t.Fatalf("delivery status should be unchanged: err=%v, status=%v, want draft", err, stillDraft.Status)
	}
}

func TestStandaloneDeliveryShipReducesStock(t *testing.T) {
	d := newTestDB(t)

	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-1"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}

	product, err := d.CreateProduct(CreateProductRequest{
		ID: "prod-1", OrganizationID: org.ID, Name: "Gadget", Type: "product", StockEnabled: 1,
	})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	if _, err := d.CreateStockMovement(CreateStockMovementRequest{
		OrganizationID: org.ID, ProductID: product.ID, Type: "in", Quantity: 10,
	}); err != nil {
		t.Fatalf("CreateStockMovement (initial stock): %v", err)
	}

	// No order involved — the line item picks the product directly.
	delivery, err := d.CreateDelivery(CreateDeliveryRequest{
		ID: "del-1", OrganizationID: org.ID, DeliveryNumber: "DEL-0001",
		DeliveryDate: 1700000000000,
		LineItems: []CreateDeliveryLineItemRequest{
			{ProductID: &product.ID, Description: "Gadget", Quantity: 4},
		},
	})
	if err != nil {
		t.Fatalf("CreateDelivery: %v", err)
	}

	if _, err := d.UpdateDeliveryStatus(delivery.ID, "shipped"); err != nil {
		t.Fatalf("UpdateDeliveryStatus(shipped): %v", err)
	}
	shipped, err := d.GetProduct(product.ID)
	if err != nil || shipped.StockQuantity != 6 {
		t.Fatalf("after ship: err=%v, stockQuantity=%v, want 6", err, shipped.StockQuantity)
	}

	items, err := d.GetDeliveryLineItems(delivery.ID)
	if err != nil || len(items) != 1 {
		t.Fatalf("GetDeliveryLineItems: err=%v, len=%d", err, len(items))
	}
	if items[0].ProductID == nil || *items[0].ProductID != product.ID {
		t.Fatalf("line item should carry productId directly: %+v", items[0])
	}
}

func TestProductCodeUniquePerOrganization(t *testing.T) {
	d := newTestDB(t)

	org1, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-1"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	org2, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-2"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}

	if _, err := d.CreateProduct(CreateProductRequest{
		ID: "prod-1", OrganizationID: org1.ID, Name: "Widget", Type: "product", SKU: ptr("WIDGET-1"),
	}); err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}

	// Same code, same organization — rejected.
	if _, err := d.CreateProduct(CreateProductRequest{
		ID: "prod-2", OrganizationID: org1.ID, Name: "Widget Mini", Type: "product", SKU: ptr("WIDGET-1"),
	}); err == nil {
		t.Fatal("expected duplicate product code within an organization to be rejected")
	} else if !strings.Contains(err.Error(), "product code already in use") {
		t.Fatalf("expected a friendly duplicate-code error, got: %v", err)
	}

	// Same code, different organization — allowed.
	if _, err := d.CreateProduct(CreateProductRequest{
		ID: "prod-3", OrganizationID: org2.ID, Name: "Widget", Type: "product", SKU: ptr("WIDGET-1"),
	}); err != nil {
		t.Fatalf("expected same code in a different organization to succeed, got: %v", err)
	}
}

func ptr[T any](v T) *T { return &v }

// TestDeliveryStatusTransitions is a table-driven matrix covering every
// (from, to) pair over the delivery status lifecycle: only draft→{shipped,
// cancelled} and shipped→{delivered,cancelled} are legal moves;
// delivered/cancelled are terminal; same-status is always a no-op. Each case
// force-sets the starting status directly via SQL so the guard in
// UpdateDeliveryStatus is isolated from the stock-movement side effects
// already covered by TestDeliveryShipReducesStockAndCancelRestores.
func TestDeliveryStatusTransitions(t *testing.T) {
	tests := []struct {
		from    string
		to      string
		wantErr bool
	}{
		{"draft", "shipped", false},
		{"draft", "cancelled", false},
		{"draft", "delivered", true},
		{"draft", "draft", false},
		{"shipped", "delivered", false},
		{"shipped", "cancelled", false},
		{"shipped", "draft", true},
		{"shipped", "shipped", false},
		{"delivered", "shipped", true},
		{"delivered", "cancelled", true},
		{"delivered", "delivered", false},
		{"cancelled", "shipped", true},
		{"cancelled", "draft", true},
		{"cancelled", "cancelled", false},
	}

	for _, tc := range tests {
		t.Run(tc.from+"_to_"+tc.to, func(t *testing.T) {
			d := newTestDB(t)
			org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-1"})
			if err != nil {
				t.Fatalf("CreateOrganization: %v", err)
			}
			delivery, err := d.CreateDelivery(CreateDeliveryRequest{
				ID: "del-1", OrganizationID: org.ID, DeliveryNumber: "DEL-0001", DeliveryDate: 1700000000000,
			})
			if err != nil {
				t.Fatalf("CreateDelivery: %v", err)
			}
			if _, err := d.DB.Exec(`UPDATE outbound_deliveries SET status = ? WHERE id = ?`, tc.from, delivery.ID); err != nil {
				t.Fatalf("force status to %q: %v", tc.from, err)
			}

			_, err = d.UpdateDeliveryStatus(delivery.ID, tc.to)
			if tc.wantErr && err == nil {
				t.Fatalf("expected transition %s -> %s to be rejected", tc.from, tc.to)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected transition %s -> %s to succeed, got: %v", tc.from, tc.to, err)
			}
		})
	}
}

// TestOrderStatusTransitions mirrors TestDeliveryStatusTransitions for the
// order lifecycle: draft→{confirmed,cancelled}, confirmed→{shipped,cancelled},
// shipped→{delivered,cancelled}; delivered/cancelled terminal; same-status a
// no-op.
func TestOrderStatusTransitions(t *testing.T) {
	tests := []struct {
		from    string
		to      string
		wantErr bool
	}{
		{"draft", "confirmed", false},
		{"draft", "cancelled", false},
		{"draft", "shipped", true},
		{"draft", "draft", false},
		{"confirmed", "shipped", false},
		{"confirmed", "cancelled", false},
		{"confirmed", "draft", true},
		{"confirmed", "confirmed", false},
		{"shipped", "delivered", false},
		{"shipped", "cancelled", false},
		{"shipped", "confirmed", true},
		{"shipped", "shipped", false},
		{"delivered", "shipped", true},
		{"delivered", "cancelled", true},
		{"delivered", "delivered", false},
		{"cancelled", "confirmed", true},
		{"cancelled", "cancelled", false},
	}

	for _, tc := range tests {
		t.Run(tc.from+"_to_"+tc.to, func(t *testing.T) {
			d := newTestDB(t)
			org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-1"})
			if err != nil {
				t.Fatalf("CreateOrganization: %v", err)
			}
			order, err := d.CreateOrder(CreateOrderRequest{
				ID: "order-1", OrganizationID: org.ID, OrderNumber: "ORD-0001", OrderDate: 1700000000000,
			})
			if err != nil {
				t.Fatalf("CreateOrder: %v", err)
			}
			if _, err := d.DB.Exec(`UPDATE orders SET status = ? WHERE id = ?`, tc.from, order.ID); err != nil {
				t.Fatalf("force status to %q: %v", tc.from, err)
			}

			_, err = d.UpdateOrderStatus(order.ID, tc.to)
			if tc.wantErr && err == nil {
				t.Fatalf("expected transition %s -> %s to be rejected", tc.from, tc.to)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected transition %s -> %s to succeed, got: %v", tc.from, tc.to, err)
			}
		})
	}
}

func TestCreateOrderStatusValidation(t *testing.T) {
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-1"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}

	order, err := d.CreateOrder(CreateOrderRequest{
		ID: "order-1", OrganizationID: org.ID, OrderNumber: "ORD-0001", OrderDate: 1700000000000,
	})
	if err != nil {
		t.Fatalf("CreateOrder with empty status: %v", err)
	}
	if order.Status != "draft" {
		t.Fatalf("expected empty status to default to draft, got %q", order.Status)
	}

	if _, err := d.CreateOrder(CreateOrderRequest{
		ID: "order-2", OrganizationID: org.ID, OrderNumber: "ORD-0002", OrderDate: 1700000000000, Status: "bogus",
	}); err == nil {
		t.Fatal("expected an invalid order status to be rejected")
	}
}

// TestCreateDeliveryLineItemFailureRollsBackAtomically covers F8: a
// mid-batch line-item failure (here, a FK violation from a nonexistent
// productId) must not leave a delivery header persisted with only some of
// its line items.
func TestCreateDeliveryLineItemFailureRollsBackAtomically(t *testing.T) {
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-1"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}

	badProductID := "does-not-exist"
	_, err = d.CreateDelivery(CreateDeliveryRequest{
		ID: "del-1", OrganizationID: org.ID, DeliveryNumber: "DEL-0001", DeliveryDate: 1700000000000,
		LineItems: []CreateDeliveryLineItemRequest{
			{Description: "Valid line", Quantity: 1},
			{Description: "Bad line", Quantity: 1, ProductID: &badProductID},
		},
	})
	if err == nil {
		t.Fatal("expected CreateDelivery to fail when a line item references a nonexistent product")
	}

	if _, err := d.GetDelivery("del-1"); err == nil {
		t.Fatal("expected the delivery header to be rolled back along with its line items")
	}
}

// TestUpdateDeliveryLineItemFailureRollsBackAtomically is the UpdateDelivery
// counterpart: a failed line-item replacement must leave the original line
// items in place, not a half-deleted state.
func TestUpdateDeliveryLineItemFailureRollsBackAtomically(t *testing.T) {
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-1"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	delivery, err := d.CreateDelivery(CreateDeliveryRequest{
		ID: "del-1", OrganizationID: org.ID, DeliveryNumber: "DEL-0001", DeliveryDate: 1700000000000,
		LineItems: []CreateDeliveryLineItemRequest{{Description: "Original", Quantity: 1}},
	})
	if err != nil {
		t.Fatalf("CreateDelivery: %v", err)
	}

	badProductID := "does-not-exist"
	newItems := []CreateDeliveryLineItemRequest{{Description: "Replacement", Quantity: 1, ProductID: &badProductID}}
	if _, err := d.UpdateDelivery(delivery.ID, UpdateDeliveryRequest{LineItems: &newItems}); err == nil {
		t.Fatal("expected UpdateDelivery to fail when a replacement line item references a nonexistent product")
	}

	items, err := d.GetDeliveryLineItems(delivery.ID)
	if err != nil {
		t.Fatalf("GetDeliveryLineItems: %v", err)
	}
	if len(items) != 1 || items[0].Description != "Original" {
		t.Fatalf("expected original line items to survive a failed update, got %+v", items)
	}
}

// TestNextDeliveryNumberSkipsGapsFromDeletions covers F9: COUNT(*)+1 would
// reissue an in-use number as soon as any non-newest delivery is deleted.
func TestNextDeliveryNumberSkipsGapsFromDeletions(t *testing.T) {
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-1"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}

	for _, num := range []string{"DEL-0001", "DEL-0002", "DEL-0003"} {
		if _, err := d.CreateDelivery(CreateDeliveryRequest{
			ID: num, OrganizationID: org.ID, DeliveryNumber: num, DeliveryDate: 1700000000000,
		}); err != nil {
			t.Fatalf("CreateDelivery(%s): %v", num, err)
		}
	}
	if got, want := d.NextDeliveryNumber(org.ID), "DEL-0004"; got != want {
		t.Fatalf("NextDeliveryNumber before delete: got %q, want %q", got, want)
	}

	// Deleting the middle delivery leaves a gap (DEL-0001, DEL-0003 remain).
	// COUNT(*)+1 would now propose DEL-0003 again, colliding with the
	// still-existing delivery of that number.
	if ok, err := d.DeleteDelivery("DEL-0002"); err != nil || !ok {
		t.Fatalf("DeleteDelivery: ok=%v, err=%v", ok, err)
	}
	if got, want := d.NextDeliveryNumber(org.ID), "DEL-0004"; got != want {
		t.Fatalf("NextDeliveryNumber after deleting a gap delivery: got %q, want %q (must not collide with DEL-0003)", got, want)
	}
}

// TestUpdateDeliveryRejectsLineItemEditAfterShip covers F4: once a delivery
// has shipped, its line items are frozen (they've already generated stock
// movements) — only header fields like tracking number remain editable.
func TestUpdateDeliveryRejectsLineItemEditAfterShip(t *testing.T) {
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-1"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	delivery, err := d.CreateDelivery(CreateDeliveryRequest{
		ID: "del-1", OrganizationID: org.ID, DeliveryNumber: "DEL-0001", DeliveryDate: 1700000000000,
		LineItems: []CreateDeliveryLineItemRequest{{Description: "Widget", Quantity: 1}},
	})
	if err != nil {
		t.Fatalf("CreateDelivery: %v", err)
	}
	if _, err := d.UpdateDeliveryStatus(delivery.ID, "shipped"); err != nil {
		t.Fatalf("UpdateDeliveryStatus(shipped): %v", err)
	}

	newItems := []CreateDeliveryLineItemRequest{{Description: "Widget", Quantity: 2}}
	if _, err := d.UpdateDelivery(delivery.ID, UpdateDeliveryRequest{LineItems: &newItems}); err == nil {
		t.Fatal("expected editing line items of a shipped delivery to be rejected")
	}

	newTracking := "TRACK-123"
	if _, err := d.UpdateDelivery(delivery.ID, UpdateDeliveryRequest{TrackingNumber: &newTracking}); err != nil {
		t.Fatalf("expected a header-only update on a shipped delivery to succeed: %v", err)
	}
}

// TestBackupFilePermissions covers F15: VACUUM INTO creates files with
// SQLite's default (world-readable) mode — Backup must tighten that down to
// owner-only since it's a full copy of the financial database.
func TestBackupFilePermissions(t *testing.T) {
	d := newTestDB(t)
	dest := filepath.Join(t.TempDir(), "backup.db")
	if err := d.Backup(dest); err != nil {
		t.Fatalf("Backup: %v", err)
	}
	info, err := os.Stat(dest)
	if err != nil {
		t.Fatalf("stat backup file: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Fatalf("backup file mode = %o, want 0600", perm)
	}
}

// TestCreateInvoiceRejectsMismatchedTotals covers F18: totals are otherwise
// client-computed and stored verbatim — a total that doesn't match the line
// items must be rejected rather than silently stored.
func TestCreateInvoiceRejectsMismatchedTotals(t *testing.T) {
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-1"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	client, err := d.CreateClient(CreateClientRequest{ID: "client-1", OrganizationID: org.ID, Name: ptr("Client")})
	if err != nil {
		t.Fatalf("CreateClient: %v", err)
	}

	_, err = d.CreateInvoice(CreateInvoiceRequest{
		ID: "inv-1", OrganizationID: org.ID, Number: "INV-001", State: "draft", ClientID: client.ID,
		Date: 1700000000000, Currency: "EUR",
		Total: 1, TaxTotal: 0, SubTotal: 1, // a Widget worth 100.00 stored as 0.01
		LineItems: []CreateInvoiceLineItemRequest{{Quantity: 1, UnitPrice: 10000}},
	})
	if err == nil {
		t.Fatal("expected a mismatched total to be rejected")
	}
	if _, ok := err.(*ValidationError); !ok {
		t.Fatalf("expected a *ValidationError (409), got %T: %v", err, err)
	}
}

// TestCreateInvoiceAcceptsRoundingBoundary is the positive counterpart,
// pinned to a case verified against the real frontend (a 3.33 unit price at
// 19.5% tax — the true tax is 0.64935, landing exactly on the halfway point
// between 0.64 and 0.65 once rounded to cents). Exercising this through the
// actual browser produced subtotal=333, tax=65, total=398; the Go-side
// recompute must agree exactly, or every invoice using this tax rate would
// start getting rejected.
func TestCreateInvoiceAcceptsRoundingBoundary(t *testing.T) {
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-1"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	client, err := d.CreateClient(CreateClientRequest{ID: "client-1", OrganizationID: org.ID, Name: ptr("Client")})
	if err != nil {
		t.Fatalf("CreateClient: %v", err)
	}
	taxRate, err := d.CreateTaxRate(CreateTaxRateRequest{
		ID: "tax-1", OrganizationID: org.ID, Name: "VAT 19.5", Percentage: 19.5,
	})
	if err != nil {
		t.Fatalf("CreateTaxRate: %v", err)
	}

	inv, err := d.CreateInvoice(CreateInvoiceRequest{
		ID: "inv-1", OrganizationID: org.ID, Number: "INV-001", State: "draft", ClientID: client.ID,
		Date: 1700000000000, Currency: "EUR",
		Total: 398, TaxTotal: 65, SubTotal: 333,
		LineItems: []CreateInvoiceLineItemRequest{
			{Quantity: 1, UnitPrice: 333, TaxRate: &taxRate.ID},
		},
	})
	if err != nil {
		t.Fatalf("expected the rounding-boundary totals to be accepted, got: %v", err)
	}
	if inv.SubTotal != 333 || inv.TaxTotal != 65 || inv.Total != 398 {
		t.Fatalf("got subtotal=%d tax=%d total=%d, want 333/65/398", inv.SubTotal, inv.TaxTotal, inv.Total)
	}
}

// TestCreateInvoiceAcceptsFractionalQuantity is a regression lock for the
// other reason the recompute uses exact rational arithmetic instead of
// float64: a fractional quantity (1.5 units at 3.33 each, 19.5% tax) still
// has to land on exactly the right cent.
func TestCreateInvoiceAcceptsFractionalQuantity(t *testing.T) {
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-1"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	client, err := d.CreateClient(CreateClientRequest{ID: "client-1", OrganizationID: org.ID, Name: ptr("Client")})
	if err != nil {
		t.Fatalf("CreateClient: %v", err)
	}
	taxRate, err := d.CreateTaxRate(CreateTaxRateRequest{
		ID: "tax-1", OrganizationID: org.ID, Name: "VAT 19.5", Percentage: 19.5,
	})
	if err != nil {
		t.Fatalf("CreateTaxRate: %v", err)
	}

	inv, err := d.CreateInvoice(CreateInvoiceRequest{
		ID: "inv-1", OrganizationID: org.ID, Number: "INV-001", State: "draft", ClientID: client.ID,
		Date: 1700000000000, Currency: "EUR",
		Total: 597, TaxTotal: 97, SubTotal: 500,
		LineItems: []CreateInvoiceLineItemRequest{
			{Quantity: 1.5, UnitPrice: 333, TaxRate: &taxRate.ID},
		},
	})
	if err != nil {
		t.Fatalf("expected the fractional-quantity totals to be accepted, got: %v", err)
	}
	if inv.SubTotal != 500 || inv.TaxTotal != 97 || inv.Total != 597 {
		t.Fatalf("got subtotal=%d tax=%d total=%d, want 500/97/597", inv.SubTotal, inv.TaxTotal, inv.Total)
	}
}

// TestUpdateInvoiceHeaderOnlyDoesNotValidateTotals: a header-only edit (no
// line items, no totals) has nothing to recompute against and must not be
// rejected.
func TestUpdateInvoiceHeaderOnlyDoesNotValidateTotals(t *testing.T) {
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-1"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	client, err := d.CreateClient(CreateClientRequest{ID: "client-1", OrganizationID: org.ID, Name: ptr("Client")})
	if err != nil {
		t.Fatalf("CreateClient: %v", err)
	}
	inv, err := d.CreateInvoice(CreateInvoiceRequest{
		ID: "inv-1", OrganizationID: org.ID, Number: "INV-001", State: "draft", ClientID: client.ID,
		Date: 1700000000000, Currency: "EUR",
		Total: 10000, TaxTotal: 0, SubTotal: 10000,
		LineItems: []CreateInvoiceLineItemRequest{{Quantity: 1, UnitPrice: 10000}},
	})
	if err != nil {
		t.Fatalf("CreateInvoice: %v", err)
	}

	notes := "Thanks for your business"
	if _, err := d.UpdateInvoice(inv.ID, UpdateInvoiceRequest{CustomerNotes: &notes}); err != nil {
		t.Fatalf("expected a header-only update to succeed: %v", err)
	}
}

// TestUpdateInvoiceRejectsTotalsOnlyMismatch covers the partial-update
// bypass: a request that sends only new totals (no lineItems) must still be
// validated against the invoice's *stored* line items, not skipped just
// because lineItems is absent from this particular request.
func TestUpdateInvoiceRejectsTotalsOnlyMismatch(t *testing.T) {
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-1"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	client, err := d.CreateClient(CreateClientRequest{ID: "client-1", OrganizationID: org.ID, Name: ptr("Client")})
	if err != nil {
		t.Fatalf("CreateClient: %v", err)
	}
	inv, err := d.CreateInvoice(CreateInvoiceRequest{
		ID: "inv-1", OrganizationID: org.ID, Number: "INV-001", State: "draft", ClientID: client.ID,
		Date: 1700000000000, Currency: "EUR",
		Total: 10000, TaxTotal: 0, SubTotal: 10000,
		LineItems: []CreateInvoiceLineItemRequest{{Quantity: 1, UnitPrice: 10000}},
	})
	if err != nil {
		t.Fatalf("CreateInvoice: %v", err)
	}

	// Line items are untouched (still worth 10000) — inflating just the total
	// must be rejected against the stored line items, not silently accepted.
	inflatedTotal := int64(999999)
	if _, err := d.UpdateInvoice(inv.ID, UpdateInvoiceRequest{Total: &inflatedTotal}); err == nil {
		t.Fatal("expected a totals-only update that doesn't match stored line items to be rejected")
	}

	// A totals-only update that's actually still correct must still succeed.
	correctTotal := int64(10000)
	if _, err := d.UpdateInvoice(inv.ID, UpdateInvoiceRequest{Total: &correctTotal}); err != nil {
		t.Fatalf("expected a totals-only update matching stored line items to succeed: %v", err)
	}
}

// TestUpdateInvoiceRejectsLineItemsOnlyMismatch is the mirror case: new,
// more expensive line items sent without updated totals must be validated
// against the invoice's *stored* totals, not skipped just because the
// totals fields are absent from this request.
func TestUpdateInvoiceRejectsLineItemsOnlyMismatch(t *testing.T) {
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-1"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	client, err := d.CreateClient(CreateClientRequest{ID: "client-1", OrganizationID: org.ID, Name: ptr("Client")})
	if err != nil {
		t.Fatalf("CreateClient: %v", err)
	}
	inv, err := d.CreateInvoice(CreateInvoiceRequest{
		ID: "inv-1", OrganizationID: org.ID, Number: "INV-001", State: "draft", ClientID: client.ID,
		Date: 1700000000000, Currency: "EUR",
		Total: 10000, TaxTotal: 0, SubTotal: 10000,
		LineItems: []CreateInvoiceLineItemRequest{{Quantity: 1, UnitPrice: 10000}},
	})
	if err != nil {
		t.Fatalf("CreateInvoice: %v", err)
	}

	// Stored totals stay at 10000 — swapping in a pricier line item without
	// updating them must be rejected against the stored totals.
	expensiveItems := []CreateInvoiceLineItemRequest{{Quantity: 1, UnitPrice: 999999}}
	if _, err := d.UpdateInvoice(inv.ID, UpdateInvoiceRequest{LineItems: &expensiveItems}); err == nil {
		t.Fatal("expected new line items that don't match stored totals to be rejected")
	}
}

// TestInvoiceStateValidation covers F20: invoice state is validated against
// the canonical set on create and on the PATCH state endpoint, empty defaults
// to draft, and unknown values are rejected.
func TestInvoiceStateValidation(t *testing.T) {
	d := newTestDB(t)
	org, _ := d.CreateOrganization(CreateOrganizationRequest{ID: "org-1"})
	client, _ := d.CreateClient(CreateClientRequest{ID: "client-1", OrganizationID: org.ID, Name: ptr("Client")})

	base := func(id, state string) CreateInvoiceRequest {
		return CreateInvoiceRequest{
			ID: id, OrganizationID: org.ID, Number: id, State: state,
			ClientID: client.ID, Date: 1700000000000, Currency: "EUR",
			Total: 0, TaxTotal: 0, SubTotal: 0,
			LineItems: []CreateInvoiceLineItemRequest{},
		}
	}

	// Unknown state on create is rejected.
	if _, err := d.CreateInvoice(base("inv-bad", "confirmed")); err == nil {
		t.Fatal("expected create with unknown state to be rejected")
	}

	// Empty state defaults to draft.
	inv, err := d.CreateInvoice(base("inv-1", ""))
	if err != nil {
		t.Fatalf("CreateInvoice with empty state: %v", err)
	}
	if inv.State != "draft" {
		t.Fatalf("expected empty state to default to draft, got %q", inv.State)
	}

	// Each canonical state is accepted by the PATCH endpoint.
	for _, s := range []string{"sent", "paid", "cancelled", "draft"} {
		if _, err := d.UpdateInvoiceState(inv.ID, s); err != nil {
			t.Fatalf("UpdateInvoiceState(%q): %v", s, err)
		}
	}

	// A non-canonical state is rejected by the PATCH endpoint.
	if _, err := d.UpdateInvoiceState(inv.ID, "confirmed"); err == nil {
		t.Fatal("expected UpdateInvoiceState with unknown state to be rejected")
	}
}

// TestGetOrganizationsOmitsLogo covers F29: the org list must not ship the
// logo BLOB (re-fetched on every auth change), while the single-org fetch
// still returns it for the invoice PDF / settings form.
func TestGetOrganizationsOmitsLogo(t *testing.T) {
	d := newTestDB(t)
	logo := []byte("PRETEND-THIS-IS-A-BIG-PNG")
	if _, err := d.CreateOrganization(CreateOrganizationRequest{
		ID: "org-1", Name: ptr("ACME"), Logo: logo,
	}); err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}

	list, err := d.GetOrganizations()
	if err != nil || len(list) != 1 {
		t.Fatalf("GetOrganizations: err=%v len=%d", err, len(list))
	}
	if list[0].Logo != nil {
		t.Fatalf("expected list logo to be omitted, got %d bytes", len(list[0].Logo))
	}

	single, err := d.GetOrganization("org-1")
	if err != nil {
		t.Fatalf("GetOrganization: %v", err)
	}
	if string(single.Logo) != string(logo) {
		t.Fatalf("expected single-org fetch to keep the logo, got %q", single.Logo)
	}
}

func TestVendorCRUD(t *testing.T) {
	d := newTestDB(t)

	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-vendor", Name: ptr("ACME Corp")})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}

	vendor, err := d.CreateVendor(CreateVendorRequest{
		OrganizationID:   org.ID,
		Name:             ptr("Supplier Ltd"),
		Code:             ptr("SU"),
		Emails:           ptr(`["sales@supplier.example"]`),
		DefaultCurrency:  ptr("EUR"),
		PaymentTermsDays: ptr(int64(30)),
	})
	if err != nil {
		t.Fatalf("CreateVendor: %v", err)
	}
	if vendor.ID == "" {
		t.Fatal("CreateVendor did not generate an id")
	}
	if vendor.PaymentTermsDays == nil || *vendor.PaymentTermsDays != 30 {
		t.Fatalf("paymentTermsDays not round-tripped: %v", vendor.PaymentTermsDays)
	}

	vendors, err := d.GetVendors(org.ID)
	if err != nil {
		t.Fatalf("GetVendors: %v", err)
	}
	if len(vendors) != 1 {
		t.Fatalf("got %d vendors, want 1", len(vendors))
	}

	updated, err := d.UpdateVendor(vendor.ID, UpdateVendorRequest{Name: ptr("Supplier GmbH")})
	if err != nil {
		t.Fatalf("UpdateVendor: %v", err)
	}
	if updated.Name == nil || *updated.Name != "Supplier GmbH" {
		t.Fatalf("name not updated: %v", updated.Name)
	}

	ok, err := d.DeleteVendor(vendor.ID)
	if err != nil {
		t.Fatalf("DeleteVendor: %v", err)
	}
	if !ok {
		t.Fatal("DeleteVendor reported no rows removed")
	}
}

// Vendors must be scoped to their organization and cascade away with it, the
// same as clients — otherwise deleting an org leaves orphaned master data.
func TestVendorCascadesWithOrganization(t *testing.T) {
	d := newTestDB(t)

	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-cascade", Name: ptr("ACME Corp")})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	if _, err := d.CreateVendor(CreateVendorRequest{OrganizationID: org.ID, Name: ptr("Supplier Ltd")}); err != nil {
		t.Fatalf("CreateVendor: %v", err)
	}

	counts, err := d.GetOrganizationUsageCount(org.ID)
	if err != nil {
		t.Fatalf("GetOrganizationUsageCount: %v", err)
	}
	if counts.Vendors != 1 {
		t.Fatalf("usage count reported %d vendors, want 1", counts.Vendors)
	}

	if _, err := d.DeleteOrganization(org.ID); err != nil {
		t.Fatalf("DeleteOrganization: %v", err)
	}

	var remaining int
	if err := d.DB.Get(&remaining, `SELECT COUNT(*) FROM vendors WHERE organizationId = ?`, org.ID); err != nil {
		t.Fatalf("count vendors: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("%d vendors survived organization deletion", remaining)
	}
}

// TestVendorDocumentCountCoversEveryReference is a tripwire for the purchasing
// phases that follow. DeleteVendor's guard is only as good as the list of
// tables GetVendorDocumentCount actually counts, and a migration that adds a
// vendorId column without updating that list turns the guard into a no-op —
// deleting a referenced vendor would then surface a raw foreign-key error as an
// opaque 500, which is exactly what the guard exists to prevent.
//
// Rather than trusting a comment, this discovers every table with a vendorId
// column from the live schema and requires it to be covered.
func TestVendorDocumentCountCoversEveryReference(t *testing.T) {
	d := newTestDB(t)

	tables := []string{}
	if err := d.DB.Select(&tables,
		`SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`,
	); err != nil {
		t.Fatalf("list tables: %v", err)
	}

	covered := map[string]bool{}
	for _, name := range vendorReferencingTables {
		covered[name] = true
	}

	for _, table := range tables {
		columns := []struct {
			Name string `db:"name"`
		}{}
		if err := d.DB.Select(&columns, `SELECT name FROM pragma_table_info(?)`, table); err != nil {
			t.Fatalf("pragma_table_info(%s): %v", table, err)
		}
		for _, col := range columns {
			if col.Name != "vendorId" {
				continue
			}
			if !covered[table] {
				t.Errorf(
					"table %q has a vendorId column but is not in vendorReferencingTables — "+
						"DeleteVendor's guard would not see its rows; add it to the list in db/vendor.go",
					table,
				)
			}
		}
	}

	// The converse: a listed table that no longer exists would make every count query fail.
	existing := map[string]bool{}
	for _, name := range tables {
		existing[name] = true
	}
	for _, name := range vendorReferencingTables {
		if !existing[name] {
			t.Errorf("vendorReferencingTables lists %q, which is not a table in the schema", name)
		}
	}
}

// TestPurchaseOrderStatusTransitions mirrors TestOrderStatusTransitions for the
// purchase order lifecycle: draft→{confirmed,cancelled},
// confirmed→{received,cancelled}; received/cancelled terminal; same-status a
// no-op. Each case force-sets the starting status directly via SQL so the guard
// is isolated from the rest of the update path.
func TestPurchaseOrderStatusTransitions(t *testing.T) {
	tests := []struct {
		from    string
		to      string
		wantErr bool
	}{
		{"draft", "confirmed", false},
		{"draft", "cancelled", false},
		{"draft", "received", true},
		{"draft", "draft", false},
		{"confirmed", "received", false},
		{"confirmed", "cancelled", false},
		{"confirmed", "draft", true},
		{"confirmed", "confirmed", false},
		{"received", "confirmed", true},
		{"received", "cancelled", true},
		{"received", "received", false},
		{"cancelled", "confirmed", true},
		{"cancelled", "draft", true},
		{"cancelled", "cancelled", false},
	}

	for _, tc := range tests {
		t.Run(tc.from+"_to_"+tc.to, func(t *testing.T) {
			d := newTestDB(t)
			org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-1"})
			if err != nil {
				t.Fatalf("CreateOrganization: %v", err)
			}
			order, err := d.CreatePurchaseOrder(CreatePurchaseOrderRequest{
				ID: "po-1", OrganizationID: org.ID, OrderNumber: "PO-0001", OrderDate: 1700000000000,
			})
			if err != nil {
				t.Fatalf("CreatePurchaseOrder: %v", err)
			}
			if _, err := d.DB.Exec(
				`UPDATE purchase_orders SET status = ? WHERE id = ?`, tc.from, order.ID,
			); err != nil {
				t.Fatalf("force status to %q: %v", tc.from, err)
			}

			_, err = d.UpdatePurchaseOrderStatus(order.ID, tc.to)
			if tc.wantErr && err == nil {
				t.Fatalf("expected transition %s -> %s to be rejected", tc.from, tc.to)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected transition %s -> %s to succeed, got: %v", tc.from, tc.to, err)
			}
		})
	}
}

func TestPurchaseOrderCRUDAndLineItems(t *testing.T) {
	d := newTestDB(t)

	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-po", Name: ptr("ACME Corp")})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	vendor, err := d.CreateVendor(CreateVendorRequest{OrganizationID: org.ID, Name: ptr("Supplier Ltd")})
	if err != nil {
		t.Fatalf("CreateVendor: %v", err)
	}

	order, err := d.CreatePurchaseOrder(CreatePurchaseOrderRequest{
		OrganizationID: org.ID,
		VendorID:       &vendor.ID,
		OrderNumber:    "PO-0001",
		OrderDate:      1700000000000,
		LineItems: []CreatePurchaseOrderLineItemRequest{
			{Description: "Widgets", Quantity: 10, UnitPrice: 250}, // cents
			{Description: "Gadgets", Quantity: 4, UnitPrice: 199},
		},
	})
	if err != nil {
		t.Fatalf("CreatePurchaseOrder: %v", err)
	}
	if order.Status != "draft" {
		t.Fatalf("expected default status draft, got %q", order.Status)
	}
	if order.VendorName == nil || *order.VendorName != "Supplier Ltd" {
		t.Fatalf("vendorName not joined: %v", order.VendorName)
	}

	items, err := d.GetPurchaseOrderLineItems(order.ID)
	if err != nil {
		t.Fatalf("GetPurchaseOrderLineItems: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("got %d line items, want 2", len(items))
	}
	if items[0].Position != 0 || items[1].Position != 1 {
		t.Fatalf("positions not assigned in slice order: %d, %d", items[0].Position, items[1].Position)
	}
	if items[0].UnitPrice != 250 {
		t.Fatalf("unitPrice not stored as cents: got %d, want 250", items[0].UnitPrice)
	}

	// Replacing line items renumbers positions and drops the old rows.
	_, err = d.UpdatePurchaseOrder(order.ID, UpdatePurchaseOrderRequest{
		LineItems: &[]CreatePurchaseOrderLineItemRequest{
			{Description: "Only one", Quantity: 1, UnitPrice: 100},
		},
	})
	if err != nil {
		t.Fatalf("UpdatePurchaseOrder: %v", err)
	}
	items, _ = d.GetPurchaseOrderLineItems(order.ID)
	if len(items) != 1 || items[0].Description != "Only one" {
		t.Fatalf("line items not replaced: %+v", items)
	}
}

// A received purchase order can't be deleted — it must be cancelled instead,
// mirroring the guard on shipped/delivered sales orders.
func TestDeleteReceivedPurchaseOrderIsRejected(t *testing.T) {
	d := newTestDB(t)
	org, _ := d.CreateOrganization(CreateOrganizationRequest{ID: "org-po-del"})
	order, err := d.CreatePurchaseOrder(CreatePurchaseOrderRequest{
		OrganizationID: org.ID, OrderNumber: "PO-0001", OrderDate: 1700000000000,
	})
	if err != nil {
		t.Fatalf("CreatePurchaseOrder: %v", err)
	}
	if _, err := d.DB.Exec(`UPDATE purchase_orders SET status = 'received' WHERE id = ?`, order.ID); err != nil {
		t.Fatalf("force status: %v", err)
	}

	ok, err := d.DeletePurchaseOrder(order.ID)
	if err == nil {
		t.Fatal("expected deleting a received purchase order to be rejected")
	}
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected a *ValidationError (surfaced as 409), got %T: %v", err, err)
	}
	if ok {
		t.Fatal("DeletePurchaseOrder reported success despite the guard")
	}
}

// NextPurchaseOrderNumber must continue from the highest number in use, not
// COUNT(*)+1 — the latter reissues a number as soon as one is deleted. The
// SUBSTR offset is prefix-length-sensitive ("PO-" is 3 chars, unlike "DEL-").
func TestNextPurchaseOrderNumber(t *testing.T) {
	d := newTestDB(t)
	org, _ := d.CreateOrganization(CreateOrganizationRequest{ID: "org-num"})

	if got := d.NextPurchaseOrderNumber(org.ID); got != "PO-0001" {
		t.Fatalf("first number: got %q, want PO-0001", got)
	}

	for _, num := range []string{"PO-0001", "PO-0002", "PO-0007"} {
		if _, err := d.CreatePurchaseOrder(CreatePurchaseOrderRequest{
			OrganizationID: org.ID, OrderNumber: num, OrderDate: 1700000000000,
		}); err != nil {
			t.Fatalf("CreatePurchaseOrder %s: %v", num, err)
		}
	}
	if got := d.NextPurchaseOrderNumber(org.ID); got != "PO-0008" {
		t.Fatalf("after PO-0007: got %q, want PO-0008 (SUBSTR offset likely wrong)", got)
	}

	// Deleting the highest must not reissue a number already used by a live row.
	var id string
	if err := d.DB.Get(&id, `SELECT id FROM purchase_orders WHERE orderNumber = 'PO-0002'`); err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if _, err := d.DeletePurchaseOrder(id); err != nil {
		t.Fatalf("DeletePurchaseOrder: %v", err)
	}
	if got := d.NextPurchaseOrderNumber(org.ID); got != "PO-0008" {
		t.Fatalf("after deleting a middle order: got %q, want PO-0008", got)
	}
}

// Status must not be settable through PUT — only through the PATCH status
// endpoint, which enforces the transition matrix.
func TestUpdatePurchaseOrderCannotChangeStatus(t *testing.T) {
	d := newTestDB(t)
	org, _ := d.CreateOrganization(CreateOrganizationRequest{ID: "org-po-status"})
	order, err := d.CreatePurchaseOrder(CreatePurchaseOrderRequest{
		OrganizationID: org.ID, OrderNumber: "PO-0001", OrderDate: 1700000000000,
	})
	if err != nil {
		t.Fatalf("CreatePurchaseOrder: %v", err)
	}

	updated, err := d.UpdatePurchaseOrder(order.ID, UpdatePurchaseOrderRequest{
		Notes: ptr("header-only edit"),
	})
	if err != nil {
		t.Fatalf("UpdatePurchaseOrder: %v", err)
	}
	if updated.Status != "draft" {
		t.Fatalf("status changed through PUT: got %q, want draft", updated.Status)
	}
}

// A partial PUT must not orphan a purchase order from its vendor. vendorId is
// COALESCE'd for the same reason its foreign key deliberately has no
// ON DELETE SET NULL: a null vendor on an existing order is never a legitimate
// state, and silently producing one would defeat DeleteVendor's guard.
func TestUpdatePurchaseOrderKeepsVendorOnPartialUpdate(t *testing.T) {
	d := newTestDB(t)
	org, _ := d.CreateOrganization(CreateOrganizationRequest{ID: "org-po-vendor"})
	vendor, err := d.CreateVendor(CreateVendorRequest{OrganizationID: org.ID, Name: ptr("Supplier Ltd")})
	if err != nil {
		t.Fatalf("CreateVendor: %v", err)
	}
	order, err := d.CreatePurchaseOrder(CreatePurchaseOrderRequest{
		OrganizationID: org.ID, VendorID: &vendor.ID, OrderNumber: "PO-0001", OrderDate: 1700000000000,
	})
	if err != nil {
		t.Fatalf("CreatePurchaseOrder: %v", err)
	}

	// A header-only edit that doesn't mention the vendor at all.
	updated, err := d.UpdatePurchaseOrder(order.ID, UpdatePurchaseOrderRequest{Notes: ptr("edited")})
	if err != nil {
		t.Fatalf("UpdatePurchaseOrder: %v", err)
	}
	if updated.VendorID == nil || *updated.VendorID != vendor.ID {
		t.Fatalf("partial update orphaned the order from its vendor: got %v, want %q", updated.VendorID, vendor.ID)
	}

	// The vendor is still referenced, so deleting it must still be refused.
	if _, err := d.DeleteVendor(vendor.ID); !errors.Is(err, ErrVendorInUse) {
		t.Fatalf("expected ErrVendorInUse after partial update, got %v", err)
	}
}

// seedCostProduct creates an org + stock-tracked product for the costing tests.
func seedCostProduct(t *testing.T, d *Database, orgID string) *Product {
	t.Helper()
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: orgID})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	product, err := d.CreateProduct(CreateProductRequest{
		OrganizationID: org.ID, Name: "Widget", SKU: ptr("WID-1"), Type: "product", StockEnabled: 1,
	})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	return product
}

func productUnitCost(t *testing.T, d *Database, productID string) *int64 {
	t.Helper()
	p, err := d.GetProduct(productID)
	if err != nil {
		t.Fatalf("GetProduct: %v", err)
	}
	return p.UnitCost
}

// TestAverageCostWeightsInflows is the core costing case: receiving 10 @ 100
// then 10 @ 200 must value stock at the weighted average of 150, and an outflow
// must consume at that average without moving it.
func TestAverageCostWeightsInflows(t *testing.T) {
	d := newTestDB(t)
	product := seedCostProduct(t, d, "org-cost-1")

	in := func(qty float64, cost int64) {
		t.Helper()
		if _, err := d.CreateStockMovement(CreateStockMovementRequest{
			OrganizationID: product.OrganizationID, ProductID: product.ID,
			Type: "in", Quantity: qty, UnitCost: ptr(cost),
		}); err != nil {
			t.Fatalf("CreateStockMovement in: %v", err)
		}
	}

	in(10, 100)
	if got := productUnitCost(t, d, product.ID); got == nil || *got != 100 {
		t.Fatalf("after first receipt: got %v, want 100", got)
	}

	in(10, 200)
	if got := productUnitCost(t, d, product.ID); got == nil || *got != 150 {
		t.Fatalf("weighted average wrong: got %v, want 150", got)
	}

	// An outflow consumes at the current average and must not change it.
	if _, err := d.CreateStockMovement(CreateStockMovementRequest{
		OrganizationID: product.OrganizationID, ProductID: product.ID,
		Type: "out", Quantity: -5,
	}); err != nil {
		t.Fatalf("CreateStockMovement out: %v", err)
	}
	if got := productUnitCost(t, d, product.ID); got == nil || *got != 150 {
		t.Fatalf("outflow moved the average: got %v, want 150", got)
	}
	p, _ := d.GetProduct(product.ID)
	if p.StockQuantity != 15 {
		t.Fatalf("stockQuantity: got %v, want 15", p.StockQuantity)
	}
}

// A product with no costed inflow keeps whatever cost the user typed — cost only
// becomes derived once real purchase data exists, so existing products are
// untouched.
func TestAverageCostLeavesManualCostAloneWithoutCostedInflows(t *testing.T) {
	d := newTestDB(t)
	org, _ := d.CreateOrganization(CreateOrganizationRequest{ID: "org-cost-2"})
	product, err := d.CreateProduct(CreateProductRequest{
		OrganizationID: org.ID, Name: "Widget", SKU: ptr("WID-1"),
		Type: "product", StockEnabled: 1, UnitCost: ptr(int64(4242)),
	})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}

	// Uncosted movements in both directions must not disturb the manual value.
	if _, err := d.CreateStockMovement(CreateStockMovementRequest{
		OrganizationID: org.ID, ProductID: product.ID, Type: "in", Quantity: 10,
	}); err != nil {
		t.Fatalf("CreateStockMovement: %v", err)
	}
	if _, err := d.CreateStockMovement(CreateStockMovementRequest{
		OrganizationID: org.ID, ProductID: product.ID, Type: "out", Quantity: -3,
	}); err != nil {
		t.Fatalf("CreateStockMovement: %v", err)
	}

	if got := productUnitCost(t, d, product.ID); got == nil || *got != 4242 {
		t.Fatalf("manual unit cost was overwritten: got %v, want 4242", got)
	}
}

// Replay must be a pure function of the movement history: recomputing twice
// over the same rows yields the same answer, with no drift.
func TestAverageCostReplayIsDeterministic(t *testing.T) {
	d := newTestDB(t)
	product := seedCostProduct(t, d, "org-cost-3")

	for _, m := range []struct {
		qty  float64
		cost *int64
	}{{10, ptr(int64(100))}, {5, ptr(int64(310))}, {-4, nil}, {8, ptr(int64(90))}} {
		if _, err := d.CreateStockMovement(CreateStockMovementRequest{
			OrganizationID: product.OrganizationID, ProductID: product.ID,
			Type: "in", Quantity: m.qty, UnitCost: m.cost,
		}); err != nil {
			t.Fatalf("CreateStockMovement: %v", err)
		}
	}

	first := productUnitCost(t, d, product.ID)
	if first == nil {
		t.Fatal("no average cost computed")
	}
	// Recompute standalone — the same history must give the same number.
	if err := recomputeAverageCostTx(d.DB, product.ID); err != nil {
		t.Fatalf("recomputeAverageCostTx: %v", err)
	}
	second := productUnitCost(t, d, product.ID)
	if second == nil || *first != *second {
		t.Fatalf("replay not deterministic: first %v, second %v", first, second)
	}
}

// seedReceipt builds a purchase order and a draft goods receipt against it.
func seedReceipt(t *testing.T, d *Database, orgID string, qty float64, unitCost int64) (*Product, *InboundDelivery) {
	t.Helper()
	product := seedCostProduct(t, d, orgID)
	vendor, err := d.CreateVendor(CreateVendorRequest{OrganizationID: orgID, Name: ptr("Supplier Ltd")})
	if err != nil {
		t.Fatalf("CreateVendor: %v", err)
	}
	po, err := d.CreatePurchaseOrder(CreatePurchaseOrderRequest{
		OrganizationID: orgID, VendorID: &vendor.ID, OrderNumber: "PO-0001", OrderDate: 1700000000000,
		LineItems: []CreatePurchaseOrderLineItemRequest{
			{ProductID: &product.ID, Description: "Widget", Quantity: qty, UnitPrice: float64(unitCost)},
		},
	})
	if err != nil {
		t.Fatalf("CreatePurchaseOrder: %v", err)
	}
	poItems, _ := d.GetPurchaseOrderLineItems(po.ID)

	receipt, err := d.CreateInboundDelivery(CreateInboundDeliveryRequest{
		OrganizationID: orgID, PurchaseOrderID: &po.ID, VendorID: &vendor.ID,
		DeliveryNumber: "GR-0001", DeliveryDate: 1700000000000,
		LineItems: []CreateInboundDeliveryLineItemRequest{
			// Neither productId nor unitCost given — both must resolve from the PO line.
			{PurchaseOrderLineItemID: &poItems[0].ID, Description: "Widget", Quantity: qty},
		},
	})
	if err != nil {
		t.Fatalf("CreateInboundDelivery: %v", err)
	}
	return product, receipt
}

// Receiving raises stock, records an "in" movement referencing the receipt, and
// values the goods at the purchase order's price — resolved server-side, since
// the request named neither the product nor the cost.
func TestInboundReceiptRaisesStockAndSetsCost(t *testing.T) {
	d := newTestDB(t)
	product, receipt := seedReceipt(t, d, "org-inb-1", 10, 250)

	items, err := d.GetInboundDeliveryLineItems(receipt.ID)
	if err != nil || len(items) != 1 {
		t.Fatalf("GetInboundDeliveryLineItems: err=%v len=%d", err, len(items))
	}
	if items[0].ProductID == nil || *items[0].ProductID != product.ID {
		t.Fatalf("productId not resolved from the purchase order line: %v", items[0].ProductID)
	}
	if items[0].UnitCost == nil || *items[0].UnitCost != 250 {
		t.Fatalf("unitCost not resolved from the purchase order line: %v", items[0].UnitCost)
	}

	if _, err := d.UpdateInboundDeliveryStatus(receipt.ID, "received"); err != nil {
		t.Fatalf("UpdateInboundDeliveryStatus: %v", err)
	}

	p, _ := d.GetProduct(product.ID)
	if p.StockQuantity != 10 {
		t.Fatalf("stockQuantity: got %v, want 10", p.StockQuantity)
	}
	if p.UnitCost == nil || *p.UnitCost != 250 {
		t.Fatalf("unitCost: got %v, want 250", p.UnitCost)
	}

	movements, _ := d.GetProductStockMovements(product.ID)
	if len(movements) != 1 || movements[0].Type != "in" || movements[0].Quantity != 10 {
		t.Fatalf("expected one +10 \"in\" movement, got %+v", movements)
	}
	if movements[0].Reference == nil || *movements[0].Reference != "GR-0001" {
		t.Fatalf("movement not referenced by receipt number: %v", movements[0].Reference)
	}

	// Received quantities feed the purchase order's per-line fulfilment view.
	received, err := d.GetPurchaseOrderReceivedQuantities(*receipt.PurchaseOrderID)
	if err != nil {
		t.Fatalf("GetPurchaseOrderReceivedQuantities: %v", err)
	}
	if len(received) != 1 {
		t.Fatalf("expected one line's received quantity, got %+v", received)
	}
	for _, qty := range received {
		if qty != 10 {
			t.Fatalf("received quantity: got %v, want 10", qty)
		}
	}
}

// Cancelling a received receipt reverses the stock it added.
func TestInboundCancelReversesStock(t *testing.T) {
	d := newTestDB(t)
	product, receipt := seedReceipt(t, d, "org-inb-2", 10, 250)

	if _, err := d.UpdateInboundDeliveryStatus(receipt.ID, "received"); err != nil {
		t.Fatalf("receive: %v", err)
	}
	if _, err := d.UpdateInboundDeliveryStatus(receipt.ID, "cancelled"); err != nil {
		t.Fatalf("cancel: %v", err)
	}

	p, _ := d.GetProduct(product.ID)
	if p.StockQuantity != 0 {
		t.Fatalf("stock not reversed: got %v, want 0", p.StockQuantity)
	}
	movements, _ := d.GetProductStockMovements(product.ID)
	if len(movements) != 2 {
		t.Fatalf("expected an original and a reversing movement, got %d", len(movements))
	}

	// Cancelling does NOT restore a previous average — a reversal removes
	// quantity at the current average. With only this receipt in history the
	// average stays at its price; the assertion guards against someone
	// "fixing" the replay into non-standard costing.
	if p.UnitCost == nil || *p.UnitCost != 250 {
		t.Fatalf("unit cost after cancellation: got %v, want 250", p.UnitCost)
	}
}

// The guard the outbound side has no equivalent for: cancelling a receipt whose
// goods have already been shipped out would drive stock negative, so it is
// rejected and nothing changes.
func TestInboundCancelRejectedWhenStockAlreadyConsumed(t *testing.T) {
	d := newTestDB(t)
	product, receipt := seedReceipt(t, d, "org-inb-3", 10, 250)

	if _, err := d.UpdateInboundDeliveryStatus(receipt.ID, "received"); err != nil {
		t.Fatalf("receive: %v", err)
	}
	// Ship 6 of the 10 units out.
	if _, err := d.CreateStockMovement(CreateStockMovementRequest{
		OrganizationID: product.OrganizationID, ProductID: product.ID, Type: "out", Quantity: -6,
	}); err != nil {
		t.Fatalf("CreateStockMovement: %v", err)
	}

	_, err := d.UpdateInboundDeliveryStatus(receipt.ID, "cancelled")
	if err == nil {
		t.Fatal("expected cancelling a partly-consumed receipt to be rejected")
	}
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected a *ValidationError (surfaced as 409), got %T: %v", err, err)
	}

	current, _ := d.GetInboundDelivery(receipt.ID)
	if current.Status != "received" {
		t.Fatalf("status changed despite rejection: %q", current.Status)
	}
	p, _ := d.GetProduct(product.ID)
	if p.StockQuantity != 4 {
		t.Fatalf("stock changed despite rejection: got %v, want 4", p.StockQuantity)
	}
}

// A standalone receipt (no purchase order) still moves stock, using the product
// named directly on the line.
func TestInboundStandaloneReceiptMovesStock(t *testing.T) {
	d := newTestDB(t)
	product := seedCostProduct(t, d, "org-inb-4")

	receipt, err := d.CreateInboundDelivery(CreateInboundDeliveryRequest{
		OrganizationID: product.OrganizationID, DeliveryNumber: "GR-0001", DeliveryDate: 1700000000000,
		LineItems: []CreateInboundDeliveryLineItemRequest{
			{ProductID: &product.ID, Description: "Widget", Quantity: 7, UnitCost: ptr(float64(400))},
		},
	})
	if err != nil {
		t.Fatalf("CreateInboundDelivery: %v", err)
	}
	if _, err := d.UpdateInboundDeliveryStatus(receipt.ID, "received"); err != nil {
		t.Fatalf("receive: %v", err)
	}

	p, _ := d.GetProduct(product.ID)
	if p.StockQuantity != 7 {
		t.Fatalf("stockQuantity: got %v, want 7", p.StockQuantity)
	}
	if p.UnitCost == nil || *p.UnitCost != 400 {
		t.Fatalf("unitCost: got %v, want 400", p.UnitCost)
	}
}

// Line items freeze once received; header-only edits stay allowed, which is why
// UpdateInboundDelivery COALESCEs every column.
func TestInboundLineItemsFrozenAfterReceipt(t *testing.T) {
	d := newTestDB(t)
	_, receipt := seedReceipt(t, d, "org-inb-5", 10, 250)

	if _, err := d.UpdateInboundDeliveryStatus(receipt.ID, "received"); err != nil {
		t.Fatalf("receive: %v", err)
	}

	_, err := d.UpdateInboundDelivery(receipt.ID, UpdateInboundDeliveryRequest{
		LineItems: &[]CreateInboundDeliveryLineItemRequest{},
	})
	if err == nil {
		t.Fatal("expected editing line items of a received receipt to be rejected")
	}
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected a *ValidationError, got %T: %v", err, err)
	}

	updated, err := d.UpdateInboundDelivery(receipt.ID, UpdateInboundDeliveryRequest{
		TrackingNumber: ptr("TRACK-1"),
	})
	if err != nil {
		t.Fatalf("header-only edit rejected: %v", err)
	}
	if updated.TrackingNumber == nil || *updated.TrackingNumber != "TRACK-1" {
		t.Fatalf("tracking number not saved: %v", updated.TrackingNumber)
	}
	if updated.DeliveryNumber != "GR-0001" {
		t.Fatalf("header-only edit blanked deliveryNumber: %q", updated.DeliveryNumber)
	}

	if _, err := d.DeleteInboundDelivery(receipt.ID); err == nil {
		t.Fatal("expected deleting a received receipt to be rejected")
	}
}

// TestInboundDeliveryStatusTransitions covers every (from, to) pair.
func TestInboundDeliveryStatusTransitions(t *testing.T) {
	tests := []struct {
		from    string
		to      string
		wantErr bool
	}{
		{"draft", "received", false},
		{"draft", "cancelled", false},
		{"draft", "draft", false},
		{"received", "cancelled", false},
		{"received", "draft", true},
		{"received", "received", false},
		{"cancelled", "received", true},
		{"cancelled", "draft", true},
		{"cancelled", "cancelled", false},
	}

	for _, tc := range tests {
		t.Run(tc.from+"_to_"+tc.to, func(t *testing.T) {
			d := newTestDB(t)
			org, _ := d.CreateOrganization(CreateOrganizationRequest{ID: "org-1"})
			receipt, err := d.CreateInboundDelivery(CreateInboundDeliveryRequest{
				OrganizationID: org.ID, DeliveryNumber: "GR-0001", DeliveryDate: 1700000000000,
			})
			if err != nil {
				t.Fatalf("CreateInboundDelivery: %v", err)
			}
			if _, err := d.DB.Exec(
				`UPDATE inbound_deliveries SET status = ? WHERE id = ?`, tc.from, receipt.ID,
			); err != nil {
				t.Fatalf("force status to %q: %v", tc.from, err)
			}

			_, err = d.UpdateInboundDeliveryStatus(receipt.ID, tc.to)
			if tc.wantErr && err == nil {
				t.Fatalf("expected transition %s -> %s to be rejected", tc.from, tc.to)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected transition %s -> %s to succeed, got: %v", tc.from, tc.to, err)
			}
		})
	}
}

// TestTaxRateUsageCountCoversEveryReference is the tripwire for the tax rate
// delete guard. taxRates is referenced by several line-item tables, some with
// ON DELETE CASCADE — a table missing from taxRateReferencingTables would let
// an in-use rate be deleted and silently strip line items off existing
// invoices, which is exactly what DeleteTaxRate exists to prevent.
func TestTaxRateUsageCountCoversEveryReference(t *testing.T) {
	d := newTestDB(t)

	tables := []string{}
	if err := d.DB.Select(&tables,
		`SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`,
	); err != nil {
		t.Fatalf("list tables: %v", err)
	}

	covered := map[string]bool{}
	for _, name := range taxRateReferencingTables {
		covered[name] = true
	}

	for _, table := range tables {
		columns := []struct {
			Name string `db:"name"`
		}{}
		if err := d.DB.Select(&columns, `SELECT name FROM pragma_table_info(?)`, table); err != nil {
			t.Fatalf("pragma_table_info(%s): %v", table, err)
		}
		for _, col := range columns {
			if col.Name == "taxRate" && !covered[table] {
				t.Errorf(
					"table %q has a taxRate column but is not in taxRateReferencingTables — "+
						"DeleteTaxRate's guard would not see its rows; add it in db/tax_rate.go",
					table,
				)
			}
		}
	}
}

// seedMatch builds an org, vendor, product, confirmed purchase order and a
// received goods receipt, returning the ids matching needs.
type matchFixture struct {
	OrgID    string
	VendorID string
	OrderID  string
	POLineID string
	TaxRate  *TaxRate
}

func seedMatch(t *testing.T, d *Database, orgID string, ordered float64, unitPrice int64, received float64) matchFixture {
	t.Helper()
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: orgID})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	vendor, err := d.CreateVendor(CreateVendorRequest{OrganizationID: org.ID, Name: ptr("Supplier Ltd")})
	if err != nil {
		t.Fatalf("CreateVendor: %v", err)
	}
	product, err := d.CreateProduct(CreateProductRequest{
		OrganizationID: org.ID, Name: "Widget", SKU: ptr("WID-1"), Type: "product", StockEnabled: 1,
	})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	po, err := d.CreatePurchaseOrder(CreatePurchaseOrderRequest{
		OrganizationID: org.ID, VendorID: &vendor.ID, OrderNumber: "PO-0001", OrderDate: 1700000000000,
		LineItems: []CreatePurchaseOrderLineItemRequest{
			{ProductID: &product.ID, Description: "Widget", Quantity: ordered, UnitPrice: float64(unitPrice)},
		},
	})
	if err != nil {
		t.Fatalf("CreatePurchaseOrder: %v", err)
	}
	poItems, _ := d.GetPurchaseOrderLineItems(po.ID)

	if received > 0 {
		receipt, err := d.CreateInboundDelivery(CreateInboundDeliveryRequest{
			OrganizationID: org.ID, PurchaseOrderID: &po.ID, VendorID: &vendor.ID,
			DeliveryNumber: "GR-0001", DeliveryDate: 1700000000000,
			LineItems: []CreateInboundDeliveryLineItemRequest{
				{PurchaseOrderLineItemID: &poItems[0].ID, Description: "Widget", Quantity: received},
			},
		})
		if err != nil {
			t.Fatalf("CreateInboundDelivery: %v", err)
		}
		if _, err := d.UpdateInboundDeliveryStatus(receipt.ID, "received"); err != nil {
			t.Fatalf("receive: %v", err)
		}
	}

	return matchFixture{OrgID: org.ID, VendorID: vendor.ID, OrderID: po.ID, POLineID: poItems[0].ID}
}

func createIncomingInvoice(t *testing.T, d *Database, f matchFixture, number string, qty float64, unitPrice int64) *IncomingInvoice {
	t.Helper()
	subTotal := int64(qty * float64(unitPrice))
	inv, err := d.CreateIncomingInvoice(CreateIncomingInvoiceRequest{
		OrganizationID: f.OrgID, VendorID: f.VendorID, PurchaseOrderID: &f.OrderID,
		VendorInvoiceNumber: number, Date: 1700000000000, Currency: "EUR",
		SubTotal: subTotal, TaxTotal: 0, Total: subTotal,
		LineItems: []CreateInvoiceLineItemRequest{
			{
				Description: ptr("Widget"), Quantity: qty, UnitPrice: float64(unitPrice),
				PurchaseOrderLineItemID: &f.POLineID,
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateIncomingInvoice: %v", err)
	}
	return inv
}

// An invoice matching what was ordered and received is clean, and approving it
// is allowed.
func TestIncomingInvoiceMatches(t *testing.T) {
	d := newTestDB(t)
	f := seedMatch(t, d, "org-match-1", 10, 250, 10)
	inv := createIncomingInvoice(t, d, f, "V-001", 10, 250)

	lines, err := d.GetIncomingInvoiceMatch(inv.ID)
	if err != nil {
		t.Fatalf("GetIncomingInvoiceMatch: %v", err)
	}
	if len(lines) != 1 {
		t.Fatalf("expected one match line, got %d", len(lines))
	}
	if lines[0].Status != MatchMatched {
		t.Fatalf("status: got %q (%s), want matched", lines[0].Status, lines[0].Message)
	}
	if lines[0].ReceivedQuantity == nil || *lines[0].ReceivedQuantity != 10 {
		t.Fatalf("receivedQuantity: %v", lines[0].ReceivedQuantity)
	}

	if _, err := d.UpdateIncomingInvoiceState(inv.ID, "approved"); err != nil {
		t.Fatalf("approving a matched invoice was rejected: %v", err)
	}
}

// Billing more than was received is the over-billing case, and it blocks
// approval — but not saving.
func TestIncomingInvoiceOverReceivedBlocksApproval(t *testing.T) {
	d := newTestDB(t)
	f := seedMatch(t, d, "org-match-2", 10, 250, 4) // only 4 of 10 received
	inv := createIncomingInvoice(t, d, f, "V-001", 10, 250)

	lines, _ := d.GetIncomingInvoiceMatch(inv.ID)
	if lines[0].Status != MatchOverReceived {
		t.Fatalf("status: got %q, want over_received", lines[0].Status)
	}

	_, err := d.UpdateIncomingInvoiceState(inv.ID, "approved")
	if err == nil {
		t.Fatal("expected approving an over-billed invoice to be rejected")
	}
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected a *ValidationError, got %T: %v", err, err)
	}
	current, _ := d.GetIncomingInvoice(inv.ID)
	if current.State != "draft" {
		t.Fatalf("state changed despite rejection: %q", current.State)
	}
}

// A second invoice against the same order line must count what the first one
// already billed — otherwise the same goods can be billed twice.
func TestIncomingInvoiceDoubleBillingDetected(t *testing.T) {
	d := newTestDB(t)
	f := seedMatch(t, d, "org-match-3", 10, 250, 10)

	first := createIncomingInvoice(t, d, f, "V-001", 10, 250)
	if _, err := d.UpdateIncomingInvoiceState(first.ID, "approved"); err != nil {
		t.Fatalf("first invoice should match: %v", err)
	}

	second := createIncomingInvoice(t, d, f, "V-002", 10, 250)
	lines, _ := d.GetIncomingInvoiceMatch(second.ID)
	if lines[0].Status != MatchOverReceived {
		t.Fatalf("second invoice status: got %q, want over_received", lines[0].Status)
	}
	if lines[0].PreviouslyInvoiced != 10 {
		t.Fatalf("previouslyInvoiced: got %v, want 10", lines[0].PreviouslyInvoiced)
	}
	if _, err := d.UpdateIncomingInvoiceState(second.ID, "approved"); err == nil {
		t.Fatal("expected double-billing to block approval")
	}
}

// A unit price above what was ordered is a price variance.
func TestIncomingInvoicePriceVariance(t *testing.T) {
	d := newTestDB(t)
	f := seedMatch(t, d, "org-match-4", 10, 250, 10)
	inv := createIncomingInvoice(t, d, f, "V-001", 10, 300) // ordered at 250

	lines, _ := d.GetIncomingInvoiceMatch(inv.ID)
	if lines[0].Status != MatchPriceVariance {
		t.Fatalf("status: got %q, want price_variance", lines[0].Status)
	}
	if _, err := d.UpdateIncomingInvoiceState(inv.ID, "approved"); err == nil {
		t.Fatal("expected a price variance to block approval")
	}

	// Widening the organization's tolerance to 25% brings 300 within range.
	if _, err := d.UpdateOrganization(f.OrgID, UpdateOrganizationRequest{
		MatchPriceTolerancePercent: ptr(25.0),
	}); err != nil {
		t.Fatalf("UpdateOrganization: %v", err)
	}
	lines, _ = d.GetIncomingInvoiceMatch(inv.ID)
	if lines[0].Status != MatchMatched {
		t.Fatalf("with a 25%% tolerance: got %q, want matched", lines[0].Status)
	}
	if _, err := d.UpdateIncomingInvoiceState(inv.ID, "approved"); err != nil {
		t.Fatalf("within tolerance should approve: %v", err)
	}
}

// The override is the documented escape hatch, and it requires a reason.
func TestIncomingInvoiceOverrideRequiresReasonAndUnblocks(t *testing.T) {
	d := newTestDB(t)
	f := seedMatch(t, d, "org-match-5", 10, 250, 4)
	inv := createIncomingInvoice(t, d, f, "V-001", 10, 250)

	// Override with no reason is refused — a silent bypass would defeat the
	// entire audit value of the flag.
	if _, err := d.UpdateIncomingInvoice(inv.ID, UpdateIncomingInvoiceRequest{
		MatchOverride: ptr(1),
	}); err == nil {
		t.Fatal("expected an override without a reason to be rejected")
	}
	if _, err := d.UpdateIncomingInvoice(inv.ID, UpdateIncomingInvoiceRequest{
		MatchOverride: ptr(1), MatchOverrideReason: ptr("   "),
	}); err == nil {
		t.Fatal("expected a blank override reason to be rejected")
	}

	if _, err := d.UpdateIncomingInvoice(inv.ID, UpdateIncomingInvoiceRequest{
		MatchOverride: ptr(1), MatchOverrideReason: ptr("freight billed separately, agreed with vendor"),
	}); err != nil {
		t.Fatalf("override with a reason: %v", err)
	}
	if _, err := d.UpdateIncomingInvoiceState(inv.ID, "approved"); err != nil {
		t.Fatalf("override should unblock approval: %v", err)
	}
}

// A free-text line with no purchase order link is informational only and must
// never block approval.
func TestIncomingInvoiceUnlinkedLineDoesNotBlock(t *testing.T) {
	d := newTestDB(t)
	f := seedMatch(t, d, "org-match-6", 10, 250, 10)

	inv, err := d.CreateIncomingInvoice(CreateIncomingInvoiceRequest{
		OrganizationID: f.OrgID, VendorID: f.VendorID,
		VendorInvoiceNumber: "V-009", Date: 1700000000000, Currency: "EUR",
		SubTotal: 500, TaxTotal: 0, Total: 500,
		LineItems: []CreateInvoiceLineItemRequest{
			{Description: ptr("Freight"), Quantity: 1, UnitPrice: 500},
		},
	})
	if err != nil {
		t.Fatalf("CreateIncomingInvoice: %v", err)
	}

	lines, _ := d.GetIncomingInvoiceMatch(inv.ID)
	if lines[0].Status != MatchUnlinked {
		t.Fatalf("status: got %q, want unlinked", lines[0].Status)
	}
	if _, err := d.UpdateIncomingInvoiceState(inv.ID, "approved"); err != nil {
		t.Fatalf("an unlinked line must not block approval: %v", err)
	}
}

// Totals are re-validated server-side by the same routine sales invoices use,
// including on a partial update that sends only new totals.
func TestIncomingInvoiceTotalsValidated(t *testing.T) {
	d := newTestDB(t)
	f := seedMatch(t, d, "org-match-7", 10, 250, 10)

	_, err := d.CreateIncomingInvoice(CreateIncomingInvoiceRequest{
		OrganizationID: f.OrgID, VendorID: f.VendorID,
		VendorInvoiceNumber: "V-BAD", Date: 1700000000000, Currency: "EUR",
		SubTotal: 9999, TaxTotal: 0, Total: 9999, // line items say 2500
		LineItems: []CreateInvoiceLineItemRequest{
			{Description: ptr("Widget"), Quantity: 10, UnitPrice: 250},
		},
	})
	if err == nil {
		t.Fatal("expected mismatched totals to be rejected")
	}
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected a *ValidationError, got %T: %v", err, err)
	}

	inv := createIncomingInvoice(t, d, f, "V-OK", 10, 250)
	// Sending only new totals must still be checked against stored line items.
	if _, err := d.UpdateIncomingInvoice(inv.ID, UpdateIncomingInvoiceRequest{
		Total: ptr(int64(1)), SubTotal: ptr(int64(1)), TaxTotal: ptr(int64(0)),
	}); err == nil {
		t.Fatal("expected a totals-only update to be validated against stored line items")
	}
}

// A vendor cannot bill the same number twice.
func TestIncomingInvoiceDuplicateNumberRejected(t *testing.T) {
	d := newTestDB(t)
	f := seedMatch(t, d, "org-match-8", 10, 250, 10)
	createIncomingInvoice(t, d, f, "V-001", 10, 250)

	_, err := d.CreateIncomingInvoice(CreateIncomingInvoiceRequest{
		OrganizationID: f.OrgID, VendorID: f.VendorID,
		VendorInvoiceNumber: "V-001", Date: 1700000000000, Currency: "EUR",
		SubTotal: 100, TaxTotal: 0, Total: 100,
		LineItems: []CreateInvoiceLineItemRequest{
			{Description: ptr("Widget"), Quantity: 1, UnitPrice: 100},
		},
	})
	if !errors.Is(err, ErrDuplicateVendorInvoiceNumber) {
		t.Fatalf("expected ErrDuplicateVendorInvoiceNumber, got %v", err)
	}
}

// A tax rate used only by an incoming invoice must not be deletable — its FK
// cascades, so deleting it would strip those line items.
func TestTaxRateUsedOnlyByIncomingInvoiceCannotBeDeleted(t *testing.T) {
	d := newTestDB(t)
	f := seedMatch(t, d, "org-match-9", 10, 250, 10)

	rate, err := d.CreateTaxRate(CreateTaxRateRequest{
		ID: "tax-1", OrganizationID: f.OrgID, Name: "VAT 20%", Percentage: 20,
	})
	if err != nil {
		t.Fatalf("CreateTaxRate: %v", err)
	}

	// 10 * 250 = 2500 subtotal; 20% = 500 tax.
	if _, err := d.CreateIncomingInvoice(CreateIncomingInvoiceRequest{
		OrganizationID: f.OrgID, VendorID: f.VendorID,
		VendorInvoiceNumber: "V-TAX", Date: 1700000000000, Currency: "EUR",
		SubTotal: 2500, TaxTotal: 500, Total: 3000,
		LineItems: []CreateInvoiceLineItemRequest{
			{Description: ptr("Widget"), Quantity: 10, UnitPrice: 250, TaxRate: &rate.ID},
		},
	}); err != nil {
		t.Fatalf("CreateIncomingInvoice: %v", err)
	}

	count, err := d.GetTaxRateUsageCount(rate.ID)
	if err != nil {
		t.Fatalf("GetTaxRateUsageCount: %v", err)
	}
	if count != 1 {
		t.Fatalf("usage count: got %d, want 1 — the incoming invoice table is not being counted", count)
	}
	if _, err := d.DeleteTaxRate(rate.ID); !errors.Is(err, ErrTaxRateInUse) {
		t.Fatalf("expected ErrTaxRateInUse, got %v", err)
	}
}
