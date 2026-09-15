package db

import (
	"errors"
	"testing"
	"time"
)

// Test backfill for the 2026-09-14 audit's F80 — behaviours that shipped
// with zero coverage, so `go test` passing proved nothing about them.
//
// The F48 concurrency test that F80 also called for lives in
// db/concurrency_test.go, next to the five it was missing from.

// --- Cross-org reference rejection --------------------------------------

// db/org_ownership_test.go has one of these for 13 other domains.
// CreateProductionOrder has two cross-org-referenceable fields and neither
// was covered.
func TestCreateProductionOrderRejectsCrossOrgReferences(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := seedProductionOrderFixture(t, d, false)

	other, err := d.CreateOrganization(CreateOrganizationRequest{ID: "other-org"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}

	t.Run("finished product from another organization", func(t *testing.T) {
		_, err := d.CreateProductionOrder(CreateProductionOrderRequest{
			OrganizationID: other.ID, OrderNumber: "PRO-X",
			FinishedProductID: fx.finished.ID, Quantity: 1, Date: fx.date,
		})
		var verr *ValidationError
		if !errors.As(err, &verr) {
			t.Fatalf("expected a *ValidationError, got %T: %v", err, err)
		}
	})

	t.Run("import from another organization", func(t *testing.T) {
		foreignImport, err := d.CreateImport(CreateImportRequest{
			OrganizationID: other.ID, ImportNumber: "IMP-X", Date: fx.date,
		})
		if err != nil {
			t.Fatalf("CreateImport: %v", err)
		}
		_, err = d.CreateProductionOrder(CreateProductionOrderRequest{
			OrganizationID: fx.orgID, OrderNumber: "PRO-Y",
			FinishedProductID: fx.finished.ID, Quantity: 1, Date: fx.date,
			ImportID: &foreignImport.ID,
		})
		var verr *ValidationError
		if !errors.As(err, &verr) {
			t.Fatalf("expected a *ValidationError, got %T: %v", err, err)
		}
	})
}

// --- DeleteImport's production-order branch -----------------------------

// db/import_test.go only ever linked a *purchase order*, so the
// GetImportProductionOrderCount half of DeleteImport's guard had never
// executed in CI.
func TestDeleteImportRefusedWhileAProductionOrderLinksToIt(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := seedProductionOrderFixture(t, d, false)

	imp, err := d.CreateImport(CreateImportRequest{
		OrganizationID: fx.orgID, ImportNumber: "IMP-001", Date: fx.date,
	})
	if err != nil {
		t.Fatalf("CreateImport: %v", err)
	}
	order, err := d.CreateProductionOrder(CreateProductionOrderRequest{
		OrganizationID: fx.orgID, OrderNumber: "PRO-0001",
		FinishedProductID: fx.finished.ID, Quantity: 1, Date: fx.date,
		ImportID: &imp.ID,
	})
	if err != nil {
		t.Fatalf("CreateProductionOrder: %v", err)
	}

	if _, err := d.DeleteImport(imp.ID); !errors.Is(err, ErrImportInUse) {
		t.Fatalf("expected ErrImportInUse while a production order links to it, got %v", err)
	}

	// And it becomes deletable again once nothing references it.
	if _, err := d.DeleteProductionOrder(order.ID); err != nil {
		t.Fatalf("DeleteProductionOrder: %v", err)
	}
	if _, err := d.DeleteImport(imp.ID); err != nil {
		t.Fatalf("DeleteImport after unlinking: %v", err)
	}
}

// --- Cancelling a completed order: the GL reversal ----------------------

// The two existing GL tests are disjoint by construction — one creates a
// rounding residual but never cancels, the other cancels but uses an
// exact-division fixture where no entry is ever posted. So no test had ever
// reversed a production order's journal entry.
func TestCancelCompletedProductionOrderReversesItsPostedEntry(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)

	// This needs a *fractional* quantityPerUnit. With whole-number BOM
	// quantities the order quantity cancels out algebraically, so
	// componentsCostTotal always divides evenly and no entry is ever posted
	// — which is why the pre-existing fixture could never reach this branch.
	//
	// 0.5 per unit x 3 units = 1.5 components at 10001 cents = 15001.5,
	// rounded to 15002. perUnitCost = round(15002/3) = 5001, so the produced
	// value is 15003 and a 1-cent residual posts against Inventory
	// Adjustment.
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-f80-reversal"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	now := time.Now()
	date := now.UnixMilli()
	if _, err := d.CreateFiscalYear(CreateFiscalYearRequest{
		OrganizationID: org.ID, Name: "Test Year",
		StartDate: now.AddDate(-1, 0, 0).UnixMilli(), EndDate: now.AddDate(1, 0, 0).UnixMilli(),
	}); err != nil {
		t.Fatalf("CreateFiscalYear: %v", err)
	}
	componentCat, finishedCat := "component", "finished"
	component, err := d.CreateProduct(CreateProductRequest{
		OrganizationID: org.ID, Name: "Resin", SKU: ptr("RSN-1"), Type: "product",
		StockEnabled: 1, Category: &componentCat,
	})
	if err != nil {
		t.Fatalf("CreateProduct(component): %v", err)
	}
	if _, err := d.CreateStockMovement(CreateStockMovementRequest{
		OrganizationID: org.ID, ProductID: component.ID, Type: "in",
		Quantity: 100, UnitCost: ptr(int64(10001)),
	}); err != nil {
		t.Fatalf("CreateStockMovement: %v", err)
	}
	finished, err := d.CreateProduct(CreateProductRequest{
		OrganizationID: org.ID, Name: "Panel", SKU: ptr("PNL-1"), Type: "product",
		StockEnabled: 1, Category: &finishedCat,
	})
	if err != nil {
		t.Fatalf("CreateProduct(finished): %v", err)
	}
	if _, err := d.ReplaceBillOfMaterials(finished.ID, []CreateBillOfMaterialsLineRequest{
		{ComponentProductID: component.ID, QuantityPerUnit: 0.5},
	}, 1); err != nil {
		t.Fatalf("ReplaceBillOfMaterials: %v", err)
	}

	order, err := d.CreateProductionOrder(CreateProductionOrderRequest{
		OrganizationID: org.ID, OrderNumber: "PRO-0001",
		FinishedProductID: finished.ID, Quantity: 3, Date: date,
	})
	if err != nil {
		t.Fatalf("CreateProductionOrder: %v", err)
	}
	if _, err := d.UpdateProductionOrderStatus(order.ID, "completed", nil); err != nil {
		t.Fatalf("UpdateProductionOrderStatus(completed): %v", err)
	}

	entry, err := d.FindPostedEntryForSourceDocument("production_order", order.ID)
	if err != nil {
		t.Fatalf("FindPostedEntryForSourceDocument: %v", err)
	}
	if entry == nil {
		t.Fatal("no residual entry was posted — this fixture is supposed to produce one, so the reversal branch would go untested again")
	}

	if _, err := d.UpdateProductionOrderStatus(order.ID, "cancelled", nil); err != nil {
		t.Fatalf("UpdateProductionOrderStatus(cancelled): %v", err)
	}

	reversed, err := d.GetJournalEntry(entry.ID)
	if err != nil {
		t.Fatalf("GetJournalEntry: %v", err)
	}
	if reversed.Status != "reversed" {
		t.Fatalf("original entry status = %q, want reversed", reversed.Status)
	}
	var reversalCount int
	if err := d.DB.Get(&reversalCount,
		`SELECT COUNT(*) FROM journal_entries WHERE reversalOfEntryId = ?`, entry.ID,
	); err != nil {
		t.Fatalf("count reversals: %v", err)
	}
	if reversalCount != 1 {
		t.Fatalf("reversal entry count = %d, want exactly 1", reversalCount)
	}
}

