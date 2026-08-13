package db

import (
	"errors"
	"strings"
	"testing"
)

// grniTestFixture is a stock-enabled counterpart to glPostingTestFixture —
// an organization, vendor, and stock-tracked product with a purchase order
// already placed, so a test can receive against it and exercise Phase 7's
// GRNI accrual.
type grniTestFixture struct {
	orgID                        string
	vendorID                     string
	productID                    string
	poID                         string
	poLineID                     string
	inventoryAccountID           string
	grniAccountID                string
	cogsAccountID                string
	apAccountID                  string
	inventoryAdjustmentAccountID string
	date                         int64
}

func newGRNITestFixture(t *testing.T, d *Database, orgID string, ordered float64, unitPrice int64) grniTestFixture {
	t.Helper()

	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: orgID, Name: ptr("GRNI Test Org")})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	date := int64(1738368000000) // 2025-02-01 in ms, matches newGLPostingTestFixture
	if _, err := d.CreateFiscalYear(CreateFiscalYearRequest{
		OrganizationID: org.ID, Name: "2025", StartDate: 1735689600000, EndDate: 1767225599000,
	}); err != nil {
		t.Fatalf("CreateFiscalYear: %v", err)
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
		OrganizationID: org.ID, VendorID: &vendor.ID, OrderNumber: "PO-0001", OrderDate: date,
		LineItems: []CreatePurchaseOrderLineItemRequest{
			{ProductID: &product.ID, Description: "Widget", Quantity: ordered, UnitPrice: float64(unitPrice)},
		},
	})
	if err != nil {
		t.Fatalf("CreatePurchaseOrder: %v", err)
	}
	poItems, err := d.GetPurchaseOrderLineItems(po.ID)
	if err != nil || len(poItems) != 1 {
		t.Fatalf("GetPurchaseOrderLineItems: %v, %+v", err, poItems)
	}

	return grniTestFixture{
		orgID:                        org.ID,
		vendorID:                     vendor.ID,
		productID:                    product.ID,
		poID:                         po.ID,
		poLineID:                     poItems[0].ID,
		inventoryAccountID:           *org.DefaultInventoryAccountID,
		grniAccountID:                *org.DefaultGRNIAccountID,
		cogsAccountID:                *org.DefaultCOGSAccountID,
		apAccountID:                  *org.DefaultApAccountID,
		inventoryAdjustmentAccountID: *org.DefaultInventoryAdjustmentAccountID,
		date:                         date,
	}
}

// ship creates and ships a standalone (no order) delivery for qty units of
// the fixture's product.
func (f grniTestFixture) ship(t *testing.T, d *Database, deliveryNumber string, qty float64) *OutboundDelivery {
	t.Helper()
	delivery, err := d.CreateDelivery(CreateDeliveryRequest{
		OrganizationID: f.orgID, DeliveryNumber: deliveryNumber, DeliveryDate: f.date,
		LineItems: []CreateDeliveryLineItemRequest{
			{ProductID: &f.productID, Description: "Widget", Quantity: qty},
		},
	})
	if err != nil {
		t.Fatalf("CreateDelivery: %v", err)
	}
	updated, err := d.UpdateDeliveryStatus(delivery.ID, "shipped", nil)
	if err != nil {
		t.Fatalf("UpdateDeliveryStatus(shipped): %v", err)
	}
	return updated
}

// bill creates a vendor bill for qty units at unitPrice, linked to the
// fixture's purchase order line (poLineID != nil) or standalone
// (poLineID == nil, used to test the no-PO-link capitalization path).
func (f grniTestFixture) bill(t *testing.T, d *Database, number string, qty float64, unitPrice int64, linkPO bool) *IncomingInvoice {
	t.Helper()
	subTotal := int64(qty * float64(unitPrice))
	item := CreateInvoiceLineItemRequest{
		Description: ptr("Widget"), Quantity: qty, UnitPrice: float64(unitPrice), ProductID: &f.productID,
	}
	if linkPO {
		item.PurchaseOrderLineItemID = &f.poLineID
	}
	req := CreateIncomingInvoiceRequest{
		OrganizationID: f.orgID, VendorID: f.vendorID, VendorInvoiceNumber: number,
		Date: f.date, Currency: "EUR", SubTotal: subTotal, TaxTotal: 0, Total: subTotal,
		LineItems: []CreateInvoiceLineItemRequest{item},
	}
	if linkPO {
		req.PurchaseOrderID = &f.poID
	}
	inv, err := d.CreateIncomingInvoice(req)
	if err != nil {
		t.Fatalf("CreateIncomingInvoice: %v", err)
	}
	return inv
}

// receive creates and receives a goods receipt against the fixture's
// purchase order line, resolving product+cost from the PO the same way
// production code does when a receipt line names neither directly.
func (f grniTestFixture) receive(t *testing.T, d *Database, deliveryNumber string, qty float64) *InboundDelivery {
	t.Helper()
	receipt, err := d.CreateInboundDelivery(CreateInboundDeliveryRequest{
		OrganizationID: f.orgID, PurchaseOrderID: &f.poID, VendorID: &f.vendorID,
		DeliveryNumber: deliveryNumber, DeliveryDate: f.date,
		LineItems: []CreateInboundDeliveryLineItemRequest{
			{PurchaseOrderLineItemID: &f.poLineID, Description: "Widget", Quantity: qty},
		},
	})
	if err != nil {
		t.Fatalf("CreateInboundDelivery: %v", err)
	}
	updated, err := d.UpdateInboundDeliveryStatus(receipt.ID, "received", nil)
	if err != nil {
		t.Fatalf("UpdateInboundDeliveryStatus(received): %v", err)
	}
	return updated
}

