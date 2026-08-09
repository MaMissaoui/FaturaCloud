package db

import (
	"testing"
	"time"
)

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
	// The costed product below already has a UnitCost, so its uncosted
	// stock movement still resolves a cost basis (falls back to the
	// product's average) and posts a GL entry dated time.Now() — needs an
	// open fiscal year covering today.
	if _, err := d.CreateFiscalYear(CreateFiscalYearRequest{
		OrganizationID: org.ID, Name: "2023-2099", StartDate: 1672531200000, EndDate: 4102444799000,
	}); err != nil {
		t.Fatalf("CreateFiscalYear: %v", err)
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
