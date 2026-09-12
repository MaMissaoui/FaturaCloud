package db

import "testing"

func newBOMTestFixture(t *testing.T) (d *Database, orgID string, finished, componentA, componentB *Product) {
	t.Helper()
	d = newTestDB(t)

	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "bom-org"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	orgID = org.ID

	finishedCategory := "finished"
	finished, err = d.CreateProduct(CreateProductRequest{
		OrganizationID: orgID, Name: "Motorcycle", Type: "product", Price: 100000, Category: &finishedCategory,
	})
	if err != nil {
		t.Fatalf("CreateProduct finished: %v", err)
	}

	componentCategory := "component"
	componentA, err = d.CreateProduct(CreateProductRequest{
		OrganizationID: orgID, Name: "Engine block", Type: "product", Price: 20000, Category: &componentCategory,
	})
	if err != nil {
		t.Fatalf("CreateProduct componentA: %v", err)
	}
	componentB, err = d.CreateProduct(CreateProductRequest{
		OrganizationID: orgID, Name: "Frame chassis", Type: "product", Price: 15000, Category: &componentCategory,
	})
	if err != nil {
		t.Fatalf("CreateProduct componentB: %v", err)
	}
	return d, orgID, finished, componentA, componentB
}

func TestReplaceBillOfMaterialsHappyPath(t *testing.T) {
	t.Parallel()
	d, _, finished, componentA, componentB := newBOMTestFixture(t)

	lines, err := d.ReplaceBillOfMaterials(finished.ID, []CreateBillOfMaterialsLineRequest{
		{ComponentProductID: componentA.ID, QuantityPerUnit: 1},
		{ComponentProductID: componentB.ID, QuantityPerUnit: 2},
	})
	if err != nil {
		t.Fatalf("ReplaceBillOfMaterials: %v", err)
	}
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	if lines[0].ComponentProductID != componentA.ID || lines[0].QuantityPerUnit != 1 {
		t.Errorf("line 0 = %+v, want componentA qty 1", lines[0])
	}
	if lines[0].ComponentName != "Engine block" {
		t.Errorf("line 0 componentName = %q, want %q", lines[0].ComponentName, "Engine block")
	}
	if lines[1].ComponentProductID != componentB.ID || lines[1].QuantityPerUnit != 2 {
		t.Errorf("line 1 = %+v, want componentB qty 2", lines[1])
	}

	got, err := d.GetBillOfMaterials(finished.ID)
	if err != nil {
		t.Fatalf("GetBillOfMaterials: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("GetBillOfMaterials returned %d lines, want 2", len(got))
	}
}

func TestReplaceBillOfMaterialsWholesaleReplace(t *testing.T) {
	t.Parallel()
	d, _, finished, componentA, componentB := newBOMTestFixture(t)

	if _, err := d.ReplaceBillOfMaterials(finished.ID, []CreateBillOfMaterialsLineRequest{
		{ComponentProductID: componentA.ID, QuantityPerUnit: 1},
		{ComponentProductID: componentB.ID, QuantityPerUnit: 2},
	}); err != nil {
		t.Fatalf("first ReplaceBillOfMaterials: %v", err)
	}

	// Second call drops componentB entirely and changes componentA's
	// quantity — a real wholesale replace, not a merge/upsert.
	lines, err := d.ReplaceBillOfMaterials(finished.ID, []CreateBillOfMaterialsLineRequest{
		{ComponentProductID: componentA.ID, QuantityPerUnit: 5},
	})
	if err != nil {
		t.Fatalf("second ReplaceBillOfMaterials: %v", err)
	}
	if len(lines) != 1 {
		t.Fatalf("expected 1 line after replace, got %d", len(lines))
	}
	if lines[0].QuantityPerUnit != 5 {
		t.Errorf("quantityPerUnit = %v, want 5", lines[0].QuantityPerUnit)
	}
}

func TestReplaceBillOfMaterialsRejectsNonFinishedProduct(t *testing.T) {
	t.Parallel()
	d, _, _, componentA, componentB := newBOMTestFixture(t)

	// componentA is a "component", not "finished" — can't have its own BOM.
	if _, err := d.ReplaceBillOfMaterials(componentA.ID, []CreateBillOfMaterialsLineRequest{
		{ComponentProductID: componentB.ID, QuantityPerUnit: 1},
	}); err == nil {
		t.Fatal("expected error defining a BOM on a non-finished product, got nil")
	}
}

func TestReplaceBillOfMaterialsRejectsNonComponentLine(t *testing.T) {
	t.Parallel()
	d, orgID, finished, _, _ := newBOMTestFixture(t)

	// A plain service product (no category at all) can't be a BOM component.
	service, err := d.CreateProduct(CreateProductRequest{OrganizationID: orgID, Name: "Consulting", Type: "service", Price: 5000})
	if err != nil {
		t.Fatalf("CreateProduct service: %v", err)
	}

	if _, err := d.ReplaceBillOfMaterials(finished.ID, []CreateBillOfMaterialsLineRequest{
		{ComponentProductID: service.ID, QuantityPerUnit: 1},
	}); err == nil {
		t.Fatal("expected error using a non-component product as a BOM line, got nil")
	}
}

func TestReplaceBillOfMaterialsRejectsCrossOrgComponent(t *testing.T) {
	t.Parallel()
	d, _, finished, _, _ := newBOMTestFixture(t)

	otherOrg, err := d.CreateOrganization(CreateOrganizationRequest{ID: "other-org"})
	if err != nil {
		t.Fatalf("CreateOrganization other-org: %v", err)
	}
	componentCategory := "component"
	foreignComponent, err := d.CreateProduct(CreateProductRequest{
		OrganizationID: otherOrg.ID, Name: "Foreign part", Type: "product", Price: 1000, Category: &componentCategory,
	})
	if err != nil {
		t.Fatalf("CreateProduct foreignComponent: %v", err)
	}

	if _, err := d.ReplaceBillOfMaterials(finished.ID, []CreateBillOfMaterialsLineRequest{
		{ComponentProductID: foreignComponent.ID, QuantityPerUnit: 1},
	}); err == nil {
		t.Fatal("expected error referencing a component from a different organization, got nil")
	}
}

func TestReplaceBillOfMaterialsRejectsDuplicateOrInvalidQuantity(t *testing.T) {
	t.Parallel()
	d, _, finished, componentA, _ := newBOMTestFixture(t)

	if _, err := d.ReplaceBillOfMaterials(finished.ID, []CreateBillOfMaterialsLineRequest{
		{ComponentProductID: componentA.ID, QuantityPerUnit: 1},
		{ComponentProductID: componentA.ID, QuantityPerUnit: 2},
	}); err == nil {
		t.Fatal("expected error for a duplicate component within one bill of materials, got nil")
	}

	if _, err := d.ReplaceBillOfMaterials(finished.ID, []CreateBillOfMaterialsLineRequest{
		{ComponentProductID: componentA.ID, QuantityPerUnit: 0},
	}); err == nil {
		t.Fatal("expected error for a zero quantity per unit, got nil")
	}
}