func TestGoodsReceiptPostsGRNIAccrual(t *testing.T) {
	d := newTestDB(t)
	fx := newGRNITestFixture(t, d, "org-grni-accrue", 10, 250) // 10 units @ 2.50 = 2500 cents
	receipt := fx.receive(t, d, "GR-0001", 10)

	entry, err := d.FindPostedEntryForSourceDocument("inbound_delivery", receipt.ID)
	if err != nil {
		t.Fatalf("FindPostedEntryForSourceDocument: %v", err)
	}
	if entry == nil {
		t.Fatal("expected a posted GRNI entry for the receipt, got none")
	}
	if entry.Status != "posted" {
		t.Fatalf("entry status = %q, want posted", entry.Status)
	}

	lines, err := d.GetJournalEntryLines(entry.ID)
	if err != nil {
		t.Fatalf("GetJournalEntryLines: %v", err)
	}
	invDebit, invCredit := sumLines(lines, fx.inventoryAccountID)
	if invDebit != 2500 || invCredit != 0 {
		t.Fatalf("Inventory line = debit %d credit %d, want debit 2500 credit 0", invDebit, invCredit)
	}
	grniDebit, grniCredit := sumLines(lines, fx.grniAccountID)
	if grniDebit != 0 || grniCredit != 2500 {
		t.Fatalf("GRNI line = debit %d credit %d, want debit 0 credit 2500", grniDebit, grniCredit)
	}

	rows, err := d.GetTrialBalance(fx.orgID, "")
	if err != nil {
		t.Fatalf("GetTrialBalance: %v", err)
	}
	for _, r := range rows {
		if r.AccountID == fx.inventoryAccountID && r.Debit-r.Credit != 2500 {
			t.Fatalf("trial balance Inventory net = %d, want 2500", r.Debit-r.Credit)
		}
	}
}

// A receipt with no resolvable cost (no PO link, no unitCost, product has
// none typed either) accrues nothing — an uncosted inflow stays out of the
// valuation pool, the same rule recomputeAverageCostTx already applies, not
// a 409. Standalone (no purchase order at all).
func TestUncostedGoodsReceiptPostsNoGRNIEntry(t *testing.T) {
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-grni-uncosted", Name: ptr("Uncosted Org")})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	date := int64(1738368000000)
	if _, err := d.CreateFiscalYear(CreateFiscalYearRequest{
		OrganizationID: org.ID, Name: "2025", StartDate: 1735689600000, EndDate: 1767225599000,
	}); err != nil {
		t.Fatalf("CreateFiscalYear: %v", err)
	}
	product, err := d.CreateProduct(CreateProductRequest{
		OrganizationID: org.ID, Name: "Widget", SKU: ptr("WID-1"), Type: "product", StockEnabled: 1,
	})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	receipt, err := d.CreateInboundDelivery(CreateInboundDeliveryRequest{
		OrganizationID: org.ID, DeliveryNumber: "GR-0001", DeliveryDate: date,
		LineItems: []CreateInboundDeliveryLineItemRequest{
			{ProductID: &product.ID, Description: "Widget", Quantity: 5}, // no UnitCost
		},
	})
	if err != nil {
		t.Fatalf("CreateInboundDelivery: %v", err)
	}
	received, err := d.UpdateInboundDeliveryStatus(receipt.ID, "received", nil)
	if err != nil {
		t.Fatalf("UpdateInboundDeliveryStatus(received): %v", err)
	}

	entry, err := d.FindPostedEntryForSourceDocument("inbound_delivery", received.ID)
	if err != nil {
		t.Fatalf("FindPostedEntryForSourceDocument: %v", err)
	}
	if entry != nil {
		t.Fatalf("expected no GRNI entry for an uncosted receipt, got %+v", entry)
	}

	p, err := d.GetProduct(product.ID)
	if err != nil {
		t.Fatalf("GetProduct: %v", err)
	}
	if p.StockQuantity != 5 {
		t.Fatalf("stockQuantity: got %v, want 5 — stock still moves even though nothing accrued", p.StockQuantity)
	}
}

func TestCancelReceivedReceiptReversesGRNIAccrual(t *testing.T) {
	d := newTestDB(t)
	fx := newGRNITestFixture(t, d, "org-grni-cancel", 10, 250)
	receipt := fx.receive(t, d, "GR-0001", 10)

	original, err := d.FindPostedEntryForSourceDocument("inbound_delivery", receipt.ID)
	if err != nil || original == nil {
		t.Fatalf("expected a posted GRNI entry, err=%v entry=%v", err, original)
	}

	if _, err := d.UpdateInboundDeliveryStatus(receipt.ID, "cancelled", nil); err != nil {
		t.Fatalf("UpdateInboundDeliveryStatus(cancelled): %v", err)
	}

	stillLive, err := d.FindPostedEntryForSourceDocument("inbound_delivery", receipt.ID)
	if err != nil {
		t.Fatalf("FindPostedEntryForSourceDocument: %v", err)
	}
	if stillLive != nil {
		t.Fatalf("expected no live posted entry after cancelling, got %+v", stillLive)
	}
	reversedOriginal, err := d.GetJournalEntry(original.ID)
	if err != nil {
		t.Fatalf("GetJournalEntry(original): %v", err)
	}
	if reversedOriginal.Status != "reversed" {
		t.Fatalf("original entry status = %q, want reversed", reversedOriginal.Status)
	}

	rows, err := d.GetTrialBalance(fx.orgID, "")
	if err != nil {
		t.Fatalf("GetTrialBalance: %v", err)
	}
	for _, r := range rows {
		if r.AccountID == fx.inventoryAccountID && r.Debit-r.Credit != 0 {
			t.Fatalf("trial balance Inventory net after reversal = %d, want 0", r.Debit-r.Credit)
		}
	}
}