// --- Cancelling a completed serialized order ----------------------------

// The whole finished.Serialized == 1 reversal branch was dead code in CI —
// the only complete-then-cancel test used a non-serialized fixture.
func TestCancelCompletedSerializedProductionOrderReversesSerials(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := seedProductionOrderFixture(t, d, true)

	order, err := d.CreateProductionOrder(CreateProductionOrderRequest{
		OrganizationID: fx.orgID, OrderNumber: "PRO-0001",
		FinishedProductID: fx.finished.ID, Quantity: 2, Date: fx.date,
	})
	if err != nil {
		t.Fatalf("CreateProductionOrder: %v", err)
	}
	if _, err := d.UpdateProductionOrderStatus(
		order.ID, "completed", []string{"SN-001", "SN-002"},
	); err != nil {
		t.Fatalf("UpdateProductionOrderStatus(completed): %v", err)
	}

	serials, err := d.GetProductSerialNumbers(fx.finished.ID)
	if err != nil {
		t.Fatalf("GetProductSerialNumbers: %v", err)
	}
	inStock := 0
	for _, s := range serials {
		if s.InStock == 1 {
			inStock++
		}
	}
	if inStock != 2 {
		t.Fatalf("serials in stock after completion = %d, want 2", inStock)
	}

	if _, err := d.UpdateProductionOrderStatus(order.ID, "cancelled", nil); err != nil {
		t.Fatalf("UpdateProductionOrderStatus(cancelled): %v", err)
	}

	after, err := d.GetProductSerialNumbers(fx.finished.ID)
	if err != nil {
		t.Fatalf("GetProductSerialNumbers after cancel: %v", err)
	}
	// The registry rows are never deleted — a serial's in-stock status is
	// computed from the sign of its most recent movement, so the reversal
	// must flip all of them back out.
	if len(after) != len(serials) {
		t.Fatalf("serial registry rows changed on cancel: %d -> %d", len(serials), len(after))
	}
	for _, s := range after {
		if s.InStock == 1 {
			t.Fatalf("serial %q still in stock after the order was cancelled", s.SerialNumber)
		}
	}
	finished, err := d.GetProduct(fx.finished.ID)
	if err != nil {
		t.Fatalf("GetProduct: %v", err)
	}
	if finished.StockQuantity != 0 {
		t.Fatalf("finished stockQuantity = %v, want 0 after cancel", finished.StockQuantity)
	}
}

