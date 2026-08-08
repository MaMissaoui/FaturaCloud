package db

import "testing"

func TestGetProfitAndLoss(t *testing.T) {
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-pl")

	inv := fx.createInvoice(t, d, "inv-1", 2, 1000) // 2000 revenue, 400 tax
	if _, err := d.UpdateInvoiceState(inv.ID, "sent"); err != nil {
		t.Fatalf("UpdateInvoiceState(sent): %v", err)
	}
	bill := fx.createIncomingInvoice(t, d, "bill-1", 1, 1200) // 1200 expense, 240 tax
	if _, err := d.UpdateIncomingInvoiceState(bill.ID, "approved"); err != nil {
		t.Fatalf("UpdateIncomingInvoiceState(approved): %v", err)
	}

	// A document dated well outside the report window must not appear.
	outsideInv := fx.createInvoice(t, d, "inv-outside", 1, 5000)
	if _, err := d.DB.Exec(`UPDATE invoices SET date = ? WHERE id = ?`, fx.date-20*86400000, outsideInv.ID); err != nil {
		t.Fatalf("backdate invoice: %v", err)
	}
	if _, err := d.UpdateInvoiceState(outsideInv.ID, "sent"); err != nil {
		t.Fatalf("UpdateInvoiceState(sent) outside: %v", err)
	}

	pl, err := d.GetProfitAndLoss(fx.orgID, fx.date-86400000, fx.date+86400000)
	if err != nil {
		t.Fatalf("GetProfitAndLoss: %v", err)
	}
	if pl.TotalRevenue != 2000 {
		t.Fatalf("TotalRevenue = %d, want 2000", pl.TotalRevenue)
	}
	if pl.TotalExpenses != 1200 {
		t.Fatalf("TotalExpenses = %d, want 1200", pl.TotalExpenses)
	}
	if pl.NetIncome != 800 {
		t.Fatalf("NetIncome = %d, want 800", pl.NetIncome)
	}
	if len(pl.Revenue) != 1 || pl.Revenue[0].AccountID != fx.revenueAccountID {
		t.Fatalf("unexpected revenue lines: %+v", pl.Revenue)
	}
	if len(pl.Expenses) != 1 || pl.Expenses[0].AccountID != fx.expenseAccountID {
		t.Fatalf("unexpected expense lines: %+v", pl.Expenses)
	}
}

// TestGetProfitAndLossAndBalanceSheetNeverReturnNilSlices is the regression
// test for a bug caught live in the browser: an org/date range with no
// equity activity left BalanceSheet.Equity as a nil Go slice, which
// json.Marshal encodes as `null` rather than `[]` — the frontend's
// `[...report.equity, ...]` then threw "report.equity is not iterable".
// Every list field must always encode as an array, empty or not.
func TestGetProfitAndLossAndBalanceSheetNeverReturnNilSlices(t *testing.T) {
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-reports-empty")

	pl, err := d.GetProfitAndLoss(fx.orgID, fx.date, fx.date)
	if err != nil {
		t.Fatalf("GetProfitAndLoss: %v", err)
	}
	if pl.Revenue == nil || pl.Expenses == nil {
		t.Fatalf("GetProfitAndLoss returned a nil slice: revenue=%v expenses=%v", pl.Revenue, pl.Expenses)
	}

	bs, err := d.GetBalanceSheet(fx.orgID, fx.date)
	if err != nil {
		t.Fatalf("GetBalanceSheet: %v", err)
	}
	if bs.Assets == nil || bs.Liabilities == nil || bs.Equity == nil {
		t.Fatalf("GetBalanceSheet returned a nil slice: assets=%v liabilities=%v equity=%v",
			bs.Assets, bs.Liabilities, bs.Equity)
	}
}

func TestGetProfitAndLossRejectsInvertedRange(t *testing.T) {
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-pl-invalid")
	if _, err := d.GetProfitAndLoss(fx.orgID, fx.date, fx.date-1); err == nil {
		t.Fatal("expected an inverted date range to be rejected")
	}
}

// The balance sheet must balance by construction: TotalAssets ==
// TotalLiabilities + TotalEquity, with CurrentEarnings folded into equity
// since nothing has been closed to retained earnings yet (Phase 6). This is
// the fundamental accounting identity, not a coincidence of this fixture —
// it holds because every journal entry balances debit=credit across every
// account including revenue/expense.
func TestGetBalanceSheetBalances(t *testing.T) {
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-bs")

	inv := fx.createInvoice(t, d, "inv-1", 2, 1000) // total 2400
	if _, err := d.UpdateInvoiceState(inv.ID, "sent"); err != nil {
		t.Fatalf("UpdateInvoiceState(sent): %v", err)
	}
	bill := fx.createIncomingInvoice(t, d, "bill-1", 1, 1200) // total 1440
	if _, err := d.UpdateIncomingInvoiceState(bill.ID, "approved"); err != nil {
		t.Fatalf("UpdateIncomingInvoiceState(approved): %v", err)
	}
	bank := accountByCode(t, d, fx.orgID, "1020")
	if _, err := d.CreatePayment(CreatePaymentRequest{
		OrganizationID: fx.orgID, Direction: "inbound", ClientID: &fx.clientID,
		BankAccountID: bank.ID, Amount: 1000, Currency: "EUR", Date: fx.date, Method: "bank_transfer",
		Applications: []CreatePaymentApplicationRequest{{DocumentType: "invoice", DocumentID: inv.ID, Amount: 1000}},
	}); err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}

	bs, err := d.GetBalanceSheet(fx.orgID, fx.date+86400000)
	if err != nil {
		t.Fatalf("GetBalanceSheet: %v", err)
	}
	if bs.TotalAssets != bs.TotalLiabilities+bs.TotalEquity {
		t.Fatalf("balance sheet does not balance: assets %d, liabilities+equity %d",
			bs.TotalAssets, bs.TotalLiabilities+bs.TotalEquity)
	}
	if bs.CurrentEarnings != 2000-1200 {
		t.Fatalf("CurrentEarnings = %d, want %d", bs.CurrentEarnings, 2000-1200)
	}

	var arAmount int64
	for _, l := range bs.Assets {
		if l.AccountID == fx.arAccountID {
			arAmount = l.Amount
		}
	}
	if arAmount != 2400-1000 {
		t.Fatalf("AR balance = %d, want %d", arAmount, 2400-1000)
	}
}

