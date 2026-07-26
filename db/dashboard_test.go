package db

import (
	"testing"
	"time"
)

// TestGetRevenueByMonth covers the "revenue" definition used consistently
// across every dashboard metric: only sent/paid invoices count, grouped by
// calendar month, ordered oldest first. draft and cancelled invoices in the
// same months must not contribute.
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

	rows, err := d.getRevenueByMonth(org.ID, 6)
	if err != nil {
		t.Fatalf("getRevenueByMonth: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d months, want 2: %+v", len(rows), rows)
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
}

// TestBucketOutstanding pins the aging-bucket boundaries exactly — this is a
// pure function, no DB needed, so every boundary (30/31, 60/61, 90/91 days,
// and the current/overdue line at the due date itself) is cheap to nail down.
func TestBucketOutstanding(t *testing.T) {
	now := time.Date(2026, 1, 31, 12, 0, 0, 0, time.UTC)
	day := int64(86400000)
	nowMillis := now.UnixMilli()

	daysAgo := func(n int64) *int64 {
		v := nowMillis - n*day
		return &v
	}
	daysAhead := func(n int64) *int64 {
		v := nowMillis + n*day
		return &v
	}

	invoices := []OutstandingInvoice{
		{ID: "no-due-date", Total: 10, DueDate: nil},
		{ID: "due-in-future", Total: 20, DueDate: daysAhead(1)},
		{ID: "due-exactly-now", Total: 30, DueDate: &nowMillis},
		{ID: "1-day-overdue", Total: 40, DueDate: daysAgo(1)},
		{ID: "30-days-overdue", Total: 50, DueDate: daysAgo(30)},
		{ID: "31-days-overdue", Total: 60, DueDate: daysAgo(31)},
		{ID: "60-days-overdue", Total: 70, DueDate: daysAgo(60)},
		{ID: "61-days-overdue", Total: 80, DueDate: daysAgo(61)},
		{ID: "90-days-overdue", Total: 90, DueDate: daysAgo(90)},
		{ID: "91-days-overdue", Total: 100, DueDate: daysAgo(91)},
	}

	summary := bucketOutstanding(invoices, now)

	wantTotal := int64(10 + 20 + 30 + 40 + 50 + 60 + 70 + 80 + 90 + 100)
	if summary.Total != wantTotal {
		t.Fatalf("Total = %d, want %d", summary.Total, wantTotal)
	}
	// Current: no due date, future, and due exactly now (>= now is not yet overdue).
	if wantCurrent := int64(10 + 20 + 30); summary.Current != wantCurrent {
		t.Fatalf("Current = %d, want %d", summary.Current, wantCurrent)
	}
	if want := int64(40 + 50); summary.Days1To30 != want {
		t.Fatalf("Days1To30 = %d, want %d", summary.Days1To30, want)
	}
	if want := int64(60 + 70); summary.Days31To60 != want {
		t.Fatalf("Days31To60 = %d, want %d", summary.Days31To60, want)
	}
	if want := int64(80 + 90); summary.Days61To90 != want {
		t.Fatalf("Days61To90 = %d, want %d", summary.Days61To90, want)
	}
	if want := int64(100); summary.Days90Plus != want {
		t.Fatalf("Days90Plus = %d, want %d", summary.Days90Plus, want)
	}

	// Per-invoice DaysOverdue is filled in too, not just the bucket totals.
	for _, inv := range summary.Invoices {
		if inv.ID == "31-days-overdue" && inv.DaysOverdue != 31 {
			t.Fatalf("31-days-overdue.DaysOverdue = %d, want 31", inv.DaysOverdue)
		}
	}
}

// TestGetOutstandingInvoices covers the DB-backed half: only state == "sent"
// counts as outstanding — draft/paid/cancelled must not appear at all,
// regardless of their due date.
func TestGetOutstandingInvoices(t *testing.T) {
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-1"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	client, err := d.CreateClient(CreateClientRequest{ID: "client-1", OrganizationID: org.ID, Name: ptr("Client")})
	if err != nil {
		t.Fatalf("CreateClient: %v", err)
	}

	pastDue := time.Now().AddDate(0, 0, -10).UnixMilli()
	mk := func(id, state string) {
		if _, err := d.CreateInvoice(CreateInvoiceRequest{
			ID: id, OrganizationID: org.ID, Number: id, State: state, ClientID: client.ID,
			Date: pastDue, DueDate: &pastDue, Currency: "EUR",
			Total: 100, TaxTotal: 0, SubTotal: 100,
			LineItems: []CreateInvoiceLineItemRequest{{Quantity: 1, UnitPrice: 100}},
		}); err != nil {
			t.Fatalf("CreateInvoice(%s): %v", id, err)
		}
	}
	mk("inv-draft", "draft")
	mk("inv-sent", "sent")
	mk("inv-paid", "paid")
	mk("inv-cancelled", "cancelled")

	summary, err := d.getOutstandingInvoices(org.ID)
	if err != nil {
		t.Fatalf("getOutstandingInvoices: %v", err)
	}
	if len(summary.Invoices) != 1 || summary.Invoices[0].ID != "inv-sent" {
		t.Fatalf("expected exactly the sent invoice, got %+v", summary.Invoices)
	}
	if summary.Invoices[0].ClientName != "Client" {
		t.Fatalf("expected the joined client name, got %q", summary.Invoices[0].ClientName)
	}
}

