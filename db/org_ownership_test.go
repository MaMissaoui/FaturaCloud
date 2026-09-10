package db

import "testing"

// TestCreateFiscalPeriodRejectsCrossOrgFiscalYear is issue #189's verified
// starting example: CreateFiscalPeriod already validated a fiscalYearId's
// date range but never checked that year's own OrganizationID against the
// request's — a member of org B could nest a period they create under org B
// inside org A's fiscal year.
func TestCreateFiscalPeriodRejectsCrossOrgFiscalYear(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	orgA, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-fk-a"})
	if err != nil {
		t.Fatalf("CreateOrganization org-a: %v", err)
	}
	orgB, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-fk-b"})
	if err != nil {
		t.Fatalf("CreateOrganization org-b: %v", err)
	}
	fyA, err := d.CreateFiscalYear(CreateFiscalYearRequest{
		OrganizationID: orgA.ID, Name: "2030", StartDate: 1893456000000, EndDate: 1924991999000,
	})
	if err != nil {
		t.Fatalf("CreateFiscalYear org-a: %v", err)
	}

	if _, err := d.CreateFiscalPeriod(CreateFiscalPeriodRequest{
		OrganizationID: orgB.ID, FiscalYearID: fyA.ID, Name: "Q1 2030",
		StartDate: 1893456000000, EndDate: 1901318400000,
	}); err == nil {
		t.Fatal("expected a fiscal period under org-b pointing at org-a's fiscal year to be rejected")
	}

	// A period under org-a's own fiscal year is unaffected.
	if _, err := d.CreateFiscalPeriod(CreateFiscalPeriodRequest{
		OrganizationID: orgA.ID, FiscalYearID: fyA.ID, Name: "Q1 2030",
		StartDate: 1893456000000, EndDate: 1901318400000,
	}); err != nil {
		t.Fatalf("expected a period under its own org's fiscal year to succeed, got: %v", err)
	}
}