// --- Document numbering and listing -------------------------------------

func TestNextProductionOrderNumberContinuesFromTheHighest(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := seedProductionOrderFixture(t, d, false)

	first := d.NextProductionOrderNumber(fx.orgID)
	if first == "" {
		t.Fatal("NextProductionOrderNumber returned an empty string")
	}

	if _, err := d.CreateProductionOrder(CreateProductionOrderRequest{
		OrganizationID: fx.orgID, OrderNumber: first,
		FinishedProductID: fx.finished.ID, Quantity: 1, Date: fx.date,
	}); err != nil {
		t.Fatalf("CreateProductionOrder: %v", err)
	}

	second := d.NextProductionOrderNumber(fx.orgID)
	if second == first {
		t.Fatalf("NextProductionOrderNumber returned %q twice — the allocation does not advance", first)
	}
}

// GetProductionOrders must be scoped to its organization — untested, and the
// kind of thing that silently leaks everything if a WHERE clause is dropped.
func TestGetProductionOrdersIsScopedToItsOrganization(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := seedProductionOrderFixture(t, d, false)

	if _, err := d.CreateProductionOrder(CreateProductionOrderRequest{
		OrganizationID: fx.orgID, OrderNumber: "PRO-0001",
		FinishedProductID: fx.finished.ID, Quantity: 1, Date: fx.date,
	}); err != nil {
		t.Fatalf("CreateProductionOrder: %v", err)
	}
	other, err := d.CreateOrganization(CreateOrganizationRequest{ID: "other-org"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}

	mine, err := d.GetProductionOrders(fx.orgID)
	if err != nil || len(mine) != 1 {
		t.Fatalf("GetProductionOrders(own): err=%v len=%d, want 1", err, len(mine))
	}
	theirs, err := d.GetProductionOrders(other.ID)
	if err != nil {
		t.Fatalf("GetProductionOrders(other): %v", err)
	}
	if len(theirs) != 0 {
		t.Fatalf("GetProductionOrders leaked %d orders into another organization", len(theirs))
	}
}

// --- Transition matrix --------------------------------------------------

// productionOrderStatusTransitions had no table-driven test, unlike other
// document types. draft -> cancelled in particular matches no switch case,
// so it is a pure status flip and nothing else exercised it.
func TestProductionOrderStatusTransitions(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		from    string
		to      string
		allowed bool
	}{
		{"draft to completed", "draft", "completed", true},
		{"draft to cancelled", "draft", "cancelled", true},
		{"completed to cancelled", "completed", "cancelled", true},
		{"completed back to draft", "completed", "draft", false},
		{"cancelled is terminal", "cancelled", "draft", false},
		{"cancelled cannot complete", "cancelled", "completed", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := productionOrderStatusTransitions[tc.from][tc.to]; got != tc.allowed {
				t.Fatalf("%s -> %s allowed = %v, want %v", tc.from, tc.to, got, tc.allowed)
			}
		})
	}
}

