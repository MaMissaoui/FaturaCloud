package db

import (
	"testing"
	"time"
)

// TestGetRevenueByMonth covers the "revenue" definition used consistently
// across every dashboard/reporting metric: only sent/paid invoices count,
// grouped by calendar month, ordered oldest first. draft and cancelled
// invoices in the same months must not contribute. Also covers the new
// upper bound: a document dated after endDate must be excluded.
func TestGetRevenueByMonth(t *testing.T) {
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-1"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	client, err := d.CreateClient(CreateClientRequest{ID: "client-1", OrganizationID: org.ID, Name: ptr("Client")})
	if err != nil {
		t.Fatalf("CreateClient: %v", err)
	}

	now := time.Now()
	thisMonth := now.UnixMilli()
	lastMonth := now.AddDate(0, -1, 0).UnixMilli()
	nextMonth := now.AddDate(0, 1, 0).UnixMilli()

	mkInvoice := func(id, state string, date int64, total int64) {
		if _, err := d.CreateInvoice(CreateInvoiceRequest{
			ID: id, OrganizationID: org.ID, Number: id, State: state, ClientID: client.ID,
			Date: date, Currency: "EUR",
			Total: total, TaxTotal: 0, SubTotal: total,
			LineItems: []CreateInvoiceLineItemRequest{{Quantity: 1, UnitPrice: float64(total)}},
		}); err != nil {
			t.Fatalf("CreateInvoice(%s): %v", id, err)
		}
	}

	mkInvoice("inv-sent-this", "sent", thisMonth, 1000)
	mkInvoice("inv-paid-this", "paid", thisMonth, 500)
	mkInvoice("inv-draft-this", "draft", thisMonth, 999999) // must not count
	mkInvoice("inv-sent-last", "sent", lastMonth, 2000)
	mkInvoice("inv-cancelled-last", "cancelled", lastMonth, 999999) // must not count
	mkInvoice("inv-sent-next", "sent", nextMonth, 999999)           // excluded by endDate below

	cutoff := lastMonth - 1
	rows, err := d.GetRevenueByMonth(org.ID, cutoff, thisMonth)
	if err != nil {
		t.Fatalf("GetRevenueByMonth: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d months, want 2 (next-month invoice excluded by endDate): %+v", len(rows), rows)
	}

	thisMonthLabel := now.Format("2006-01")
	lastMonthLabel := now.AddDate(0, -1, 0).Format("2006-01")

	// Ordered oldest first.
	if rows[0].Month != lastMonthLabel || rows[0].Revenue != 2000 {
		t.Fatalf("rows[0] = %+v, want month=%s revenue=2000", rows[0], lastMonthLabel)
	}
	if rows[1].Month != thisMonthLabel || rows[1].Revenue != 1500 {
		t.Fatalf("rows[1] = %+v, want month=%s revenue=1500", rows[1], thisMonthLabel)
	}

	// endDate=0 is unbounded — the next-month invoice must now be included.
	unbounded, err := d.GetRevenueByMonth(org.ID, cutoff, 0)
	if err != nil {
		t.Fatalf("GetRevenueByMonth (unbounded): %v", err)
	}
	if len(unbounded) != 3 {
		t.Fatalf("got %d months with endDate=0, want 3: %+v", len(unbounded), unbounded)
	}
}

