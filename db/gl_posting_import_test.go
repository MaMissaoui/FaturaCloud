package db

import (
	"errors"
	"testing"
)

// TestReceiptWithImportAllocatesLandedCostToInventory is F114's core
// correctness test: receiving against a PO linked to an import spreads that
// import's freight+customs across the received value, landing on Inventory
// (full landed value) / GRNI (vendor value only) / Import Costs Payable
// (the markup) — three legs of ONE posted entry, not two entries.
func TestReceiptWithImportAllocatesLandedCostToInventory(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGRNITestFixture(t, d, "org-import-landed-cost", 10, 250) // 10 * 250 = 2500 cents vendor value

	imp, err := d.CreateImport(CreateImportRequest{
		OrganizationID: fx.orgID, ImportNumber: "IMP-0001", Date: fx.date,
		FreightCost: 500, CustomsCost: 300, // 800 total -> rate = 800/2500 = 0.32
	})
	if err != nil {
		t.Fatalf("CreateImport: %v", err)
	}
	org, err := d.GetOrganization(fx.orgID)
	if err != nil {
		t.Fatalf("GetOrganization: %v", err)
	}
	if org.DefaultImportCostsPayableAccountID == nil {
		t.Fatal("expected Import Costs Payable to be auto-seeded for a new organization")
	}
	importCostsPayableAccountID := *org.DefaultImportCostsPayableAccountID

	importID := imp.ID
	if _, err := d.UpdatePurchaseOrder(fx.poID, UpdatePurchaseOrderRequest{ImportID: &importID}); err != nil {
		t.Fatalf("UpdatePurchaseOrder: %v", err)
	}

	receipt := fx.receive(t, d, "GR-0001", 10)

	entry, err := d.FindPostedEntryForSourceDocument("inbound_delivery", receipt.ID)
	if err != nil {
		t.Fatalf("FindPostedEntryForSourceDocument: %v", err)
	}
	if entry == nil {
		t.Fatal("expected a posted entry for the receipt, got none")
	}
	lines, err := d.GetJournalEntryLines(entry.ID)
	if err != nil {
		t.Fatalf("GetJournalEntryLines: %v", err)
	}
	if len(lines) != 3 {
		t.Fatalf("expected exactly 3 lines (Inventory/GRNI/Import Costs Payable), got %d: %+v", len(lines), lines)
	}

	// 250/unit vendor cost * 0.32 rate = 80/unit markup, landed = 330/unit.
	invDebit, invCredit := sumLines(lines, fx.inventoryAccountID)
	if invDebit != 3300 || invCredit != 0 {
		t.Fatalf("Inventory line = debit %d credit %d, want debit 3300 (2500 vendor + 800 markup) credit 0", invDebit, invCredit)
	}
	grniDebit, grniCredit := sumLines(lines, fx.grniAccountID)
	if grniDebit != 0 || grniCredit != 2500 {
		t.Fatalf("GRNI line = debit %d credit %d, want debit 0 credit 2500 (vendor value only)", grniDebit, grniCredit)
	}
	icpDebit, icpCredit := sumLines(lines, importCostsPayableAccountID)
	if icpDebit != 0 || icpCredit != 800 {
		t.Fatalf("Import Costs Payable line = debit %d credit %d, want debit 0 credit 800", icpDebit, icpCredit)
	}

	product, err := d.GetProduct(fx.productID)
	if err != nil {
		t.Fatalf("GetProduct: %v", err)
	}
	if product.UnitCost == nil || *product.UnitCost != 330 {
		t.Fatalf("product.UnitCost = %v, want 330 (landed cost, not the 250 vendor price)", product.UnitCost)
	}
}

// TestBillAfterImportReceiptClearsGRNIVendorOnlyLeavingMarkupUntouched is the
// downstream half of the landed-cost design: GRNI was deliberately valued at
// vendor-only cost so a matching vendor bill clears it exactly, the same as
// any non-import receipt (TestBillMatchingReceiptClearsGRNIWithNoNetInventoryLine).
// This checks that still holds with the markup in the picture — GRNI nets to
// zero, Inventory keeps the full landed value (no spurious price variance
// from the bill not knowing about the markup), and Import Costs Payable is
// left completely untouched, since nothing in this codebase settles it yet.
func TestBillAfterImportReceiptClearsGRNIVendorOnlyLeavingMarkupUntouched(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGRNITestFixture(t, d, "org-import-bill-clears-grni", 10, 250) // 10 * 250 = 2500 vendor value

	imp, err := d.CreateImport(CreateImportRequest{
		OrganizationID: fx.orgID, ImportNumber: "IMP-0001", Date: fx.date,
		FreightCost: 500, CustomsCost: 300, // 800 total -> rate 0.32, markup 800
	})
	if err != nil {
		t.Fatalf("CreateImport: %v", err)
	}
	org, err := d.GetOrganization(fx.orgID)
	if err != nil {
		t.Fatalf("GetOrganization: %v", err)
	}
	importCostsPayableAccountID := *org.DefaultImportCostsPayableAccountID

	importID := imp.ID
	if _, err := d.UpdatePurchaseOrder(fx.poID, UpdatePurchaseOrderRequest{ImportID: &importID}); err != nil {
		t.Fatalf("UpdatePurchaseOrder: %v", err)
	}
	fx.receive(t, d, "GR-0001", 10)

	// The vendor bill only ever charges for the goods (2500) — it has no
	// concept of the import's freight/customs, exactly like a real vendor
	// invoice never would.
	inv := fx.bill(t, d, "V-001", 10, 250, true)
	if _, err := d.UpdateIncomingInvoiceState(inv.ID, "approved"); err != nil {
		t.Fatalf("UpdateIncomingInvoiceState(approved): %v", err)
	}

	rows, err := d.GetTrialBalance(fx.orgID, "", "")
	if err != nil {
		t.Fatalf("GetTrialBalance: %v", err)
	}
	netByAccount := map[string]int64{}
	for _, r := range rows {
		netByAccount[r.AccountID] = r.Debit - r.Credit
	}

	if net := netByAccount[fx.grniAccountID]; net != 0 {
		t.Fatalf("GRNI net = %d, want 0 (fully cleared by the matching bill)", net)
	}
	if net := netByAccount[fx.inventoryAccountID]; net != 3300 {
		t.Fatalf("Inventory net = %d, want 3300 (2500 vendor + 800 markup, untouched by the bill)", net)
	}
	if net := netByAccount[importCostsPayableAccountID]; net != -800 {
		t.Fatalf("Import Costs Payable net = %d, want -800 (untouched credit balance — nothing settles it)", net)
	}
}