// TestReceivableAgingReflectsPartialPayment is the regression test for the
// bug caught live in the browser: before Phase 3 payments existed, a 'sent'
// invoice's outstanding amount was always its full total. A real partial
// payment must reduce it, and a fully-paid invoice must drop off the list
// entirely even though its state is still 'sent' (state is a free,
// payment-independent flag — see CLAUDE.md).
func TestReceivableAgingReflectsPartialPayment(t *testing.T) {
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-ar-aging")

	partial := fx.createInvoice(t, d, "inv-partial", 1, 1000) // total 1200
	if _, err := d.UpdateInvoiceState(partial.ID, "sent"); err != nil {
		t.Fatalf("UpdateInvoiceState(sent): %v", err)
	}
	full := fx.createInvoice(t, d, "inv-full", 1, 1000) // total 1200
	if _, err := d.UpdateInvoiceState(full.ID, "sent"); err != nil {
		t.Fatalf("UpdateInvoiceState(sent) full: %v", err)
	}
	bank := accountByCode(t, d, fx.orgID, "1020")
	if _, err := d.CreatePayment(CreatePaymentRequest{
		OrganizationID: fx.orgID, Direction: "inbound", ClientID: &fx.clientID,
		BankAccountID: bank.ID, Amount: 500, Currency: "EUR", Date: fx.date, Method: "bank_transfer",
		Applications: []CreatePaymentApplicationRequest{{DocumentType: "invoice", DocumentID: partial.ID, Amount: 500}},
	}); err != nil {
		t.Fatalf("CreatePayment(partial): %v", err)
	}
	if _, err := d.CreatePayment(CreatePaymentRequest{
		OrganizationID: fx.orgID, Direction: "inbound", ClientID: &fx.clientID,
		BankAccountID: bank.ID, Amount: 1200, Currency: "EUR", Date: fx.date, Method: "bank_transfer",
		Applications: []CreatePaymentApplicationRequest{{DocumentType: "invoice", DocumentID: full.ID, Amount: 1200}},
	}); err != nil {
		t.Fatalf("CreatePayment(full): %v", err)
	}

	summary, err := d.GetReceivableAging(fx.orgID)
	if err != nil {
		t.Fatalf("GetReceivableAging: %v", err)
	}
	if len(summary.Invoices) != 1 || summary.Invoices[0].ID != partial.ID {
		t.Fatalf("expected only the partially-paid invoice, got %+v", summary.Invoices)
	}
	if summary.Invoices[0].Total != 700 {
		t.Fatalf("remaining balance = %d, want 700 (1200 - 500)", summary.Invoices[0].Total)
	}
}

func TestGetPayableAging(t *testing.T) {
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-ap-aging")

	bill := fx.createIncomingInvoice(t, d, "bill-1", 1, 1000) // total 1200
	if _, err := d.UpdateIncomingInvoiceState(bill.ID, "approved"); err != nil {
		t.Fatalf("UpdateIncomingInvoiceState(approved): %v", err)
	}
	bank := accountByCode(t, d, fx.orgID, "1020")
	if _, err := d.CreatePayment(CreatePaymentRequest{
		OrganizationID: fx.orgID, Direction: "outbound", VendorID: &fx.vendorID,
		BankAccountID: bank.ID, Amount: 200, Currency: "EUR", Date: fx.date, Method: "bank_transfer",
		Applications: []CreatePaymentApplicationRequest{{DocumentType: "incoming_invoice", DocumentID: bill.ID, Amount: 200}},
	}); err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}

	summary, err := d.GetPayableAging(fx.orgID)
	if err != nil {
		t.Fatalf("GetPayableAging: %v", err)
	}
	if len(summary.Bills) != 1 || summary.Bills[0].ID != bill.ID {
		t.Fatalf("expected exactly the approved bill, got %+v", summary.Bills)
	}
	if summary.Bills[0].Total != 1000 {
		t.Fatalf("remaining balance = %d, want 1000 (1200 - 200)", summary.Bills[0].Total)
	}
	if summary.Bills[0].VendorName == "" {
		t.Fatal("expected the joined vendor name")
	}
}
