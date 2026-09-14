package db

import (
	"testing"
	"time"
)

func TestCreateProductionOrderHappyPath(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	f := seedProductionOrderFixture(t, d, false)

	order, err := d.CreateProductionOrder(CreateProductionOrderRequest{
		OrganizationID: f.orgID, OrderNumber: "PRO-0001", FinishedProductID: f.finished.ID,
		Quantity: 3, Date: f.date,
	})
	if err != nil {
		t.Fatalf("CreateProductionOrder: %v", err)
	}
	if order.Status != "draft" {
		t.Errorf("status = %q, want draft", order.Status)
	}

	lines, err := d.GetProductionOrderComponentLines(order.ID)
	if err != nil {
		t.Fatalf("GetProductionOrderComponentLines: %v", err)
	}
	if len(lines) != 2 {
		t.Fatalf("got %d component lines, want 2", len(lines))
	}
	byComponent := map[string]float64{}
	for _, l := range lines {
		byComponent[*l.ComponentProductID] = l.TotalQuantity
	}
	if byComponent[f.componentA.ID] != 3 {
		t.Errorf("componentA total = %v, want 3 (1 * 3)", byComponent[f.componentA.ID])
	}
	if byComponent[f.componentB.ID] != 6 {
		t.Errorf("componentB total = %v, want 6 (2 * 3)", byComponent[f.componentB.ID])
	}
}

func TestCreateProductionOrderRejectsNonFinishedProduct(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	f := seedProductionOrderFixture(t, d, false)

	_, err := d.CreateProductionOrder(CreateProductionOrderRequest{
		OrganizationID: f.orgID, OrderNumber: "PRO-0001", FinishedProductID: f.componentA.ID,
		Quantity: 1, Date: f.date,
	})
	if err == nil {
		t.Fatal("expected error for non-finished product")
	}
}

func TestCreateProductionOrderRejectsEmptyBOM(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	f := seedProductionOrderFixture(t, d, false)

	finishedCategory := "finished"
	noBOM, err := d.CreateProduct(CreateProductRequest{
		OrganizationID: f.orgID, Name: "Scooter", Type: "product", StockEnabled: 1, Category: &finishedCategory,
	})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}

	_, err = d.CreateProductionOrder(CreateProductionOrderRequest{
		OrganizationID: f.orgID, OrderNumber: "PRO-0001", FinishedProductID: noBOM.ID,
		Quantity: 1, Date: f.date,
	})
	if err == nil {
		t.Fatal("expected error for finished product with no BOM")
	}
}

func TestCreateProductionOrderRejectsSerializedComponent(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	f := seedProductionOrderFixture(t, d, false)

	componentCategory := "component"
	serializedComponent, err := d.CreateProduct(CreateProductRequest{
		OrganizationID: f.orgID, Name: "ECU", Type: "product", StockEnabled: 1,
		Category: &componentCategory, Serialized: 1,
	})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	if _, err := d.ReplaceBillOfMaterials(f.finished.ID, []CreateBillOfMaterialsLineRequest{
		{ComponentProductID: f.componentA.ID, QuantityPerUnit: 1},
		{ComponentProductID: serializedComponent.ID, QuantityPerUnit: 1},
	}, 1); err != nil {
		t.Fatalf("ReplaceBillOfMaterials: %v", err)
	}

	_, err = d.CreateProductionOrder(CreateProductionOrderRequest{
		OrganizationID: f.orgID, OrderNumber: "PRO-0001", FinishedProductID: f.finished.ID,
		Quantity: 1, Date: f.date,
	})
	if err == nil {
		t.Fatal("expected error for a BOM containing a serialized component")
	}
}

func TestCreateProductionOrderRejectsInsufficientComponentStock(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	f := seedProductionOrderFixture(t, d, false)

	order, err := d.CreateProductionOrder(CreateProductionOrderRequest{
		OrganizationID: f.orgID, OrderNumber: "PRO-0001", FinishedProductID: f.finished.ID,
		Quantity: 1000, Date: f.date, // way more than the 100 units seeded per component
	})
	if err != nil {
		t.Fatalf("CreateProductionOrder: %v", err)
	}
	if _, err := d.UpdateProductionOrderStatus(order.ID, "completed", nil); err == nil {
		t.Fatal("expected insufficient-stock error completing an order that exceeds component stock")
	}
}

