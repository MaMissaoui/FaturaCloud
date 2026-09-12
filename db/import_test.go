package db

import (
	"errors"
	"testing"
)

func TestImportCRUDAndNumbering(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-import-crud", Name: ptr("Import CRUD Org")})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}

	if n := d.NextImportNumber(org.ID); n != "IMP-0001" {
		t.Fatalf("NextImportNumber = %q, want IMP-0001", n)
	}

	imp, err := d.CreateImport(CreateImportRequest{
		OrganizationID: org.ID, ImportNumber: "IMP-0001", Date: 1738368000000,
		FreightCost: 50000, CustomsCost: 30000, Notes: ptr("First China container"),
	})
	if err != nil {
		t.Fatalf("CreateImport: %v", err)
	}
	if imp.FreightCost != 50000 || imp.CustomsCost != 30000 {
		t.Fatalf("freight/customs = %d/%d, want 50000/30000", imp.FreightCost, imp.CustomsCost)
	}

	if n := d.NextImportNumber(org.ID); n != "IMP-0002" {
		t.Fatalf("NextImportNumber after create = %q, want IMP-0002", n)
	}

	newFreight := 60000.0
	updated, err := d.UpdateImport(imp.ID, UpdateImportRequest{FreightCost: &newFreight})
	if err != nil {
		t.Fatalf("UpdateImport: %v", err)
	}
	if updated.FreightCost != 60000 {
		t.Fatalf("freight after update = %d, want 60000", updated.FreightCost)
	}
	if updated.CustomsCost != 30000 {
		t.Fatalf("customs after unrelated update = %d, want unchanged 30000", updated.CustomsCost)
	}
}

func TestImportSerialNumberRange(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-import-range", Name: ptr("Import Range Org")})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}

	// Start without end (and vice versa) is rejected.
	if _, err := d.CreateImport(CreateImportRequest{
		OrganizationID: org.ID, ImportNumber: "IMP-0001", Date: 1738368000000,
		SerialNumberRangeStart: ptr(int64(1001)),
	}); err == nil {
		t.Fatal("expected error for a range start with no end, got nil")
	}

	// End before start is rejected.
	if _, err := d.CreateImport(CreateImportRequest{
		OrganizationID: org.ID, ImportNumber: "IMP-0002", Date: 1738368000000,
		SerialNumberRangeStart: ptr(int64(1050)), SerialNumberRangeEnd: ptr(int64(1001)),
	}); err == nil {
		t.Fatal("expected error for a range end before its start, got nil")
	}

	// A valid range is stored and round-trips.
	imp, err := d.CreateImport(CreateImportRequest{
		OrganizationID: org.ID, ImportNumber: "IMP-0003", Date: 1738368000000,
		SerialNumberPrefix: ptr("SN-"), SerialNumberRangeStart: ptr(int64(1001)), SerialNumberRangeEnd: ptr(int64(1050)),
	})
	if err != nil {
		t.Fatalf("CreateImport with a valid range: %v", err)
	}
	if imp.SerialNumberPrefix == nil || *imp.SerialNumberPrefix != "SN-" {
		t.Errorf("SerialNumberPrefix = %v, want \"SN-\"", imp.SerialNumberPrefix)
	}
	if imp.SerialNumberRangeStart == nil || *imp.SerialNumberRangeStart != 1001 {
		t.Errorf("SerialNumberRangeStart = %v, want 1001", imp.SerialNumberRangeStart)
	}
	if imp.SerialNumberRangeEnd == nil || *imp.SerialNumberRangeEnd != 1050 {
		t.Errorf("SerialNumberRangeEnd = %v, want 1050", imp.SerialNumberRangeEnd)
	}

	// An unrelated field update (notes) doesn't disturb the stored range.
	updated, err := d.UpdateImport(imp.ID, UpdateImportRequest{Notes: ptr("unrelated update")})
	if err != nil {
		t.Fatalf("UpdateImport unrelated field: %v", err)
	}
	if updated.SerialNumberRangeStart == nil || *updated.SerialNumberRangeStart != 1001 {
		t.Errorf("SerialNumberRangeStart after unrelated update = %v, want unchanged 1001", updated.SerialNumberRangeStart)
	}

	// An update sending none of the three range fields is a true no-op for
	// them (merge-with-current, not a wipe) — the same guarantee the
	// "unrelated field update" case above already covers, exercised here
	// explicitly against an update with nothing at all set.
	untouched, err := d.UpdateImport(imp.ID, UpdateImportRequest{})
	if err != nil {
		t.Fatalf("UpdateImport with nothing set: %v", err)
	}
	if untouched.SerialNumberRangeStart == nil || *untouched.SerialNumberRangeStart != 1001 {
		t.Errorf("SerialNumberRangeStart after empty update = %v, want unchanged 1001", untouched.SerialNumberRangeStart)
	}
}

