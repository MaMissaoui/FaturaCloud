package db

import (
	"errors"
	"testing"
	"time"
)

// A paper loan from mid-2024 (before the fixture's only fiscal year, 2025),
// brought forward at the fixture's 2025-02-01 cutover.
var openingLoanSaleDate = time.Date(2024, 6, 15, 10, 0, 0, 0, time.UTC).UnixMilli()

func openingLoanRequest(fx glPostingTestFixture, number string, paid int64) OpeningLoanRequest {
	return OpeningLoanRequest{
		OrganizationID: fx.orgID,
		ClientID:       fx.clientID,
		Number:         number,
		Date:           openingLoanSaleDate,
		CutoverDate:    fx.date,
		Lines: []OpeningLoanLine{
			{ProductID: &fx.productID, Quantity: 1, Amount: 60000},
			{Description: "Washing machine (free text)", Quantity: 2, Amount: 40000},
		},
		PaidToDate: paid,
	}
}

type openingLoanGLLine struct {
	AccountID string  `db:"accountId"`
	Debit     int64   `db:"debit"`
	Credit    int64   `db:"credit"`
	ClientID  *string `db:"clientId"`
}

func openingLoanEntries(t *testing.T, d *Database, invoiceID string) (sourceTypes []string, lines []openingLoanGLLine) {
	t.Helper()
	if err := d.DB.Select(&sourceTypes,
		`SELECT sourceDocumentType FROM journal_entries WHERE sourceDocumentId = ? ORDER BY createdAt`, invoiceID,
	); err != nil {
		t.Fatalf("select entries: %v", err)
	}
	if err := d.DB.Select(&lines, `
		SELECT jl.accountId, jl.debit, jl.credit, jl.clientId FROM journal_lines jl
		JOIN journal_entries je ON je.id = jl.journalEntryId
		WHERE je.sourceDocumentId = ? ORDER BY jl.debit DESC`, invoiceID,
	); err != nil {
		t.Fatalf("select lines: %v", err)
	}
	return sourceTypes, lines
}

func retainedEarningsID(t *testing.T, d *Database, orgID string) string {
	t.Helper()
	org, err := d.GetOrganization(orgID)
	if err != nil || org.RetainedEarningsAccountID == nil {
		t.Fatalf("organization retained earnings account: %v, %v", org, err)
	}
	return *org.RetainedEarningsAccountID
}

// An open loan posts one cutover-dated entry for what's still owed — Dr AR
// (tagged with the customer) / Cr Retained Earnings — and nothing else: no
// sale entry, no tax, no stock; its paid-to-date is a GL-free payment record.
func TestOpeningLoanPostsOutstandingAgainstRetainedEarnings(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-opening-open")

	invoice, err := d.CreateOpeningLoan(openingLoanRequest(fx, "B1-P012-3", 30000))
	if err != nil {
		t.Fatalf("CreateOpeningLoan: %v", err)
	}
	if !isOpeningLoan(invoice) || invoice.State != "sent" || invoice.Date != openingLoanSaleDate ||
		invoice.Total != 100000 || invoice.TaxTotal != 0 || invoice.Number != "B1-P012-3" || invoice.MovesStock != 0 {
		t.Fatalf("invoice = %+v, want an opening loan: sent, dated the sale, total 100000, no tax", invoice)
	}

	sourceTypes, lines := openingLoanEntries(t, d, invoice.ID)
	if len(sourceTypes) != 1 || sourceTypes[0] != openingLoanSourceType {
		t.Fatalf("entries = %v, want exactly one %q entry", sourceTypes, openingLoanSourceType)
	}
	re := retainedEarningsID(t, d, fx.orgID)
	if len(lines) != 2 ||
		lines[0].AccountID != fx.arAccountID || lines[0].Debit != 70000 || lines[0].ClientID == nil || *lines[0].ClientID != fx.clientID ||
		lines[1].AccountID != re || lines[1].Credit != 70000 {
		t.Fatalf("lines = %+v, want Dr AR 70000 (client) / Cr Retained Earnings 70000", lines)
	}
	var entryDate int64
	if err := d.DB.Get(&entryDate, `SELECT date FROM journal_entries WHERE sourceDocumentId = ?`, invoice.ID); err != nil || entryDate != fx.date {
		t.Fatalf("entry date = %d, %v; want the cutover %d", entryDate, err, fx.date)
	}

	paid, err := d.GetInvoiceAmountPaid(invoice.ID)
	if err != nil || paid != 30000 {
		t.Fatalf("amount paid = %d, %v; want 30000", paid, err)
	}
	var payment Payment
	if err := d.DB.Get(&payment, `
		SELECT p.* FROM payments p JOIN payment_applications pa ON pa.paymentId = p.id
		WHERE pa.documentId = ?`, invoice.ID); err != nil {
		t.Fatalf("payment: %v", err)
	}
	if payment.Origin == nil || *payment.Origin != OpeningOrigin || payment.JournalEntryID != nil || payment.BankAccountID != re {
		t.Fatalf("payment = %+v, want an opening payment with no GL entry, booked against Retained Earnings", payment)
	}

	var movements int
	if err := d.DB.Get(&movements, `SELECT COUNT(*) FROM stockMovements WHERE sourceDocumentId = ?`, invoice.ID); err != nil || movements != 0 {
		t.Fatalf("stock movements = %d, %v; want none", movements, err)
	}
	var counter int64
	if err := d.DB.Get(&counter, `SELECT invoice_number_counter FROM organizations WHERE id = ?`, fx.orgID); err != nil || counter != 0 {
		t.Fatalf("invoice counter = %d, %v; a migrated loan must not consume it", counter, err)
	}
}