// TestCreateJournalEntryRejectsCrossOrgReferences covers CreateJournalEntry's
// journalId and every per-line reference (accountId, clientId, vendorId,
// taxRateId) — the highest-severity gap the #189 audit found, since a
// journal entry posts directly to the general ledger.
func TestCreateJournalEntryRejectsCrossOrgReferences(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	orgA, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-je-a"})
	if err != nil {
		t.Fatalf("CreateOrganization org-a: %v", err)
	}
	orgB, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-je-b"})
	if err != nil {
		t.Fatalf("CreateOrganization org-b: %v", err)
	}
	if orgA.DefaultRevenueAccountID == nil || orgB.DefaultRevenueAccountID == nil {
		t.Fatal("expected both organizations' auto-seeded chart of accounts to set a default revenue account")
	}

	journalA, err := d.CreateJournal(CreateJournalRequest{
		OrganizationID: orgA.ID, Code: "TSTJ", Name: "Test Journal", Type: "miscellaneous",
	})
	if err != nil {
		t.Fatalf("CreateJournal org-a: %v", err)
	}
	journalB, err := d.CreateJournal(CreateJournalRequest{
		OrganizationID: orgB.ID, Code: "TSTJ", Name: "Test Journal", Type: "miscellaneous",
	})
	if err != nil {
		t.Fatalf("CreateJournal org-b: %v", err)
	}
	clientA, err := d.CreateClient(CreateClientRequest{OrganizationID: orgA.ID, Name: ptr("Org A Client")})
	if err != nil {
		t.Fatalf("CreateClient org-a: %v", err)
	}
	vendorA, err := d.CreateVendor(CreateVendorRequest{OrganizationID: orgA.ID, Name: ptr("Org A Vendor")})
	if err != nil {
		t.Fatalf("CreateVendor org-a: %v", err)
	}
	taxRateA, err := d.CreateTaxRate(CreateTaxRateRequest{OrganizationID: orgA.ID, Name: "Org A Rate", Percentage: 10})
	if err != nil {
		t.Fatalf("CreateTaxRate org-a: %v", err)
	}
	// resolveFiscalPeriodForDate (called after every ownership check, once
	// CreateJournalEntry opens its tx) needs an open fiscal year covering
	// the test date, or even a fully self-consistent org-b entry 409s for
	// an unrelated reason.
	if _, err := d.CreateFiscalYear(CreateFiscalYearRequest{
		OrganizationID: orgB.ID, Name: "2030", StartDate: 1893456000000, EndDate: 1924991999000,
	}); err != nil {
		t.Fatalf("CreateFiscalYear org-b: %v", err)
	}

	// journalId pointing at org-a's journal, entry itself under org-b.
	if _, err := d.CreateJournalEntry(CreateJournalEntryRequest{
		OrganizationID: orgB.ID, JournalID: journalA.ID, Date: 1893456000000, Description: "cross-org journal",
		Lines: []CreateJournalLineRequest{
			{AccountID: *orgB.DefaultRevenueAccountID, Debit: 100},
			{AccountID: *orgB.DefaultRevenueAccountID, Credit: 100},
		},
	}); err == nil {
		t.Fatal("expected a journal entry under org-b pointing at org-a's journal to be rejected")
	}

	// journalId valid (org-b's own), but a line's accountId points at org-a.
	if _, err := d.CreateJournalEntry(CreateJournalEntryRequest{
		OrganizationID: orgB.ID, JournalID: journalB.ID, Date: 1893456000000, Description: "cross-org account",
		Lines: []CreateJournalLineRequest{
			{AccountID: *orgA.DefaultRevenueAccountID, Debit: 100},
			{AccountID: *orgB.DefaultRevenueAccountID, Credit: 100},
		},
	}); err == nil {
		t.Fatal("expected a journal line posting to org-a's account from org-b to be rejected")
	}

	// A line's clientId points at org-a.
	if _, err := d.CreateJournalEntry(CreateJournalEntryRequest{
		OrganizationID: orgB.ID, JournalID: journalB.ID, Date: 1893456000000, Description: "cross-org client",
		Lines: []CreateJournalLineRequest{
			{AccountID: *orgB.DefaultRevenueAccountID, Debit: 100, ClientID: &clientA.ID},
			{AccountID: *orgB.DefaultRevenueAccountID, Credit: 100},
		},
	}); err == nil {
		t.Fatal("expected a journal line referencing org-a's client from org-b to be rejected")
	}

	// A line's vendorId points at org-a.
	if _, err := d.CreateJournalEntry(CreateJournalEntryRequest{
		OrganizationID: orgB.ID, JournalID: journalB.ID, Date: 1893456000000, Description: "cross-org vendor",
		Lines: []CreateJournalLineRequest{
			{AccountID: *orgB.DefaultRevenueAccountID, Debit: 100, VendorID: &vendorA.ID},
			{AccountID: *orgB.DefaultRevenueAccountID, Credit: 100},
		},
	}); err == nil {
		t.Fatal("expected a journal line referencing org-a's vendor from org-b to be rejected")
	}

	// A line's taxRateId points at org-a.
	if _, err := d.CreateJournalEntry(CreateJournalEntryRequest{
		OrganizationID: orgB.ID, JournalID: journalB.ID, Date: 1893456000000, Description: "cross-org tax rate",
		Lines: []CreateJournalLineRequest{
			{AccountID: *orgB.DefaultRevenueAccountID, Debit: 100, TaxRateID: &taxRateA.ID},
			{AccountID: *orgB.DefaultRevenueAccountID, Credit: 100},
		},
	}); err == nil {
		t.Fatal("expected a journal line referencing org-a's tax rate from org-b to be rejected")
	}

	// A fully self-consistent org-b entry still succeeds.
	if _, err := d.CreateJournalEntry(CreateJournalEntryRequest{
		OrganizationID: orgB.ID, JournalID: journalB.ID, Date: 1893456000000, Description: "all within org-b",
		Lines: []CreateJournalLineRequest{
			{AccountID: *orgB.DefaultRevenueAccountID, Debit: 100},
			{AccountID: *orgB.DefaultRevenueAccountID, Credit: 100},
		},
	}); err != nil {
		t.Fatalf("expected a fully self-consistent org-b journal entry to succeed, got: %v", err)
	}
}