func TestDeleteImportBlockedWhilePOLinked(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGRNITestFixture(t, d, "org-import-delete-guard", 10, 250)
	imp, err := d.CreateImport(CreateImportRequest{
		OrganizationID: fx.orgID, ImportNumber: "IMP-0001", Date: fx.date,
	})
	if err != nil {
		t.Fatalf("CreateImport: %v", err)
	}
	importID := imp.ID
	if _, err := d.UpdatePurchaseOrder(fx.poID, UpdatePurchaseOrderRequest{ImportID: &importID}); err != nil {
		t.Fatalf("UpdatePurchaseOrder: %v", err)
	}

	if _, err := d.DeleteImport(imp.ID); !errors.Is(err, ErrImportInUse) {
		t.Fatalf("DeleteImport while linked = %v, want ErrImportInUse", err)
	}

	// UpdatePurchaseOrder writes importId directly (not COALESCE'd, since
	// it's an optional link a user must be able to clear) — a nil ImportID
	// unlinks the order.
	if _, err := d.UpdatePurchaseOrder(fx.poID, UpdatePurchaseOrderRequest{}); err != nil {
		t.Fatalf("UpdatePurchaseOrder unlink: %v", err)
	}
	po, err := d.GetPurchaseOrder(fx.poID)
	if err != nil {
		t.Fatalf("GetPurchaseOrder: %v", err)
	}
	if po.ImportID != nil {
		t.Fatalf("expected the PO to be unlinked, got importId = %v", *po.ImportID)
	}

	ok, err := d.DeleteImport(imp.ID)
	if err != nil {
		t.Fatalf("DeleteImport after unlink: %v", err)
	}
	if !ok {
		t.Fatal("expected DeleteImport to succeed once unlinked")
	}
}

func TestImportSummaryComputesLandedCostRate(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGRNITestFixture(t, d, "org-import-summary", 10, 250) // 10 * 250 = 2500 cents committed
	imp, err := d.CreateImport(CreateImportRequest{
		OrganizationID: fx.orgID, ImportNumber: "IMP-0001", Date: fx.date,
		FreightCost: 500, CustomsCost: 300, // 800 total
	})
	if err != nil {
		t.Fatalf("CreateImport: %v", err)
	}

	// Before any PO links to it: nothing committed, rate reads as 0 rather
	// than dividing by zero.
	summary, err := d.GetImportSummary(imp.ID)
	if err != nil {
		t.Fatalf("GetImportSummary (unlinked): %v", err)
	}
	if summary.TotalCommittedValue != 0 || summary.LandedCostRate != 0 || summary.PurchaseOrderCount != 0 {
		t.Fatalf("unlinked summary = %+v, want all zero", summary)
	}

	importID := imp.ID
	if _, err := d.UpdatePurchaseOrder(fx.poID, UpdatePurchaseOrderRequest{ImportID: &importID}); err != nil {
		t.Fatalf("UpdatePurchaseOrder: %v", err)
	}

	summary, err = d.GetImportSummary(imp.ID)
	if err != nil {
		t.Fatalf("GetImportSummary: %v", err)
	}
	if summary.TotalCommittedValue != 2500 {
		t.Fatalf("totalCommittedValue = %d, want 2500", summary.TotalCommittedValue)
	}
	if summary.PurchaseOrderCount != 1 {
		t.Fatalf("purchaseOrderCount = %d, want 1", summary.PurchaseOrderCount)
	}
	wantRate := 800.0 / 2500.0
	if diff := summary.LandedCostRate - wantRate; diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("landedCostRate = %v, want %v", summary.LandedCostRate, wantRate)
	}

	// A cancelled PO isn't committed spend — excluding it drops the
	// denominator back to zero and the rate back to 0.
	if _, err := d.UpdatePurchaseOrderStatus(fx.poID, "confirmed"); err != nil {
		t.Fatalf("UpdatePurchaseOrderStatus(confirmed): %v", err)
	}
	if _, err := d.UpdatePurchaseOrderStatus(fx.poID, "cancelled"); err != nil {
		t.Fatalf("UpdatePurchaseOrderStatus(cancelled): %v", err)
	}
	summary, err = d.GetImportSummary(imp.ID)
	if err != nil {
		t.Fatalf("GetImportSummary (cancelled PO): %v", err)
	}
	if summary.TotalCommittedValue != 0 || summary.LandedCostRate != 0 {
		t.Fatalf("summary after cancelling the only PO = %+v, want zeroed", summary)
	}
}

