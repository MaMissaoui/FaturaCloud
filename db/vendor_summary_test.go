package db

import (
	"database/sql"
	"errors"
	"testing"
	"time"
)

// TestVendorSummaries covers GetVendorSummaries (the batch list) and
// GetVendorSummary (the single-vendor detail): per-vendor outstanding,
// aggregation, aging buckets, bill stats, payment stats, foreign-currency
// exchange-rate conversion, the 409 when masterDataSummaries is off, and the
// sql.ErrNoRows a vendor in another org gets.
func TestVendorSummaries(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)

	// Organization uses TND so EUR bills exercise exchangeRate.
	org, err := d.CreateOrganization(CreateOrganizationRequest{
		ID:       "org-vs",
		Currency: ptr("TND"),
	})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	// masterDataSummaries defaults to true.
	if !org.MasterDataSummaries {
		t.Fatal("masterDataSummaries should default to true")
	}

	if _, err := d.CreateFiscalYear(CreateFiscalYearRequest{
		OrganizationID: org.ID, Name: "FY",
		StartDate: 1609459200000, // 2021-01-01
		EndDate:   1893456000000, // 2030-01-01
	}); err != nil {
		t.Fatalf("CreateFiscalYear: %v", err)
	}

	acme, err := d.CreateVendor(CreateVendorRequest{OrganizationID: org.ID, Name: ptr("Acme")})
	if err != nil {
		t.Fatalf("CreateVendor acme: %v", err)
	}
	global, err := d.CreateVendor(CreateVendorRequest{OrganizationID: org.ID, Name: ptr("Global")})
	if err != nil {
		t.Fatalf("CreateVendor global: %v", err)
	}

	// Tax rate with both input and output accounts (required for GL posting
	// on approve).
	accounts, err := d.GetAccounts(org.ID)
	if err != nil {
		t.Fatalf("GetAccounts: %v", err)
	}
	var inputTaxAcctID, outputTaxAcctID string
	for _, a := range accounts {
		switch a.Code {
		case "1200":
			inputTaxAcctID = a.ID
		case "2200":
			outputTaxAcctID = a.ID
		}
	}
	taxRate, err := d.CreateTaxRate(CreateTaxRateRequest{
		OrganizationID: org.ID, Name: "Zero", Percentage: 0,
		InputTaxAccountID: &inputTaxAcctID, OutputTaxAccountID: &outputTaxAcctID,
	})
	if err != nil {
		t.Fatalf("CreateTaxRate: %v", err)
	}

	nowMs := time.Now().UnixMilli()
	dueSoon := nowMs + 7*86400000  // 7 days from now
	duePast := nowMs - 45*86400000 // 45 days ago

	// --- Acme's EUR bill: approved, partially paid (500 EUR total, 200 EUR paid) ---
	// exchangeRate 3.4 means 1 EUR = 3.4 TND.
	eurRate := 3.4
	acmeBill1, err := d.CreateIncomingInvoice(CreateIncomingInvoiceRequest{
		OrganizationID: org.ID, VendorID: acme.ID, VendorInvoiceNumber: "BILL-A1",
		Date: nowMs - 60*86400000, DueDate: &dueSoon,
		Currency: "EUR", ExchangeRate: &eurRate,
		Total: 50000, SubTotal: 50000, TaxTotal: 0,
		LineItems: []CreateInvoiceLineItemRequest{
			{Quantity: 1, UnitPrice: 50000, TaxRate: &taxRate.ID},
		},
	})
	if err != nil {
		t.Fatalf("CreateIncomingInvoice acme 1: %v", err)
	}
	if _, err := d.UpdateIncomingInvoiceState(acmeBill1.ID, "approved"); err != nil {
		t.Fatalf("approve acme 1: %v", err)
	}
	// Outbound payment against acme 1 — 200 EUR, same exchangeRate.
	bank := accountByCode(t, d, org.ID, "1020")
	if _, err := d.CreatePayment(CreatePaymentRequest{
		OrganizationID: org.ID, Direction: "outbound", VendorID: &acme.ID,
		BankAccountID: bank.ID,
		Amount:        20000, Currency: "EUR", ExchangeRate: &eurRate,
		Date: nowMs - 30*86400000, Method: "bank_transfer",
		Applications: []CreatePaymentApplicationRequest{
			{DocumentType: "incoming_invoice", DocumentID: acmeBill1.ID, Amount: 20000},
		},
	}); err != nil {
		t.Fatalf("CreatePayment acme 1: %v", err)
	}

	// --- Acme's TND bill: approved, unpaid, overdue (300 TND) ---
	acmeBill2, err := d.CreateIncomingInvoice(CreateIncomingInvoiceRequest{
		OrganizationID: org.ID, VendorID: acme.ID, VendorInvoiceNumber: "BILL-A2",
		Date: nowMs - 60*86400000, DueDate: &duePast,
		Currency: "TND",
		Total:    30000, SubTotal: 30000, TaxTotal: 0,
		LineItems: []CreateInvoiceLineItemRequest{
			{Quantity: 1, UnitPrice: 30000, TaxRate: &taxRate.ID},
		},
	})
	if err != nil {
		t.Fatalf("CreateIncomingInvoice acme 2: %v", err)
	}
	if _, err := d.UpdateIncomingInvoiceState(acmeBill2.ID, "approved"); err != nil {
		t.Fatalf("approve acme 2: %v", err)
	}

	// --- Global's TND bill: approved, unpaid, current (100 TND) ---
	globalBill1, err := d.CreateIncomingInvoice(CreateIncomingInvoiceRequest{
		OrganizationID: org.ID, VendorID: global.ID, VendorInvoiceNumber: "BILL-G1",
		Date: nowMs - 5*86400000, DueDate: &dueSoon,
		Currency: "TND",
		Total:    10000, SubTotal: 10000, TaxTotal: 0,
		LineItems: []CreateInvoiceLineItemRequest{
			{Quantity: 1, UnitPrice: 10000, TaxRate: &taxRate.ID},
		},
	})
	if err != nil {
		t.Fatalf("CreateIncomingInvoice global 1: %v", err)
	}
	if _, err := d.UpdateIncomingInvoiceState(globalBill1.ID, "approved"); err != nil {
		t.Fatalf("approve global 1: %v", err)
	}

	// Expected outstanding balances in org currency (TND):
	// Acme A1: (50000 - 20000) * 3.4 = 102000 TND
	// Acme A2: 30000 * 1 = 30000 TND   (TND = org currency, NULL rate)
	// Global G1: 10000 * 1 = 10000 TND
	acmeOwed := int64(102000 + 30000) // 132000
	acmeOverdue := int64(30000)       // A2 only is overdue
	globalOwed := int64(10000)
	totalOwed := acmeOwed + globalOwed // 142000

	// --- GetVendorSummaries (list) ---
	list, err := d.GetVendorSummaries(org.ID)
	if err != nil {
		t.Fatalf("GetVendorSummaries: %v", err)
	}
	if list.TotalOwed != totalOwed {
		t.Fatalf("TotalOwed = %d, want %d", list.TotalOwed, totalOwed)
	}
	if list.OwingCount != 2 {
		t.Fatalf("OwingCount = %d, want 2", list.OwingCount)
	}
	if list.OverdueCount != 1 {
		t.Fatalf("OverdueCount = %d, want 1 (only Acme)", list.OverdueCount)
	}
	if len(list.Vendors) != 2 {
		t.Fatalf("len(Vendors) = %d, want 2", len(list.Vendors))
	}

	var acmeRow, globalRow VendorSummaryRow
	for _, v := range list.Vendors {
		switch v.VendorID {
		case acme.ID:
			acmeRow = v
		case global.ID:
			globalRow = v
		}
	}
	if acmeRow.Owed != acmeOwed {
		t.Fatalf("Acme Owed = %d, want %d (EUR converted)", acmeRow.Owed, acmeOwed)
	}
	if acmeRow.Overdue != acmeOverdue {
		t.Fatalf("Acme Overdue = %d, want %d", acmeRow.Overdue, acmeOverdue)
	}
	if acmeRow.BillCount != 2 {
		t.Fatalf("Acme BillCount = %d, want 2", acmeRow.BillCount)
	}
	if acmeRow.LastPurchase == nil {
		t.Fatal("Acme LastPurchase is nil")
	}
	if globalRow.Owed != globalOwed {
		t.Fatalf("Global Owed = %d, want %d", globalRow.Owed, globalOwed)
	}
	if globalRow.Overdue != 0 {
		t.Fatalf("Global Overdue = %d, want 0", globalRow.Overdue)
	}
	if globalRow.BillCount != 1 {
		t.Fatalf("Global BillCount = %d, want 1", globalRow.BillCount)
	}

	// sum(Vendors[].Owed) must equal TotalOwed.
	var sumOwed int64
	for _, v := range list.Vendors {
		sumOwed += v.Owed
	}
	if sumOwed != list.TotalOwed {
		t.Fatalf("sum(Vendors[].Owed) = %d, want %d", sumOwed, list.TotalOwed)
	}

	// TotalOwed must equal GetPayableAging(org).Total.
	aging, err := d.GetPayableAging(org.ID)
	if err != nil {
		t.Fatalf("GetPayableAging: %v", err)
	}
	if aging.Total != list.TotalOwed {
		t.Fatalf("aging.Total (%d) != list.TotalOwed (%d)", aging.Total, list.TotalOwed)
	}

	// --- GetVendorSummary (detail) for Acme ---
	summary, err := d.GetVendorSummary(org.ID, acme.ID)
	if err != nil {
		t.Fatalf("GetVendorSummary acme: %v", err)
	}
	if summary.VendorID != acme.ID {
		t.Fatalf("VendorID = %s, want %s", summary.VendorID, acme.ID)
	}
	if summary.Owed != acmeOwed {
		t.Fatalf("Owed = %d, want %d", summary.Owed, acmeOwed)
	}
	// Aging buckets: BILL-A2 is 45 days overdue (days31To60, 30000 TND),
	// BILL-A1 is current (102000 TND).
	if summary.Days31To60 != 30000 {
		t.Fatalf("Days31To60 = %d, want 30000", summary.Days31To60)
	}
	if summary.Current != 102000 {
		t.Fatalf("Current = %d, want 102000", summary.Current)
	}
	if len(summary.OpenBills) != 2 {
		t.Fatalf("len(OpenBills) = %d, want 2", len(summary.OpenBills))
	}
	// OpenBills sorted by dueDate ASC — A2 (overdue) first.
	if summary.OpenBills[0].ID != acmeBill2.ID {
		t.Fatalf("OpenBills[0].ID = %s, want %s", summary.OpenBills[0].ID, acmeBill2.ID)
	}
	if summary.OpenBills[0].VendorID != acme.ID {
		t.Fatalf("OpenBills[0].VendorID = %s, want %s", summary.OpenBills[0].VendorID, acme.ID)
	}
	if summary.OpenBills[0].Bucket != "days31To60" {
		t.Fatalf("OpenBills[0].Bucket = %q, want days31To60", summary.OpenBills[0].Bucket)
	}
	// EUR bill's OpenBills entry carries Currency, ForeignTotal and VendorID.
	eurBill := summary.OpenBills[1]
	if eurBill.ID != acmeBill1.ID {
		t.Fatalf("OpenBills[1].ID = %s, want %s", eurBill.ID, acmeBill1.ID)
	}
	if eurBill.VendorID != acme.ID {
		t.Fatalf("EUR bill VendorID = %s, want %s", eurBill.VendorID, acme.ID)
	}
	if eurBill.Currency != "EUR" {
		t.Fatalf("EUR bill Currency = %q, want EUR", eurBill.Currency)
	}
	if eurBill.ForeignTotal != 30000 {
		t.Fatalf("EUR bill ForeignTotal = %d, want 30000 (50000-20000)", eurBill.ForeignTotal)
	}
	if eurBill.Total != 102000 {
		t.Fatalf("EUR bill Total = %d, want 102000 (30000*3.4)", eurBill.Total)
	}

	if summary.BillCount != 2 {
		t.Fatalf("BillCount = %d, want 2", summary.BillCount)
	}
	// BilledTotal: sum of (total * COALESCE(exchangeRate,1)) over approved+paid bills.
	// A1: 50000*3.4 = 170000, A2: 30000*1 = 30000 → 200000.
	if summary.BilledTotal != 200000 {
		t.Fatalf("BilledTotal = %d, want 200000", summary.BilledTotal)
	}
	// PaidTotal: sum of (amount * COALESCE(exchangeRate,1)) over non-voided
	// outbound payments. 20000 EUR * 3.4 = 68000 TND.
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

	// --- GetVendorSummary for Global (simpler — org-currency only) ---
	globalSummary, err := d.GetVendorSummary(org.ID, global.ID)
	if err != nil {
		t.Fatalf("GetVendorSummary global: %v", err)
	}
	if globalSummary.Owed != globalOwed {
		t.Fatalf("Global Owed = %d, want %d", globalSummary.Owed, globalOwed)
	}
	if globalSummary.Current != 10000 {
		t.Fatalf("Global Current = %d, want 10000", globalSummary.Current)
	}
	if len(globalSummary.OpenBills) != 1 {
		t.Fatalf("Global len(OpenBills) = %d, want 1", len(globalSummary.OpenBills))
	}
	if globalSummary.OpenBills[0].Currency != "TND" {
		t.Fatalf("Global bill Currency = %q, want TND", globalSummary.OpenBills[0].Currency)
	}
	if globalSummary.OpenBills[0].ForeignTotal != 10000 {
		t.Fatalf("Global bill ForeignTotal = %d, want 10000", globalSummary.OpenBills[0].ForeignTotal)
	}
	if globalSummary.PaymentCount != 0 {
		t.Fatalf("Global PaymentCount = %d, want 0", globalSummary.PaymentCount)
	}
	if globalSummary.LastPayment != nil {
		t.Fatalf("Global LastPayment should be nil, got %+v", globalSummary.LastPayment)
	}
}

