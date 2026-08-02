package db

import (
	"encoding/base64"
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

// TestInvoiceLineItemProductRoundTrips guards against the sales-invoice path
// silently dropping productId the way it used to before the productId column
// existed on invoiceLineItems (it was only ever wired for incoming invoices).
func TestInvoiceLineItemProductRoundTrips(t *testing.T) {
	d := newTestDB(t)

	org, _ := d.CreateOrganization(CreateOrganizationRequest{ID: "org-1"})
	client, _ := d.CreateClient(CreateClientRequest{
		ID: "client-1", OrganizationID: org.ID, Name: ptr("Client"),
	})
	product, err := d.CreateProduct(CreateProductRequest{
		ID: "prod-1", OrganizationID: org.ID, Name: "Widget", Type: "product",
	})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}

	inv, err := d.CreateInvoice(CreateInvoiceRequest{
		ID:             "inv-1",
		OrganizationID: org.ID,
		Number:         "INV-001",
		ClientID:       client.ID,
		Date:           1700000000000,
		Currency:       "EUR",
		Total:          5000,
		SubTotal:       5000,
		LineItems: []CreateInvoiceLineItemRequest{
			{Quantity: 1, UnitPrice: 5000, ProductID: &product.ID},
		},
	})
	if err != nil {
		t.Fatalf("CreateInvoice: %v", err)
	}

	items, err := d.GetInvoiceLineItems(inv.ID)
	if err != nil || len(items) != 1 {
		t.Fatalf("GetInvoiceLineItems: err=%v, len=%d", err, len(items))
	}
	if items[0].ProductID == nil || *items[0].ProductID != product.ID {
		t.Fatalf("productId did not round-trip on create: got %v, want %q", items[0].ProductID, product.ID)
	}

	// UpdateInvoice replaces line items wholesale — confirm it persists
	// productId too, not just CreateInvoice's insert path.
	_, err = d.UpdateInvoice(inv.ID, UpdateInvoiceRequest{
		LineItems: &[]CreateInvoiceLineItemRequest{
			{Quantity: 1, UnitPrice: 5000, ProductID: &product.ID},
		},
		Total:    ptr(int64(5000)),
		SubTotal: ptr(int64(5000)),
	})
	if err != nil {
		t.Fatalf("UpdateInvoice: %v", err)
	}
	items, err = d.GetInvoiceLineItems(inv.ID)
	if err != nil || len(items) != 1 {
		t.Fatalf("GetInvoiceLineItems after update: err=%v, len=%d", err, len(items))
	}
	if items[0].ProductID == nil || *items[0].ProductID != product.ID {
		t.Fatalf("productId did not round-trip on update: got %v", items[0].ProductID)
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

// TestGetProductsPagination covers the Limit/Offset behavior added for the
// Products list page: Limit == 0 must still return everything (the behavior
// every other caller — the line-item product pickers — relies on), and a
// nonzero Limit/Offset must page correctly while total always reflects the
// full matching count, not just the current page's length.
func TestGetProductsPagination(t *testing.T) {
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-1"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	for _, name := range []string{"Alpha", "Bravo", "Charlie", "Delta", "Echo"} {
		if _, err := d.CreateProduct(CreateProductRequest{
			OrganizationID: org.ID, Name: name, Type: "product",
		}); err != nil {
			t.Fatalf("CreateProduct(%s): %v", name, err)
		}
	}

	// Limit == 0: unpaginated, full-fetch behavior preserved.
	all, total, err := d.GetProducts(org.ID, ProductListOptions{})
	if err != nil {
		t.Fatalf("GetProducts (no limit): %v", err)
	}
	if len(all) != 5 || total != 5 {
		t.Fatalf("got len=%d total=%d, want 5/5", len(all), total)
	}
	if all[0].Name != "Alpha" || all[4].Name != "Echo" {
		t.Fatalf("expected name-ascending order, got %q..%q", all[0].Name, all[4].Name)
	}

	// Page through with Limit=2: three pages, the last partial, total always 5.
	page1, total, err := d.GetProducts(org.ID, ProductListOptions{Limit: 2, Offset: 0})
	if err != nil || len(page1) != 2 || total != 5 {
		t.Fatalf("page1: err=%v len=%d total=%d, want 2/5", err, len(page1), total)
	}
	if page1[0].Name != "Alpha" || page1[1].Name != "Bravo" {
		t.Fatalf("page1 names: got %q, %q", page1[0].Name, page1[1].Name)
	}

	page3, total, err := d.GetProducts(org.ID, ProductListOptions{Limit: 2, Offset: 4})
	if err != nil || len(page3) != 1 || total != 5 {
		t.Fatalf("page3: err=%v len=%d total=%d, want 1/5", err, len(page3), total)
	}
	if page3[0].Name != "Echo" {
		t.Fatalf("page3 name: got %q, want Echo", page3[0].Name)
	}
}

// TestGetProductsSearch pins the search semantics to substring ("contains"),
// matching the client-side `includes()` filter this replaces — not a
// prefix/leading-anchor search — and confirms it matches across every field
// the old client-side filter covered (name, sku, description, unit, type).
func TestGetProductsSearch(t *testing.T) {
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-1"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	if _, err := d.CreateProduct(CreateProductRequest{
		OrganizationID: org.ID, Name: "Steel Bracket", SKU: ptr("PRD-BRK-042"), Type: "product",
	}); err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	if _, err := d.CreateProduct(CreateProductRequest{
		OrganizationID: org.ID, Name: "Consulting Hour", Type: "service",
	}); err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}

	// Mid-string match on name — would fail if search were prefix-only.
	got, total, err := d.GetProducts(org.ID, ProductListOptions{Search: "Bracket"})
	if err != nil || total != 1 || len(got) != 1 || got[0].Name != "Steel Bracket" {
		t.Fatalf("search %q: err=%v total=%d results=%+v", "Bracket", err, total, got)
	}

	// Mid-string match on SKU.
	got, total, err = d.GetProducts(org.ID, ProductListOptions{Search: "BRK"})
	if err != nil || total != 1 || len(got) != 1 || got[0].Name != "Steel Bracket" {
		t.Fatalf("search %q: err=%v total=%d results=%+v", "BRK", err, total, got)
	}

	// Match on type.
	got, total, err = d.GetProducts(org.ID, ProductListOptions{Search: "service"})
	if err != nil || total != 1 || len(got) != 1 || got[0].Name != "Consulting Hour" {
		t.Fatalf("search %q: err=%v total=%d results=%+v", "service", err, total, got)
	}

	// No match.
	if _, total, err := d.GetProducts(org.ID, ProductListOptions{Search: "nonexistent"}); err != nil || total != 0 {
		t.Fatalf("search with no match: err=%v total=%d, want 0", err, total)
	}
}