// Cancelling an uncosted receipt (no GRNI entry ever posted) is a clean
// no-op on the GL side — FindPostedEntryForSourceDocument returns nil, and
// UpdateInboundDeliveryStatus must not treat that as an error.
func TestCancelUncostedReceivedReceiptIsCleanNoOp(t *testing.T) {
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-grni-cancel-uncosted", Name: ptr("Uncosted Cancel Org")})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	date := int64(1738368000000)
	if _, err := d.CreateFiscalYear(CreateFiscalYearRequest{
		OrganizationID: org.ID, Name: "2025", StartDate: 1735689600000, EndDate: 1767225599000,
	}); err != nil {
		t.Fatalf("CreateFiscalYear: %v", err)
	}
	product, err := d.CreateProduct(CreateProductRequest{
		OrganizationID: org.ID, Name: "Widget", SKU: ptr("WID-1"), Type: "product", StockEnabled: 1,
	})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	receipt, err := d.CreateInboundDelivery(CreateInboundDeliveryRequest{
		OrganizationID: org.ID, DeliveryNumber: "GR-0001", DeliveryDate: date,
		LineItems: []CreateInboundDeliveryLineItemRequest{
			{ProductID: &product.ID, Description: "Widget", Quantity: 5},
		},
	})
	if err != nil {
		t.Fatalf("CreateInboundDelivery: %v", err)
	}
	if _, err := d.UpdateInboundDeliveryStatus(receipt.ID, "received", nil); err != nil {
		t.Fatalf("UpdateInboundDeliveryStatus(received): %v", err)
	}
	if _, err := d.UpdateInboundDeliveryStatus(receipt.ID, "cancelled", nil); err != nil {
		t.Fatalf("UpdateInboundDeliveryStatus(cancelled): %v", err)
	}
	p, err := d.GetProduct(product.ID)
	if err != nil {
		t.Fatalf("GetProduct: %v", err)
	}
	if p.StockQuantity != 0 {
		t.Fatalf("stockQuantity after cancel: got %v, want 0", p.StockQuantity)
	}
}

// TestCancelReceivedReceiptBlockedOnceBilled is the regression test for
// Finding B: cancelling a received receipt after its GRNI accrual has
// already been billed (state approved/paid) must be refused, or the
// receipt's reversal would leave GRNI permanently unbalanced while the
// bill's AP obligation still stands. Uses seedMatch/createIncomingInvoice
// (db_test.go) — the existing 3-way-match fixture, which already receives
// via UpdateInboundDeliveryStatus, so GRNI accrual already applies to it.
func TestCancelReceivedReceiptBlockedOnceBilled(t *testing.T) {
	d := newTestDB(t)
	f := seedMatch(t, d, "org-grni-finding-b", 10, 250, 10)
	inv := createIncomingInvoice(t, d, f, "V-001", 10, 250)
	if _, err := d.UpdateIncomingInvoiceState(inv.ID, "approved"); err != nil {
		t.Fatalf("UpdateIncomingInvoiceState(approved): %v", err)
	}

	receipts, err := d.GetInboundDeliveries(f.OrgID)
	if err != nil || len(receipts) != 1 {
		t.Fatalf("GetInboundDeliveries: %v, %+v", err, receipts)
	}

	_, err = d.UpdateInboundDeliveryStatus(receipts[0].ID, "cancelled", nil)
	if err == nil {
		t.Fatal("expected cancelling a receipt already billed against to be rejected")
	}
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected a *ValidationError, got %T: %v", err, err)
	}

	current, err := d.GetInboundDelivery(receipts[0].ID)
	if err != nil {
		t.Fatalf("GetInboundDelivery: %v", err)
	}
	if current.Status != "received" {
		t.Fatalf("status changed despite rejection: %q", current.Status)
	}
}

// A stock-enabled product's bill line with no purchase order link (nothing
// to clear against) capitalizes straight to Inventory — the behavior
// change to existing Phase 2 code: previously every bill line expensed
// immediately regardless of whether the product was stock-tracked.
func TestBillForStockEnabledProductWithNoPOLinkCapitalizesToInventory(t *testing.T) {
	d := newTestDB(t)
	fx := newGRNITestFixture(t, d, "org-grni-nolink", 10, 250)
	inv := fx.bill(t, d, "V-001", 4, 300, false) // no PO link, priced independently

	if _, err := d.UpdateIncomingInvoiceState(inv.ID, "approved"); err != nil {
		t.Fatalf("UpdateIncomingInvoiceState(approved): %v", err)
	}

	entry, err := d.FindPostedEntryForSourceDocument("incoming_invoice", inv.ID)
	if err != nil || entry == nil {
		t.Fatalf("expected a posted entry, err=%v entry=%v", err, entry)
	}
	lines, err := d.GetJournalEntryLines(entry.ID)
	if err != nil {
		t.Fatalf("GetJournalEntryLines: %v", err)
	}
	invDebit, invCredit := sumLines(lines, fx.inventoryAccountID)
	if invDebit != 1200 || invCredit != 0 {
		t.Fatalf("Inventory line = debit %d credit %d, want debit 1200 credit 0", invDebit, invCredit)
	}
	grniDebit, grniCredit := sumLines(lines, fx.grniAccountID)
	if grniDebit != 0 || grniCredit != 0 {
		t.Fatalf("expected no GRNI line for an unlinked bill, got debit %d credit %d", grniDebit, grniCredit)
	}
}