// TestGetSalesByClientAndProduct covers ranking order, revenue summation
// across multiple invoices for the same client/product, exclusion of
// draft/cancelled invoices, that a line item with no productId (e.g. a
// service line with nothing selected) doesn't appear or break the query,
// and that limit=0 returns the full ranked list uncapped.
func TestGetSalesByClientAndProduct(t *testing.T) {
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-1"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	bigClient, err := d.CreateClient(CreateClientRequest{ID: "client-big", OrganizationID: org.ID, Name: ptr("Big Client")})
	if err != nil {
		t.Fatalf("CreateClient(big): %v", err)
	}
	smallClient, err := d.CreateClient(CreateClientRequest{ID: "client-small", OrganizationID: org.ID, Name: ptr("Small Client")})
	if err != nil {
		t.Fatalf("CreateClient(small): %v", err)
	}
	product, err := d.CreateProduct(CreateProductRequest{OrganizationID: org.ID, Name: "Widget", Type: "product"})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}

	now := time.Now().UnixMilli()

	// Big client: two paid/sent invoices with the product, summing to 3000.
	if _, err := d.CreateInvoice(CreateInvoiceRequest{
		ID: "inv-1", OrganizationID: org.ID, Number: "inv-1", State: "paid", ClientID: bigClient.ID,
		Date: now, Currency: "EUR", Total: 2000, TaxTotal: 0, SubTotal: 2000,
		LineItems: []CreateInvoiceLineItemRequest{{Quantity: 2, UnitPrice: 1000, ProductID: &product.ID}},
	}); err != nil {
		t.Fatalf("CreateInvoice inv-1: %v", err)
	}
	if _, err := d.CreateInvoice(CreateInvoiceRequest{
		ID: "inv-2", OrganizationID: org.ID, Number: "inv-2", State: "sent", ClientID: bigClient.ID,
		Date: now, Currency: "EUR", Total: 1000, TaxTotal: 0, SubTotal: 1000,
		LineItems: []CreateInvoiceLineItemRequest{{Quantity: 1, UnitPrice: 1000, ProductID: &product.ID}},
	}); err != nil {
		t.Fatalf("CreateInvoice inv-2: %v", err)
	}
	// Small client: one invoice, no product on the line (e.g. a free-text
	// service line) — must not appear in sales by product, must not error.
	if _, err := d.CreateInvoice(CreateInvoiceRequest{
		ID: "inv-3", OrganizationID: org.ID, Number: "inv-3", State: "paid", ClientID: smallClient.ID,
		Date: now, Currency: "EUR", Total: 500, TaxTotal: 0, SubTotal: 500,
		LineItems: []CreateInvoiceLineItemRequest{{Quantity: 1, UnitPrice: 500}},
	}); err != nil {
		t.Fatalf("CreateInvoice inv-3: %v", err)
	}
	// Draft/cancelled invoices for the big client — must not inflate its total.
	if _, err := d.CreateInvoice(CreateInvoiceRequest{
		ID: "inv-draft", OrganizationID: org.ID, Number: "inv-draft", State: "draft", ClientID: bigClient.ID,
		Date: now, Currency: "EUR", Total: 999999, TaxTotal: 0, SubTotal: 999999,
		LineItems: []CreateInvoiceLineItemRequest{{Quantity: 1, UnitPrice: 999999, ProductID: &product.ID}},
	}); err != nil {
		t.Fatalf("CreateInvoice inv-draft: %v", err)
	}

	cutoff := time.Now().AddDate(0, -12, 0).UnixMilli()

	clients, err := d.GetSalesByClient(org.ID, cutoff, 0, 10)
	if err != nil {
		t.Fatalf("GetSalesByClient: %v", err)
	}
	if len(clients) != 2 {
		t.Fatalf("got %d clients, want 2: %+v", len(clients), clients)
	}
	if clients[0].Name != "Big Client" || clients[0].Revenue != 3000 {
		t.Fatalf("clients[0] = %+v, want Big Client / 3000", clients[0])
	}
	if clients[1].Name != "Small Client" || clients[1].Revenue != 500 {
		t.Fatalf("clients[1] = %+v, want Small Client / 500", clients[1])
	}

	// limit=0 must still return every client, uncapped.
	allClients, err := d.GetSalesByClient(org.ID, cutoff, 0, 0)
	if err != nil {
		t.Fatalf("GetSalesByClient (limit=0): %v", err)
	}
	if len(allClients) != 2 {
		t.Fatalf("got %d clients with limit=0, want 2: %+v", len(allClients), allClients)
	}

	products, err := d.GetSalesByProduct(org.ID, cutoff, 0, 10)
	if err != nil {
		t.Fatalf("GetSalesByProduct: %v", err)
	}
	if len(products) != 1 {
		t.Fatalf("got %d products, want 1 (productless line excluded): %+v", len(products), products)
	}
	if products[0].Name != "Widget" || products[0].Revenue != 3000 {
		t.Fatalf("products[0] = %+v, want Widget / 3000", products[0])
	}
}