// A settled loan is history only: paid, and nothing posted at all.
func TestOpeningLoanSettledPostsNothing(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-opening-settled")

	invoice, err := d.CreateOpeningLoan(openingLoanRequest(fx, "B1-P001-1", 100000))
	if err != nil {
		t.Fatalf("CreateOpeningLoan: %v", err)
	}
	if invoice.State != "paid" {
		t.Fatalf("state = %q, want paid", invoice.State)
	}
	if sourceTypes, _ := openingLoanEntries(t, d, invoice.ID); len(sourceTypes) != 0 {
		t.Fatalf("entries = %v, want none for a settled loan", sourceTypes)
	}
}

// The Loan status lists every migrated loan — the settled one too, although
// one whole-invoice payment is exactly what its "was ever a loan" filter
// otherwise reads as a pure cash sale.
func TestOpeningLoansAppearInLoanStatus(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-opening-loanstatus")

	open, err := d.CreateOpeningLoan(openingLoanRequest(fx, "OPEN-1", 30000))
	if err != nil {
		t.Fatalf("CreateOpeningLoan(open): %v", err)
	}
	settled, err := d.CreateOpeningLoan(openingLoanRequest(fx, "DONE-1", 100000))
	if err != nil {
		t.Fatalf("CreateOpeningLoan(settled): %v", err)
	}
	rows, err := d.GetLoanStatus(fx.orgID, "")
	if err != nil {
		t.Fatalf("GetLoanStatus: %v", err)
	}
	outstanding := map[string]int64{}
	lines := map[string]int{}
	for _, r := range rows {
		outstanding[r.InvoiceID] += r.Outstanding
		lines[r.InvoiceID]++
	}
	if lines[open.ID] != 2 || outstanding[open.ID] != 70000 {
		t.Fatalf("open loan: %d lines, outstanding %d; want 2 lines, 70000", lines[open.ID], outstanding[open.ID])
	}
	if lines[settled.ID] != 2 || outstanding[settled.ID] != 0 {
		t.Fatalf("settled loan: %d lines, outstanding %d; want 2 lines, 0", lines[settled.ID], outstanding[settled.ID])
	}
}

// Migrated loans are sales the app never made: out of the sales analytics,
// the Tax Summary and (through them) the Dashboard's revenue figures.
func TestOpeningLoansAreExcludedFromSalesReports(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-opening-reports")

	if _, err := d.CreateOpeningLoan(openingLoanRequest(fx, "OPEN-1", 30000)); err != nil {
		t.Fatalf("CreateOpeningLoan: %v", err)
	}
	if _, err := d.CreateOpeningLoan(openingLoanRequest(fx, "DONE-1", 100000)); err != nil {
		t.Fatalf("CreateOpeningLoan: %v", err)
	}
	from := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
	to := time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC).UnixMilli()

	months, err := d.GetRevenueByMonth(fx.orgID, from, to)
	if err != nil {
		t.Fatalf("GetRevenueByMonth: %v", err)
	}
	for _, m := range months {
		if m.Revenue != 0 {
			t.Fatalf("revenue by month = %+v, want no revenue from migrated loans", months)
		}
	}
	if clients, err := d.GetSalesByClient(fx.orgID, from, to, 0); err != nil || len(clients) != 0 {
		t.Fatalf("sales by client = %+v, %v; want none", clients, err)
	}
	if products, err := d.GetSalesByProduct(fx.orgID, from, to, 0); err != nil || len(products) != 0 {
		t.Fatalf("sales by product = %+v, %v; want none", products, err)
	}
	if summary, err := d.GetTaxSummary(fx.orgID, from, to); err != nil || len(summary.Output) != 0 {
		t.Fatalf("tax summary = %+v, %v; want no output lines", summary, err)
	}
}

func requireFrozen(t *testing.T, what string, err error) {
	t.Helper()
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("%s: err = %v, want a validation error (the loan is frozen)", what, err)
	}
}