// A bill exactly matching what a receipt already accrued clears GRNI in
// full and posts zero net Inventory — the goods were already capitalized
// at receipt time; nothing new to add.
func TestBillMatchingReceiptClearsGRNIWithNoNetInventoryLine(t *testing.T) {
	d := newTestDB(t)
	fx := newGRNITestFixture(t, d, "org-grni-match", 10, 250) // 10 @ 2.50
	fx.receive(t, d, "GR-0001", 10)                           // accrues 2500

	inv := fx.bill(t, d, "V-001", 10, 250, true) // bills the same 10 @ 2.50 = 2500
	if _, err := d.UpdateIncomingInvoiceState(inv.ID, "approved"); err != nil {
		t.Fatalf("UpdateIncomingInvoiceState(approved): %v", err)
	}

	entry, err := d.FindPostedEntryForSourceDocument("incoming_invoice", inv.ID)
	if err != nil || entry == nil {
		t.Fatalf("expected a posted entry, err=%v entry=%v", err, entry)
	}
	lines, err := d.GetJournalEntryLines(entry.ID)
	if err != nil {
		t.Fatalf("GetJournalEntryLines: %v", err)
	}
	invDebit, invCredit := sumLines(lines, fx.inventoryAccountID)
	if invDebit != 0 || invCredit != 0 {
		t.Fatalf("expected no net Inventory line, got debit %d credit %d", invDebit, invCredit)
	}
	grniDebit, grniCredit := sumLines(lines, fx.grniAccountID)
	if grniDebit != 2500 || grniCredit != 0 {
		t.Fatalf("GRNI line = debit %d credit %d, want debit 2500 credit 0 (fully cleared)", grniDebit, grniCredit)
	}
	apDebit, apCredit := sumLines(lines, fx.apAccountID)
	if apDebit != 0 || apCredit != 2500 {
		t.Fatalf("AP line = debit %d credit %d, want debit 0 credit 2500", apDebit, apCredit)
	}

	// Balanced by construction.
	var totalDebit, totalCredit int64
	for _, l := range lines {
		totalDebit += l.Debit
		totalCredit += l.Credit
	}
	if totalDebit != totalCredit {
		t.Fatalf("entry does not balance: debit %d, credit %d", totalDebit, totalCredit)
	}
}

// A bill priced ABOVE the receipt's accrued cost clears GRNI at the
// accrued value and lands the difference on Inventory as a positive price
// variance.
func TestBillAboveReceiptCostPostsPositiveInventoryVariance(t *testing.T) {
	d := newTestDB(t)
	fx := newGRNITestFixture(t, d, "org-grni-above", 10, 250) // accrues 2500
	fx.receive(t, d, "GR-0001", 10)

	inv := fx.bill(t, d, "V-001", 10, 260, true) // bills 10 @ 2.60 = 2600
	if _, err := d.UpdateIncomingInvoice(inv.ID, UpdateIncomingInvoiceRequest{
		MatchOverride: ptr(1), MatchOverrideReason: ptr("test: price above accrual"),
	}); err != nil {
		t.Fatalf("UpdateIncomingInvoice (override): %v", err)
	}
	if _, err := d.UpdateIncomingInvoiceState(inv.ID, "approved"); err != nil {
		t.Fatalf("UpdateIncomingInvoiceState(approved): %v", err)
	}

	entry, err := d.FindPostedEntryForSourceDocument("incoming_invoice", inv.ID)
	if err != nil || entry == nil {
		t.Fatalf("expected a posted entry, err=%v entry=%v", err, entry)
	}
	lines, err := d.GetJournalEntryLines(entry.ID)
	if err != nil {
		t.Fatalf("GetJournalEntryLines: %v", err)
	}
	invDebit, invCredit := sumLines(lines, fx.inventoryAccountID)
	if invDebit != 100 || invCredit != 0 {
		t.Fatalf("Inventory variance line = debit %d credit %d, want debit 100 credit 0", invDebit, invCredit)
	}
	grniDebit, _ := sumLines(lines, fx.grniAccountID)
	if grniDebit != 2500 {
		t.Fatalf("GRNI line debit = %d, want 2500 (full accrual, valued at accrued cost)", grniDebit)
	}
	_, apCredit := sumLines(lines, fx.apAccountID)
	if apCredit != 2600 {
		t.Fatalf("AP credit = %d, want 2600 (the bill's own total)", apCredit)
	}
}

