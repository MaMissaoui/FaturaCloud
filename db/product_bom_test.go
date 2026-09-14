package db

import (
	"math"
	"testing"
)

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
	}, 1)
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
	}, 1); err != nil {
		t.Fatalf("first ReplaceBillOfMaterials: %v", err)
	}

	// Second call drops componentB entirely and changes componentA's
	// quantity — a real wholesale replace, not a merge/upsert.
	lines, err := d.ReplaceBillOfMaterials(finished.ID, []CreateBillOfMaterialsLineRequest{
		{ComponentProductID: componentA.ID, QuantityPerUnit: 5},
	}, 1)
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
	}, 1); err == nil {
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
	}, 1); err == nil {
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
	}, 1); err == nil {
		t.Fatal("expected error referencing a component from a different organization, got nil")
	}
}

func TestGetBillOfMaterialsSummaries(t *testing.T) {
	t.Parallel()
	d, orgID, finished, componentA, componentB := newBOMTestFixture(t)

	// A second "finished" product with no BOM defined yet — proves it gets
	// no entry in the summary rather than a spurious zero-count row.
	finishedCategory := "finished"
	if _, err := d.CreateProduct(CreateProductRequest{
		OrganizationID: orgID, Name: "Scooter", Type: "product", Price: 50000, Category: &finishedCategory,
	}); err != nil {
		t.Fatalf("CreateProduct secondFinished: %v", err)
	}

	if _, err := d.ReplaceBillOfMaterials(finished.ID, []CreateBillOfMaterialsLineRequest{
		{ComponentProductID: componentA.ID, QuantityPerUnit: 1},
		{ComponentProductID: componentB.ID, QuantityPerUnit: 2},
	}, 1); err != nil {
		t.Fatalf("ReplaceBillOfMaterials finished: %v", err)
	}

	summaries, err := d.GetBillOfMaterialsSummaries(orgID)
	if err != nil {
		t.Fatalf("GetBillOfMaterialsSummaries: %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("expected 1 summary (secondFinished has no BOM), got %d: %+v", len(summaries), summaries)
	}
	if summaries[0].FinishedProductID != finished.ID || summaries[0].ComponentCount != 2 {
		t.Errorf("summary = %+v, want finishedProductId=%q componentCount=2", summaries[0], finished.ID)
	}
}

func TestReplaceBillOfMaterialsCreatesVersionHistory(t *testing.T) {
	t.Parallel()
	d, _, finished, componentA, componentB := newBOMTestFixture(t)

	if _, err := d.ReplaceBillOfMaterials(finished.ID, []CreateBillOfMaterialsLineRequest{
		{ComponentProductID: componentA.ID, QuantityPerUnit: 1},
	}, 1); err != nil {
		t.Fatalf("v1 ReplaceBillOfMaterials: %v", err)
	}
	if _, err := d.ReplaceBillOfMaterials(finished.ID, []CreateBillOfMaterialsLineRequest{
		{ComponentProductID: componentA.ID, QuantityPerUnit: 1},
		{ComponentProductID: componentB.ID, QuantityPerUnit: 2},
	}, 1); err != nil {
		t.Fatalf("v2 ReplaceBillOfMaterials: %v", err)
	}

	versions, err := d.GetBillOfMaterialsVersions(finished.ID)
	if err != nil {
		t.Fatalf("GetBillOfMaterialsVersions: %v", err)
	}
	if len(versions) != 2 {
		t.Fatalf("expected 2 versions, got %d: %+v", len(versions), versions)
	}
	// Newest first.
	if versions[0].VersionNumber != 2 || versions[0].ComponentCount != 2 {
		t.Errorf("versions[0] = %+v, want versionNumber=2 componentCount=2", versions[0])
	}
	if versions[1].VersionNumber != 1 || versions[1].ComponentCount != 1 {
		t.Errorf("versions[1] = %+v, want versionNumber=1 componentCount=1", versions[1])
	}

	detail, err := d.GetBillOfMaterialsVersionDetail(finished.ID, versions[1].ID)
	if err != nil {
		t.Fatalf("GetBillOfMaterialsVersionDetail v1: %v", err)
	}
	if len(detail.Lines) != 1 || detail.Lines[0].ComponentProductID == nil || *detail.Lines[0].ComponentProductID != componentA.ID {
		t.Errorf("v1 detail lines = %+v, want 1 line for componentA", detail.Lines)
	}
	if detail.Lines[0].ComponentName != "Engine block" {
		t.Errorf("v1 detail line componentName = %q, want %q", detail.Lines[0].ComponentName, "Engine block")
	}
}

func TestReplaceBillOfMaterialsSkipsNoOpVersion(t *testing.T) {
	t.Parallel()
	d, _, finished, componentA, _ := newBOMTestFixture(t)

	for range 3 {
		if _, err := d.ReplaceBillOfMaterials(finished.ID, []CreateBillOfMaterialsLineRequest{
			{ComponentProductID: componentA.ID, QuantityPerUnit: 1},
		}, 1); err != nil {
			t.Fatalf("ReplaceBillOfMaterials: %v", err)
		}
	}

	versions, err := d.GetBillOfMaterialsVersions(finished.ID)
	if err != nil {
		t.Fatalf("GetBillOfMaterialsVersions: %v", err)
	}
	if len(versions) != 1 {
		t.Fatalf("expected 1 version after 3 identical saves, got %d: %+v", len(versions), versions)
	}
}

func TestReplaceBillOfMaterialsBatchSizeDivision(t *testing.T) {
	t.Parallel()
	d, _, finished, componentA, _ := newBOMTestFixture(t)

	// A batch of 3 units needing 1 of componentA total -> 1/3 per unit,
	// rounded to 4 decimals rather than stored as an infinite fraction.
	lines, err := d.ReplaceBillOfMaterials(finished.ID, []CreateBillOfMaterialsLineRequest{
		{ComponentProductID: componentA.ID, QuantityPerUnit: 1.0 / 3.0},
	}, 3)
	if err != nil {
		t.Fatalf("ReplaceBillOfMaterials: %v", err)
	}
	if lines[0].QuantityPerUnit != 0.3333 {
		t.Errorf("quantityPerUnit = %v, want 0.3333 (rounded)", lines[0].QuantityPerUnit)
	}

	versions, err := d.GetBillOfMaterialsVersions(finished.ID)
	if err != nil {
		t.Fatalf("GetBillOfMaterialsVersions: %v", err)
	}
	if len(versions) != 1 || versions[0].BatchSize != 3 {
		t.Fatalf("versions = %+v, want 1 version with batchSize=3", versions)
	}

	// The frontend redisplays this stored 0.3333 by multiplying back by the
	// batch size (its own roundDisplay(quantityPerUnit * batchSize)) — that
	// lands on 0.9999, not exactly 1, an inherent, arithmetically honest
	// consequence of storing only 4 decimal places (not something this
	// round trip can be made exact without). What matters is that it
	// doesn't compound: re-saving that redisplayed value (divided back down
	// through the identical roundBOMQuantity rounding) must settle back on
	// the same 0.3333 and stay a no-op for history, not drift further on
	// every open-then-save cycle.
	redisplayed := math.Round(lines[0].QuantityPerUnit*3*10000) / 10000
	if redisplayed != 0.9999 {
		t.Fatalf("redisplayed = %v, want 0.9999 (documenting the inherent rounding gap)", redisplayed)
	}
	resaved, err := d.ReplaceBillOfMaterials(finished.ID, []CreateBillOfMaterialsLineRequest{
		{ComponentProductID: componentA.ID, QuantityPerUnit: redisplayed / 3},
	}, 3)
	if err != nil {
		t.Fatalf("re-save ReplaceBillOfMaterials: %v", err)
	}
	if resaved[0].QuantityPerUnit != 0.3333 {
		t.Errorf("resaved quantityPerUnit = %v, want 0.3333 (settled, not drifted)", resaved[0].QuantityPerUnit)
	}
	versionsAfterResave, err := d.GetBillOfMaterialsVersions(finished.ID)
	if err != nil {
		t.Fatalf("GetBillOfMaterialsVersions after resave: %v", err)
	}
	if len(versionsAfterResave) != 1 {
		t.Fatalf("versions after resave = %+v, want still 1 (settled value is a no-op)", versionsAfterResave)
	}
}

// TestReplaceBillOfMaterialsRecordsChangeAfterComponentDeletion guards
// against comparing a no-op save against the live bill_of_materials table
// instead of the latest version's own (denormalized) lines. Deleting a
// component cascades it out of the live table (ON DELETE CASCADE) but the
// historical version line survives with a nil componentProductId (ON DELETE
// SET NULL) — so after the deletion, live and the latest version
// legitimately disagree, and a save matching live must still be recorded as
// a real change relative to version history.
func TestReplaceBillOfMaterialsRecordsChangeAfterComponentDeletion(t *testing.T) {
	t.Parallel()
	d, _, finished, componentA, componentB := newBOMTestFixture(t)

	if _, err := d.ReplaceBillOfMaterials(finished.ID, []CreateBillOfMaterialsLineRequest{
		{ComponentProductID: componentA.ID, QuantityPerUnit: 1},
		{ComponentProductID: componentB.ID, QuantityPerUnit: 2},
	}, 1); err != nil {
		t.Fatalf("v1 ReplaceBillOfMaterials: %v", err)
	}

	if ok, err := d.DeleteProduct(componentB.ID); err != nil || !ok {
		t.Fatalf("DeleteProduct componentB: ok=%v err=%v", ok, err)
	}

	// Live bill_of_materials now only has componentA (componentB's row was
	// cascade-deleted) — saving exactly that must still create v2, not be
	// treated as a no-op against v1's now-divergent live mirror.
	if _, err := d.ReplaceBillOfMaterials(finished.ID, []CreateBillOfMaterialsLineRequest{
		{ComponentProductID: componentA.ID, QuantityPerUnit: 1},
	}, 1); err != nil {
		t.Fatalf("v2 ReplaceBillOfMaterials: %v", err)
	}

	versions, err := d.GetBillOfMaterialsVersions(finished.ID)
	if err != nil {
		t.Fatalf("GetBillOfMaterialsVersions: %v", err)
	}
	if len(versions) != 2 {
		t.Fatalf("expected 2 versions (component loss must be recorded as a change), got %d: %+v", len(versions), versions)
	}
}

// TestReplaceBillOfMaterialsInheritsBatchSizeWhenOmitted guards a caller
// with no batch-size concept of its own (the product form's embedded BOM
// card, which always sends batchSize<=0 — see ReplaceProductBOM's frontend
// comment) from silently resetting the latest version's batchSize to 1 and
// padding history with a no-content-change version every time it saves.
func TestReplaceBillOfMaterialsInheritsBatchSizeWhenOmitted(t *testing.T) {
	t.Parallel()
	d, _, finished, componentA, _ := newBOMTestFixture(t)

	if _, err := d.ReplaceBillOfMaterials(finished.ID, []CreateBillOfMaterialsLineRequest{
		{ComponentProductID: componentA.ID, QuantityPerUnit: 1.0 / 3.0},
	}, 3); err != nil {
		t.Fatalf("v1 (batchSize=3) ReplaceBillOfMaterials: %v", err)
	}

	// A caller that omits batchSize (0, the Go zero value for a field the
	// client never sent) must inherit 3, not reset to 1 — and since nothing
	// else changed either, this save must be a true no-op for history.
	lines, err := d.ReplaceBillOfMaterials(finished.ID, []CreateBillOfMaterialsLineRequest{
		{ComponentProductID: componentA.ID, QuantityPerUnit: 1.0 / 3.0},
	}, 0)
	if err != nil {
		t.Fatalf("v1 replay (batchSize=0) ReplaceBillOfMaterials: %v", err)
	}
	if lines[0].QuantityPerUnit != 0.3333 {
		t.Errorf("quantityPerUnit = %v, want 0.3333 unchanged", lines[0].QuantityPerUnit)
	}

	versions, err := d.GetBillOfMaterialsVersions(finished.ID)
	if err != nil {
		t.Fatalf("GetBillOfMaterialsVersions: %v", err)
	}
	if len(versions) != 1 || versions[0].BatchSize != 3 {
		t.Fatalf("versions = %+v, want exactly 1 version with batchSize=3 (no spurious version, no reset)", versions)
	}
}

func TestRestoreBillOfMaterialsVersion(t *testing.T) {
	t.Parallel()
	d, _, finished, componentA, componentB := newBOMTestFixture(t)

	if _, err := d.ReplaceBillOfMaterials(finished.ID, []CreateBillOfMaterialsLineRequest{
		{ComponentProductID: componentA.ID, QuantityPerUnit: 1},
	}, 1); err != nil {
		t.Fatalf("v1 ReplaceBillOfMaterials: %v", err)
	}
	if _, err := d.ReplaceBillOfMaterials(finished.ID, []CreateBillOfMaterialsLineRequest{
		{ComponentProductID: componentB.ID, QuantityPerUnit: 2},
	}, 1); err != nil {
		t.Fatalf("v2 ReplaceBillOfMaterials: %v", err)
	}

	versionsBefore, err := d.GetBillOfMaterialsVersions(finished.ID)
	if err != nil {
		t.Fatalf("GetBillOfMaterialsVersions: %v", err)
	}
	v1ID := versionsBefore[len(versionsBefore)-1].ID

	restored, err := d.RestoreBillOfMaterialsVersion(finished.ID, v1ID)
	if err != nil {
		t.Fatalf("RestoreBillOfMaterialsVersion: %v", err)
	}
	if len(restored) != 1 || restored[0].ComponentProductID != componentA.ID {
		t.Fatalf("restored current recipe = %+v, want 1 line for componentA", restored)
	}

	// Restoring is non-destructive — it adds a new v3 matching v1's content
	// rather than deleting v2 or rewriting v1.
	versionsAfter, err := d.GetBillOfMaterialsVersions(finished.ID)
	if err != nil {
		t.Fatalf("GetBillOfMaterialsVersions after restore: %v", err)
	}
	if len(versionsAfter) != 3 {
		t.Fatalf("expected 3 versions after restore, got %d: %+v", len(versionsAfter), versionsAfter)
	}
	if versionsAfter[0].VersionNumber != 3 {
		t.Errorf("newest version = %+v, want versionNumber=3", versionsAfter[0])
	}
}

func TestRestoreBillOfMaterialsVersionRejectsDeletedComponent(t *testing.T) {
	t.Parallel()
	d, orgID, finished, _, _ := newBOMTestFixture(t)

	componentCategory := "component"
	doomed, err := d.CreateProduct(CreateProductRequest{
		OrganizationID: orgID, Name: "Doomed part", Type: "product", Price: 500, Category: &componentCategory,
	})
	if err != nil {
		t.Fatalf("CreateProduct doomed: %v", err)
	}
	if _, err := d.ReplaceBillOfMaterials(finished.ID, []CreateBillOfMaterialsLineRequest{
		{ComponentProductID: doomed.ID, QuantityPerUnit: 1},
	}, 1); err != nil {
		t.Fatalf("v1 ReplaceBillOfMaterials: %v", err)
	}

	versions, err := d.GetBillOfMaterialsVersions(finished.ID)
	if err != nil {
		t.Fatalf("GetBillOfMaterialsVersions: %v", err)
	}
	v1ID := versions[0].ID

	if ok, err := d.DeleteProduct(doomed.ID); err != nil || !ok {
		t.Fatalf("DeleteProduct doomed: ok=%v err=%v", ok, err)
	}

	if _, err := d.RestoreBillOfMaterialsVersion(finished.ID, v1ID); err == nil {
		t.Fatal("expected error restoring a version whose component was deleted, got nil")
	}
}

func TestGetBillOfMaterialsVersionDetailRejectsMismatchedProduct(t *testing.T) {
	t.Parallel()
	d, orgID, finished, componentA, _ := newBOMTestFixture(t)

	if _, err := d.ReplaceBillOfMaterials(finished.ID, []CreateBillOfMaterialsLineRequest{
		{ComponentProductID: componentA.ID, QuantityPerUnit: 1},
	}, 1); err != nil {
		t.Fatalf("ReplaceBillOfMaterials: %v", err)
	}
	versions, err := d.GetBillOfMaterialsVersions(finished.ID)
	if err != nil {
		t.Fatalf("GetBillOfMaterialsVersions: %v", err)
	}

	finishedCategory := "finished"
	otherFinished, err := d.CreateProduct(CreateProductRequest{
		OrganizationID: orgID, Name: "Scooter", Type: "product", Price: 50000, Category: &finishedCategory,
	})
	if err != nil {
		t.Fatalf("CreateProduct otherFinished: %v", err)
	}

	if _, err := d.GetBillOfMaterialsVersionDetail(otherFinished.ID, versions[0].ID); err == nil {
		t.Fatal("expected error fetching a version under the wrong finished product, got nil")
	}
}

func TestReplaceBillOfMaterialsRejectsDuplicateOrInvalidQuantity(t *testing.T) {
	t.Parallel()
	d, _, finished, componentA, _ := newBOMTestFixture(t)

	if _, err := d.ReplaceBillOfMaterials(finished.ID, []CreateBillOfMaterialsLineRequest{
		{ComponentProductID: componentA.ID, QuantityPerUnit: 1},
		{ComponentProductID: componentA.ID, QuantityPerUnit: 2},
	}, 1); err == nil {
		t.Fatal("expected error for a duplicate component within one bill of materials, got nil")
	}

	if _, err := d.ReplaceBillOfMaterials(finished.ID, []CreateBillOfMaterialsLineRequest{
		{ComponentProductID: componentA.ID, QuantityPerUnit: 0},
	}, 1); err == nil {
		t.Fatal("expected error for a zero quantity per unit, got nil")
	}
}