// draft -> cancelled is legal and matches no switch case in
// UpdateProductionOrderStatus, so it must flip the status and touch nothing
// else — no stock movement, no GL entry.
func TestCancelDraftProductionOrderTouchesNothingElse(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := seedProductionOrderFixture(t, d, false)

	componentBefore, err := d.GetProduct(fx.componentA.ID)
	if err != nil {
		t.Fatalf("GetProduct: %v", err)
	}
	order, err := d.CreateProductionOrder(CreateProductionOrderRequest{
		OrganizationID: fx.orgID, OrderNumber: "PRO-0001",
		FinishedProductID: fx.finished.ID, Quantity: 1, Date: fx.date,
	})
	if err != nil {
		t.Fatalf("CreateProductionOrder: %v", err)
	}

	cancelled, err := d.UpdateProductionOrderStatus(order.ID, "cancelled", nil)
	if err != nil {
		t.Fatalf("UpdateProductionOrderStatus(cancelled): %v", err)
	}
	if cancelled.Status != "cancelled" {
		t.Fatalf("status = %q, want cancelled", cancelled.Status)
	}

	componentAfter, err := d.GetProduct(fx.componentA.ID)
	if err != nil {
		t.Fatalf("GetProduct after: %v", err)
	}
	if componentAfter.StockQuantity != componentBefore.StockQuantity {
		t.Fatalf("cancelling a draft moved component stock: %v -> %v",
			componentBefore.StockQuantity, componentAfter.StockQuantity)
	}
	entry, err := d.FindPostedEntryForSourceDocument("production_order", order.ID)
	if err != nil {
		t.Fatalf("FindPostedEntryForSourceDocument: %v", err)
	}
	if entry != nil {
		t.Fatal("cancelling a draft posted a GL entry")
	}
}

// --- Units of measure: the seed backfill --------------------------------

// SeedDefaultUnitsOfMeasureForAllOrganizations had no coverage at all,
// despite CLAUDE.md describing its per-org gate, its idempotency and its
// case-insensitive product backfill in detail.
func TestSeedDefaultUnitsOfMeasureForAllOrganizationsIsIdempotent(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-seed-uom"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}

	before, err := d.GetUnitsOfMeasure(org.ID)
	if err != nil {
		t.Fatalf("GetUnitsOfMeasure: %v", err)
	}
	if len(before) == 0 {
		t.Fatal("creating an organization seeded no units of measure")
	}

	// Running the backfill repeatedly must not duplicate the list — the gate
	// is a COUNT(*)==0 check per organization.
	for range 3 {
		if err := d.SeedDefaultUnitsOfMeasureForAllOrganizations(); err != nil {
			t.Fatalf("SeedDefaultUnitsOfMeasureForAllOrganizations: %v", err)
		}
	}

	after, err := d.GetUnitsOfMeasure(org.ID)
	if err != nil {
		t.Fatalf("GetUnitsOfMeasure after: %v", err)
	}
	if len(after) != len(before) {
		t.Fatalf("unit of measure count changed across repeated seeding: %d -> %d", len(before), len(after))
	}
}

// The pre-existing TestCreateProductRejectsCrossOrgUnitOfMeasure asserts
// only err != nil, so it would still pass if the error class regressed to a
// 500 — which is exactly the F72 bug. This pins the type.
func TestCreateProductCrossOrgUnitOfMeasureIsAValidationError(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	orgA, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-uom-a"})
	if err != nil {
		t.Fatalf("CreateOrganization(a): %v", err)
	}
	orgB, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-uom-b"})
	if err != nil {
		t.Fatalf("CreateOrganization(b): %v", err)
	}
	foreign, err := d.CreateUnitOfMeasure(CreateUnitOfMeasureRequest{
		OrganizationID: orgB.ID, Name: "testunit-foreign",
	})
	if err != nil {
		t.Fatalf("CreateUnitOfMeasure: %v", err)
	}

	_, err = d.CreateProduct(CreateProductRequest{
		OrganizationID: orgA.ID, Name: "Widget", SKU: ptr("WID-1"), Type: "product",
		UnitOfMeasureID: &foreign.ID,
	})
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected a *ValidationError (409), got %T: %v — a 500 would reach the user as \"internal error\"", err, err)
	}
}
