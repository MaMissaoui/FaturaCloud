package db

import (
	"errors"
	"testing"
)

// Regression tests for the 2026-09-14 audit's Phase 3 findings.

// --- F75: the component-line snapshot must be complete ------------------

// Deleting a component product must leave the whole snapshot readable, not
// just componentName. Before the fix componentSku/componentUnit were joined
// from products at read time, so they went NULL while the name survived.
func TestProductionOrderComponentLinesKeepSkuAndUnitAfterComponentDeleted(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-f75"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	finishedCat, componentCat := "finished", "component"
	unit := "pcs"
	component, err := d.CreateProduct(CreateProductRequest{
		OrganizationID: org.ID, Name: "Bolt", SKU: ptr("BLT-1"), Type: "product",
		StockEnabled: 1, Category: &componentCat, Unit: &unit,
	})
	if err != nil {
		t.Fatalf("CreateProduct(component): %v", err)
	}
	finished, err := d.CreateProduct(CreateProductRequest{
		OrganizationID: org.ID, Name: "Frame", SKU: ptr("FRM-1"), Type: "product",
		StockEnabled: 1, Category: &finishedCat,
	})
	if err != nil {
		t.Fatalf("CreateProduct(finished): %v", err)
	}
	if _, err := d.ReplaceBillOfMaterials(finished.ID, []CreateBillOfMaterialsLineRequest{
		{ComponentProductID: component.ID, QuantityPerUnit: 2},
	}, 1); err != nil {
		t.Fatalf("ReplaceBillOfMaterials: %v", err)
	}

	order, err := d.CreateProductionOrder(CreateProductionOrderRequest{
		OrganizationID: org.ID, OrderNumber: "PRO-0001",
		FinishedProductID: finished.ID, Quantity: 2, Date: 1700000000000,
	})
	if err != nil {
		t.Fatalf("CreateProductionOrder: %v", err)
	}

	before, err := d.GetProductionOrderComponentLines(order.ID)
	if err != nil || len(before) != 1 {
		t.Fatalf("GetProductionOrderComponentLines: err=%v len=%d", err, len(before))
	}
	if before[0].ComponentSKU == nil || *before[0].ComponentSKU != "BLT-1" {
		t.Fatalf("component SKU not snapshotted at creation: %v", before[0].ComponentSKU)
	}
	if before[0].ComponentUnit == nil || *before[0].ComponentUnit != "pcs" {
		t.Fatalf("component unit not snapshotted at creation: %v", before[0].ComponentUnit)
	}

	if _, err := d.DeleteProduct(component.ID); err != nil {
		t.Fatalf("DeleteProduct: %v", err)
	}

	after, err := d.GetProductionOrderComponentLines(order.ID)
	if err != nil || len(after) != 1 {
		t.Fatalf("GetProductionOrderComponentLines after delete: err=%v len=%d", err, len(after))
	}
	if after[0].ComponentName != "Bolt" {
		t.Fatalf("component name lost after deletion: %q", after[0].ComponentName)
	}
	if after[0].ComponentSKU == nil || *after[0].ComponentSKU != "BLT-1" {
		t.Fatalf("component SKU lost after the component was deleted: %v", after[0].ComponentSKU)
	}
	if after[0].ComponentUnit == nil || *after[0].ComponentUnit != "pcs" {
		t.Fatalf("component unit lost after the component was deleted: %v", after[0].ComponentUnit)
	}
}

// --- F76: inventory accounts are required deterministically -------------

// An organization with no Inventory account configured must be refused at
// completion regardless of whether the batch happens to produce a rounding
// residual. It used to be checked only when a residual existed, so the
// common exact-division case silently succeeded and one odd batch failed
// months later.
func TestCompleteProductionOrderRequiresInventoryAccountsEvenWithNoResidual(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	f := seedProductionOrderFixture(t, d, false)

	// Clear the inventory GL wiring the fixture's organization was seeded
	// with, the same state a user reaches by clearing the Accounting card.
	if _, err := d.DB.Exec(
		`UPDATE organizations SET defaultInventoryAccountId = NULL WHERE id = ?`, f.orgID,
	); err != nil {
		t.Fatalf("clear defaultInventoryAccountId: %v", err)
	}

	// Quantity 1 divides exactly, so this batch produces no residual at all
	// — precisely the case that used to skip the check.
	order, err := d.CreateProductionOrder(CreateProductionOrderRequest{
		OrganizationID: f.orgID, OrderNumber: "PRO-0001",
		FinishedProductID: f.finished.ID, Quantity: 1, Date: f.date,
	})
	if err != nil {
		t.Fatalf("CreateProductionOrder: %v", err)
	}

	_, err = d.UpdateProductionOrderStatus(order.ID, "completed", nil)
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected a *ValidationError for an unconfigured organization, got %T: %v", err, err)
	}
}

// --- F77: restore always records a version ------------------------------

// Restoring a version whose content equals the current recipe must still
// record the restore. It used to hit ReplaceBillOfMaterials' no-op branch
// and write nothing while reporting success.
func TestRestoreBillOfMaterialsVersionIdenticalToCurrentStillRecordsAVersion(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-f77"})
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
		StockEnabled: 1, Category: &finishedCat,
	})
	if err != nil {
		t.Fatalf("CreateProduct(finished): %v", err)
	}

	lines := []CreateBillOfMaterialsLineRequest{{ComponentProductID: component.ID, QuantityPerUnit: 2}}
	if _, err := d.ReplaceBillOfMaterials(finished.ID, lines, 1); err != nil {
		t.Fatalf("ReplaceBillOfMaterials: %v", err)
	}
	versions, err := d.GetBillOfMaterialsVersions(finished.ID)
	if err != nil || len(versions) != 1 {
		t.Fatalf("GetBillOfMaterialsVersions: err=%v len=%d", err, len(versions))
	}

	// Restoring v1 while v1 *is* the current recipe: a no-op content-wise,
	// but the restore itself is an event the history must record.
	if _, err := d.RestoreBillOfMaterialsVersion(finished.ID, versions[0].ID); err != nil {
		t.Fatalf("RestoreBillOfMaterialsVersion: %v", err)
	}
	after, err := d.GetBillOfMaterialsVersions(finished.ID)
	if err != nil {
		t.Fatalf("GetBillOfMaterialsVersions after: %v", err)
	}
	if len(after) != 2 {
		t.Fatalf("restore recorded no version: %d versions, want 2", len(after))
	}

	// An ordinary save of the same content must still be skipped as a no-op
	// — the forceVersion flag must not leak into the normal path.
	if _, err := d.ReplaceBillOfMaterials(finished.ID, lines, 1); err != nil {
		t.Fatalf("ReplaceBillOfMaterials (no-op): %v", err)
	}
	stillTwo, err := d.GetBillOfMaterialsVersions(finished.ID)
	if err != nil {
		t.Fatalf("GetBillOfMaterialsVersions: %v", err)
	}
	if len(stillTwo) != 2 {
		t.Fatalf("an unchanged save recorded a version: %d versions, want 2", len(stillTwo))
	}
}
