package db

import (
	"errors"
	"strings"
	"testing"
)

// newTwoLineLoanSale records a loan sale of two lines — 2 × 10.00 and
// 1 × 5.00 at the fixture's 20% rate, so the lines are worth 24.00 and 6.00
// of the 30.00 total — with amountReceived paid upfront, and returns the
// invoice id and its line ids keyed by net line value.
func newTwoLineLoanSale(t *testing.T, d *Database, fx glPostingTestFixture, amountReceived int64) (invoiceID, bigLine, smallLine string) {
	t.Helper()
	// The Cash Book always settles into the register account.
	register := accountByCode(t, d, fx.orgID, "1010")
	if _, err := d.UpdateOrganization(fx.orgID, UpdateOrganizationRequest{
		DefaultCashRegisterAccountID: &register.ID,
		InvoiceNumberFormat:          ptr("LOAN-{number}"),
	}); err != nil {
		t.Fatalf("UpdateOrganization: %v", err)
	}
	result, err := d.CreateCashSale(CreateCashSaleRequest{
		OrganizationID: fx.orgID, ClientID: fx.clientID, Date: fx.date, Currency: "EUR",
		LineItems: []CreateInvoiceLineItemRequest{
			{Quantity: 2, UnitPrice: 1000, TaxRate: &fx.taxRateID, ProductID: &fx.productID},
			{Quantity: 1, UnitPrice: 500, TaxRate: &fx.taxRateID, ProductID: &fx.productID},
		},
		SubTotal: 2500, TaxTotal: 500, Total: 3000,
		AmountReceived: amountReceived, PaymentMethod: "cash",
	})
	if err != nil {
		t.Fatalf("CreateCashSale: %v", err)
	}
	items, err := d.GetInvoiceLineItems(result.Invoice.ID)
	if err != nil {
		t.Fatalf("GetInvoiceLineItems: %v", err)
	}
	for _, item := range items {
		if item.UnitPrice == 1000 {
			bigLine = item.ID
		} else {
			smallLine = item.ID
		}
	}
	return result.Invoice.ID, bigLine, smallLine
}

func loanRowsByLine(t *testing.T, d *Database, orgID string) map[string]LoanStatusRow {
	t.Helper()
	rows, err := d.GetLoanStatus(orgID, "")
	if err != nil {
		t.Fatalf("GetLoanStatus: %v", err)
	}
	byLine := map[string]LoanStatusRow{}
	for _, r := range rows {
		byLine[r.LineID] = r
	}
	return byLine
}

func assertLine(t *testing.T, rows map[string]LoanStatusRow, lineID string, amount, paid int64) {
	t.Helper()
	r, ok := rows[lineID]
	if !ok {
		t.Fatalf("line %s missing from loan status", lineID)
	}
	if r.Amount != amount || r.Paid != paid || r.Outstanding != amount-paid {
		t.Fatalf("line %s = amount %d paid %d outstanding %d, want %d/%d/%d",
			lineID, r.Amount, r.Paid, r.Outstanding, amount, paid, amount-paid)
	}
}

func TestCashSalePaymentSettlesOnlyItsLine(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-line-pay")
	invoiceID, bigLine, smallLine := newTwoLineLoanSale(t, d, fx, 0)

	rows := loanRowsByLine(t, d, fx.orgID)
	assertLine(t, rows, bigLine, 2400, 0)
	assertLine(t, rows, smallLine, 600, 0)

	result, err := d.CreateCashSalePayment(CreateCashSalePaymentRequest{
		InvoiceID: invoiceID, InvoiceLineItemID: smallLine, Amount: 600, Date: fx.date,
	})
	if err != nil {
		t.Fatalf("CreateCashSalePayment: %v", err)
	}
	if result.Payment.JournalEntryID == nil {
		t.Fatal("payment should have posted a journal entry")
	}
	if result.Payment.Method != "cash" || result.Payment.Direction != "inbound" {
		t.Fatalf("payment = %s/%s, want cash/inbound", result.Payment.Method, result.Payment.Direction)
	}
	if result.Invoice.State != "sent" {
		t.Fatalf("invoice state = %q after a partial settlement, want sent", result.Invoice.State)
	}
	apps, err := d.GetPaymentApplications(result.Payment.ID)
	if err != nil {
		t.Fatalf("GetPaymentApplications: %v", err)
	}
	if len(apps) != 1 || apps[0].InvoiceLineItemID == nil || *apps[0].InvoiceLineItemID != smallLine {
		t.Fatalf("applications = %+v, want one targeting line %s", apps, smallLine)
	}

	rows = loanRowsByLine(t, d, fx.orgID)
	assertLine(t, rows, bigLine, 2400, 0)
	assertLine(t, rows, smallLine, 600, 600)

	// The invoice number is the loan number, on the loan rows and on the
	// payment (the Cash Book's movements/history show it).
	if rows[bigLine].InvoiceNumber != result.Invoice.Number || result.Invoice.Number == "" {
		t.Fatalf("loan row invoice number = %q, want %q", rows[bigLine].InvoiceNumber, result.Invoice.Number)
	}
	payments, err := d.GetPayments(fx.orgID)
	if err != nil {
		t.Fatalf("GetPayments: %v", err)
	}
	for _, p := range payments {
		if p.ID == result.Payment.ID && (len(p.InvoiceNumbers) != 1 || p.InvoiceNumbers[0] != result.Invoice.Number) {
			t.Fatalf("payment invoiceNumbers = %v, want [%s]", p.InvoiceNumbers, result.Invoice.Number)
		}
	}

	result, err = d.CreateCashSalePayment(CreateCashSalePaymentRequest{
		InvoiceID: invoiceID, InvoiceLineItemID: bigLine, Amount: 2400, Date: fx.date,
	})
	if err != nil {
		t.Fatalf("CreateCashSalePayment (second line): %v", err)
	}
	if result.Invoice.State != "paid" {
		t.Fatalf("invoice state = %q once every line is settled, want paid", result.Invoice.State)
	}
	paid, err := d.GetInvoiceAmountPaid(invoiceID)
	if err != nil {
		t.Fatalf("GetInvoiceAmountPaid: %v", err)
	}
	if paid != 3000 {
		t.Fatalf("GetInvoiceAmountPaid = %d, want 3000", paid)
	}
	rows = loanRowsByLine(t, d, fx.orgID)
	assertLine(t, rows, bigLine, 2400, 2400)
	assertLine(t, rows, smallLine, 600, 600)
}

