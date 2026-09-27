package db

import (
	"errors"
	"testing"
)

// newQuantityOnlyOrg creates an organization in quantity-only inventory
// valuation with one uncosted, stock-enabled product. Deliberately no fiscal
// year: nothing in quantity-only mode posts to the GL, so nothing should
// need one.
func newQuantityOnlyOrg(t *testing.T, d *Database, orgID string, serialized bool) (*Organization, *Product) {
	t.Helper()
	org, err := d.CreateOrganization(CreateOrganizationRequest{
		ID: orgID, Name: ptr("Quantity Only Org"), InventoryValuation: ptr(InventoryValuationQuantityOnly),
	})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	s := 0
	if serialized {
		s = 1
	}
	product, err := d.CreateProduct(CreateProductRequest{
		OrganizationID: org.ID, Name: "Washing machine", SKU: ptr("MAL-001"), Type: "product",
		StockEnabled: 1, Serialized: s,
	})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	return org, product
}

func requireNoJournalEntries(t *testing.T, d *Database, orgID string) {
	t.Helper()
	entries, err := d.GetJournalEntries(orgID, "", "")
	if err != nil {
		t.Fatalf("GetJournalEntries: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected no journal entries in quantity-only mode, got %d", len(entries))
	}
}

func requireStock(t *testing.T, d *Database, productID string, want float64) *Product {
	t.Helper()
	p, err := d.GetProduct(productID)
	if err != nil {
		t.Fatalf("GetProduct: %v", err)
	}
	if p.StockQuantity != want {
		t.Fatalf("stockQuantity = %v, want %v", p.StockQuantity, want)
	}
	return p
}

// The perpetual counterpart (TestShipmentWithNoCostBasisIsRejectedAndTouchesNoStock)
// 409s; quantity-only ships, moves stock, and posts nothing — and cancelling
// the shipment restores the stock with nothing to reverse.
func TestQuantityOnlyShipmentWithoutCostShipsAndPostsNothing(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	org, product := newQuantityOnlyOrg(t, d, "org-qo-ship", false)
	if _, err := d.CreateStockMovement(CreateStockMovementRequest{
		OrganizationID: org.ID, ProductID: product.ID, Type: "count_addition", Quantity: 5,
	}); err != nil {
		t.Fatalf("CreateStockMovement: %v", err)
	}

	delivery, err := d.CreateDelivery(CreateDeliveryRequest{
		OrganizationID: org.ID, DeliveryNumber: "DEL-0001", DeliveryDate: 1738368000000,
		LineItems: []CreateDeliveryLineItemRequest{
			{ProductID: &product.ID, Description: "Washing machine", Quantity: 2},
		},
	})
	if err != nil {
		t.Fatalf("CreateDelivery: %v", err)
	}
	if _, err := d.UpdateDeliveryStatus(delivery.ID, "shipped", nil); err != nil {
		t.Fatalf("UpdateDeliveryStatus(shipped): %v", err)
	}
	requireStock(t, d, product.ID, 3)
	requireNoJournalEntries(t, d, org.ID)

	if _, err := d.UpdateDeliveryStatus(delivery.ID, "cancelled", nil); err != nil {
		t.Fatalf("UpdateDeliveryStatus(cancelled): %v", err)
	}
	requireStock(t, d, product.ID, 5)
	requireNoJournalEntries(t, d, org.ID)
}

// The perpetual counterpart (TestOutflowStockAdjustmentWithNoCostBasisIsRejected)
// 409s.
func TestQuantityOnlyShortageWithoutCostSucceeds(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	org, product := newQuantityOnlyOrg(t, d, "org-qo-shortage", false)
	if _, err := d.CreateStockMovement(CreateStockMovementRequest{
		OrganizationID: org.ID, ProductID: product.ID, Type: "in", Quantity: 5,
	}); err != nil {
		t.Fatalf("CreateStockMovement (in): %v", err)
	}
	if _, err := d.CreateStockMovement(CreateStockMovementRequest{
		OrganizationID: org.ID, ProductID: product.ID, Type: "count_subtraction", Quantity: -2,
	}); err != nil {
		t.Fatalf("CreateStockMovement (shortage): %v", err)
	}
	requireStock(t, d, product.ID, 3)
	requireNoJournalEntries(t, d, org.ID)
}