// TestCreateInvoiceRejectsCrossOrgReferences covers CreateInvoice/UpdateInvoice's
// clientId and line-item productId/taxRate.
func TestCreateInvoiceRejectsCrossOrgReferences(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	orgA, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-inv-a"})
	if err != nil {
		t.Fatalf("CreateOrganization org-a: %v", err)
	}
	orgB, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-inv-b"})
	if err != nil {
		t.Fatalf("CreateOrganization org-b: %v", err)
	}
	clientA, err := d.CreateClient(CreateClientRequest{OrganizationID: orgA.ID, Name: ptr("Org A Client")})
	if err != nil {
		t.Fatalf("CreateClient org-a: %v", err)
	}
	clientB, err := d.CreateClient(CreateClientRequest{OrganizationID: orgB.ID, Name: ptr("Org B Client")})
	if err != nil {
		t.Fatalf("CreateClient org-b: %v", err)
	}
	productA, err := d.CreateProduct(CreateProductRequest{OrganizationID: orgA.ID, Name: "Org A Product", Type: "product", Price: 1000})
	if err != nil {
		t.Fatalf("CreateProduct org-a: %v", err)
	}
	taxRateA, err := d.CreateTaxRate(CreateTaxRateRequest{OrganizationID: orgA.ID, Name: "Org A Rate", Percentage: 10})
	if err != nil {
		t.Fatalf("CreateTaxRate org-a: %v", err)
	}

	// clientId points at org-a while the invoice itself is under org-b.
	if _, err := d.CreateInvoice(CreateInvoiceRequest{
		OrganizationID: orgB.ID, Number: "INV-X", ClientID: clientA.ID, Date: 1700000000000, Currency: "EUR",
	}); err == nil {
		t.Fatal("expected an org-b invoice pointing at org-a's client to be rejected")
	}

	// clientId valid, but a line's productId points at org-a.
	if _, err := d.CreateInvoice(CreateInvoiceRequest{
		OrganizationID: orgB.ID, Number: "INV-X", ClientID: clientB.ID, Date: 1700000000000, Currency: "EUR",
		LineItems: []CreateInvoiceLineItemRequest{{Quantity: 1, UnitPrice: 100, ProductID: &productA.ID}},
	}); err == nil {
		t.Fatal("expected an org-b invoice line referencing org-a's product to be rejected")
	}

	// clientId valid, but a line's taxRate points at org-a.
	if _, err := d.CreateInvoice(CreateInvoiceRequest{
		OrganizationID: orgB.ID, Number: "INV-X", ClientID: clientB.ID, Date: 1700000000000, Currency: "EUR",
		LineItems: []CreateInvoiceLineItemRequest{{Quantity: 1, UnitPrice: 100, TaxRate: &taxRateA.ID}},
	}); err == nil {
		t.Fatal("expected an org-b invoice line referencing org-a's tax rate to be rejected")
	}

	// A fully self-consistent org-b invoice still succeeds.
	invoiceB, err := d.CreateInvoice(CreateInvoiceRequest{
		OrganizationID: orgB.ID, Number: "INV-B1", ClientID: clientB.ID, Date: 1700000000000, Currency: "EUR",
	})
	if err != nil {
		t.Fatalf("expected a fully self-consistent org-b invoice to succeed, got: %v", err)
	}

	// UpdateInvoice: switching clientId to org-a's client is rejected.
	if _, err := d.UpdateInvoice(invoiceB.ID, UpdateInvoiceRequest{ClientID: &clientA.ID}); err == nil {
		t.Fatal("expected UpdateInvoice to reject switching to org-a's client")
	}

	// UpdateInvoice: a replacement line item referencing org-a's product is rejected.
	if _, err := d.UpdateInvoice(invoiceB.ID, UpdateInvoiceRequest{
		LineItems: &[]CreateInvoiceLineItemRequest{{Quantity: 1, UnitPrice: 100, ProductID: &productA.ID}},
	}); err == nil {
		t.Fatal("expected UpdateInvoice to reject a line item referencing org-a's product")
	}
}

// TestCreateOrderRejectsCrossOrgReferences covers CreateOrder/UpdateOrder's
// clientId and line-item productId.
func TestCreateOrderRejectsCrossOrgReferences(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	orgA, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-ord-a"})
	if err != nil {
		t.Fatalf("CreateOrganization org-a: %v", err)
	}
	orgB, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-ord-b"})
	if err != nil {
		t.Fatalf("CreateOrganization org-b: %v", err)
	}
	clientA, err := d.CreateClient(CreateClientRequest{OrganizationID: orgA.ID, Name: ptr("Org A Client")})
	if err != nil {
		t.Fatalf("CreateClient org-a: %v", err)
	}
	productA, err := d.CreateProduct(CreateProductRequest{OrganizationID: orgA.ID, Name: "Org A Product", Type: "product", Price: 1000})
	if err != nil {
		t.Fatalf("CreateProduct org-a: %v", err)
	}

	if _, err := d.CreateOrder(CreateOrderRequest{
		OrganizationID: orgB.ID, OrderNumber: "ORD-X", OrderDate: 1700000000000, ClientID: &clientA.ID,
	}); err == nil {
		t.Fatal("expected an org-b order pointing at org-a's client to be rejected")
	}
	if _, err := d.CreateOrder(CreateOrderRequest{
		OrganizationID: orgB.ID, OrderNumber: "ORD-X", OrderDate: 1700000000000,
		LineItems: []CreateOrderLineItemRequest{{Description: "line", Quantity: 1, UnitPrice: 100, ProductID: &productA.ID}},
	}); err == nil {
		t.Fatal("expected an org-b order line referencing org-a's product to be rejected")
	}

	orderB, err := d.CreateOrder(CreateOrderRequest{OrganizationID: orgB.ID, OrderNumber: "ORD-B1", OrderDate: 1700000000000})
	if err != nil {
		t.Fatalf("expected a fully self-consistent org-b order to succeed, got: %v", err)
	}
	if _, err := d.UpdateOrder(orderB.ID, UpdateOrderRequest{ClientID: &clientA.ID}); err == nil {
		t.Fatal("expected UpdateOrder to reject switching to org-a's client")
	}
}