func TestCashSalePaymentRejectsMoreThanTheLineOwes(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-line-overpay")
	invoiceID, _, smallLine := newTwoLineLoanSale(t, d, fx, 0)

	_, err := d.CreateCashSalePayment(CreateCashSalePaymentRequest{
		InvoiceID: invoiceID, InvoiceLineItemID: smallLine, Amount: 601, Date: fx.date,
	})
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("err = %v, want a validation error for paying more than the line owes", err)
	}
	if !strings.Contains(err.Error(), "exceeds the line's outstanding balance 600") {
		t.Fatalf("err = %q, want it to name the line's outstanding balance", err.Error())
	}
	if paid, _ := d.GetInvoiceAmountPaid(invoiceID); paid != 0 {
		t.Fatalf("a rejected payment still recorded %d", paid)
	}
}

func TestCashSalePaymentRejectsALineFromAnotherInvoice(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-line-foreign")
	invoiceA, _, _ := newTwoLineLoanSale(t, d, fx, 0)
	_, _, lineOfB := newTwoLineLoanSale(t, d, fx, 0)

	_, err := d.CreateCashSalePayment(CreateCashSalePaymentRequest{
		InvoiceID: invoiceA, InvoiceLineItemID: lineOfB, Amount: 100, Date: fx.date,
	})
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("err = %v, want a validation error for another invoice's line", err)
	}
	if !strings.Contains(err.Error(), "line item not found on this invoice") {
		t.Fatalf("err = %q, want the line-not-on-invoice error", err.Error())
	}
}

// An upfront deposit is an invoice-level payment: it's spread over the lines
// by value. Settling one line afterwards must clear exactly that line's
// remaining balance and leave the other line's untouched.
func TestCashSalePaymentAfterAnInvoiceLevelDeposit(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-line-deposit")
	invoiceID, bigLine, smallLine := newTwoLineLoanSale(t, d, fx, 900)

	rows := loanRowsByLine(t, d, fx.orgID)
	assertLine(t, rows, bigLine, 2400, 720)
	assertLine(t, rows, smallLine, 600, 180)

	if _, err := d.CreateCashSalePayment(CreateCashSalePaymentRequest{
		InvoiceID: invoiceID, InvoiceLineItemID: smallLine, Amount: 420, Date: fx.date,
	}); err != nil {
		t.Fatalf("CreateCashSalePayment: %v", err)
	}
	rows = loanRowsByLine(t, d, fx.orgID)
	assertLine(t, rows, bigLine, 2400, 720)
	assertLine(t, rows, smallLine, 600, 600)
}

func TestAllocateCappedRespectsCapsAndConservesTotal(t *testing.T) {
	cases := []struct {
		name    string
		total   int64
		weights []int64
		caps    []int64
		want    []int64
	}{
		{"proportional", 900, []int64{2400, 600}, []int64{2400, 600}, []int64{720, 180}},
		{"capped slot overflows to the other", 1000, []int64{2400, 600}, []int64{2400, 0}, []int64{1000, 0}},
		{"partially capped", 900, []int64{1000, 1000}, []int64{1000, 100}, []int64{800, 100}},
		{"leftover cents", 100, []int64{1, 1, 1}, []int64{100, 100, 100}, []int64{34, 33, 33}},
		{"nothing to allocate", 0, []int64{1, 1}, []int64{5, 5}, []int64{0, 0}},
		{"exceeds caps", 12, []int64{1, 1}, []int64{5, 5}, []int64{5, 7}},
	}
	for _, tc := range cases {
		got := allocateCapped(tc.total, tc.weights, tc.caps)
		var sum int64
		for k := range got {
			sum += got[k]
			if got[k] != tc.want[k] {
				t.Errorf("%s: allocateCapped = %v, want %v", tc.name, got, tc.want)
				break
			}
		}
		if tc.total > 0 && sum != tc.total {
			t.Errorf("%s: allocated %d, want %d", tc.name, sum, tc.total)
		}
	}
}