// TestGetStockValuation covers COALESCE(unitCost, 0) for a product that's
// never been costed, and that Total agrees with SUM(Items[].Value).
func TestGetStockValuation(t *testing.T) {
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-1"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}

	costed, err := d.CreateProduct(CreateProductRequest{
		OrganizationID: org.ID, Name: "Costed Widget", Type: "product", StockEnabled: 1,
		UnitCost: ptr(int64(500)), // 5.00
	})
	if err != nil {
		t.Fatalf("CreateProduct(costed): %v", err)
	}
	// Give it real stock via a movement, since stockQuantity is derived, not settable directly.
	if _, err := d.CreateStockMovement(CreateStockMovementRequest{
		OrganizationID: org.ID, ProductID: costed.ID, Type: "in", Quantity: 10,
	}); err != nil {
		t.Fatalf("CreateStockMovement: %v", err)
	}

	uncosted, err := d.CreateProduct(CreateProductRequest{
		OrganizationID: org.ID, Name: "Uncosted Widget", Type: "product", StockEnabled: 1,
	})
	if err != nil {
		t.Fatalf("CreateProduct(uncosted): %v", err)
	}
	if _, err := d.CreateStockMovement(CreateStockMovementRequest{
		OrganizationID: org.ID, ProductID: uncosted.ID, Type: "in", Quantity: 3,
	}); err != nil {
		t.Fatalf("CreateStockMovement: %v", err)
	}

	// A non-stock-tracked product must be excluded entirely.
	if _, err := d.CreateProduct(CreateProductRequest{
		OrganizationID: org.ID, Name: "Service", Type: "service",
	}); err != nil {
		t.Fatalf("CreateProduct(service): %v", err)
	}

	valuation, err := d.getStockValuation(org.ID)
	if err != nil {
		t.Fatalf("getStockValuation: %v", err)
	}
	if len(valuation.Items) != 2 {
		t.Fatalf("got %d items, want 2 (service product excluded): %+v", len(valuation.Items), valuation.Items)
	}
	// 10 * 500 = 5000 for the costed product; uncosted contributes 0.
	if valuation.Total != 5000 {
		t.Fatalf("Total = %d, want 5000", valuation.Total)
	}
	var sum int64
	for _, item := range valuation.Items {
		sum += item.Value
	}
	if sum != valuation.Total {
		t.Fatalf("SUM(Items[].Value) = %d does not match Total = %d", sum, valuation.Total)
	}
}

// TestGetTopClientsAndProducts covers ranking order, revenue summation
// across multiple invoices for the same client/product, exclusion of
// draft/cancelled invoices, and that a line item with no productId (e.g. a
// service line with nothing selected) doesn't appear or break the query.
func TestGetTopClientsAndProducts(t *testing.T) {
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
	// service line) — must not appear in top products, must not error.
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

	clients, err := d.getTopClients(org.ID, 12, 10)
	if err != nil {
		t.Fatalf("getTopClients: %v", err)
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

	products, err := d.getTopProducts(org.ID, 12, 10)
	if err != nil {
		t.Fatalf("getTopProducts: %v", err)
	}
	if len(products) != 1 {
		t.Fatalf("got %d products, want 1 (productless line excluded): %+v", len(products), products)
	}
	if products[0].Name != "Widget" || products[0].Revenue != 3000 {
		t.Fatalf("products[0] = %+v, want Widget / 3000", products[0])
	}
}

// TestGetDashboardData is a thin smoke test confirming the composite entry
// point wires all five sub-queries together without error on a freshly
// migrated, empty database — the zero-invoices/zero-products case a real
// new organization starts in.
func TestGetDashboardData(t *testing.T) {
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-1"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}

	data, err := d.GetDashboardData(org.ID, 12)
	if err != nil {
		t.Fatalf("GetDashboardData: %v", err)
	}
	if len(data.RevenueByMonth) != 0 || data.Outstanding.Total != 0 ||
		data.StockValuation.Total != 0 || len(data.TopClients) != 0 || len(data.TopProducts) != 0 {
		t.Fatalf("expected all-empty result for a fresh org, got %+v", data)
	}
}