// TestGetPurchasesByVendor covers the purchasing-side counterpart to
// GetSalesByClient: only approved/paid incoming invoices count, and a
// vendor with a NULL name doesn't error (vendors.name has no NOT NULL
// constraint).
func TestGetPurchasesByVendor(t *testing.T) {
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-1"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	vendor, err := d.CreateVendor(CreateVendorRequest{OrganizationID: org.ID, Name: ptr("Acme Supplies")})
	if err != nil {
		t.Fatalf("CreateVendor: %v", err)
	}
	nullNamedVendor, err := d.CreateVendor(CreateVendorRequest{OrganizationID: org.ID})
	if err != nil {
		t.Fatalf("CreateVendor(nil name): %v", err)
	}

	now := time.Now().UnixMilli()
	mkBill := func(id, vendorID, state string, total int64) {
		if _, err := d.CreateIncomingInvoice(CreateIncomingInvoiceRequest{
			ID: id, OrganizationID: org.ID, VendorID: vendorID, VendorInvoiceNumber: id,
			State: state, Date: now, Currency: "EUR",
			Total: total, TaxTotal: 0, SubTotal: total,
			LineItems: []CreateInvoiceLineItemRequest{{Quantity: 1, UnitPrice: float64(total)}},
		}); err != nil {
			t.Fatalf("CreateIncomingInvoice(%s): %v", id, err)
		}
	}

	mkBill("bill-approved", vendor.ID, "approved", 1000)
	mkBill("bill-paid", vendor.ID, "paid", 500)
	mkBill("bill-draft", vendor.ID, "draft", 999999)         // must not count
	mkBill("bill-cancelled", vendor.ID, "cancelled", 999999) // must not count
	mkBill("bill-null-vendor", nullNamedVendor.ID, "approved", 250)

	cutoff := time.Now().AddDate(0, -12, 0).UnixMilli()
	rows, err := d.GetPurchasesByVendor(org.ID, cutoff, 0, 0)
	if err != nil {
		t.Fatalf("GetPurchasesByVendor: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d vendors, want 2: %+v", len(rows), rows)
	}
	if rows[0].Name != "Acme Supplies" || rows[0].Spend != 1500 {
		t.Fatalf("rows[0] = %+v, want Acme Supplies / 1500", rows[0])
	}
	if rows[1].Name != "" || rows[1].Spend != 250 {
		t.Fatalf("rows[1] = %+v, want empty name / 250 (NULL vendor name coalesced)", rows[1])
	}
}

// TestGetTaxSummary covers: two tax rates aggregated separately, a 0%-rate
// line still appearing with Tax=0 (unlike the GL, which would skip it), a
// line with no taxRate landing in the "" unrated bucket, and that only
// sent/paid invoices (output) vs approved/paid bills (input) count.
func TestGetTaxSummary(t *testing.T) {
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-1"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	client, err := d.CreateClient(CreateClientRequest{ID: "client-1", OrganizationID: org.ID, Name: ptr("Client")})
	if err != nil {
		t.Fatalf("CreateClient: %v", err)
	}
	vendor, err := d.CreateVendor(CreateVendorRequest{OrganizationID: org.ID, Name: ptr("Vendor")})
	if err != nil {
		t.Fatalf("CreateVendor: %v", err)
	}
	standardRate, err := d.CreateTaxRate(CreateTaxRateRequest{ID: "rate-standard", OrganizationID: org.ID, Name: "Standard", Percentage: 20})
	if err != nil {
		t.Fatalf("CreateTaxRate(standard): %v", err)
	}
	zeroRate, err := d.CreateTaxRate(CreateTaxRateRequest{ID: "rate-zero", OrganizationID: org.ID, Name: "Zero-rated", Percentage: 0})
	if err != nil {
		t.Fatalf("CreateTaxRate(zero): %v", err)
	}

	now := time.Now().UnixMilli()

	if _, err := d.CreateInvoice(CreateInvoiceRequest{
		ID: "inv-standard", OrganizationID: org.ID, Number: "inv-standard", State: "sent", ClientID: client.ID,
		Date: now, Currency: "EUR", Total: 1200, TaxTotal: 200, SubTotal: 1000,
		LineItems: []CreateInvoiceLineItemRequest{{Quantity: 1, UnitPrice: 1000, TaxRate: &standardRate.ID}},
	}); err != nil {
		t.Fatalf("CreateInvoice(standard): %v", err)
	}
	if _, err := d.CreateInvoice(CreateInvoiceRequest{
		ID: "inv-zero", OrganizationID: org.ID, Number: "inv-zero", State: "paid", ClientID: client.ID,
		Date: now, Currency: "EUR", Total: 500, TaxTotal: 0, SubTotal: 500,
		LineItems: []CreateInvoiceLineItemRequest{{Quantity: 1, UnitPrice: 500, TaxRate: &zeroRate.ID}},
	}); err != nil {
		t.Fatalf("CreateInvoice(zero): %v", err)
	}
	if _, err := d.CreateInvoice(CreateInvoiceRequest{
		ID: "inv-unrated", OrganizationID: org.ID, Number: "inv-unrated", State: "sent", ClientID: client.ID,
		Date: now, Currency: "EUR", Total: 300, TaxTotal: 0, SubTotal: 300,
		LineItems: []CreateInvoiceLineItemRequest{{Quantity: 1, UnitPrice: 300}},
	}); err != nil {
		t.Fatalf("CreateInvoice(unrated): %v", err)
	}
	if _, err := d.CreateInvoice(CreateInvoiceRequest{
		ID: "inv-draft", OrganizationID: org.ID, Number: "inv-draft", State: "draft", ClientID: client.ID,
		Date: now, Currency: "EUR", Total: 999999, TaxTotal: 0, SubTotal: 999999,
		LineItems: []CreateInvoiceLineItemRequest{{Quantity: 1, UnitPrice: 999999}},
	}); err != nil {
		t.Fatalf("CreateInvoice(draft): %v", err)
	}

	if _, err := d.CreateIncomingInvoice(CreateIncomingInvoiceRequest{
		ID: "bill-standard", OrganizationID: org.ID, VendorID: vendor.ID, VendorInvoiceNumber: "bill-standard",
		State: "approved", Date: now, Currency: "EUR", Total: 600, TaxTotal: 100, SubTotal: 500,
		LineItems: []CreateInvoiceLineItemRequest{{Quantity: 1, UnitPrice: 500, TaxRate: &standardRate.ID}},
	}); err != nil {
		t.Fatalf("CreateIncomingInvoice: %v", err)
	}

	summary, err := d.GetTaxSummary(org.ID, time.Now().AddDate(0, -12, 0).UnixMilli(), 0)
	if err != nil {
		t.Fatalf("GetTaxSummary: %v", err)
	}

	if len(summary.Output) != 3 {
		t.Fatalf("got %d output lines, want 3 (standard/zero/unrated): %+v", len(summary.Output), summary.Output)
	}
	byName := map[string]TaxSummaryLine{}
	for _, line := range summary.Output {
		byName[line.Name] = line
	}
	if l := byName["Standard"]; l.Base != 1000 || l.Tax != 200 {
		t.Fatalf("Standard output line = %+v, want base=1000 tax=200", l)
	}
	if l := byName["Zero-rated"]; l.Base != 500 || l.Tax != 0 {
		t.Fatalf("Zero-rated output line = %+v, want base=500 tax=0 (must not be skipped)", l)
	}
	if l := byName[""]; l.Base != 300 || l.Tax != 0 || l.TaxRateID != "" {
		t.Fatalf("Unrated output line = %+v, want base=300 tax=0 taxRateId=\"\"", l)
	}

	if len(summary.Input) != 1 {
		t.Fatalf("got %d input lines, want 1: %+v", len(summary.Input), summary.Input)
	}
	if summary.Input[0].Base != 500 || summary.Input[0].Tax != 100 {
		t.Fatalf("input line = %+v, want base=500 tax=100", summary.Input[0])
	}
}