// TestGetImportSummariesMatchesPerImportSummary is the batch counterpart to
// TestImportSummaryComputesLandedCostRate (issue: "show linked purchase
// orders on the Imports list" — GetImportSummaries exists specifically so
// that page doesn't call GetImportSummary once per row). Two imports in the
// same organization — one with a linked, non-cancelled PO and one with
// nothing linked — must both appear in the batch map with values identical
// to what GetImportSummary computes for each individually.
func TestGetImportSummariesMatchesPerImportSummary(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGRNITestFixture(t, d, "org-import-summaries-batch", 10, 250) // 10 * 250 = 2500 cents committed

	linkedImport, err := d.CreateImport(CreateImportRequest{
		OrganizationID: fx.orgID, ImportNumber: "IMP-0001", Date: fx.date,
		FreightCost: 500, CustomsCost: 300, // 800 total
	})
	if err != nil {
		t.Fatalf("CreateImport (linked): %v", err)
	}
	unlinkedImport, err := d.CreateImport(CreateImportRequest{
		OrganizationID: fx.orgID, ImportNumber: "IMP-0002", Date: fx.date,
	})
	if err != nil {
		t.Fatalf("CreateImport (unlinked): %v", err)
	}

	importID := linkedImport.ID
	if _, err := d.UpdatePurchaseOrder(fx.poID, UpdatePurchaseOrderRequest{ImportID: &importID}); err != nil {
		t.Fatalf("UpdatePurchaseOrder: %v", err)
	}

	summaries, err := d.GetImportSummaries(fx.orgID)
	if err != nil {
		t.Fatalf("GetImportSummaries: %v", err)
	}
	if len(summaries) != 2 {
		t.Fatalf("len(summaries) = %d, want 2", len(summaries))
	}

	linked, ok := summaries[linkedImport.ID]
	if !ok {
		t.Fatalf("summaries missing linked import %s", linkedImport.ID)
	}
	wantLinked, err := d.GetImportSummary(linkedImport.ID)
	if err != nil {
		t.Fatalf("GetImportSummary (linked, for comparison): %v", err)
	}
	if linked != *wantLinked {
		t.Fatalf("batch summary for linked import = %+v, want %+v (matching GetImportSummary)", linked, *wantLinked)
	}
	if linked.TotalCommittedValue != 2500 || linked.PurchaseOrderCount != 1 {
		t.Fatalf("linked summary = %+v, want TotalCommittedValue=2500 PurchaseOrderCount=1", linked)
	}

	unlinked, ok := summaries[unlinkedImport.ID]
	if !ok {
		t.Fatalf("summaries missing unlinked import %s", unlinkedImport.ID)
	}
	if unlinked.TotalCommittedValue != 0 || unlinked.PurchaseOrderCount != 0 || unlinked.LandedCostRate != 0 {
		t.Fatalf("unlinked summary = %+v, want all zero", unlinked)
	}
}
