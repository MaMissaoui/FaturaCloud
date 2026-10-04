package db

import (
	"testing"
	"time"
)

// TestClientSummaries covers GetClientSummaries (the batch list) and
// GetClientSummary (the single-client detail): per-client outstanding,
// aggregation, aging buckets, invoice stats, payment stats, foreign-currency
// exchange-rate conversion, and the 409 when masterDataSummaries is off.
func TestClientSummaries(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)

	// Organization uses TND so that EUR invoices exercise exchangeRate.
	org, err := d.CreateOrganization(CreateOrganizationRequest{
		ID:       "org-cs",
		Currency: ptr("TND"),
	})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	// masterDataSummaries defaults to true.
	if !org.MasterDataSummaries {
		t.Fatal("masterDataSummaries should default to true")
	}

	// A fiscal year covering the invoice dates — use a wide range so dates
	// are always in scope regardless of when the test runs.
	if _, err := d.CreateFiscalYear(CreateFiscalYearRequest{
		OrganizationID: org.ID, Name: "FY",
		StartDate: 1609459200000, // 2021-01-01
		EndDate:   1893456000000, // 2030-01-01
	}); err != nil {
		t.Fatalf("CreateFiscalYear: %v", err)
	}

	alice, err := d.CreateClient(CreateClientRequest{OrganizationID: org.ID, Name: ptr("Alice")})
	if err != nil {
		t.Fatalf("CreateClient alice: %v", err)
	}
	bob, err := d.CreateClient(CreateClientRequest{OrganizationID: org.ID, Name: ptr("Bob")})
	if err != nil {
		t.Fatalf("CreateClient bob: %v", err)
	}

	// Tax rate with output account (required for GL posting on send).
	accounts, err := d.GetAccounts(org.ID)
	if err != nil {
		t.Fatalf("GetAccounts: %v", err)
	}
	var outputTaxAcctID string
	for _, a := range accounts {
		if a.Code == "2200" {
			outputTaxAcctID = a.ID
			break
		}
	}
	taxRate, err := d.CreateTaxRate(CreateTaxRateRequest{
		OrganizationID: org.ID, Name: "Zero", Percentage: 0,
		OutputTaxAccountID: &outputTaxAcctID,
	})
	if err != nil {
		t.Fatalf("CreateTaxRate: %v", err)
	}

	nowMs := time.Now().UnixMilli()
	dueSoon := nowMs + 7*86400000  // 7 days from now
	duePast := nowMs - 45*86400000 // 45 days ago

	// --- Alice's EUR invoice: sent, partially paid (500 EUR total, 200 EUR paid) ---
	// exchangeRate 3.4 means 1 EUR = 3.4 TND.
	eurRate := 3.4
	invA1, err := d.CreateInvoice(CreateInvoiceRequest{
		OrganizationID: org.ID, Number: "INV-A1", ClientID: alice.ID,
		Date: nowMs - 60*86400000, DueDate: &dueSoon,
		Currency: "EUR", ExchangeRate: &eurRate,
		Total: 50000, SubTotal: 50000, TaxTotal: 0,
		LineItems: []CreateInvoiceLineItemRequest{
			{Quantity: 1, UnitPrice: 50000, TaxRate: &taxRate.ID},
		},
	})
	if err != nil {
		t.Fatalf("CreateInvoice A1: %v", err)
	}
	if _, err := d.UpdateInvoiceState(invA1.ID, "sent"); err != nil {
		t.Fatalf("send A1: %v", err)
	}
	// Payment against A1 — 200 EUR, same exchangeRate.
	if _, err := d.CreatePayment(CreatePaymentRequest{
		OrganizationID: org.ID, Direction: "inbound", ClientID: &alice.ID,
		BankAccountID: *org.DefaultCashAccountID,
		Amount:        20000, Currency: "EUR", ExchangeRate: &eurRate,
		Date: nowMs - 30*86400000, Method: "bank_transfer",
		Applications: []CreatePaymentApplicationRequest{
			{DocumentType: "invoice", DocumentID: invA1.ID, Amount: 20000},
		},
	}); err != nil {
		t.Fatalf("CreatePayment A1: %v", err)
	}

	// --- Alice's TND invoice: sent, unpaid, overdue (300 TND) ---
	invA2, err := d.CreateInvoice(CreateInvoiceRequest{
		OrganizationID: org.ID, Number: "INV-A2", ClientID: alice.ID,
		Date: nowMs - 60*86400000, DueDate: &duePast,
		Currency: "TND",
		Total:    30000, SubTotal: 30000, TaxTotal: 0,
		LineItems: []CreateInvoiceLineItemRequest{
			{Quantity: 1, UnitPrice: 30000, TaxRate: &taxRate.ID},
		},
	})
	if err != nil {
		t.Fatalf("CreateInvoice A2: %v", err)
	}
	if _, err := d.UpdateInvoiceState(invA2.ID, "sent"); err != nil {
		t.Fatalf("send A2: %v", err)
	}

	// --- Bob's TND invoice: sent, unpaid, current (100 TND) ---
	invB1, err := d.CreateInvoice(CreateInvoiceRequest{
		OrganizationID: org.ID, Number: "INV-B1", ClientID: bob.ID,
		Date: nowMs - 5*86400000, DueDate: &dueSoon,
		Currency: "TND",
		Total:    10000, SubTotal: 10000, TaxTotal: 0,
		LineItems: []CreateInvoiceLineItemRequest{
			{Quantity: 1, UnitPrice: 10000, TaxRate: &taxRate.ID},
		},
	})
	if err != nil {
		t.Fatalf("CreateInvoice B1: %v", err)
	}
	if _, err := d.UpdateInvoiceState(invB1.ID, "sent"); err != nil {
		t.Fatalf("send B1: %v", err)
	}

	// Expected outstanding balances in org currency (TND):
	// Alice A1: (50000 - 20000) * 3.4 = 102000 TND
	// Alice A2: 30000 * 1 = 30000 TND   (TND = org currency, NULL rate)
	// Bob B1:   10000 * 1 = 10000 TND
	aliceOwed := int64(102000 + 30000) // 132000
	bobOwed := int64(10000)
	totalOwed := aliceOwed + bobOwed // 142000

	// --- GetClientSummaries (list) ---
	list, err := d.GetClientSummaries(org.ID)
	if err != nil {
		t.Fatalf("GetClientSummaries: %v", err)
	}
	if list.TotalOwed != totalOwed {
		t.Fatalf("TotalOwed = %d, want %d", list.TotalOwed, totalOwed)
	}
	if list.OwingCount != 2 {
		t.Fatalf("OwingCount = %d, want 2", list.OwingCount)
	}
	if len(list.Clients) != 2 {
		t.Fatalf("len(Clients) = %d, want 2", len(list.Clients))
	}

	// Find per-client rows.
	var aliceRow, bobRow ClientSummaryRow
	for _, c := range list.Clients {
		switch c.ClientID {
		case alice.ID:
			aliceRow = c
		case bob.ID:
			bobRow = c
		}
	}
	if aliceRow.Owed != aliceOwed {
		t.Fatalf("Alice Owed = %d, want %d (EUR converted)", aliceRow.Owed, aliceOwed)
	}
	if aliceRow.InvoiceCount != 2 {
		t.Fatalf("Alice InvoiceCount = %d, want 2", aliceRow.InvoiceCount)
	}
	if aliceRow.LastPurchase == nil {
		t.Fatal("Alice LastPurchase is nil")
	}
	if bobRow.Owed != bobOwed {
		t.Fatalf("Bob Owed = %d, want %d", bobRow.Owed, bobOwed)
	}
	if bobRow.InvoiceCount != 1 {
		t.Fatalf("Bob InvoiceCount = %d, want 1", bobRow.InvoiceCount)
	}

	// sum(Clients[].Owed) must equal TotalOwed.
	var sumOwed int64
	for _, c := range list.Clients {
		sumOwed += c.Owed
	}
	if sumOwed != list.TotalOwed {
		t.Fatalf("sum(Clients[].Owed) = %d, want %d", sumOwed, list.TotalOwed)
	}

	// TotalOwed must equal GetReceivableAging(org).Total.
	aging, err := d.GetReceivableAging(org.ID)
	if err != nil {
		t.Fatalf("GetReceivableAging: %v", err)
	}
	if aging.Total != list.TotalOwed {
		t.Fatalf("aging.Total (%d) != list.TotalOwed (%d)", aging.Total, list.TotalOwed)
	}

	// --- GetClientSummary (detail) for Alice ---
	summary, err := d.GetClientSummary(org.ID, alice.ID)
	if err != nil {
		t.Fatalf("GetClientSummary alice: %v", err)
	}
	if summary.ClientID != alice.ID {
		t.Fatalf("ClientID = %s, want %s", summary.ClientID, alice.ID)
	}
	if summary.Owed != aliceOwed {
		t.Fatalf("Owed = %d, want %d", summary.Owed, aliceOwed)
	}
	// Aging buckets: INV-A2 is 45 days overdue (days31To60, 30000 TND),
	// INV-A1 is current (102000 TND).
	if summary.Days31To60 != 30000 {
		t.Fatalf("Days31To60 = %d, want 30000", summary.Days31To60)
	}
	if summary.Current != 102000 {
		t.Fatalf("Current = %d, want 102000", summary.Current)
	}
	if len(summary.OpenInvoices) != 2 {
		t.Fatalf("len(OpenInvoices) = %d, want 2", len(summary.OpenInvoices))
	}
	// OpenInvoices sorted by dueDate ASC — A2 (overdue) first.
	if summary.OpenInvoices[0].ID != invA2.ID {
		t.Fatalf("OpenInvoices[0].ID = %s, want %s", summary.OpenInvoices[0].ID, invA2.ID)
	}
	// EUR invoice's OpenInvoices entry carries Currency and ForeignTotal.
	eurInv := summary.OpenInvoices[1]
	if eurInv.ID != invA1.ID {
		t.Fatalf("OpenInvoices[1].ID = %s, want %s", eurInv.ID, invA1.ID)
	}
	if eurInv.Currency != "EUR" {
		t.Fatalf("EUR invoice Currency = %q, want %q", eurInv.Currency, "EUR")
	}
	if eurInv.ForeignTotal != 30000 {
		t.Fatalf("EUR invoice ForeignTotal = %d, want 30000 (50000-20000)", eurInv.ForeignTotal)
	}
	if eurInv.Total != 102000 {
		t.Fatalf("EUR invoice Total = %d, want 102000 (30000*3.4)", eurInv.Total)
	}

	if summary.InvoiceCount != 2 {
		t.Fatalf("InvoiceCount = %d, want 2", summary.InvoiceCount)
	}
	// BilledTotal: sum of (total * COALESCE(exchangeRate,1)) over sent+paid invoices.
	// A1: 50000*3.4 = 170000, A2: 30000*1 = 30000 → 200000.
	if summary.BilledTotal != 200000 {
		t.Fatalf("BilledTotal = %d, want 200000", summary.BilledTotal)
	}
	// PaidTotal: sum of (amount * COALESCE(exchangeRate,1)) over non-voided
	// inbound payments. 20000 EUR * 3.4 = 68000 TND.
	if summary.PaidTotal != 68000 {
		t.Fatalf("PaidTotal = %d, want 68000", summary.PaidTotal)
	}
	if summary.PaymentCount != 1 {
		t.Fatalf("PaymentCount = %d, want 1", summary.PaymentCount)
	}
	if summary.LastPayment == nil {
		t.Fatal("LastPayment is nil")
	}
	if summary.LastPayment.Amount != 68000 {
		t.Fatalf("LastPayment.Amount = %d, want 68000", summary.LastPayment.Amount)
	}
	if summary.LastPayment.Method != "bank_transfer" {
		t.Fatalf("LastPayment.Method = %s, want bank_transfer", summary.LastPayment.Method)
	}

	// --- GetClientSummary for Bob (simpler — org-currency only) ---
	bobSummary, err := d.GetClientSummary(org.ID, bob.ID)
	if err != nil {
		t.Fatalf("GetClientSummary bob: %v", err)
	}
	if bobSummary.Owed != bobOwed {
		t.Fatalf("Bob Owed = %d, want %d", bobSummary.Owed, bobOwed)
	}
	if bobSummary.Current != 10000 {
		t.Fatalf("Bob Current = %d, want 10000", bobSummary.Current)
	}
	if len(bobSummary.OpenInvoices) != 1 {
		t.Fatalf("Bob len(OpenInvoices) = %d, want 1", len(bobSummary.OpenInvoices))
	}
	if bobSummary.OpenInvoices[0].Currency != "TND" {
		t.Fatalf("Bob invoice Currency = %q, want %q", bobSummary.OpenInvoices[0].Currency, "TND")
	}
	if bobSummary.OpenInvoices[0].ForeignTotal != 10000 {
		t.Fatalf("Bob invoice ForeignTotal = %d, want 10000", bobSummary.OpenInvoices[0].ForeignTotal)
	}
	if bobSummary.PaymentCount != 0 {
		t.Fatalf("Bob PaymentCount = %d, want 0", bobSummary.PaymentCount)
	}
	if bobSummary.LastPayment != nil {
		t.Fatalf("Bob LastPayment should be nil, got %+v", bobSummary.LastPayment)
	}
}

// TestClientSummariesSwitchedOff verifies the 409 when masterDataSummaries
// is false.
func TestClientSummariesSwitchedOff(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)

	org, err := d.CreateOrganization(CreateOrganizationRequest{
		ID: "org-off", MasterDataSummaries: ptr(false),
	})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	if org.MasterDataSummaries {
		t.Fatal("masterDataSummaries should be false")
	}

	_, err = d.GetClientSummaries(org.ID)
	if err == nil {
		t.Fatal("expected an error when summaries are switched off")
	}
	if _, ok := err.(*ValidationError); !ok {
		t.Fatalf("expected *ValidationError, got %T", err)
	}

	_, err = d.GetClientSummary(org.ID, "nonexistent")
	if err == nil {
		t.Fatal("expected an error when summaries are switched off")
	}
	if _, ok := err.(*ValidationError); !ok {
		t.Fatalf("expected *ValidationError, got %T", err)
	}
}