// TestGetTaxSummaryRoundsPerDocument pins the rounding granularity: tax must
// be rounded once per tax-rate group PER DOCUMENT (matching
// validateInvoiceTotals, which is what produces invoices.taxTotal), then
// those already-rounded cents summed — not summed raw and rounded once at
// the end. Two documents each at base=300, rate=8.5%: per document,
// 300*0.085=25.5 rounds half-up to 26, so the correct total is 52. Rounding
// the combined 600*0.085=51.0 once would wrongly give 51 — a real
// divergence even in a single currency with no FX involved.
func TestGetTaxSummaryRoundsPerDocument(t *testing.T) {
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-1"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	client, err := d.CreateClient(CreateClientRequest{ID: "client-1", OrganizationID: org.ID, Name: ptr("Client")})
	if err != nil {
		t.Fatalf("CreateClient: %v", err)
	}
	rate, err := d.CreateTaxRate(CreateTaxRateRequest{ID: "rate-8.5", OrganizationID: org.ID, Name: "8.5%", Percentage: 8.5})
	if err != nil {
		t.Fatalf("CreateTaxRate: %v", err)
	}

	now := time.Now().UnixMilli()
	for _, id := range []string{"inv-a", "inv-b"} {
		if _, err := d.CreateInvoice(CreateInvoiceRequest{
			ID: id, OrganizationID: org.ID, Number: id, State: "sent", ClientID: client.ID,
			Date: now, Currency: "EUR", Total: 326, TaxTotal: 26, SubTotal: 300,
			LineItems: []CreateInvoiceLineItemRequest{{Quantity: 1, UnitPrice: 300, TaxRate: &rate.ID}},
		}); err != nil {
			t.Fatalf("CreateInvoice(%s): %v", id, err)
		}
	}

	summary, err := d.GetTaxSummary(org.ID, time.Now().AddDate(0, -12, 0).UnixMilli(), 0)
	if err != nil {
		t.Fatalf("GetTaxSummary: %v", err)
	}
	if len(summary.Output) != 1 {
		t.Fatalf("got %d output lines, want 1: %+v", len(summary.Output), summary.Output)
	}
	if summary.Output[0].Tax != 52 {
		t.Fatalf("Tax = %d, want 52 (26+26 rounded per document, not round(51.0))", summary.Output[0].Tax)
	}
	if summary.Output[0].Base != 600 {
		t.Fatalf("Base = %d, want 600", summary.Output[0].Base)
	}
}