// Every invoice path but a Cash Book collection refuses to change a migrated
// loan; sent<->paid is allowed and never touches the GL.
func TestOpeningLoanIsFrozen(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-opening-frozen")

	invoice, err := d.CreateOpeningLoan(openingLoanRequest(fx, "B1-P012-3", 30000))
	if err != nil {
		t.Fatalf("CreateOpeningLoan: %v", err)
	}
	notes := "changed"
	_, err = d.UpdateInvoice(invoice.ID, UpdateInvoiceRequest{CustomerNotes: &notes})
	requireFrozen(t, "UpdateInvoice", err)
	for _, state := range []string{"draft", "cancelled"} {
		_, err = d.UpdateInvoiceState(invoice.ID, state)
		requireFrozen(t, "UpdateInvoiceState "+state, err)
	}
	_, err = d.DeleteInvoice(invoice.ID)
	requireFrozen(t, "DeleteInvoice", err)
	_, err = d.GenerateEInvoice(invoice.ID)
	requireFrozen(t, "GenerateEInvoice", err)
	_, _, _, _, _, _, _, _, err = d.FetchInvoiceExportData(invoice.ID)
	requireFrozen(t, "FetchInvoiceExportData", err)

	var openingPaymentID string
	if err := d.DB.Get(&openingPaymentID, `
		SELECT paymentId FROM payment_applications WHERE documentId = ?`, invoice.ID); err != nil {
		t.Fatalf("opening payment: %v", err)
	}
	_, err = d.VoidPayment(openingPaymentID, fx.date)
	requireFrozen(t, "VoidPayment(opening)", err)

	// sent -> paid -> sent: allowed, and the GL keeps its single entry.
	for _, state := range []string{"paid", "sent"} {
		got, err := d.UpdateInvoiceState(invoice.ID, state)
		if err != nil || got.State != state {
			t.Fatalf("UpdateInvoiceState(%s) = %+v, %v", state, got, err)
		}
	}
	if sourceTypes, _ := openingLoanEntries(t, d, invoice.ID); len(sourceTypes) != 1 || sourceTypes[0] != openingLoanSourceType {
		t.Fatalf("entries after sent<->paid = %v, want only the opening entry", sourceTypes)
	}
}

// The Cash Book collects a migrated loan like any other, line by line, and
// clearing it marks it paid; each collection posts Dr register / Cr AR,
// running down the AR the opening entry put there.
func TestOpeningLoanCollectedAtTheCounter(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-opening-collect")
	accounts, err := d.GetAccounts(fx.orgID)
	if err != nil {
		t.Fatalf("GetAccounts: %v", err)
	}
	var register string
	for _, a := range accounts {
		if a.Code == "1010" {
			register = a.ID
		}
	}
	if _, err := d.UpdateOrganization(fx.orgID, UpdateOrganizationRequest{DefaultCashRegisterAccountID: &register}); err != nil {
		t.Fatalf("set register: %v", err)
	}

	invoice, err := d.CreateOpeningLoan(openingLoanRequest(fx, "B1-P012-3", 30000))
	if err != nil {
		t.Fatalf("CreateOpeningLoan: %v", err)
	}
	rows, err := d.GetLoanStatus(fx.orgID, "")
	if err != nil {
		t.Fatalf("GetLoanStatus: %v", err)
	}
	for _, r := range rows {
		if r.InvoiceID != invoice.ID || r.Outstanding == 0 {
			continue
		}
		if _, err := d.CreateCashSalePayment(CreateCashSalePaymentRequest{
			InvoiceID: invoice.ID, InvoiceLineItemID: r.LineID, Amount: r.Outstanding, Date: fx.date,
		}); err != nil {
			t.Fatalf("CreateCashSalePayment(line %s): %v", r.LineID, err)
		}
	}
	got, err := d.GetInvoice(invoice.ID)
	if err != nil || got.State != "paid" {
		t.Fatalf("after collecting every line: %+v, %v; want paid", got, err)
	}
	var arBalance int64
	if err := d.DB.Get(&arBalance, `
		SELECT COALESCE(SUM(jl.debit - jl.credit), 0) FROM journal_lines jl
		JOIN journal_entries je ON je.id = jl.journalEntryId
		WHERE je.organizationId = ? AND jl.accountId = ? AND je.status IN ('posted', 'reversed')`,
		fx.orgID, fx.arAccountID); err != nil || arBalance != 0 {
		t.Fatalf("AR balance = %d, %v; want 0 once the loan is collected", arBalance, err)
	}
}