// A bill priced BELOW the receipt's accrued cost still clears GRNI in full
// (quantity-first, valued at the accrued cost — not the bill's lower
// price), and the difference lands on Inventory as a write-down (credit).
func TestBillBelowReceiptCostPostsNegativeInventoryVariance(t *testing.T) {
	d := newTestDB(t)
	fx := newGRNITestFixture(t, d, "org-grni-below", 10, 250) // accrues 2500
	fx.receive(t, d, "GR-0001", 10)

	inv := fx.bill(t, d, "V-001", 10, 240, true) // bills 10 @ 2.40 = 2400
	if _, err := d.UpdateIncomingInvoice(inv.ID, UpdateIncomingInvoiceRequest{
		MatchOverride: ptr(1), MatchOverrideReason: ptr("test: price below accrual"),
	}); err != nil {
		t.Fatalf("UpdateIncomingInvoice (override): %v", err)
	}
	if _, err := d.UpdateIncomingInvoiceState(inv.ID, "approved"); err != nil {
		t.Fatalf("UpdateIncomingInvoiceState(approved): %v", err)
	}

	entry, err := d.FindPostedEntryForSourceDocument("incoming_invoice", inv.ID)
	if err != nil || entry == nil {
		t.Fatalf("expected a posted entry, err=%v entry=%v", err, entry)
	}
	lines, err := d.GetJournalEntryLines(entry.ID)
	if err != nil {
		t.Fatalf("GetJournalEntryLines: %v", err)
	}
	invDebit, invCredit := sumLines(lines, fx.inventoryAccountID)
	if invDebit != 0 || invCredit != 100 {
		t.Fatalf("Inventory write-down line = debit %d credit %d, want debit 0 credit 100", invDebit, invCredit)
	}
	grniDebit, _ := sumLines(lines, fx.grniAccountID)
	if grniDebit != 2500 {
		t.Fatalf("GRNI line debit = %d, want 2500 (full accrual cleared regardless of the bill's lower price)", grniDebit)
	}
	_, apCredit := sumLines(lines, fx.apAccountID)
	if apCredit != 2400 {
		t.Fatalf("AP credit = %d, want 2400 (the bill's own lower total)", apCredit)
	}

	var totalDebit, totalCredit int64
	for _, l := range lines {
		totalDebit += l.Debit
		totalCredit += l.Credit
	}
	if totalDebit != totalCredit {
		t.Fatalf("entry does not balance: debit %d, credit %d", totalDebit, totalCredit)
	}
}

// Two bills, each covering half the received quantity, each clear their
// own proportional share of GRNI — no double-clearing, and the remainder
// stays open after the first.
func TestPartialBillingAcrossTwoBillsClearsGRNIProportionally(t *testing.T) {
	d := newTestDB(t)
	fx := newGRNITestFixture(t, d, "org-grni-partial", 10, 250) // accrues 2500 for 10 units
	fx.receive(t, d, "GR-0001", 10)

	first := fx.bill(t, d, "V-001", 5, 250, true) // half the quantity, matching price
	if _, err := d.UpdateIncomingInvoiceState(first.ID, "approved"); err != nil {
		t.Fatalf("UpdateIncomingInvoiceState(approved) first: %v", err)
	}
	firstEntry, err := d.FindPostedEntryForSourceDocument("incoming_invoice", first.ID)
	if err != nil || firstEntry == nil {
		t.Fatalf("expected a posted entry for the first bill, err=%v entry=%v", err, firstEntry)
	}
	firstLines, _ := d.GetJournalEntryLines(firstEntry.ID)
	grniDebit1, _ := sumLines(firstLines, fx.grniAccountID)
	if grniDebit1 != 1250 {
		t.Fatalf("first bill GRNI debit = %d, want 1250 (half of 2500)", grniDebit1)
	}

	second := fx.bill(t, d, "V-002", 5, 250, true) // the other half
	if _, err := d.UpdateIncomingInvoiceState(second.ID, "approved"); err != nil {
		t.Fatalf("UpdateIncomingInvoiceState(approved) second: %v", err)
	}
	secondEntry, err := d.FindPostedEntryForSourceDocument("incoming_invoice", second.ID)
	if err != nil || secondEntry == nil {
		t.Fatalf("expected a posted entry for the second bill, err=%v entry=%v", err, secondEntry)
	}
	secondLines, _ := d.GetJournalEntryLines(secondEntry.ID)
	grniDebit2, _ := sumLines(secondLines, fx.grniAccountID)
	if grniDebit2 != 1250 {
		t.Fatalf("second bill GRNI debit = %d, want 1250 (the remaining half)", grniDebit2)
	}

	// Together, both bills fully clear the 2500 accrued — GRNI nets to zero.
	rows, err := d.GetTrialBalance(fx.orgID, "")
	if err != nil {
		t.Fatalf("GetTrialBalance: %v", err)
	}
	for _, r := range rows {
		if r.AccountID == fx.grniAccountID && r.Debit-r.Credit != 0 {
			t.Fatalf("trial balance GRNI net = %d, want 0 (the 2500 credit accrual is now fully offset by both bills' debits)", r.Debit-r.Credit)
		}
	}
}

// Shipping a fungible product posts Dr COGS / Cr Inventory at the current
// weighted average.
func TestShipmentPostsCOGSAtWeightedAverage(t *testing.T) {
	d := newTestDB(t)
	fx := newGRNITestFixture(t, d, "org-cogs-avg", 10, 250) // accrues at 2.50/unit
	fx.receive(t, d, "GR-0001", 10)

	delivery := fx.ship(t, d, "DEL-0001", 4)

	entry, err := d.FindPostedEntryForSourceDocument("outbound_delivery", delivery.ID)
	if err != nil || entry == nil {
		t.Fatalf("expected a posted COGS entry, err=%v entry=%v", err, entry)
	}
	lines, err := d.GetJournalEntryLines(entry.ID)
	if err != nil {
		t.Fatalf("GetJournalEntryLines: %v", err)
	}
	cogsDebit, cogsCredit := sumLines(lines, fx.cogsAccountID)
	if cogsDebit != 1000 || cogsCredit != 0 { // 4 units @ 250
		t.Fatalf("COGS line = debit %d credit %d, want debit 1000 credit 0", cogsDebit, cogsCredit)
	}
	invDebit, invCredit := sumLines(lines, fx.inventoryAccountID)
	if invDebit != 0 || invCredit != 1000 {
		t.Fatalf("Inventory line = debit %d credit %d, want debit 0 credit 1000", invDebit, invCredit)
	}
}