// TestGetStockMovementsPagination covers the same Limit/Offset contract as
// GetProducts, plus the ProductID filter that replaced Inventory's
// client-side filtering.
func TestGetStockMovementsPagination(t *testing.T) {
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-1"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	productA, err := d.CreateProduct(CreateProductRequest{
		OrganizationID: org.ID, Name: "Widget A", Type: "product", StockEnabled: 1,
	})
	if err != nil {
		t.Fatalf("CreateProduct A: %v", err)
	}
	productB, err := d.CreateProduct(CreateProductRequest{
		OrganizationID: org.ID, Name: "Widget B", Type: "product", StockEnabled: 1,
	})
	if err != nil {
		t.Fatalf("CreateProduct B: %v", err)
	}
	for i := 0; i < 3; i++ {
		if _, err := d.CreateStockMovement(CreateStockMovementRequest{
			OrganizationID: org.ID, ProductID: productA.ID, Type: "in", Quantity: 1,
		}); err != nil {
			t.Fatalf("CreateStockMovement A: %v", err)
		}
	}
	if _, err := d.CreateStockMovement(CreateStockMovementRequest{
		OrganizationID: org.ID, ProductID: productB.ID, Type: "in", Quantity: 1,
	}); err != nil {
		t.Fatalf("CreateStockMovement B: %v", err)
	}

	// Unfiltered, unpaginated: all 4 movements across both products.
	all, total, err := d.GetStockMovements(org.ID, StockMovementListOptions{})
	if err != nil || len(all) != 4 || total != 4 {
		t.Fatalf("GetStockMovements (no filter): err=%v len=%d total=%d, want 4/4", err, len(all), total)
	}

	// Filtered to product A only.
	forA, total, err := d.GetStockMovements(org.ID, StockMovementListOptions{ProductID: productA.ID})
	if err != nil || len(forA) != 3 || total != 3 {
		t.Fatalf("GetStockMovements (product A): err=%v len=%d total=%d, want 3/3", err, len(forA), total)
	}

	// Paginated: first page of 2 out of the unfiltered 4, total still 4.
	page1, total, err := d.GetStockMovements(org.ID, StockMovementListOptions{Limit: 2, Offset: 0})
	if err != nil || len(page1) != 2 || total != 4 {
		t.Fatalf("GetStockMovements page1: err=%v len=%d total=%d, want 2/4", err, len(page1), total)
	}
}