func TestQuantityOnlySerializedStockOutWithoutCostSucceeds(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	org, product := newQuantityOnlyOrg(t, d, "org-qo-serial", true)
	if _, err := d.CreateStockMovement(CreateStockMovementRequest{
		OrganizationID: org.ID, ProductID: product.ID, Type: "in", SerialNumbers: []string{"SN-1", "SN-2"},
	}); err != nil {
		t.Fatalf("CreateStockMovement (in): %v", err)
	}
	if _, err := d.CreateStockMovement(CreateStockMovementRequest{
		OrganizationID: org.ID, ProductID: product.ID, Type: "out", SerialNumbers: []string{"SN-1"},
	}); err != nil {
		t.Fatalf("CreateStockMovement (out): %v", err)
	}
	requireStock(t, d, product.ID, 1)
	requireNoJournalEntries(t, d, org.ID)
}

// A cost entered anyway is still kept as information — it drives the
// average cost — but posts nothing.
func TestQuantityOnlyCostedInflowKeepsAverageButPostsNothing(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	org, product := newQuantityOnlyOrg(t, d, "org-qo-costed", false)
	if _, err := d.CreateStockMovement(CreateStockMovementRequest{
		OrganizationID: org.ID, ProductID: product.ID, Type: "in", Quantity: 4, UnitCost: ptr(int64(90000)),
	}); err != nil {
		t.Fatalf("CreateStockMovement: %v", err)
	}
	p := requireStock(t, d, product.ID, 4)
	if p.UnitCost == nil || *p.UnitCost != 90000 {
		t.Fatalf("unitCost = %v, want 90000", p.UnitCost)
	}
	requireNoJournalEntries(t, d, org.ID)
}

// Receiving posts no GRNI, and the bill for those goods is expensed rather
// than capitalized — with nothing accrued, there is no GRNI to clear.
func TestQuantityOnlyReceiptPostsNoGRNIAndBillIsExpensed(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGRNITestFixture(t, d, "org-qo-receipt", 10, 250)
	if _, err := d.UpdateOrganization(fx.orgID, UpdateOrganizationRequest{
		InventoryValuation: ptr(InventoryValuationQuantityOnly),
	}); err != nil {
		t.Fatalf("UpdateOrganization (switch before any stock): %v", err)
	}

	receipt := fx.receive(t, d, "GR-0001", 10)
	entry, err := d.FindPostedEntryForSourceDocument("inbound_delivery", receipt.ID)
	if err != nil {
		t.Fatalf("FindPostedEntryForSourceDocument: %v", err)
	}
	if entry != nil {
		t.Fatal("expected no GRNI entry for a receipt in quantity-only mode")
	}
	requireStock(t, d, fx.productID, 10)

	bill := fx.bill(t, d, "V-001", 10, 250, true)
	if _, err := d.UpdateIncomingInvoiceState(bill.ID, "approved"); err != nil {
		t.Fatalf("UpdateIncomingInvoiceState(approved): %v", err)
	}
	billEntry, err := d.FindPostedEntryForSourceDocument("incoming_invoice", bill.ID)
	if err != nil || billEntry == nil {
		t.Fatalf("expected a posted bill entry, err=%v entry=%v", err, billEntry)
	}
	lines, err := d.GetJournalEntryLines(billEntry.ID)
	if err != nil {
		t.Fatalf("GetJournalEntryLines: %v", err)
	}
	org, err := d.GetOrganization(fx.orgID)
	if err != nil {
		t.Fatalf("GetOrganization: %v", err)
	}
	if dr, cr := sumLines(lines, fx.inventoryAccountID); dr != 0 || cr != 0 {
		t.Fatalf("Inventory line = debit %d credit %d, want none", dr, cr)
	}
	if dr, cr := sumLines(lines, fx.grniAccountID); dr != 0 || cr != 0 {
		t.Fatalf("GRNI line = debit %d credit %d, want none", dr, cr)
	}
	if dr, _ := sumLines(lines, *org.DefaultExpenseAccountID); dr != 2500 {
		t.Fatalf("expense debit = %d, want 2500", dr)
	}
}