// A serialized product's COGS is that specific unit's own receipt cost —
// specific identification — not the blended weighted average across other
// units of the same product.
func TestShipmentPostsCOGSAtSpecificSerialCostNotAverage(t *testing.T) {
	d, product := seedSerializedProduct(t, "org-cogs-serial")
	date := int64(1700000000000)

	receipt, err := d.CreateInboundDelivery(CreateInboundDeliveryRequest{
		OrganizationID: product.OrganizationID, DeliveryNumber: "GR-0001", DeliveryDate: date,
		LineItems: []CreateInboundDeliveryLineItemRequest{
			{ProductID: &product.ID, Description: "Serial Widget", Quantity: 1, UnitCost: ptr(float64(700))},
			{ProductID: &product.ID, Description: "Serial Widget", Quantity: 1, UnitCost: ptr(float64(900))},
		},
	})
	if err != nil {
		t.Fatalf("CreateInboundDelivery: %v", err)
	}
	rItems, _ := d.GetInboundDeliveryLineItems(receipt.ID)
	if _, err := d.UpdateInboundDeliveryStatus(receipt.ID, "received", map[string][]string{
		rItems[0].ID: {"SN-CHEAP"},
		rItems[1].ID: {"SN-EXPENSIVE"},
	}); err != nil {
		t.Fatalf("receive: %v", err)
	}
	// Blended average would be 800 — confirm the two units really did land
	// at their own distinct costs, not that average, before shipping.
	afterReceive, err := d.GetProduct(product.ID)
	if err != nil {
		t.Fatalf("GetProduct: %v", err)
	}
	if afterReceive.UnitCost == nil || *afterReceive.UnitCost != 800 {
		t.Fatalf("sanity check: blended average = %v, want 800", afterReceive.UnitCost)
	}

	delivery, err := d.CreateDelivery(CreateDeliveryRequest{
		OrganizationID: product.OrganizationID, DeliveryNumber: "DEL-0001", DeliveryDate: date,
		LineItems: []CreateDeliveryLineItemRequest{
			{ProductID: &product.ID, Description: "Serial Widget", Quantity: 1},
		},
	})
	if err != nil {
		t.Fatalf("CreateDelivery: %v", err)
	}
	dItems, _ := d.GetDeliveryLineItems(delivery.ID)
	if _, err := d.UpdateDeliveryStatus(delivery.ID, "shipped", map[string][]string{
		dItems[0].ID: {"SN-CHEAP"},
	}); err != nil {
		t.Fatalf("UpdateDeliveryStatus(shipped): %v", err)
	}

	entry, err := d.FindPostedEntryForSourceDocument("outbound_delivery", delivery.ID)
	if err != nil || entry == nil {
		t.Fatalf("expected a posted COGS entry, err=%v entry=%v", err, entry)
	}
	org, err := d.GetOrganization(product.OrganizationID)
	if err != nil {
		t.Fatalf("GetOrganization: %v", err)
	}
	lines, err := d.GetJournalEntryLines(entry.ID)
	if err != nil {
		t.Fatalf("GetJournalEntryLines: %v", err)
	}
	cogsDebit, _ := sumLines(lines, *org.DefaultCOGSAccountID)
	if cogsDebit != 700 {
		t.Fatalf("COGS debit = %d, want 700 (SN-CHEAP's own receipt cost, not the 800 blended average)", cogsDebit)
	}
}

// Shipping a product with no cost basis at all is a clean 409 — and, since
// buildDeliveryCOGSGLLines runs before the transaction opens, no stock
// movement is ever attempted.
func TestShipmentWithNoCostBasisIsRejectedAndTouchesNoStock(t *testing.T) {
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-cogs-nocost", Name: ptr("No Cost Org")})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	date := int64(1738368000000)
	if _, err := d.CreateFiscalYear(CreateFiscalYearRequest{
		OrganizationID: org.ID, Name: "2025", StartDate: 1735689600000, EndDate: 1767225599000,
	}); err != nil {
		t.Fatalf("CreateFiscalYear: %v", err)
	}
	product, err := d.CreateProduct(CreateProductRequest{
		OrganizationID: org.ID, Name: "Widget", SKU: ptr("WID-1"), Type: "product", StockEnabled: 1,
	})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	// Stock in with no cost — legal (an opening-balance adjustment with no
	// price attached), but leaves nothing for COGS to value a sale at.
	if _, err := d.CreateStockMovement(CreateStockMovementRequest{
		OrganizationID: org.ID, ProductID: product.ID, Type: "in", Quantity: 5,
	}); err != nil {
		t.Fatalf("CreateStockMovement: %v", err)
	}

	delivery, err := d.CreateDelivery(CreateDeliveryRequest{
		OrganizationID: org.ID, DeliveryNumber: "DEL-0001", DeliveryDate: date,
		LineItems: []CreateDeliveryLineItemRequest{
			{ProductID: &product.ID, Description: "Widget", Quantity: 2},
		},
	})
	if err != nil {
		t.Fatalf("CreateDelivery: %v", err)
	}

	_, err = d.UpdateDeliveryStatus(delivery.ID, "shipped", nil)
	if err == nil {
		t.Fatal("expected shipping a product with no cost basis to be rejected")
	}
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected a *ValidationError, got %T: %v", err, err)
	}

	p, err := d.GetProduct(product.ID)
	if err != nil {
		t.Fatalf("GetProduct: %v", err)
	}
	if p.StockQuantity != 5 {
		t.Fatalf("stockQuantity: got %v, want 5 (unchanged — no movement should have been attempted)", p.StockQuantity)
	}
	current, err := d.GetDelivery(delivery.ID)
	if err != nil {
		t.Fatalf("GetDelivery: %v", err)
	}
	if current.Status != "draft" {
		t.Fatalf("status changed despite rejection: %q", current.Status)
	}
}

