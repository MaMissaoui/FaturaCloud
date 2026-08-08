package db

import (
	"errors"
	"testing"
)

// grniTestFixture is a stock-enabled counterpart to glPostingTestFixture —
// an organization, vendor, and stock-tracked product with a purchase order
// already placed, so a test can receive against it and exercise Phase 7's
// GRNI accrual.
type grniTestFixture struct {
	orgID              string
	vendorID           string
	productID          string
	poID               string
	poLineID           string
	inventoryAccountID string
	grniAccountID      string
	apAccountID        string
	date               int64
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
		orgID:              org.ID,
		vendorID:           vendor.ID,
		productID:          product.ID,
		poID:               po.ID,
		poLineID:           poItems[0].ID,
		inventoryAccountID: *org.DefaultInventoryAccountID,
		grniAccountID:      *org.DefaultGRNIAccountID,
		apAccountID:        *org.DefaultApAccountID,
		date:               date,
	}
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