// TestVendorSummariesSwitchedOff verifies the 409 when masterDataSummaries
// is false.
func TestVendorSummariesSwitchedOff(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)

	org, err := d.CreateOrganization(CreateOrganizationRequest{
		ID: "org-vs-off", MasterDataSummaries: ptr(false),
	})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	if org.MasterDataSummaries {
		t.Fatal("masterDataSummaries should be false")
	}

	_, err = d.GetVendorSummaries(org.ID)
	if err == nil {
		t.Fatal("expected an error when summaries are switched off")
	}
	if _, ok := err.(*ValidationError); !ok {
		t.Fatalf("expected *ValidationError, got %T", err)
	}

	_, err = d.GetVendorSummary(org.ID, "nonexistent")
	if err == nil {
		t.Fatal("expected an error when summaries are switched off")
	}
	if _, ok := err.(*ValidationError); !ok {
		t.Fatalf("expected *ValidationError, got %T", err)
	}
}

// TestVendorSummariesFilterExclusions covers the bits the batch summary
// would otherwise hide behind its surface test: a draft bill is ignored, a
// cancelled bill is ignored, a paid bill contributes to BillCount/BilledTotal
// but not to Owed, and a voided payment is ignored when computing how much a
// bill has left to pay.
func TestVendorSummariesFilterExclusions(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)

	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-vs-filt", Currency: ptr("TND")})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	if _, err := d.CreateFiscalYear(CreateFiscalYearRequest{
		OrganizationID: org.ID, Name: "FY",
		StartDate: 1609459200000, EndDate: 1893456000000,
	}); err != nil {
		t.Fatalf("CreateFiscalYear: %v", err)
	}

	accounts, err := d.GetAccounts(org.ID)
	if err != nil {
		t.Fatalf("GetAccounts: %v", err)
	}
	var inputTaxAcctID, outputTaxAcctID string
	for _, a := range accounts {
		switch a.Code {
		case "1200":
			inputTaxAcctID = a.ID
		case "2200":
			outputTaxAcctID = a.ID
		}
	}
	taxRate, err := d.CreateTaxRate(CreateTaxRateRequest{
		OrganizationID: org.ID, Name: "Zero", Percentage: 0,
		InputTaxAccountID: &inputTaxAcctID, OutputTaxAccountID: &outputTaxAcctID,
	})
	if err != nil {
		t.Fatalf("CreateTaxRate: %v", err)
	}

	vendor, err := d.CreateVendor(CreateVendorRequest{OrganizationID: org.ID, Name: ptr("Vendor")})
	if err != nil {
		t.Fatalf("CreateVendor: %v", err)
	}

	nowMs := time.Now().UnixMilli()
	dueFuture := nowMs + 7*86400000

	// 1) Draft bill — state stays "draft", contributes nothing to Owed.
	if _, err := d.CreateIncomingInvoice(CreateIncomingInvoiceRequest{
		OrganizationID: org.ID, VendorID: vendor.ID, VendorInvoiceNumber: "BILL-DRAFT",
		Date: nowMs, DueDate: &dueFuture,
		Currency: "TND", Total: 7000, SubTotal: 7000, TaxTotal: 0,
		LineItems: []CreateInvoiceLineItemRequest{{Quantity: 1, UnitPrice: 7000, TaxRate: &taxRate.ID}},
	}); err != nil {
		t.Fatalf("CreateIncomingInvoice draft: %v", err)
	}

	// 2) Cancelled bill — contributes nothing to Owed.
	cancelled, err := d.CreateIncomingInvoice(CreateIncomingInvoiceRequest{
		OrganizationID: org.ID, VendorID: vendor.ID, VendorInvoiceNumber: "BILL-CXL",
		Date: nowMs, DueDate: &dueFuture,
		Currency: "TND", Total: 8000, SubTotal: 8000, TaxTotal: 0,
		LineItems: []CreateInvoiceLineItemRequest{{Quantity: 1, UnitPrice: 8000, TaxRate: &taxRate.ID}},
	})
	if err != nil {
		t.Fatalf("CreateIncomingInvoice cancelled: %v", err)
	}
	if _, err := d.UpdateIncomingInvoiceState(cancelled.ID, "approved"); err != nil {
		t.Fatalf("approve cancelled: %v", err)
	}
	if _, err := d.UpdateIncomingInvoiceState(cancelled.ID, "cancelled"); err != nil {
		t.Fatalf("cancel: %v", err)
	}

	// 3) Paid bill — BillCount and BilledTotal count it, Owed does not.
	paid, err := d.CreateIncomingInvoice(CreateIncomingInvoiceRequest{
		OrganizationID: org.ID, VendorID: vendor.ID, VendorInvoiceNumber: "BILL-PAID",
		Date: nowMs, DueDate: &dueFuture,
		Currency: "TND", Total: 9000, SubTotal: 9000, TaxTotal: 0,
		LineItems: []CreateInvoiceLineItemRequest{{Quantity: 1, UnitPrice: 9000, TaxRate: &taxRate.ID}},
	})
	if err != nil {
		t.Fatalf("CreateIncomingInvoice paid: %v", err)
	}
	if _, err := d.UpdateIncomingInvoiceState(paid.ID, "approved"); err != nil {
		t.Fatalf("approve paid: %v", err)
	}
	if _, err := d.UpdateIncomingInvoiceState(paid.ID, "paid"); err != nil {
		t.Fatalf("mark paid: %v", err)
	}

	// 4) Approved bill with a partial payment that we then void — owed must
	//    come back to the full pre-payment amount (voided payments ignored).
	voidedBill, err := d.CreateIncomingInvoice(CreateIncomingInvoiceRequest{
		OrganizationID: org.ID, VendorID: vendor.ID, VendorInvoiceNumber: "BILL-VOID",
		Date: nowMs, DueDate: &dueFuture,
		Currency: "TND", Total: 20000, SubTotal: 20000, TaxTotal: 0,
		LineItems: []CreateInvoiceLineItemRequest{{Quantity: 1, UnitPrice: 20000, TaxRate: &taxRate.ID}},
	})
	if err != nil {
		t.Fatalf("CreateIncomingInvoice void: %v", err)
	}
	if _, err := d.UpdateIncomingInvoiceState(voidedBill.ID, "approved"); err != nil {
		t.Fatalf("approve void: %v", err)
	}
	bank := accountByCode(t, d, org.ID, "1020")
	pay, err := d.CreatePayment(CreatePaymentRequest{
		OrganizationID: org.ID, Direction: "outbound", VendorID: &vendor.ID,
		BankAccountID: bank.ID,
		Amount:        5000, Currency: "TND",
		Date: nowMs, Method: "bank_transfer",
		Applications: []CreatePaymentApplicationRequest{
			{DocumentType: "incoming_invoice", DocumentID: voidedBill.ID, Amount: 5000},
		},
	})
	if err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}
	if _, err := d.VoidPayment(pay.ID, nowMs); err != nil {
		t.Fatalf("VoidPayment: %v", err)
	}

	list, err := d.GetVendorSummaries(org.ID)
	if err != nil {
		t.Fatalf("GetVendorSummaries: %v", err)
	}
	if len(list.Vendors) != 1 {
		t.Fatalf("len(Vendors) = %d, want 1 (paid/cancelled/draft still surface the vendor)", len(list.Vendors))
	}
	row := list.Vendors[0]
	// Only the voided bill is owed: 20000 - 0 (voided) = 20000.
	if row.Owed != 20000 {
		t.Fatalf("Owed = %d, want 20000 (draft/cancelled/paid excluded, voided payment ignored)", row.Owed)
	}
	if row.Overdue != 0 {
		t.Fatalf("Overdue = %d, want 0 (only future-due bill)", row.Overdue)
	}
	// BillCount covers approved+paid (paid + voided = 2); draft and cancelled
	// don't.
	if row.BillCount != 2 {
		t.Fatalf("BillCount = %d, want 2 (paid + approved-with-voided-payment)", row.BillCount)
	}

	summary, err := d.GetVendorSummary(org.ID, vendor.ID)
	if err != nil {
		t.Fatalf("GetVendorSummary: %v", err)
	}
	// OpenBills = the one bill whose remaining balance is still > 0.
	if len(summary.OpenBills) != 1 {
		t.Fatalf("len(OpenBills) = %d, want 1 (paid/cancelled excluded)", len(summary.OpenBills))
	}
	if summary.OpenBills[0].ID != voidedBill.ID {
		t.Fatalf("OpenBills[0].ID = %s, want %s", summary.OpenBills[0].ID, voidedBill.ID)
	}
	// BilledTotal = paid (9000) + approved-with-voided (20000) = 29000.
	if summary.BilledTotal != 29000 {
		t.Fatalf("BilledTotal = %d, want 29000", summary.BilledTotal)
	}
	// PaidTotal = 0 — the only payment was voided.
	if summary.PaidTotal != 0 {
		t.Fatalf("PaidTotal = %d, want 0 (voided payment)", summary.PaidTotal)
	}
	if summary.PaymentCount != 0 {
		t.Fatalf("PaymentCount = %d, want 0 (voided payment)", summary.PaymentCount)
	}
	if summary.LastPayment != nil {
		t.Fatalf("LastPayment = %+v, want nil (only payment was voided)", summary.LastPayment)
	}
}