// TestCreateDeliveryRejectsCrossOrgReferences covers CreateDelivery/UpdateDelivery's
// orderId/clientId and line-item orderLineItemId/productId — including the
// orderLineItemId path's own cross-org product lookup inside
// replaceDeliveryLineItemsTx.
func TestCreateDeliveryRejectsCrossOrgReferences(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	orgA, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-del-a"})
	if err != nil {
		t.Fatalf("CreateOrganization org-a: %v", err)
	}
	orgB, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-del-b"})
	if err != nil {
		t.Fatalf("CreateOrganization org-b: %v", err)
	}
	clientA, err := d.CreateClient(CreateClientRequest{OrganizationID: orgA.ID, Name: ptr("Org A Client")})
	if err != nil {
		t.Fatalf("CreateClient org-a: %v", err)
	}
	productA, err := d.CreateProduct(CreateProductRequest{OrganizationID: orgA.ID, Name: "Org A Product", Type: "product", Price: 1000})
	if err != nil {
		t.Fatalf("CreateProduct org-a: %v", err)
	}
	orderA, err := d.CreateOrder(CreateOrderRequest{
		OrganizationID: orgA.ID, OrderNumber: "ORD-A1", OrderDate: 1700000000000,
		LineItems: []CreateOrderLineItemRequest{{Description: "line", Quantity: 1, UnitPrice: 100, ProductID: &productA.ID}},
	})
	if err != nil {
		t.Fatalf("CreateOrder org-a: %v", err)
	}
	orderLinesA, err := d.GetOrderLineItems(orderA.ID)
	if err != nil || len(orderLinesA) == 0 {
		t.Fatalf("GetOrderLineItems org-a: %v (lines=%d)", err, len(orderLinesA))
	}

	if _, err := d.CreateDelivery(CreateDeliveryRequest{
		OrganizationID: orgB.ID, DeliveryNumber: "DEL-X", DeliveryDate: 1700000000000, OrderID: &orderA.ID,
	}); err == nil {
		t.Fatal("expected an org-b delivery pointing at org-a's order to be rejected")
	}
	if _, err := d.CreateDelivery(CreateDeliveryRequest{
		OrganizationID: orgB.ID, DeliveryNumber: "DEL-X", DeliveryDate: 1700000000000, ClientID: &clientA.ID,
	}); err == nil {
		t.Fatal("expected an org-b delivery pointing at org-a's client to be rejected")
	}
	if _, err := d.CreateDelivery(CreateDeliveryRequest{
		OrganizationID: orgB.ID, DeliveryNumber: "DEL-X", DeliveryDate: 1700000000000,
		LineItems: []CreateDeliveryLineItemRequest{{Description: "line", Quantity: 1, ProductID: &productA.ID}},
	}); err == nil {
		t.Fatal("expected an org-b delivery line referencing org-a's product to be rejected")
	}
	if _, err := d.CreateDelivery(CreateDeliveryRequest{
		OrganizationID: orgB.ID, DeliveryNumber: "DEL-X", DeliveryDate: 1700000000000,
		LineItems: []CreateDeliveryLineItemRequest{{Description: "line", Quantity: 1, OrderLineItemID: &orderLinesA[0].ID}},
	}); err == nil {
		t.Fatal("expected an org-b delivery line referencing org-a's order line item to be rejected")
	}

	if _, err := d.CreateDelivery(CreateDeliveryRequest{
		OrganizationID: orgB.ID, DeliveryNumber: "DEL-B1", DeliveryDate: 1700000000000,
	}); err != nil {
		t.Fatalf("expected a fully self-consistent org-b delivery to succeed, got: %v", err)
	}
}
