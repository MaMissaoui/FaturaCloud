package db

import (
	"path/filepath"
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
		Total:          12000,
		TaxTotal:       2000,
		SubTotal:       10000,
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

func ptr[T any](v T) *T { return &v }