// TestUpdateProductionOrderStatusCompleteExactDivision covers the common
// case: componentsCostTotal divides evenly by quantity, so the finished
// good's produced value exactly matches what was consumed and no GL entry
// posts at all (buildProductionOrderGLLines' zero-residual branch).
func TestUpdateProductionOrderStatusCompleteExactDivision(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	f := seedProductionOrderFixture(t, d, false)

	// Recipe cost per finished unit is 1*100.00 + 2*50.00 = 200.00 — quantity-
	// invariant by construction (both totalQuantity and its cost scale
	// linearly with order quantity, so dividing back out always recovers
	// the same per-unit recipe cost exactly, for any quantity).
	order, err := d.CreateProductionOrder(CreateProductionOrderRequest{
		OrganizationID: f.orgID, OrderNumber: "PRO-0001", FinishedProductID: f.finished.ID,
		Quantity: 2, Date: f.date,
	})
	if err != nil {
		t.Fatalf("CreateProductionOrder: %v", err)
	}

	updated, err := d.UpdateProductionOrderStatus(order.ID, "completed", nil)
	if err != nil {
		t.Fatalf("UpdateProductionOrderStatus(completed): %v", err)
	}
	if updated.Status != "completed" {
		t.Errorf("status = %q, want completed", updated.Status)
	}

	compA, err := d.GetProduct(f.componentA.ID)
	if err != nil {
		t.Fatalf("GetProduct componentA: %v", err)
	}
	if compA.StockQuantity != 98 { // 100 - 1*2
		t.Errorf("componentA stock = %v, want 98", compA.StockQuantity)
	}
	compB, err := d.GetProduct(f.componentB.ID)
	if err != nil {
		t.Fatalf("GetProduct componentB: %v", err)
	}
	if compB.StockQuantity != 96 { // 100 - 2*2
		t.Errorf("componentB stock = %v, want 96", compB.StockQuantity)
	}

	finished, err := d.GetProduct(f.finished.ID)
	if err != nil {
		t.Fatalf("GetProduct finished: %v", err)
	}
	if finished.StockQuantity != 2 {
		t.Errorf("finished stock = %v, want 2", finished.StockQuantity)
	}
	if finished.UnitCost == nil || *finished.UnitCost != 20000 {
		t.Errorf("finished unitCost = %v, want 20000 (200.00)", *finished.UnitCost)
	}

	entry, err := d.FindPostedEntryForSourceDocument("production_order", order.ID)
	if err != nil {
		t.Fatalf("FindPostedEntryForSourceDocument: %v", err)
	}
	if entry != nil {
		t.Errorf("expected no GL entry for an exact-division production order, got one: %+v", entry)
	}
}