// TestGetProductsSort covers server-side sorting: the default (name
// ascending, unchanged from before pagination existed), an explicit
// descending sort on a plain column, and — the case that actually needs a
// JOIN — sorting by "taxRate", which must order by the tax rate's *name*
// (matching the old client-side sorter's resolved-name comparison), not the
// raw taxRateId. An unrecognized SortField must not error or inject
// anything; it falls back to the default.
func TestGetProductsSort(t *testing.T) {
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-1"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	// Tax rate names deliberately sort opposite to their id order, so a
	// correct name-sort and an accidental id-sort disagree on the result.
	rateZ, err := d.CreateTaxRate(CreateTaxRateRequest{ID: "tax-a", OrganizationID: org.ID, Name: "Zulu", Percentage: 10})
	if err != nil {
		t.Fatalf("CreateTaxRate Zulu: %v", err)
	}
	rateA, err := d.CreateTaxRate(CreateTaxRateRequest{ID: "tax-b", OrganizationID: org.ID, Name: "Alpha", Percentage: 20})
	if err != nil {
		t.Fatalf("CreateTaxRate Alpha: %v", err)
	}
	if _, err := d.CreateProduct(CreateProductRequest{
		OrganizationID: org.ID, Name: "Widget", Type: "product", TaxRateID: &rateZ.ID,
	}); err != nil {
		t.Fatalf("CreateProduct Widget: %v", err)
	}
	if _, err := d.CreateProduct(CreateProductRequest{
		OrganizationID: org.ID, Name: "Gadget", Type: "product", TaxRateID: &rateA.ID,
	}); err != nil {
		t.Fatalf("CreateProduct Gadget: %v", err)
	}

	// Default: name ascending, unchanged from the pre-pagination behavior.
	byDefault, _, err := d.GetProducts(org.ID, ProductListOptions{})
	if err != nil || len(byDefault) != 2 || byDefault[0].Name != "Gadget" || byDefault[1].Name != "Widget" {
		t.Fatalf("default sort: err=%v names=%v, want [Gadget Widget]", err, namesOf(byDefault))
	}

	// Explicit descending on a plain column.
	nameDesc, _, err := d.GetProducts(org.ID, ProductListOptions{SortField: "name", SortDesc: true})
	if err != nil || len(nameDesc) != 2 || nameDesc[0].Name != "Widget" || nameDesc[1].Name != "Gadget" {
		t.Fatalf("name desc: err=%v names=%v, want [Widget Gadget]", err, namesOf(nameDesc))
	}

	// "taxRate" sorts by the joined tax rate's name (Alpha < Zulu), which is
	// the opposite order from the products' own name or the raw taxRateId
	// ("tax-a" < "tax-b") — proves the JOIN, not an id-sort, is what ran.
	byTaxRate, _, err := d.GetProducts(org.ID, ProductListOptions{SortField: "taxRate"})
	if err != nil || len(byTaxRate) != 2 || byTaxRate[0].Name != "Gadget" || byTaxRate[1].Name != "Widget" {
		t.Fatalf("taxRate sort: err=%v names=%v, want [Gadget Widget] (Alpha before Zulu)", err, namesOf(byTaxRate))
	}

	// Unrecognized field: falls back to the default, does not error.
	fallback, _, err := d.GetProducts(org.ID, ProductListOptions{SortField: "'; DROP TABLE products; --"})
	if err != nil || len(fallback) != 2 || fallback[0].Name != "Gadget" {
		t.Fatalf("unrecognized sort field should fall back to default: err=%v names=%v", err, namesOf(fallback))
	}
}

func namesOf(products []Product) []string {
	names := make([]string, len(products))
	for i, p := range products {
		names[i] = p.Name
	}
	return names
}

