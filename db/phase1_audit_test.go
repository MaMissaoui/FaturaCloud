package db

import (
	"errors"
	"testing"
)

// Regression tests for the 2026-09-14 audit's Phase 1 findings other than
// F70/F93 (those live in db/line_item_reconcile_test.go).

// --- F71: products.unit is derived, so it must track its source ----------

// Renaming a unit of measure must re-derive products.unit for every product
// linked to it. Before the fix nothing ever re-derived it, so a rename left
// linked products displaying the old text indefinitely.
func TestUpdateUnitOfMeasureRenamePropagatesToLinkedProducts(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-f71-rename"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	uom, err := d.CreateUnitOfMeasure(CreateUnitOfMeasureRequest{
		OrganizationID: org.ID, Name: "testunit-mass",
	})
	if err != nil {
		t.Fatalf("CreateUnitOfMeasure: %v", err)
	}
	other, err := d.CreateUnitOfMeasure(CreateUnitOfMeasureRequest{
		OrganizationID: org.ID, Name: "testunit-count",
	})
	if err != nil {
		t.Fatalf("CreateUnitOfMeasure(other): %v", err)
	}

	linked, err := d.CreateProduct(CreateProductRequest{
		OrganizationID: org.ID, Name: "Flour", SKU: ptr("FLR-1"), Type: "product",
		UnitOfMeasureID: &uom.ID,
	})
	if err != nil {
		t.Fatalf("CreateProduct(linked): %v", err)
	}
	if linked.Unit == nil || *linked.Unit != "testunit-mass" {
		t.Fatalf("product unit at create = %v, want \"testunit-mass\"", linked.Unit)
	}
	unrelated, err := d.CreateProduct(CreateProductRequest{
		OrganizationID: org.ID, Name: "Widget", SKU: ptr("WID-1"), Type: "product",
		UnitOfMeasureID: &other.ID,
	})
	if err != nil {
		t.Fatalf("CreateProduct(unrelated): %v", err)
	}

	newName := "testunit-mass-renamed"
	if _, err := d.UpdateUnitOfMeasure(uom.ID, UpdateUnitOfMeasureRequest{Name: &newName}); err != nil {
		t.Fatalf("UpdateUnitOfMeasure: %v", err)
	}

	got, err := d.GetProduct(linked.ID)
	if err != nil {
		t.Fatalf("GetProduct: %v", err)
	}
	if got.Unit == nil || *got.Unit != "testunit-mass-renamed" {
		t.Fatalf("linked product unit after rename = %v, want \"testunit-mass-renamed\"", got.Unit)
	}

	untouched, err := d.GetProduct(unrelated.ID)
	if err != nil {
		t.Fatalf("GetProduct(unrelated): %v", err)
	}
	if untouched.Unit == nil || *untouched.Unit != "testunit-count" {
		t.Fatalf("unrelated product unit changed: %v, want \"testunit-count\"", untouched.Unit)
	}
}

// Clearing unitOfMeasureId must clear the derived unit text too, even when
// the caller echoes the old value back — otherwise a ghost value nothing
// owns survives. This is the server-side half of the fix commit 47e1e0c made
// only in the product form.
func TestUpdateProductClearingUnitOfMeasureClearsDerivedUnit(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-f71-clear"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	uom, err := d.CreateUnitOfMeasure(CreateUnitOfMeasureRequest{OrganizationID: org.ID, Name: "testunit-mass"})
	if err != nil {
		t.Fatalf("CreateUnitOfMeasure: %v", err)
	}
	p, err := d.CreateProduct(CreateProductRequest{
		OrganizationID: org.ID, Name: "Flour", SKU: ptr("FLR-2"), Type: "product",
		UnitOfMeasureID: &uom.ID,
	})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}

	// The client clears the picker but echoes the legacy unit text back, the
	// exact shape that used to leave a ghost value.
	stale := "testunit-mass"
	updated, err := d.UpdateProduct(p.ID, UpdateProductRequest{
		Name: "Flour", SKU: ptr("FLR-2"), Type: "product",
		UnitOfMeasureID: nil, Unit: &stale,
	})
	if err != nil {
		t.Fatalf("UpdateProduct: %v", err)
	}
	if updated.UnitOfMeasureID != nil {
		t.Fatalf("unitOfMeasureId not cleared: %v", updated.UnitOfMeasureID)
	}
	if updated.Unit != nil && *updated.Unit != "" {
		t.Fatalf("derived unit survived the clear as a ghost value: %q", *updated.Unit)
	}
}

// A product that never had a structured unit keeps its free-text unit — the
// clear rule above must not confiscate plain free-text entry from callers
// that never adopted units of measure (cmd/seed-demo, direct API clients).
func TestUpdateProductWithoutUnitOfMeasureKeepsFreeTextUnit(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-f71-freetext"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	free := "pcs"
	p, err := d.CreateProduct(CreateProductRequest{
		OrganizationID: org.ID, Name: "Widget", SKU: ptr("WID-2"), Type: "product", Unit: &free,
	})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	updated, err := d.UpdateProduct(p.ID, UpdateProductRequest{
		Name: "Widget", SKU: ptr("WID-2"), Type: "product", Unit: &free,
	})
	if err != nil {
		t.Fatalf("UpdateProduct: %v", err)
	}
	if updated.Unit == nil || *updated.Unit != "pcs" {
		t.Fatalf("free-text unit lost: %v", updated.Unit)
	}
}