// TestVendorSummaryCrossOrgReturnsErrNoRows is the sql.ErrNoRows guard for
// a vendor from another organization.
func TestVendorSummaryCrossOrgReturnsErrNoRows(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)

	orgA, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-a"})
	if err != nil {
		t.Fatalf("CreateOrganization A: %v", err)
	}
	orgB, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-b"})
	if err != nil {
		t.Fatalf("CreateOrganization B: %v", err)
	}

	vendorA, err := d.CreateVendor(CreateVendorRequest{OrganizationID: orgA.ID, Name: ptr("VendorA")})
	if err != nil {
		t.Fatalf("CreateVendor A: %v", err)
	}

	_, err = d.GetVendorSummary(orgB.ID, vendorA.ID)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("GetVendorSummary(cross-org) = %v, want sql.ErrNoRows", err)
	}

	// With summaries switched off, the same call returns the validation
	// error first — the cross-org check has to come after the setting
	// check, otherwise a denied caller can't distinguish "your org has
	// summaries off" from "this vendor belongs to someone else".
	if _, err := d.UpdateOrganization(orgB.ID, UpdateOrganizationRequest{MasterDataSummaries: ptr(false)}); err != nil {
		t.Fatalf("switch off: %v", err)
	}
	_, err = d.GetVendorSummary(orgB.ID, vendorA.ID)
	if _, ok := err.(*ValidationError); !ok {
		t.Fatalf("GetVendorSummary(cross-org, summaries off) = %v, want *ValidationError", err)
	}
}