// TestUpdateProductionOrderStatusCompletePostsRoundingResidual covers a
// non-exact division. Note that a BOM built from whole-number
// quantityPerUnit values can never produce a residual: totalQuantity is
// always exactly quantityPerUnit*orderQty (no rounding loss), so
// componentsCostTotal is always an exact multiple of orderQty and divides
// back out clean regardless of quantity (see the exact-division test
// above). A residual needs a fractional per-unit cost — here, a component
// consumed at 0.3333 units/finished-unit (a recipe fraction stored to
// product_bom.go's 4-decimal precision) costing 7 cents/unit: over
// quantity 3, componentsCostTotal rounds to 7 cents, but 7/3 rounds to 2
// cents/unit, and 3*2 = 6 — a genuine 1-cent gap between what was consumed
// and what the produced good is valued at, which must post against
// Inventory Adjustment rather than silently vanish.
func TestUpdateProductionOrderStatusCompletePostsRoundingResidual(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	f := seedProductionOrderFixture(t, d, false)

	componentCategory := "component"
	fractional, err := d.CreateProduct(CreateProductRequest{
		OrganizationID: f.orgID, Name: "Gasket", Type: "product", StockEnabled: 1,
		Category: &componentCategory, UnitCost: ptr(int64(7)),
	})
	if err != nil {
		t.Fatalf("CreateProduct fractional component: %v", err)
	}
	if _, err := d.CreateStockMovement(CreateStockMovementRequest{
		OrganizationID: f.orgID, ProductID: fractional.ID, Type: "in", Quantity: 100,
	}); err != nil {
		t.Fatalf("CreateStockMovement seed stock: %v", err)
	}
	if _, err := d.ReplaceBillOfMaterials(f.finished.ID, []CreateBillOfMaterialsLineRequest{
		{ComponentProductID: fractional.ID, QuantityPerUnit: 0.3333},
	}, 1); err != nil {
		t.Fatalf("ReplaceBillOfMaterials: %v", err)
	}

	order, err := d.CreateProductionOrder(CreateProductionOrderRequest{
		OrganizationID: f.orgID, OrderNumber: "PRO-0001", FinishedProductID: f.finished.ID,
		Quantity: 3, Date: f.date,
	})
	if err != nil {
		t.Fatalf("CreateProductionOrder: %v", err)
	}

	if _, err := d.UpdateProductionOrderStatus(order.ID, "completed", nil); err != nil {
		t.Fatalf("UpdateProductionOrderStatus(completed): %v", err)
	}

	finished, err := d.GetProduct(f.finished.ID)
	if err != nil {
		t.Fatalf("GetProduct finished: %v", err)
	}
	if finished.UnitCost == nil || *finished.UnitCost != 2 {
		t.Fatalf("finished unitCost = %v, want 2 (0.02)", *finished.UnitCost)
	}

	entry, err := d.FindPostedEntryForSourceDocument("production_order", order.ID)
	if err != nil {
		t.Fatalf("FindPostedEntryForSourceDocument: %v", err)
	}
	if entry == nil {
		t.Fatal("expected a GL entry posting the 1-cent rounding residual, got none")
	}
	lines, err := d.GetJournalEntryLines(entry.ID)
	if err != nil {
		t.Fatalf("GetJournalEntryLines: %v", err)
	}
	var totalDebit, totalCredit int64
	for _, l := range lines {
		totalDebit += l.Debit
		totalCredit += l.Credit
	}
	if totalDebit != totalCredit {
		t.Errorf("entry doesn't balance: debit=%d credit=%d", totalDebit, totalCredit)
	}
	if totalDebit != 1 { // producedValueCents(6) - componentsCostTotal(7) = -1
		t.Errorf("residual entry total = %d, want 1 cent", totalDebit)
	}
}

func TestUpdateProductionOrderStatusCompleteThenCancelReversesEverything(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	f := seedProductionOrderFixture(t, d, false)

	order, err := d.CreateProductionOrder(CreateProductionOrderRequest{
		OrganizationID: f.orgID, OrderNumber: "PRO-0001", FinishedProductID: f.finished.ID,
		Quantity: 2, Date: f.date,
	})
	if err != nil {
		t.Fatalf("CreateProductionOrder: %v", err)
	}
	if _, err := d.UpdateProductionOrderStatus(order.ID, "completed", nil); err != nil {
		t.Fatalf("UpdateProductionOrderStatus(completed): %v", err)
	}

	updated, err := d.UpdateProductionOrderStatus(order.ID, "cancelled", nil)
	if err != nil {
		t.Fatalf("UpdateProductionOrderStatus(cancelled): %v", err)
	}
	if updated.Status != "cancelled" {
		t.Errorf("status = %q, want cancelled", updated.Status)
	}

	compA, err := d.GetProduct(f.componentA.ID)
	if err != nil {
		t.Fatalf("GetProduct componentA: %v", err)
	}
	if compA.StockQuantity != 100 {
		t.Errorf("componentA stock after cancel = %v, want 100 (restored)", compA.StockQuantity)
	}
	finished, err := d.GetProduct(f.finished.ID)
	if err != nil {
		t.Fatalf("GetProduct finished: %v", err)
	}
	if finished.StockQuantity != 0 {
		t.Errorf("finished stock after cancel = %v, want 0", finished.StockQuantity)
	}
}