func TestOpeningLoanValidation(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-opening-validation")
	other := newGLPostingTestFixture(t, d, "org-opening-validation-other")

	cases := map[string]func(*OpeningLoanRequest){
		"paid above total":       func(r *OpeningLoanRequest) { r.PaidToDate = 100001 },
		"sale after cutover":     func(r *OpeningLoanRequest) { r.Date = r.CutoverDate + 1 },
		"no reference":           func(r *OpeningLoanRequest) { r.Number = "  " },
		"no lines":               func(r *OpeningLoanRequest) { r.Lines = nil },
		"zero amount":            func(r *OpeningLoanRequest) { r.Lines[0].Amount = 0 },
		"zero quantity":          func(r *OpeningLoanRequest) { r.Lines[0].Quantity = 0 },
		"another org's product":  func(r *OpeningLoanRequest) { r.Lines[0].ProductID = &other.productID },
		"another org's customer": func(r *OpeningLoanRequest) { r.ClientID = other.clientID },
		"no product, no description": func(r *OpeningLoanRequest) {
			r.Lines[1].Description = ""
		},
		"payment date after cutover": func(r *OpeningLoanRequest) {
			after := r.CutoverDate + 1
			r.LastPaymentDate = &after
		},
	}
	for name, mutate := range cases {
		req := openingLoanRequest(fx, "REF-"+name, 30000)
		mutate(&req)
		_, err := d.CreateOpeningLoan(req)
		requireFrozen(t, name, err) // any validation error
	}

	if _, err := d.CreateOpeningLoan(openingLoanRequest(fx, "DUP-1", 0)); err != nil {
		t.Fatalf("first DUP-1: %v", err)
	}
	_, err := d.CreateOpeningLoan(openingLoanRequest(fx, "DUP-1", 0))
	requireFrozen(t, "duplicate reference", err)
}

// A migrated loan can also be settled by an ordinary payment (a bank
// transfer from the invoice page): its opening entry is the receivable the
// payment clears, and the payment can't exceed what's still owed.
func TestOpeningLoanSettledByOrdinaryPayment(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-opening-payment")
	bank := accountByCode(t, d, fx.orgID, "1020").ID

	invoice, err := d.CreateOpeningLoan(openingLoanRequest(fx, "B1-P012-3", 30000))
	if err != nil {
		t.Fatalf("CreateOpeningLoan: %v", err)
	}
	pay := func(amount int64) error {
		_, err := d.CreatePayment(CreatePaymentRequest{
			OrganizationID: fx.orgID, Direction: "inbound", ClientID: &fx.clientID,
			BankAccountID: bank, Amount: amount, Currency: invoice.Currency, Date: fx.date, Method: "bank_transfer",
			Applications: []CreatePaymentApplicationRequest{{DocumentType: "invoice", DocumentID: invoice.ID, Amount: amount}},
		})
		return err
	}
	requireFrozen(t, "paying more than the outstanding", pay(70001))
	if err := pay(70000); err != nil {
		t.Fatalf("paying the outstanding: %v", err)
	}
	if paid, err := d.GetInvoiceAmountPaid(invoice.ID); err != nil || paid != 100000 {
		t.Fatalf("amount paid = %d, %v; want 100000", paid, err)
	}
}

// Receivables include migrated loans: the Dashboard's outstanding and AR
// aging show what's still owed, aged from the sale date (its due date),
// while the Dashboard's revenue leaves the loans out.
func TestOpeningLoansAreReceivables(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-opening-receivables")

	open, err := d.CreateOpeningLoan(openingLoanRequest(fx, "OPEN-1", 30000))
	if err != nil {
		t.Fatalf("CreateOpeningLoan: %v", err)
	}
	if open.DueDate == nil || *open.DueDate != openingLoanSaleDate {
		t.Fatalf("due date = %v, want the sale date", open.DueDate)
	}

	aging, err := d.GetReceivableAging(fx.orgID)
	if err != nil {
		t.Fatalf("GetReceivableAging: %v", err)
	}
	if aging.Total != 70000 || aging.Days90Plus != 70000 || len(aging.Invoices) != 1 || aging.Invoices[0].ID != open.ID {
		t.Fatalf("aging = %+v, want the loan's 70000 outstanding, 90+ days old", aging)
	}

	from := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
	to := time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC).UnixMilli()
	dashboard, err := d.GetDashboardData(fx.orgID, from, to)
	if err != nil {
		t.Fatalf("GetDashboardData: %v", err)
	}
	if dashboard.Outstanding.Total != 70000 {
		t.Fatalf("dashboard outstanding = %d, want 70000", dashboard.Outstanding.Total)
	}
	if len(dashboard.TopClients) != 0 || len(dashboard.TopProducts) != 0 {
		t.Fatalf("dashboard top clients/products = %+v / %+v, want none from migrated loans", dashboard.TopClients, dashboard.TopProducts)
	}
	for _, m := range dashboard.RevenueByMonth {
		if m.Revenue != 0 {
			t.Fatalf("dashboard revenue = %+v, want none from migrated loans", dashboard.RevenueByMonth)
		}
	}
}