// Uncosted components don't block completion; the finished units enter
// with no cost (not a 0 that would drag the average down) and nothing posts.
func TestQuantityOnlyProductionOrderWithUncostedComponentsCompletes(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{
		ID: "org-qo-production", InventoryValuation: ptr(InventoryValuationQuantityOnly),
	})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	finishedCategory, componentCategory := "finished", "component"
	finished, err := d.CreateProduct(CreateProductRequest{
		OrganizationID: org.ID, Name: "Bundle", Type: "product", StockEnabled: 1, Category: &finishedCategory,
	})
	if err != nil {
		t.Fatalf("CreateProduct finished: %v", err)
	}
	costed, err := d.CreateProduct(CreateProductRequest{
		OrganizationID: org.ID, Name: "Costed part", Type: "product", StockEnabled: 1,
		Category: &componentCategory, UnitCost: ptr(int64(1000)),
	})
	if err != nil {
		t.Fatalf("CreateProduct costed: %v", err)
	}
	uncosted, err := d.CreateProduct(CreateProductRequest{
		OrganizationID: org.ID, Name: "Uncosted part", Type: "product", StockEnabled: 1, Category: &componentCategory,
	})
	if err != nil {
		t.Fatalf("CreateProduct uncosted: %v", err)
	}
	for _, p := range []*Product{costed, uncosted} {
		if _, err := d.CreateStockMovement(CreateStockMovementRequest{
			OrganizationID: org.ID, ProductID: p.ID, Type: "in", Quantity: 10,
		}); err != nil {
			t.Fatalf("CreateStockMovement %s: %v", p.Name, err)
		}
	}
	if _, err := d.ReplaceBillOfMaterials(finished.ID, []CreateBillOfMaterialsLineRequest{
		{ComponentProductID: costed.ID, QuantityPerUnit: 1},
		{ComponentProductID: uncosted.ID, QuantityPerUnit: 1},
	}, 1); err != nil {
		t.Fatalf("ReplaceBillOfMaterials: %v", err)
	}
	order, err := d.CreateProductionOrder(CreateProductionOrderRequest{
		OrganizationID: org.ID, OrderNumber: "PRO-0001", FinishedProductID: finished.ID,
		Quantity: 2, Date: 1738368000000,
	})
	if err != nil {
		t.Fatalf("CreateProductionOrder: %v", err)
	}
	if _, err := d.UpdateProductionOrderStatus(order.ID, "completed", nil); err != nil {
		t.Fatalf("UpdateProductionOrderStatus(completed): %v", err)
	}

	p := requireStock(t, d, finished.ID, 2)
	if p.UnitCost != nil {
		t.Fatalf("finished unitCost = %d, want nil (an uncosted component leaves the output uncosted)", *p.UnitCost)
	}
	requireStock(t, d, uncosted.ID, 8)
	requireNoJournalEntries(t, d, org.ID)

	if _, err := d.UpdateProductionOrderStatus(order.ID, "cancelled", nil); err != nil {
		t.Fatalf("UpdateProductionOrderStatus(cancelled): %v", err)
	}
	requireStock(t, d, finished.ID, 0)
	requireStock(t, d, uncosted.ID, 10)
}