func TestCancelShippedDeliveryReversesCOGS(t *testing.T) {
	d := newTestDB(t)
	fx := newGRNITestFixture(t, d, "org-cogs-cancel", 10, 250)
	fx.receive(t, d, "GR-0001", 10)
	delivery := fx.ship(t, d, "DEL-0001", 4)

	original, err := d.FindPostedEntryForSourceDocument("outbound_delivery", delivery.ID)
	if err != nil || original == nil {
		t.Fatalf("expected a posted COGS entry, err=%v entry=%v", err, original)
	}

	if _, err := d.UpdateDeliveryStatus(delivery.ID, "cancelled", nil); err != nil {
		t.Fatalf("UpdateDeliveryStatus(cancelled): %v", err)
	}

	stillLive, err := d.FindPostedEntryForSourceDocument("outbound_delivery", delivery.ID)
	if err != nil {
		t.Fatalf("FindPostedEntryForSourceDocument: %v", err)
	}
	if stillLive != nil {
		t.Fatalf("expected no live posted entry after cancelling, got %+v", stillLive)
	}
	reversedOriginal, err := d.GetJournalEntry(original.ID)
	if err != nil {
		t.Fatalf("GetJournalEntry(original): %v", err)
	}
	if reversedOriginal.Status != "reversed" {
		t.Fatalf("original entry status = %q, want reversed", reversedOriginal.Status)
	}

	rows, err := d.GetTrialBalance(fx.orgID, "")
	if err != nil {
		t.Fatalf("GetTrialBalance: %v", err)
	}
	for _, r := range rows {
		if r.AccountID == fx.cogsAccountID && r.Debit-r.Credit != 0 {
			t.Fatalf("trial balance COGS net after reversal = %d, want 0", r.Debit-r.Credit)
		}
	}
}

// requireFiscalYearCoveringNow gives an org a fiscal year covering
// time.Now() — CreateStockMovement dates its GL entry "now" (stockMovements
// has no business-date field of its own), unlike every other posting path
// in this codebase, which dates its entry off the source document itself.
// Starts the day after newGRNITestFixture's own "2025" year ends (F52,
// 2026-08-13 audit: CreateFiscalYear now rejects a range overlapping an
// existing open year) — callers using a bare CreateOrganization with no
// prior year still get full 2026-2099 coverage either way.
func requireFiscalYearCoveringNow(t *testing.T, d *Database, orgID string) {
	t.Helper()
	if _, err := d.CreateFiscalYear(CreateFiscalYearRequest{
		OrganizationID: orgID, Name: "current", StartDate: 1767225600000, EndDate: 4102444799000, // 2026-2099
	}); err != nil {
		t.Fatalf("CreateFiscalYear: %v", err)
	}
}

// A manual stock adjustment posts one signed Inventory line against the
// Inventory Adjustment account — Dr Inventory for a net increase, Cr
// Inventory for a net decrease, valued at the product's current average.
func TestManualStockAdjustmentPostsSignedInventoryLine(t *testing.T) {
	d := newTestDB(t)
	fx := newGRNITestFixture(t, d, "org-adj-signed", 10, 250) // establishes average via receive below
	fx.receive(t, d, "GR-0001", 10)                           // average now 2.50/unit
	requireFiscalYearCoveringNow(t, d, fx.orgID)

	// Increase: +3 units with no explicit cost falls back to the current
	// average (2.50) — Dr Inventory 750 / Cr Inventory Adjustment 750.
	increase, err := d.CreateStockMovement(CreateStockMovementRequest{
		OrganizationID: fx.orgID, ProductID: fx.productID, Type: "in", Quantity: 3,
	})
	if err != nil {
		t.Fatalf("CreateStockMovement (increase): %v", err)
	}
	incEntry, err := d.FindPostedEntryForSourceDocument("stock_movement", increase.Movements[0].ID)
	if err != nil || incEntry == nil {
		t.Fatalf("expected a posted adjustment entry, err=%v entry=%v", err, incEntry)
	}
	incLines, err := d.GetJournalEntryLines(incEntry.ID)
	if err != nil {
		t.Fatalf("GetJournalEntryLines: %v", err)
	}
	invDebit, invCredit := sumLines(incLines, fx.inventoryAccountID)
	if invDebit != 750 || invCredit != 0 {
		t.Fatalf("Inventory line (increase) = debit %d credit %d, want debit 750 credit 0", invDebit, invCredit)
	}
	adjDebit, adjCredit := sumLines(incLines, fx.inventoryAdjustmentAccountID)
	if adjDebit != 0 || adjCredit != 750 {
		t.Fatalf("Adjustment line (increase) = debit %d credit %d, want debit 0 credit 750", adjDebit, adjCredit)
	}

	// Decrease: -2 units at the same average — Dr Inventory Adjustment 500 /
	// Cr Inventory 500.
	decrease, err := d.CreateStockMovement(CreateStockMovementRequest{
		OrganizationID: fx.orgID, ProductID: fx.productID, Type: "out", Quantity: -2,
	})
	if err != nil {
		t.Fatalf("CreateStockMovement (decrease): %v", err)
	}
	decEntry, err := d.FindPostedEntryForSourceDocument("stock_movement", decrease.Movements[0].ID)
	if err != nil || decEntry == nil {
		t.Fatalf("expected a posted adjustment entry, err=%v entry=%v", err, decEntry)
	}
	decLines, err := d.GetJournalEntryLines(decEntry.ID)
	if err != nil {
		t.Fatalf("GetJournalEntryLines: %v", err)
	}
	invDebit2, invCredit2 := sumLines(decLines, fx.inventoryAccountID)
	if invDebit2 != 0 || invCredit2 != 500 {
		t.Fatalf("Inventory line (decrease) = debit %d credit %d, want debit 0 credit 500", invDebit2, invCredit2)
	}
	adjDebit2, adjCredit2 := sumLines(decLines, fx.inventoryAdjustmentAccountID)
	if adjDebit2 != 500 || adjCredit2 != 0 {
		t.Fatalf("Adjustment line (decrease) = debit %d credit %d, want debit 500 credit 0", adjDebit2, adjCredit2)
	}
}