// --- F72: validation errors must reach the caller as validation errors ---

// UpdateProduct's user-reachable rejections must be *ValidationError, which
// is what api/products.go's writeMutationError maps to a 409 carrying the
// real message. The pre-existing test asserted only err != nil, so it would
// have passed while the handler returned a bare 500.
func TestUpdateProductRejectionsAreValidationErrors(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-f72"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	if _, err := d.CreateFiscalYear(CreateFiscalYearRequest{
		OrganizationID: org.ID, Name: "FY-wide", StartDate: 1690000000000, EndDate: 2000000000000,
	}); err != nil {
		t.Fatalf("CreateFiscalYear: %v", err)
	}
	p, err := d.CreateProduct(CreateProductRequest{
		OrganizationID: org.ID, Name: "Widget", SKU: ptr("WID-3"), Type: "product", StockEnabled: 1,
	})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}

	bogus := "not-a-category"
	_, err = d.UpdateProduct(p.ID, UpdateProductRequest{
		Name: "Widget", SKU: ptr("WID-3"), Type: "product", Category: &bogus,
	})
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("invalid category: expected *ValidationError, got %T: %v", err, err)
	}

	// Serialized toggle against non-zero stock.
	if _, err := d.CreateStockMovement(CreateStockMovementRequest{
		OrganizationID: org.ID, ProductID: p.ID, Type: "in", Quantity: 3, UnitCost: ptr(int64(100)),
	}); err != nil {
		t.Fatalf("CreateStockMovement: %v", err)
	}
	_, err = d.UpdateProduct(p.ID, UpdateProductRequest{
		Name: "Widget", SKU: ptr("WID-3"), Type: "product", StockEnabled: 1, Serialized: 1,
	})
	if !errors.As(err, &verr) {
		t.Fatalf("serialized toggle: expected *ValidationError, got %T: %v", err, err)
	}
}

// --- F73: production orders are a stock-moving path ----------------------

// A "finished" product with stock tracking switched off must not be able to
// start a production order — it would otherwise get real stockMovements and
// a non-zero stockQuantity for a product the rest of the app treats as
// outside inventory.
func TestCreateProductionOrderRejectsStockDisabledFinishedProduct(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-f73"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	finishedCat, componentCat := "finished", "component"
	component, err := d.CreateProduct(CreateProductRequest{
		OrganizationID: org.ID, Name: "Bolt", SKU: ptr("BLT-1"), Type: "product",
		StockEnabled: 1, Category: &componentCat,
	})
	if err != nil {
		t.Fatalf("CreateProduct(component): %v", err)
	}
	finished, err := d.CreateProduct(CreateProductRequest{
		OrganizationID: org.ID, Name: "Frame", SKU: ptr("FRM-1"), Type: "product",
		StockEnabled: 0, Category: &finishedCat,
	})
	if err != nil {
		t.Fatalf("CreateProduct(finished): %v", err)
	}
	if _, err := d.ReplaceBillOfMaterials(finished.ID, []CreateBillOfMaterialsLineRequest{
		{ComponentProductID: component.ID, QuantityPerUnit: 2},
	}, 1); err != nil {
		t.Fatalf("ReplaceBillOfMaterials: %v", err)
	}

	_, err = d.CreateProductionOrder(CreateProductionOrderRequest{
		OrganizationID: org.ID, OrderNumber: "PRO-0001", FinishedProductID: finished.ID,
		Quantity: 1, Date: 1700000000000,
	})
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected a *ValidationError for a stock-disabled finished product, got %T: %v", err, err)
	}
}

// --- F74: delete guards -------------------------------------------------

// Deleting a non-draft production order is refused, and the refusal for an
// already-cancelled order no longer tells the user to cancel it.
func TestDeleteProductionOrderRefusesNonDraftWithAccurateMessage(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	f := seedProductionOrderFixture(t, d, false)
	order, err := d.CreateProductionOrder(CreateProductionOrderRequest{
		OrganizationID: f.orgID, OrderNumber: "PRO-0001",
		FinishedProductID: f.finished.ID, Quantity: 1, Date: f.date,
	})
	if err != nil {
		t.Fatalf("CreateProductionOrder: %v", err)
	}

	if _, err := d.UpdateProductionOrderStatus(order.ID, "cancelled", nil); err != nil {
		t.Fatalf("UpdateProductionOrderStatus(cancelled): %v", err)
	}

	_, err = d.DeleteProductionOrder(order.ID)
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected a *ValidationError, got %T: %v", err, err)
	}
	if got := verr.Error(); got != "cannot delete a cancelled production order" {
		t.Fatalf("misleading refusal message: %q", got)
	}

	// And it really is still there.
	if _, err := d.GetProductionOrder(order.ID); err != nil {
		t.Fatalf("order was deleted despite the refusal: %v", err)
	}
}