func TestInventoryValuationSwitchGuard(t *testing.T) {
	t.Parallel()

	t.Run("clean organization can switch both ways", func(t *testing.T) {
		t.Parallel()
		d := newTestDB(t)
		org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-switch-clean"})
		if err != nil {
			t.Fatalf("CreateOrganization: %v", err)
		}
		updated, err := d.UpdateOrganization(org.ID, UpdateOrganizationRequest{InventoryValuation: ptr(InventoryValuationQuantityOnly)})
		if err != nil {
			t.Fatalf("switch to quantity_only: %v", err)
		}
		if normalizeInventoryValuation(updated.InventoryValuation) != InventoryValuationQuantityOnly {
			t.Fatalf("inventoryValuation = %v, want quantity_only", updated.InventoryValuation)
		}
		if _, err := d.UpdateOrganization(org.ID, UpdateOrganizationRequest{InventoryValuation: ptr(InventoryValuationPerpetual)}); err != nil {
			t.Fatalf("switch back to perpetual: %v", err)
		}
	})

	t.Run("rejected once stock has moved", func(t *testing.T) {
		t.Parallel()
		d := newTestDB(t)
		org, product := newQuantityOnlyOrg(t, d, "org-switch-moved", false)
		if _, err := d.CreateStockMovement(CreateStockMovementRequest{
			OrganizationID: org.ID, ProductID: product.ID, Type: "in", Quantity: 1,
		}); err != nil {
			t.Fatalf("CreateStockMovement: %v", err)
		}
		_, err := d.UpdateOrganization(org.ID, UpdateOrganizationRequest{InventoryValuation: ptr(InventoryValuationPerpetual)})
		var verr *ValidationError
		if !errors.As(err, &verr) {
			t.Fatalf("expected a *ValidationError, got %T: %v", err, err)
		}
	})

	t.Run("rejected once the Inventory account carries an entry", func(t *testing.T) {
		t.Parallel()
		d := newTestDB(t)
		// A stock-product bill with no receipt capitalizes to Inventory in
		// perpetual mode without any stock movement existing.
		fx := newGRNITestFixture(t, d, "org-switch-gl", 10, 250)
		inv := fx.bill(t, d, "V-001", 4, 300, false)
		if _, err := d.UpdateIncomingInvoiceState(inv.ID, "approved"); err != nil {
			t.Fatalf("UpdateIncomingInvoiceState(approved): %v", err)
		}
		_, err := d.UpdateOrganization(fx.orgID, UpdateOrganizationRequest{InventoryValuation: ptr(InventoryValuationQuantityOnly)})
		var verr *ValidationError
		if !errors.As(err, &verr) {
			t.Fatalf("expected a *ValidationError, got %T: %v", err, err)
		}
	})

	t.Run("re-saving the current mode passes despite history", func(t *testing.T) {
		t.Parallel()
		d := newTestDB(t)
		fx := newGRNITestFixture(t, d, "org-switch-resave", 10, 250)
		fx.receive(t, d, "GR-0001", 10)
		// Stored NULL, sent "perpetual" — the drawer re-sends the whole form.
		if _, err := d.UpdateOrganization(fx.orgID, UpdateOrganizationRequest{InventoryValuation: ptr(InventoryValuationPerpetual)}); err != nil {
			t.Fatalf("re-save perpetual: %v", err)
		}
		if _, err := d.UpdateOrganization(fx.orgID, UpdateOrganizationRequest{InventoryValuation: ptr("")}); err != nil {
			t.Fatalf("re-save empty: %v", err)
		}
	})

	t.Run("unknown value rejected", func(t *testing.T) {
		t.Parallel()
		d := newTestDB(t)
		org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-switch-bad"})
		if err != nil {
			t.Fatalf("CreateOrganization: %v", err)
		}
		_, err = d.UpdateOrganization(org.ID, UpdateOrganizationRequest{InventoryValuation: ptr("fifo")})
		var verr *ValidationError
		if !errors.As(err, &verr) {
			t.Fatalf("expected a *ValidationError, got %T: %v", err, err)
		}
	})
}

func TestInventoryValuationReportCarriesMode(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	org, _ := newQuantityOnlyOrg(t, d, "org-qo-report", false)
	report, err := d.GetInventoryValuation(org.ID)
	if err != nil {
		t.Fatalf("GetInventoryValuation: %v", err)
	}
	if report.InventoryValuation != InventoryValuationQuantityOnly {
		t.Fatalf("report mode = %q, want quantity_only", report.InventoryValuation)
	}
}