func TestUpdateProductionOrderStatusCancelBlockedWhenFinishedGoodShipped(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	f := seedProductionOrderFixture(t, d, false)

	order, err := d.CreateProductionOrder(CreateProductionOrderRequest{
		OrganizationID: f.orgID, OrderNumber: "PRO-0001", FinishedProductID: f.finished.ID,
		Quantity: 2, Date: f.date,
	})
	if err != nil {
		t.Fatalf("CreateProductionOrder: %v", err)
	}
	if _, err := d.UpdateProductionOrderStatus(order.ID, "completed", nil); err != nil {
		t.Fatalf("UpdateProductionOrderStatus(completed): %v", err)
	}

	// Ship one of the two produced units out, simulating an ordinary sale.
	if _, err := d.CreateStockMovement(CreateStockMovementRequest{
		OrganizationID: f.orgID, ProductID: f.finished.ID, Type: "out", Quantity: -1,
	}); err != nil {
		t.Fatalf("CreateStockMovement ship: %v", err)
	}

	if _, err := d.UpdateProductionOrderStatus(order.ID, "cancelled", nil); err == nil {
		t.Fatal("expected cancel to be blocked once a produced unit has shipped")
	}
}

func TestUpdateProductionOrderStatusSerializedFinishedGoodRequiresSerials(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	f := seedProductionOrderFixture(t, d, true)

	order, err := d.CreateProductionOrder(CreateProductionOrderRequest{
		OrganizationID: f.orgID, OrderNumber: "PRO-0001", FinishedProductID: f.finished.ID,
		Quantity: 2, Date: f.date,
	})
	if err != nil {
		t.Fatalf("CreateProductionOrder: %v", err)
	}

	if _, err := d.UpdateProductionOrderStatus(order.ID, "completed", nil); err == nil {
		t.Fatal("expected error completing a serialized finished good with no serials supplied")
	}

	updated, err := d.UpdateProductionOrderStatus(order.ID, "completed", []string{"VIN-001", "VIN-002"})
	if err != nil {
		t.Fatalf("UpdateProductionOrderStatus(completed) with serials: %v", err)
	}
	if updated.Status != "completed" {
		t.Errorf("status = %q, want completed", updated.Status)
	}

	serials, err := d.GetProductSerialNumbers(f.finished.ID)
	if err != nil {
		t.Fatalf("GetProductSerialNumbers: %v", err)
	}
	if len(serials) != 2 {
		t.Fatalf("got %d serials, want 2", len(serials))
	}
	for _, s := range serials {
		if s.InStock != 1 {
			t.Errorf("serial %q inStock = %d, want 1", s.SerialNumber, s.InStock)
		}
	}
}

func TestUpdateProductionOrderStatusValidatesSerialAgainstImportRange(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	f := seedProductionOrderFixture(t, d, true)

	imp, err := d.CreateImport(CreateImportRequest{
		OrganizationID: f.orgID, ImportNumber: "IMP-0001", Date: f.date,
		SerialNumberPrefix: ptr("VIN-"), SerialNumberRangeStart: ptr(int64(1)), SerialNumberRangeEnd: ptr(int64(10)),
	})
	if err != nil {
		t.Fatalf("CreateImport: %v", err)
	}

	order, err := d.CreateProductionOrder(CreateProductionOrderRequest{
		OrganizationID: f.orgID, OrderNumber: "PRO-0001", FinishedProductID: f.finished.ID,
		Quantity: 1, Date: f.date, ImportID: &imp.ID,
	})
	if err != nil {
		t.Fatalf("CreateProductionOrder: %v", err)
	}

	if _, err := d.UpdateProductionOrderStatus(order.ID, "completed", []string{"VIN-999"}); err == nil {
		t.Fatal("expected error for a serial outside the import's reserved range")
	}
	if _, err := d.UpdateProductionOrderStatus(order.ID, "completed", []string{"VIN-5"}); err != nil {
		t.Fatalf("UpdateProductionOrderStatus(completed) with an in-range serial: %v", err)
	}
}