// TestLoanStatusKeepsALoanClearedByOneLinePayment pins the fix for the loan
// that vanished from the tracker: a zero-deposit loan cleared by exactly one
// payment used to look like a pure cash sale (one full payment), so it
// dropped out even though it was a loan. A line payment names its line,
// which a cash sale's upfront payment never does, and that keeps it listed.
func TestLoanStatusKeepsALoanClearedByOneLinePayment(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-one-pay-loan")
	register := accountByCode(t, d, fx.orgID, "1010")
	if _, err := d.UpdateOrganization(fx.orgID, UpdateOrganizationRequest{
		DefaultCashRegisterAccountID: &register.ID,
		InvoiceNumberFormat:          ptr("LOAN-{number}"),
	}); err != nil {
		t.Fatalf("UpdateOrganization: %v", err)
	}
	sale := func(amountReceived int64) string {
		t.Helper()
		result, err := d.CreateCashSale(CreateCashSaleRequest{
			OrganizationID: fx.orgID, ClientID: fx.clientID, Date: fx.date, Currency: "EUR",
			LineItems: []CreateInvoiceLineItemRequest{
				{Quantity: 1, UnitPrice: 1000, TaxRate: &fx.taxRateID, ProductID: &fx.productID},
			},
			SubTotal: 1000, TaxTotal: 200, Total: 1200,
			AmountReceived: amountReceived, PaymentMethod: "cash",
		})
		if err != nil {
			t.Fatalf("CreateCashSale: %v", err)
		}
		return result.Invoice.ID
	}
	cashSaleID := sale(1200)
	loanID := sale(0)
	items, err := d.GetInvoiceLineItems(loanID)
	if err != nil || len(items) != 1 {
		t.Fatalf("GetInvoiceLineItems = %v, %v; want one line", items, err)
	}
	if _, err := d.CreateCashSalePayment(CreateCashSalePaymentRequest{
		InvoiceID: loanID, InvoiceLineItemID: items[0].ID, Amount: 1200, Date: fx.date + dayLaterMs,
	}); err != nil {
		t.Fatalf("CreateCashSalePayment: %v", err)
	}

	rows, err := d.GetLoanStatus(fx.orgID, "")
	if err != nil {
		t.Fatalf("GetLoanStatus: %v", err)
	}
	var loanRows int
	for _, r := range rows {
		if r.InvoiceID == cashSaleID {
			t.Fatalf("a cash sale paid in full upfront should not appear in loan status: %+v", r)
		}
		if r.InvoiceID == loanID {
			loanRows++
			if r.Amount != 1200 || r.Paid != 1200 || r.Outstanding != 0 {
				t.Fatalf("settled loan row = %+v, want amount=1200 paid=1200 outstanding=0", r)
			}
		}
	}
	if loanRows != 1 {
		t.Fatalf("loan cleared by one line payment: %d rows in loan status, want 1", loanRows)
	}
}

// TestAllocateInvoiceLinesWithANegativeLine checks that a credit line (a
// negative unit price) can't drive any line's amount or paid below zero, and
// that both still add up to the invoice's figures.
func TestAllocateInvoiceLinesWithANegativeLine(t *testing.T) {
	for _, invoicePaid := range []int64{0, 700, 1800} {
		lines := []loanLineRaw{
			{LineID: "a", NetLine: 2000, InvoiceTotal: 1800, InvoicePaid: invoicePaid},
			{LineID: "b", NetLine: -500, InvoiceTotal: 1800, InvoicePaid: invoicePaid},
		}
		amounts, paid := allocateInvoiceLines(lines)
		var amountSum, paidSum int64
		for k := range lines {
			if amounts[k] < 0 || paid[k] < 0 || paid[k] > amounts[k] {
				t.Fatalf("paid %d: line %s amount %d paid %d, want 0 <= paid <= amount",
					invoicePaid, lines[k].LineID, amounts[k], paid[k])
			}
			amountSum += amounts[k]
			paidSum += paid[k]
		}
		if amountSum != 1800 || paidSum != invoicePaid {
			t.Fatalf("paid %d: amounts sum %d, paid sum %d; want 1800 and %d", invoicePaid, amountSum, paidSum, invoicePaid)
		}
	}
}
