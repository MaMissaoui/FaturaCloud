package db

import (
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