// An uncosted inflow on a product with no cost basis at all — nothing
// stated, no average yet established — contributes nothing and posts no
// entry, mirroring GRNI's "uncosted inflows stay out of the valuation pool"
// rule rather than COGS's 409.
func TestUncostedStockAdjustmentWithNoAveragePostsNoEntry(t *testing.T) {
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-adj-nocost", Name: ptr("No Cost Org")})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	requireFiscalYearCoveringNow(t, d, org.ID)
	product, err := d.CreateProduct(CreateProductRequest{
		OrganizationID: org.ID, Name: "Widget", SKU: ptr("WID-1"), Type: "product", StockEnabled: 1,
	})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}

	result, err := d.CreateStockMovement(CreateStockMovementRequest{
		OrganizationID: org.ID, ProductID: product.ID, Type: "in", Quantity: 5,
	})
	if err != nil {
		t.Fatalf("CreateStockMovement: %v", err)
	}

	entry, err := d.FindPostedEntryForSourceDocument("stock_movement", result.Movements[0].ID)
	if err != nil {
		t.Fatalf("FindPostedEntryForSourceDocument: %v", err)
	}
	if entry != nil {
		t.Fatalf("expected no posted entry for an uncosted adjustment with no average, got %+v", entry)
	}
}

// An outflow with no cost basis anywhere is a 409, consistent with COGS —
// a real stock loss needs a value to record, not a silently understated
// write-off — and the error message reads correctly for a manual
// adjustment, not COGS's wording.
func TestOutflowStockAdjustmentWithNoCostBasisIsRejected(t *testing.T) {
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-adj-out-nocost", Name: ptr("No Cost Org")})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	requireFiscalYearCoveringNow(t, d, org.ID)
	product, err := d.CreateProduct(CreateProductRequest{
		OrganizationID: org.ID, Name: "Widget", SKU: ptr("WID-1"), Type: "product", StockEnabled: 1,
	})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	if _, err := d.CreateStockMovement(CreateStockMovementRequest{
		OrganizationID: org.ID, ProductID: product.ID, Type: "in", Quantity: 5,
	}); err != nil {
		t.Fatalf("CreateStockMovement (uncosted in): %v", err)
	}

	_, err = d.CreateStockMovement(CreateStockMovementRequest{
		OrganizationID: org.ID, ProductID: product.ID, Type: "out", Quantity: -2,
	})
	if err == nil {
		t.Fatal("expected an outflow with no cost basis to be rejected")
	}
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected a *ValidationError, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "cannot post stock adjustment") {
		t.Fatalf("expected the error to read as a stock adjustment, not COGS: %v", err)
	}
}

// A zero-value adjustment (net signed value rounds to exactly zero cents)
// posts no entry at all.
func TestZeroValueStockAdjustmentPostsNoEntry(t *testing.T) {
	d := newTestDB(t)
	fx := newGRNITestFixture(t, d, "org-adj-zero", 10, 0) // 0 unit price -> zero-cost average
	fx.receive(t, d, "GR-0001", 10)
	requireFiscalYearCoveringNow(t, d, fx.orgID)

	result, err := d.CreateStockMovement(CreateStockMovementRequest{
		OrganizationID: fx.orgID, ProductID: fx.productID, Type: "in", Quantity: 3,
	})
	if err != nil {
		t.Fatalf("CreateStockMovement: %v", err)
	}

	entry, err := d.FindPostedEntryForSourceDocument("stock_movement", result.Movements[0].ID)
	if err != nil {
		t.Fatalf("FindPostedEntryForSourceDocument: %v", err)
	}
	if entry != nil {
		t.Fatalf("expected no posted entry for a zero-value adjustment, got %+v", entry)
	}
}

// Once a manual stock movement has a posted GL entry, deleting it is
// blocked — the same immutability every other posted entry gets, corrected
// by recording an offsetting adjustment instead.
func TestDeleteStockMovementBlockedOnceGLPosted(t *testing.T) {
	d := newTestDB(t)
	fx := newGRNITestFixture(t, d, "org-adj-delete", 10, 250)
	fx.receive(t, d, "GR-0001", 10)
	requireFiscalYearCoveringNow(t, d, fx.orgID)

	result, err := d.CreateStockMovement(CreateStockMovementRequest{
		OrganizationID: fx.orgID, ProductID: fx.productID, Type: "in", Quantity: 3,
	})
	if err != nil {
		t.Fatalf("CreateStockMovement: %v", err)
	}

	_, err = d.DeleteStockMovement(result.Movements[0].ID)
	if err == nil {
		t.Fatal("expected deleting a GL-posted stock movement to be rejected")
	}
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected a *ValidationError, got %T: %v", err, err)
	}
}