// TestGetStockMovementsSort mirrors TestGetProductsSort for the movements
// list: default (createdAt descending, unchanged), explicit ascending on a
// plain column, and "product" sorting by the joined product's *name* rather
// than productId.
func TestGetStockMovementsSort(t *testing.T) {
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-1"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	// Product names deliberately sort opposite to creation/id order.
	productZ, err := d.CreateProduct(CreateProductRequest{
		OrganizationID: org.ID, Name: "Zulu Widget", Type: "product", StockEnabled: 1,
	})
	if err != nil {
		t.Fatalf("CreateProduct Zulu: %v", err)
	}
	productA, err := d.CreateProduct(CreateProductRequest{
		OrganizationID: org.ID, Name: "Alpha Widget", Type: "product", StockEnabled: 1,
	})
	if err != nil {
		t.Fatalf("CreateProduct Alpha: %v", err)
	}
	if _, err := d.CreateStockMovement(CreateStockMovementRequest{
		OrganizationID: org.ID, ProductID: productZ.ID, Type: "in", Quantity: 1,
	}); err != nil {
		t.Fatalf("CreateStockMovement (Zulu's): %v", err)
	}
	if _, err := d.CreateStockMovement(CreateStockMovementRequest{
		OrganizationID: org.ID, ProductID: productA.ID, Type: "in", Quantity: 2,
	}); err != nil {
		t.Fatalf("CreateStockMovement (Alpha's): %v", err)
	}

	// Quantity ascending — a plain, unjoined column.
	byQty, _, err := d.GetStockMovements(org.ID, StockMovementListOptions{SortField: "quantity"})
	if err != nil || len(byQty) != 2 || byQty[0].Quantity != 1 || byQty[1].Quantity != 2 {
		t.Fatalf("quantity asc: err=%v quantities=[%v %v], want [1 2]", err, byQty[0].Quantity, byQty[1].Quantity)
	}

	// "product" sorts by the joined product's name (Alpha before Zulu) —
	// opposite of creation order, proving the JOIN drove the sort.
	byProduct, _, err := d.GetStockMovements(org.ID, StockMovementListOptions{SortField: "product"})
	if err != nil || len(byProduct) != 2 || byProduct[0].ProductID != productA.ID || byProduct[1].ProductID != productZ.ID {
		t.Fatalf("product sort: err=%v productIds=[%v %v], want [%v %v]",
			err, byProduct[0].ProductID, byProduct[1].ProductID, productA.ID, productZ.ID)
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

// Orders got a currency column but were missed when exchangeRate/
// exchangeRateDate were added to every other document type. This covers the
// same resolveExchangeRateForSave cases purchase_order.go relies on:
// required when the currency differs from the org's, carried over on update
// when the currency doesn't change, and rejected if the currency changes
// without a fresh rate.
func TestOrderExchangeRate(t *testing.T) {
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-fx-1", Currency: ptr("EUR")})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}

	// Foreign currency with no rate: rejected.
	if _, err := d.CreateOrder(CreateOrderRequest{
		ID: "order-fx-1", OrganizationID: org.ID, OrderNumber: "ORD-FX-1",
		OrderDate: 1700000000000, Currency: ptr("USD"),
	}); err == nil {
		t.Fatal("expected a foreign-currency order with no exchange rate to be rejected")
	}

	// Foreign currency with a rate: accepted and stored.
	order, err := d.CreateOrder(CreateOrderRequest{
		ID: "order-fx-2", OrganizationID: org.ID, OrderNumber: "ORD-FX-2",
		OrderDate: 1700000000000, Currency: ptr("USD"), ExchangeRate: ptr(0.92),
	})
	if err != nil {
		t.Fatalf("CreateOrder with exchange rate: %v", err)
	}
	if order.ExchangeRate == nil {
		t.Fatal("expected exchangeRate to be stored")
	}

	// Same currency on update, no new rate submitted: the stored rate carries over.
	updated, err := d.UpdateOrder(order.ID, UpdateOrderRequest{Notes: ptr("updated")})
	if err != nil {
		t.Fatalf("UpdateOrder (no currency change): %v", err)
	}
	if updated.ExchangeRate == nil || *updated.ExchangeRate != *order.ExchangeRate {
		t.Fatalf("exchange rate did not carry over: got %v, want %v", updated.ExchangeRate, order.ExchangeRate)
	}

	// Currency changes without a fresh rate: rejected.
	if _, err := d.UpdateOrder(order.ID, UpdateOrderRequest{Currency: ptr("GBP")}); err == nil {
		t.Fatal("expected a currency change with no new exchange rate to be rejected")
	}

	// Currency changes back to the org's own currency: rate is cleared.
	cleared, err := d.UpdateOrder(order.ID, UpdateOrderRequest{Currency: ptr("EUR")})
	if err != nil {
		t.Fatalf("UpdateOrder (revert to org currency): %v", err)
	}
	if cleared.ExchangeRate != nil {
		t.Fatalf("expected exchangeRate to be cleared, got %v", *cleared.ExchangeRate)
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

// TestOrganizationLogoRoundTrip covers the dedicated logo storage path used
// by GET/POST/DELETE /organizations/{id}/logo: neither the Organization
// struct returned by GetOrganizations/GetOrganization carries the logo BLOB
// (F29 — the list is re-fetched on every auth change, and a multi-MB logo has
// no business riding along with either), only GetOrganizationLogo does.
func TestOrganizationLogoRoundTrip(t *testing.T) {
	d := newTestDB(t)
	if _, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-1", Name: ptr("ACME")}); err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}

	if logo, err := d.GetOrganizationLogo("org-1"); err != nil || len(logo) != 0 {
		t.Fatalf("expected no logo yet, got logo=%v err=%v", logo, err)
	}

	logo := []byte("PRETEND-THIS-IS-A-BIG-PNG")
	if ok, err := d.SetOrganizationLogo("org-1", logo); err != nil || !ok {
		t.Fatalf("SetOrganizationLogo: ok=%v err=%v", ok, err)
	}

	got, err := d.GetOrganizationLogo("org-1")
	if err != nil || string(got) != string(logo) {
		t.Fatalf("GetOrganizationLogo: got=%q err=%v", got, err)
	}

	if list, err := d.GetOrganizations(); err != nil || len(list) != 1 {
		t.Fatalf("GetOrganizations: err=%v len=%d", err, len(list))
	}
	if _, err := d.GetOrganization("org-1"); err != nil {
		t.Fatalf("GetOrganization: %v", err)
	}

	if ok, err := d.SetOrganizationLogo("org-1", nil); err != nil || !ok {
		t.Fatalf("SetOrganizationLogo(nil): ok=%v err=%v", ok, err)
	}
	if logo, err := d.GetOrganizationLogo("org-1"); err != nil || len(logo) != 0 {
		t.Fatalf("expected logo cleared, got logo=%v err=%v", logo, err)
	}

	if ok, err := d.SetOrganizationLogo("does-not-exist", []byte("x")); err != nil || ok {
		t.Fatalf("expected SetOrganizationLogo on unknown org to report ok=false, got ok=%v err=%v", ok, err)
	}
}

// TestOrganizationLogoLegacyDataURI covers organizations whose logo column
// still holds the browser's full "data:image/png;base64,..." string as text
// (the format used before the /logo endpoint existed) — GetOrganizationLogo
// must decode it back to raw image bytes rather than returning the text.
func TestOrganizationLogoLegacyDataURI(t *testing.T) {
	d := newTestDB(t)
	if _, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-1", Name: ptr("ACME")}); err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}

	raw := []byte("not-really-a-png-but-stands-in-for-one")
	dataURI := "data:image/png;base64," + base64.StdEncoding.EncodeToString(raw)
	if ok, err := d.SetOrganizationLogo("org-1", []byte(dataURI)); err != nil || !ok {
		t.Fatalf("SetOrganizationLogo: ok=%v err=%v", ok, err)
	}

	got, err := d.GetOrganizationLogo("org-1")
	if err != nil || string(got) != string(raw) {
		t.Fatalf("expected legacy data URI decoded to raw bytes, got %q err=%v", got, err)
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

// TestResetOrganizationDataCoversEveryOrganizationScopedTable is a tripwire
// for the opposite mistake from TestVendorDocumentCountCoversEveryReference:
// a migration that adds a new organizationId-scoped table without adding it
// to transactionalDataTables or masterDataTables wouldn't fail loudly — it
// would just leave that table's rows behind, silently, every time someone
// resets an organization's data.
//
// legacyUnusedTables are schema leftovers from before the current app's
// feature set (see CLAUDE.md's Project Origin note) with no Go or frontend
// code referencing them at all — nothing ever populates them, so there's
// nothing for a reset to cover.
func TestResetOrganizationDataCoversEveryOrganizationScopedTable(t *testing.T) {
	d := newTestDB(t)

	legacyUnusedTables := map[string]bool{
		"tags":        true,
		"timeEntries": true,
		"projects":    true,
	}

	tables := []string{}
	if err := d.DB.Select(&tables,
		`SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`,
	); err != nil {
		t.Fatalf("list tables: %v", err)
	}

	covered := map[string]bool{"organizations": true}
	for _, name := range transactionalDataTables {
		covered[name] = true
	}
	for _, name := range masterDataTables {
		covered[name] = true
	}

	for _, table := range tables {
		columns := []struct {
			Name string `db:"name"`
		}{}
		if err := d.DB.Select(&columns, `SELECT name FROM pragma_table_info(?)`, table); err != nil {
			t.Fatalf("pragma_table_info(%s): %v", table, err)
		}
		hasOrgColumn := false
		for _, col := range columns {
			if col.Name == "organizationId" {
				hasOrgColumn = true
				break
			}
		}
		if !hasOrgColumn {
			continue
		}
		if !covered[table] && !legacyUnusedTables[table] {
			t.Errorf(
				"table %q has an organizationId column but is not in transactionalDataTables or "+
					"masterDataTables — ResetOrganizationData would leave its rows behind; add it to "+
					"one of those lists in db/reset.go (or to legacyUnusedTables in this test, if it's "+
					"genuinely dead)",
				table,
			)
		}
	}
}

// TestResetOrganizationData exercises all three modes: transactional-only
// (master data survives untouched), the neither-checked validation error, and
// master data forcing transactional data along with it (the referential-
// integrity constraint documented on ResetOrganizationData).
func TestResetOrganizationData(t *testing.T) {
	d := newTestDB(t)

	seed := func(t *testing.T, orgID string) (client *Client, product *Product) {
		t.Helper()
		client, err := d.CreateClient(CreateClientRequest{
			ID: orgID + "-client", OrganizationID: orgID, Name: ptr("Client"),
		})
		if err != nil {
			t.Fatalf("CreateClient: %v", err)
		}
		if _, err := d.CreateVendor(CreateVendorRequest{
			ID: orgID + "-vendor", OrganizationID: orgID, Name: ptr("Vendor"),
		}); err != nil {
			t.Fatalf("CreateVendor: %v", err)
		}
		product, err = d.CreateProduct(CreateProductRequest{
			ID: orgID + "-product", OrganizationID: orgID, Name: "Widget",
			Type: "product", StockEnabled: 1,
		})
		if err != nil {
			t.Fatalf("CreateProduct: %v", err)
		}
		if _, err := d.CreateTaxRate(CreateTaxRateRequest{
			ID: orgID + "-tax", OrganizationID: orgID, Name: "VAT", Percentage: 20,
		}); err != nil {
			t.Fatalf("CreateTaxRate: %v", err)
		}
		if _, err := d.CreateInvoice(CreateInvoiceRequest{
			ID: orgID + "-inv", OrganizationID: orgID, Number: "INV-001",
			ClientID: client.ID, Date: 1700000000000, Currency: "EUR",
			Total: 5000, SubTotal: 5000,
			LineItems: []CreateInvoiceLineItemRequest{{Quantity: 1, UnitPrice: 5000}},
		}); err != nil {
			t.Fatalf("CreateInvoice: %v", err)
		}
		if _, err := d.CreateStockMovement(CreateStockMovementRequest{
			ID: orgID + "-move", OrganizationID: orgID, ProductID: product.ID,
			Type: "in", Quantity: 10,
		}); err != nil {
			t.Fatalf("CreateStockMovement: %v", err)
		}
		if _, err := d.DB.Exec(
			`UPDATE organizations SET invoice_number_counter = 5 WHERE id = ?`, orgID,
		); err != nil {
			t.Fatalf("bump invoice_number_counter: %v", err)
		}
		return client, product
	}

	assertGone := func(t *testing.T, table, orgID string) {
		t.Helper()
		var n int64
		if err := d.DB.Get(&n, `SELECT COUNT(*) FROM `+table+` WHERE organizationId = ?`, orgID); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if n != 0 {
			t.Errorf("%s: got %d rows, want 0", table, n)
		}
	}
	assertPresent := func(t *testing.T, table, orgID string) {
		t.Helper()
		var n int64
		if err := d.DB.Get(&n, `SELECT COUNT(*) FROM `+table+` WHERE organizationId = ?`, orgID); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if n == 0 {
			t.Errorf("%s: got 0 rows, want at least 1", table)
		}
	}

	t.Run("neither flag is rejected", func(t *testing.T) {
		org, _ := d.CreateOrganization(CreateOrganizationRequest{ID: "org-neither"})
		seed(t, org.ID)
		if _, err := d.ResetOrganizationData(org.ID, ResetOrganizationDataRequest{}); err == nil {
			t.Fatal("expected a validation error, got nil")
		}
	})

	t.Run("transactional only leaves master data untouched", func(t *testing.T) {
		org, _ := d.CreateOrganization(CreateOrganizationRequest{ID: "org-txn"})
		_, product := seed(t, org.ID)

		deleted, err := d.ResetOrganizationData(org.ID, ResetOrganizationDataRequest{ResetTransactionalData: true})
		if err != nil {
			t.Fatalf("ResetOrganizationData: %v", err)
		}
		if deleted.Invoices != 1 || deleted.StockMovements != 1 {
			t.Fatalf("unexpected deleted counts: %+v", deleted)
		}
		if deleted.Clients != 0 || deleted.Vendors != 0 || deleted.Products != 0 || deleted.TaxRates != 0 {
			t.Fatalf("master data counts should read 0 (not requested): %+v", deleted)
		}

		for _, table := range []string{"invoices", "stockMovements"} {
			assertGone(t, table, org.ID)
		}
		for _, table := range []string{"clients", "vendors", "products", "taxRates"} {
			assertPresent(t, table, org.ID)
		}

		refreshed, err := d.GetProduct(product.ID)
		if err != nil {
			t.Fatalf("GetProduct: %v", err)
		}
		if refreshed.StockQuantity != 0 {
			t.Errorf("stockQuantity: got %v, want 0", refreshed.StockQuantity)
		}

		refreshedOrg, err := d.GetOrganization(org.ID)
		if err != nil {
			t.Fatalf("GetOrganization: %v", err)
		}
		if refreshedOrg.InvoiceNumberCounter == nil || *refreshedOrg.InvoiceNumberCounter != 0 {
			t.Errorf("invoice_number_counter: got %v, want 0", refreshedOrg.InvoiceNumberCounter)
		}
	})

	t.Run("master data forces transactional data along with it", func(t *testing.T) {
		org, _ := d.CreateOrganization(CreateOrganizationRequest{ID: "org-master"})
		seed(t, org.ID)

		// Deliberately requesting master data ONLY — transactional data must
		// still be wiped, since clients.CASCADE/vendors.RESTRICT make the two
		// inseparable (see the comment on ResetOrganizationData).
		deleted, err := d.ResetOrganizationData(org.ID, ResetOrganizationDataRequest{ResetMasterData: true})
		if err != nil {
			t.Fatalf("ResetOrganizationData: %v", err)
		}
		if deleted.Clients != 1 || deleted.Vendors != 1 || deleted.Products != 1 || deleted.TaxRates != 1 {
			t.Fatalf("unexpected master data counts: %+v", deleted)
		}
		if deleted.Invoices != 1 || deleted.StockMovements != 1 {
			t.Fatalf("transactional data should have been wiped too: %+v", deleted)
		}

		for _, table := range []string{
			"invoices", "stockMovements", "clients", "vendors", "products", "taxRates",
		} {
			assertGone(t, table, org.ID)
		}
	})
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

// An unknown BT-118 category code would produce invalid XRechnung/ZUGFeRD
// XML at export time, so it's rejected up front on both create and update.
func TestTaxRateRejectsUnknownCategoryCode(t *testing.T) {
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-tax-cat"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}

	var verr *ValidationError
	if _, err := d.CreateTaxRate(CreateTaxRateRequest{
		ID: "tax-bad-cat", OrganizationID: org.ID, Name: "Bogus", Percentage: 10,
		CategoryCode: "XX",
	}); !errors.As(err, &verr) {
		t.Fatalf("expected a *ValidationError, got %T: %v", err, err)
	}

	rate, err := d.CreateTaxRate(CreateTaxRateRequest{
		ID: "tax-good-cat", OrganizationID: org.ID, Name: "Standard", Percentage: 19,
		CategoryCode: "S",
	})
	if err != nil {
		t.Fatalf("CreateTaxRate: %v", err)
	}

	if _, err := d.UpdateTaxRate(rate.ID, UpdateTaxRateRequest{CategoryCode: ptr("XX")}); !errors.As(err, &verr) {
		t.Fatalf("expected a *ValidationError on update, got %T: %v", err, err)
	}
}

// Uncosted stock must not dilute the first costed receipt. A product's opening
// stock is typically a manual adjustment with no cost; letting those units into
// the valuation pool at a value of zero would halve the price actually paid.
func TestAverageCostIgnoresUncostedStockBeforeFirstCostedReceipt(t *testing.T) {
	d := newTestDB(t)
	product := seedCostProduct(t, d, "org-cost-dilute")

	// Opening stock, no cost recorded.
	if _, err := d.CreateStockMovement(CreateStockMovementRequest{
		OrganizationID: product.OrganizationID, ProductID: product.ID, Type: "in", Quantity: 10,
	}); err != nil {
		t.Fatalf("CreateStockMovement: %v", err)
	}
	// First actual purchase.
	if _, err := d.CreateStockMovement(CreateStockMovementRequest{
		OrganizationID: product.OrganizationID, ProductID: product.ID,
		Type: "in", Quantity: 10, UnitCost: ptr(int64(200)),
	}); err != nil {
		t.Fatalf("CreateStockMovement: %v", err)
	}

	p, _ := d.GetProduct(product.ID)
	if p.UnitCost == nil || *p.UnitCost != 200 {
		t.Fatalf("unit cost: got %v, want 200 — unvalued opening stock diluted the price paid", p.UnitCost)
	}
	// Both movements still count toward stock.
	if p.StockQuantity != 20 {
		t.Fatalf("stockQuantity: got %v, want 20", p.StockQuantity)
	}

	// Once an average exists, an uncosted inflow moves at it and leaves it alone.
	if _, err := d.CreateStockMovement(CreateStockMovementRequest{
		OrganizationID: product.OrganizationID, ProductID: product.ID, Type: "in", Quantity: 5,
	}); err != nil {
		t.Fatalf("CreateStockMovement: %v", err)
	}
	p, _ = d.GetProduct(product.ID)
	if p.UnitCost == nil || *p.UnitCost != 200 {
		t.Fatalf("uncosted inflow after an average exists moved it: got %v, want 200", p.UnitCost)
	}
}

// The purchase order's "received" figure and the invoice matcher's must agree.
// They are separate queries, and an earlier version disagreed: the order page
// counted draft receipts while matching counted only received ones, so the same
// goods read as both fully received and not received at all.
func TestReceivedQuantityAgreesBetweenOrderAndMatch(t *testing.T) {
	d := newTestDB(t)
	f := seedMatch(t, d, "org-received-agree", 10, 250, 0)

	// A receipt for the full quantity, deliberately left in draft.
	receipt, err := d.CreateInboundDelivery(CreateInboundDeliveryRequest{
		OrganizationID: f.OrgID, PurchaseOrderID: &f.OrderID, VendorID: &f.VendorID,
		DeliveryNumber: "GR-0001", DeliveryDate: 1700000000000,
		LineItems: []CreateInboundDeliveryLineItemRequest{
			{PurchaseOrderLineItemID: &f.POLineID, Description: "Widget", Quantity: 10},
		},
	})
	if err != nil {
		t.Fatalf("CreateInboundDelivery: %v", err)
	}

	assertAgrees := func(stage string, want float64) {
		t.Helper()
		onOrder, err := d.GetPurchaseOrderReceivedQuantities(f.OrderID)
		if err != nil {
			t.Fatalf("GetPurchaseOrderReceivedQuantities: %v", err)
		}
		inv := createIncomingInvoice(t, d, f, "V-"+stage, 10, 250)
		lines, err := d.GetIncomingInvoiceMatch(inv.ID)
		if err != nil {
			t.Fatalf("GetIncomingInvoiceMatch: %v", err)
		}
		inMatch := *lines[0].ReceivedQuantity

		if onOrder[f.POLineID] != want || inMatch != want {
			t.Fatalf(
				"%s: purchase order reports %v received, matching reports %v — both should be %v",
				stage, onOrder[f.POLineID], inMatch, want,
			)
		}
	}

	// A draft receipt has moved no stock, so nothing is received yet.
	assertAgrees("draft", 0)

	if _, err := d.UpdateInboundDeliveryStatus(receipt.ID, "received"); err != nil {
		t.Fatalf("receive: %v", err)
	}
	assertAgrees("received", 10)

	if _, err := d.UpdateInboundDeliveryStatus(receipt.ID, "cancelled"); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	assertAgrees("cancelled", 0)
}

// An override justifies one specific variance. Editing the financials can turn
// it into a different one, so the flag is cleared unless the same request
// re-states it — otherwise an approved invoice could be edited and re-approved
// against a reason that no longer describes it.
func TestIncomingInvoiceOverrideClearedWhenFinancialsChange(t *testing.T) {
	d := newTestDB(t)
	f := seedMatch(t, d, "org-override-stale", 10, 250, 4)
	inv := createIncomingInvoice(t, d, f, "V-001", 10, 250)

	if _, err := d.UpdateIncomingInvoice(inv.ID, UpdateIncomingInvoiceRequest{
		MatchOverride: ptr(1), MatchOverrideReason: ptr("balance shipping separately"),
	}); err != nil {
		t.Fatalf("set override: %v", err)
	}
	if _, err := d.UpdateIncomingInvoiceState(inv.ID, "approved"); err != nil {
		t.Fatalf("approve with override: %v", err)
	}

	// Now edit the line items — a different variance entirely.
	if _, err := d.UpdateIncomingInvoice(inv.ID, UpdateIncomingInvoiceRequest{
		SubTotal: ptr(int64(5000)), TaxTotal: ptr(int64(0)), Total: ptr(int64(5000)),
		LineItems: &[]CreateInvoiceLineItemRequest{
			{
				Description: ptr("Widget"), Quantity: 20, UnitPrice: 250,
				PurchaseOrderLineItemID: &f.POLineID,
			},
		},
	}); err != nil {
		t.Fatalf("edit line items: %v", err)
	}

	current, _ := d.GetIncomingInvoice(inv.ID)
	if current.MatchOverride != 0 {
		t.Fatalf("override survived a financial edit: %d", current.MatchOverride)
	}
	if current.MatchOverrideReason != nil {
		t.Fatalf("stale override reason survived: %v", current.MatchOverrideReason)
	}
	// And approval is gated again.
	if _, err := d.UpdateIncomingInvoiceState(inv.ID, "approved"); err == nil {
		t.Fatal("expected re-approval to be blocked after the override was cleared")
	}

	// Re-stating the override in the same request keeps it.
	if _, err := d.UpdateIncomingInvoice(inv.ID, UpdateIncomingInvoiceRequest{
		SubTotal: ptr(int64(5000)), TaxTotal: ptr(int64(0)), Total: ptr(int64(5000)),
		MatchOverride: ptr(1), MatchOverrideReason: ptr("revised agreement covers the full 20"),
	}); err != nil {
		t.Fatalf("re-state override: %v", err)
	}
	current, _ = d.GetIncomingInvoice(inv.ID)
	if current.MatchOverride != 1 {
		t.Fatal("re-stated override was cleared")
	}
}