// TestReceiptWithImportRequiresImportCostsPayableAccount checks the same 409
// (not 500) discipline every Phase 7 default-account gap follows.
func TestReceiptWithImportRequiresImportCostsPayableAccount(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGRNITestFixture(t, d, "org-import-no-account", 10, 250)
	imp, err := d.CreateImport(CreateImportRequest{
		OrganizationID: fx.orgID, ImportNumber: "IMP-0001", Date: fx.date,
		FreightCost: 500, CustomsCost: 300,
	})
	if err != nil {
		t.Fatalf("CreateImport: %v", err)
	}
	importID := imp.ID
	if _, err := d.UpdatePurchaseOrder(fx.poID, UpdatePurchaseOrderRequest{ImportID: &importID}); err != nil {
		t.Fatalf("UpdatePurchaseOrder: %v", err)
	}
	if _, err := d.DB.Exec(
		`UPDATE organizations SET defaultImportCostsPayableAccountId = NULL WHERE id = ?`, fx.orgID,
	); err != nil {
		t.Fatalf("clear defaultImportCostsPayableAccountId: %v", err)
	}

	receipt, err := d.CreateInboundDelivery(CreateInboundDeliveryRequest{
		OrganizationID: fx.orgID, PurchaseOrderID: &fx.poID, VendorID: &fx.vendorID,
		DeliveryNumber: "GR-0001", DeliveryDate: fx.date,
		LineItems: []CreateInboundDeliveryLineItemRequest{
			{PurchaseOrderLineItemID: &fx.poLineID, Description: "Widget", Quantity: 10},
		},
	})
	if err != nil {
		t.Fatalf("CreateInboundDelivery: %v", err)
	}
	_, err = d.UpdateInboundDeliveryStatus(receipt.ID, "received", nil)
	if err == nil {
		t.Fatal("expected UpdateInboundDeliveryStatus to fail without a configured Import Costs Payable account")
	}
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected a *ValidationError, got %T: %v", err, err)
	}
}

// TestCancelReceiptWithImportReversesLandedCostEntry checks that cancelling
// finds and reverses the whole 3-legged entry as one unit — the payable
// credit doesn't dangle behind after GRNI is reversed.
func TestCancelReceiptWithImportReversesLandedCostEntry(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGRNITestFixture(t, d, "org-import-cancel", 10, 250)
	imp, err := d.CreateImport(CreateImportRequest{
		OrganizationID: fx.orgID, ImportNumber: "IMP-0001", Date: fx.date,
		FreightCost: 500, CustomsCost: 300,
	})
	if err != nil {
		t.Fatalf("CreateImport: %v", err)
	}
	importID := imp.ID
	if _, err := d.UpdatePurchaseOrder(fx.poID, UpdatePurchaseOrderRequest{ImportID: &importID}); err != nil {
		t.Fatalf("UpdatePurchaseOrder: %v", err)
	}
	receipt := fx.receive(t, d, "GR-0001", 10)

	if _, err := d.UpdateInboundDeliveryStatus(receipt.ID, "cancelled", nil); err != nil {
		t.Fatalf("UpdateInboundDeliveryStatus(cancelled): %v", err)
	}

	entry, err := d.FindPostedEntryForSourceDocument("inbound_delivery", receipt.ID)
	if err != nil {
		t.Fatalf("FindPostedEntryForSourceDocument: %v", err)
	}
	if entry != nil {
		t.Fatalf("expected no posted entry after cancel (only a reversed one), got %+v", entry)
	}

	product, err := d.GetProduct(fx.productID)
	if err != nil {
		t.Fatalf("GetProduct: %v", err)
	}
	if product.StockQuantity != 0 {
		t.Fatalf("stockQuantity after cancel = %v, want 0", product.StockQuantity)
	}

	org, err := d.GetOrganization(fx.orgID)
	if err != nil {
		t.Fatalf("GetOrganization: %v", err)
	}
	rows, err := d.GetTrialBalance(fx.orgID, "", "")
	if err != nil {
		t.Fatalf("GetTrialBalance: %v", err)
	}
	for _, r := range rows {
		if r.AccountID == *org.DefaultImportCostsPayableAccountID && r.Debit-r.Credit != 0 {
			t.Fatalf("Import Costs Payable net after cancel = %d, want 0 (fully reversed)", r.Debit-r.Credit)
		}
	}
}