// TestVendorSummaryLargeTotals guards the summary's sums against SQLite
// returning them as REAL: from a million cents on, database/sql renders a
// float64 like 3.73409e+06 and refuses to scan it into an int64, which made
// GET /api/clients/{id}/summary answer 500 for any client who had paid more
// than 10 000 in total. Same shape on the vendor side, so pay out more than
// 10 000 000 cents here and prove the sums scan without error.
func TestVendorSummaryLargeTotals(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)

	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-vs-big", Currency: ptr("TND")})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	if _, err := d.CreateFiscalYear(CreateFiscalYearRequest{
		OrganizationID: org.ID, Name: "FY",
		StartDate: 1609459200000, EndDate: 1893456000000,
	}); err != nil {
		t.Fatalf("CreateFiscalYear: %v", err)
	}
	vendor, err := d.CreateVendor(CreateVendorRequest{OrganizationID: org.ID, Name: ptr("Big vendor")})
	if err != nil {
		t.Fatalf("CreateVendor: %v", err)
	}
	accounts, err := d.GetAccounts(org.ID)
	if err != nil {
		t.Fatalf("GetAccounts: %v", err)
	}
	var inputTaxAcctID, outputTaxAcctID string
	for _, a := range accounts {
		switch a.Code {
		case "1200":
			inputTaxAcctID = a.ID
		case "2200":
			outputTaxAcctID = a.ID
		}
	}
	taxRate, err := d.CreateTaxRate(CreateTaxRateRequest{
		OrganizationID: org.ID, Name: "Zero", Percentage: 0,
		InputTaxAccountID: &inputTaxAcctID, OutputTaxAccountID: &outputTaxAcctID,
	})
	if err != nil {
		t.Fatalf("CreateTaxRate: %v", err)
	}

	nowMs := time.Now().UnixMilli()
	dueFuture := nowMs + 7*86400000
	inv, err := d.CreateIncomingInvoice(CreateIncomingInvoiceRequest{
		OrganizationID: org.ID, VendorID: vendor.ID, VendorInvoiceNumber: "BILL-BIG",
		Date: nowMs - 10*86400000, DueDate: &dueFuture, Currency: "TND",
		Total: 3734090, SubTotal: 3734090, TaxTotal: 0,
		LineItems: []CreateInvoiceLineItemRequest{{Quantity: 1, UnitPrice: 3734090, TaxRate: &taxRate.ID}},
	})
	if err != nil {
		t.Fatalf("CreateIncomingInvoice: %v", err)
	}
	if _, err := d.UpdateIncomingInvoiceState(inv.ID, "approved"); err != nil {
		t.Fatalf("approve: %v", err)
	}
	bank := accountByCode(t, d, org.ID, "1020")
	if _, err := d.CreatePayment(CreatePaymentRequest{
		OrganizationID: org.ID, Direction: "outbound", VendorID: &vendor.ID,
		BankAccountID: bank.ID,
		Amount:        3000000, Currency: "TND", Date: nowMs - 86400000, Method: "cash",
		Applications: []CreatePaymentApplicationRequest{
			{DocumentType: "incoming_invoice", DocumentID: inv.ID, Amount: 3000000},
		},
	}); err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}

	s, err := d.GetVendorSummary(org.ID, vendor.ID)
	if err != nil {
		t.Fatalf("GetVendorSummary: %v", err)
	}
	if s.PaidTotal != 3000000 || s.BilledTotal != 3734090 || s.Owed != 734090 {
		t.Fatalf("paid %d billed %d owed %d, want 3000000 3734090 734090", s.PaidTotal, s.BilledTotal, s.Owed)
	}
}