func TestDeleteProductionOrderOnlyDraft(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	f := seedProductionOrderFixture(t, d, false)

	order, err := d.CreateProductionOrder(CreateProductionOrderRequest{
		OrganizationID: f.orgID, OrderNumber: "PRO-0001", FinishedProductID: f.finished.ID,
		Quantity: 1, Date: f.date,
	})
	if err != nil {
		t.Fatalf("CreateProductionOrder: %v", err)
	}
	if _, err := d.UpdateProductionOrderStatus(order.ID, "completed", nil); err != nil {
		t.Fatalf("UpdateProductionOrderStatus(completed): %v", err)
	}
	if _, err := d.DeleteProductionOrder(order.ID); err == nil {
		t.Fatal("expected delete to be rejected for a completed production order")
	}

	order2, err := d.CreateProductionOrder(CreateProductionOrderRequest{
		OrganizationID: f.orgID, OrderNumber: "PRO-0002", FinishedProductID: f.finished.ID,
		Quantity: 1, Date: f.date,
	})
	if err != nil {
		t.Fatalf("CreateProductionOrder: %v", err)
	}
	ok, err := d.DeleteProductionOrder(order2.ID)
	if err != nil || !ok {
		t.Fatalf("DeleteProductionOrder(draft) = %v, %v; want true, nil", ok, err)
	}
}

// seedProductionOrderFixture wraps newProductionOrderTestFixture but keeps
// the *Database it built against alongside it via the passed-in d, since
// newTestDB opens a fresh, isolated DB per call.
func seedProductionOrderFixture(t *testing.T, d *Database, serializedFinished bool) productionOrderFixture {
	t.Helper()

	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "prod-order-org"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	// Spans a year on either side of "now": production order dates
	// (f.date) are caller-controlled, but the raw CreateStockMovement calls
	// this fixture uses to seed component stock resolve their GL date via
	// time.Now() internally (db/stock.go), not a caller-supplied date, so
	// the fiscal year must cover both.
	now := time.Now()
	date := now.UnixMilli()
	if _, err := d.CreateFiscalYear(CreateFiscalYearRequest{
		OrganizationID: org.ID, Name: "Test Year",
		StartDate: now.AddDate(-1, 0, 0).UnixMilli(), EndDate: now.AddDate(1, 0, 0).UnixMilli(),
	}); err != nil {
		t.Fatalf("CreateFiscalYear: %v", err)
	}

	finishedCategory := "finished"
	finishedSerialized := 0
	if serializedFinished {
		finishedSerialized = 1
	}
	finished, err := d.CreateProduct(CreateProductRequest{
		OrganizationID: org.ID, Name: "Motorcycle", Type: "product", StockEnabled: 1,
		Category: &finishedCategory, Serialized: finishedSerialized,
	})
	if err != nil {
		t.Fatalf("CreateProduct finished: %v", err)
	}

	componentCategory := "component"
	componentA, err := d.CreateProduct(CreateProductRequest{
		OrganizationID: org.ID, Name: "Engine block", Type: "product", StockEnabled: 1,
		Category: &componentCategory, UnitCost: ptr(int64(10000)),
	})
	if err != nil {
		t.Fatalf("CreateProduct componentA: %v", err)
	}
	componentB, err := d.CreateProduct(CreateProductRequest{
		OrganizationID: org.ID, Name: "Frame chassis", Type: "product", StockEnabled: 1,
		Category: &componentCategory, UnitCost: ptr(int64(5000)),
	})
	if err != nil {
		t.Fatalf("CreateProduct componentB: %v", err)
	}
	for _, p := range []*Product{componentA, componentB} {
		if _, err := d.CreateStockMovement(CreateStockMovementRequest{
			OrganizationID: org.ID, ProductID: p.ID, Type: "in", Quantity: 100,
		}); err != nil {
			t.Fatalf("CreateStockMovement seed stock for %s: %v", p.Name, err)
		}
	}

	if _, err := d.ReplaceBillOfMaterials(finished.ID, []CreateBillOfMaterialsLineRequest{
		{ComponentProductID: componentA.ID, QuantityPerUnit: 1},
		{ComponentProductID: componentB.ID, QuantityPerUnit: 2},
	}, 1); err != nil {
		t.Fatalf("ReplaceBillOfMaterials: %v", err)
	}

	return productionOrderFixture{
		orgID: org.ID, finished: finished, componentA: componentA, componentB: componentB, date: date,
	}
}

type productionOrderFixture struct {
	orgID      string
	finished   *Product
	componentA *Product
	componentB *Product
	date       int64
}
