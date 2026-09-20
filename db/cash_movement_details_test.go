package db

import "testing"

// dayLaterMs is an arbitrary "later day" offset for a repayment made after
// the original sale — large enough to be a clearly separate day, small
// enough to stay inside every test's open fiscal year.
const dayLaterMs = 86400000

func TestGetCashMovementDetailsClassifiesSaleLoanAndRepayment(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-cash-movement-details")
	register := accountByCode(t, d, fx.orgID, "1010")

	// A cash sale — paid in full, in one shot.
	cashSale, err := d.CreateCashSale(CreateCashSaleRequest{
		OrganizationID: fx.orgID, ClientID: fx.clientID, Date: fx.date, Currency: "EUR",
		LineItems: []CreateInvoiceLineItemRequest{
			{Quantity: 2, UnitPrice: 1000, TaxRate: &fx.taxRateID, ProductID: &fx.productID},
		},
		SubTotal: 2000, TaxTotal: 400, Total: 2400,
		AmountReceived: 2400, PaymentMethod: "cash", BankAccountID: register.ID,
	})
	if err != nil {
		t.Fatalf("CreateCashSale (cash): %v", err)
	}

	// A loan sale — a deposit up front, the rest collected later.
	loanSale, err := d.CreateCashSale(CreateCashSaleRequest{
		OrganizationID: fx.orgID, ClientID: fx.clientID, Date: fx.date, Currency: "EUR",
		LineItems: []CreateInvoiceLineItemRequest{
			{Quantity: 2, UnitPrice: 1000, TaxRate: &fx.taxRateID, ProductID: &fx.productID},
		},
		SubTotal: 2000, TaxTotal: 400, Total: 2400,
		AmountReceived: 900, PaymentMethod: "cash", BankAccountID: register.ID,
	})
	if err != nil {
		t.Fatalf("CreateCashSale (loan): %v", err)
	}

	// The loan's remaining balance, collected a day later — this is the
	// case that must NOT come back labeled "sale", even though it's the
	// payment that finally settles the invoice to state=paid.
	repaymentDate := fx.date + dayLaterMs
	repayment, err := d.CreatePayment(CreatePaymentRequest{
		OrganizationID: fx.orgID, Direction: "inbound", ClientID: &fx.clientID,
		BankAccountID: register.ID, Amount: 1500, Currency: "EUR", Date: repaymentDate, Method: "cash",
		Applications: []CreatePaymentApplicationRequest{
			{DocumentType: "invoice", DocumentID: loanSale.Invoice.ID, Amount: 1500},
		},
	})
	if err != nil {
		t.Fatalf("CreatePayment (repayment): %v", err)
	}
	// invoices.state is a manual flag, never derived from payments (see
	// db/CLAUDE.md) — CreatePayment alone doesn't flip it. The Cash Book
	// screen's own "Pay" flow does this as an explicit follow-up call once
	// a balance clears (cash-book.tsx's handleSettled); mirror that here so
	// this test actually exercises the retroactive-mislabeling risk a
	// state-based Kind would have.
	if _, err := d.UpdateInvoiceState(loanSale.Invoice.ID, "paid"); err != nil {
		t.Fatalf("UpdateInvoiceState: %v", err)
	}

	details, err := d.GetCashMovementDetails(fx.orgID, register.ID, fx.date, repaymentDate)
	if err != nil {
		t.Fatalf("GetCashMovementDetails: %v", err)
	}

	kindByPaymentID := map[string]string{}
	for _, row := range details {
		kindByPaymentID[row.ID] = row.Kind
		if row.Direction != "in" {
			t.Fatalf("row %s: direction = %q, want in", row.ID, row.Direction)
		}
	}

	if got := kindByPaymentID[cashSale.Payment.ID]; got != "sale" {
		t.Fatalf("cash sale payment kind = %q, want sale", got)
	}
	if got := kindByPaymentID[loanSale.Payment.ID]; got != "loan" {
		t.Fatalf("loan deposit payment kind = %q, want loan", got)
	}
	if got := kindByPaymentID[repayment.ID]; got != "repayment" {
		t.Fatalf("repayment kind = %q, want repayment (not state-derived, e.g. not %q)", got, "sale")
	}

	// Sanity: the invoice this repayment settles really is "paid" now —
	// confirming the state-based label this test guards against would have
	// gotten this case wrong.
	settled, err := d.GetInvoice(loanSale.Invoice.ID)
	if err != nil {
		t.Fatalf("GetInvoice: %v", err)
	}
	if settled.State != "paid" {
		t.Fatalf("loan invoice state = %q, want paid after the repayment", settled.State)
	}
}

func TestGetCashMovementDetailsIncludesWithdrawals(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-cash-movement-details-withdraw")
	register := accountByCode(t, d, fx.orgID, "1010")
	bank := accountByCode(t, d, fx.orgID, "1020")

	note := "Deposit to bank"
	movement, err := d.CreateCashMovement(CreateCashMovementRequest{
		OrganizationID: fx.orgID, AccountID: register.ID, Date: fx.date,
		CounterAccountType: "bank", CounterAccountID: bank.ID, Amount: 5000, Note: &note,
	})
	if err != nil {
		t.Fatalf("CreateCashMovement: %v", err)
	}

	details, err := d.GetCashMovementDetails(fx.orgID, register.ID, fx.date, fx.date)
	if err != nil {
		t.Fatalf("GetCashMovementDetails: %v", err)
	}
	if len(details) != 1 {
		t.Fatalf("len(details) = %d, want 1", len(details))
	}
	row := details[0]
	if row.ID != movement.CashMovement.ID || row.Direction != "out" || row.Kind != "withdrawal" {
		t.Fatalf("withdrawal row = %+v, want id=%s direction=out kind=withdrawal", row, movement.CashMovement.ID)
	}
	if row.ClientName != nil {
		t.Fatalf("withdrawal row has a client name: %v", *row.ClientName)
	}
}

// TestGetCashMovementDetailsIncludesNonMidnightMovement is F100's
// regression test: every caller passes the same UTC-midnight value as both
// endpoints to mean "this whole day", but a withdrawal/sale is stamped at
// the moment it actually happened, so the old literal `date >= start AND
// date <= end` matched nothing and the detail table looked empty while the
// day-summary above it (floored to UTC day) showed non-zero totals. A
// movement created at 14:37 must come back for its day.
func TestGetCashMovementDetailsIncludesNonMidnightMovement(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-cash-movement-details-time")
	register := accountByCode(t, d, fx.orgID, "1010")
	bank := accountByCode(t, d, fx.orgID, "1020")

	// fx.date is 2025-02-01 00:00:00 UTC; stamp the movement well inside
	// that same UTC day, not on its midnight boundary.
	at := fx.date + 14*60*60*1000 + 37*60*1000 // 14:37 UTC
	note := "Afternoon deposit"
	movement, err := d.CreateCashMovement(CreateCashMovementRequest{
		OrganizationID: fx.orgID, AccountID: register.ID, Date: at,
		CounterAccountType: "bank", CounterAccountID: bank.ID, Amount: 1234, Note: &note,
	})
	if err != nil {
		t.Fatalf("CreateCashMovement: %v", err)
	}

	// Same UTC-midnight value twice — exactly what cash-book.tsx /
	// report_export.go send for one day.
	details, err := d.GetCashMovementDetails(fx.orgID, register.ID, fx.date, fx.date)
	if err != nil {
		t.Fatalf("GetCashMovementDetails: %v", err)
	}
	found := false
	for _, row := range details {
		if row.ID == movement.CashMovement.ID {
			found = true
			if row.Direction != "out" || row.Kind != "withdrawal" || row.Amount != 1234 {
				t.Fatalf("non-midnight row = %+v, want direction=out kind=withdrawal amount=1234", row)
			}
		}
	}
	if !found {
		t.Fatalf("movement stamped %d (14:37 UTC) was not returned for its UTC day [%d, %d]", at, fx.date, fx.date)
	}
}

func TestGetLoanStatusExcludesCashSaleIncludesSettledLoan(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-loan-status")
	register := accountByCode(t, d, fx.orgID, "1010")

	cashSale, err := d.CreateCashSale(CreateCashSaleRequest{
		OrganizationID: fx.orgID, ClientID: fx.clientID, Date: fx.date, Currency: "EUR",
		LineItems: []CreateInvoiceLineItemRequest{
			{Quantity: 2, UnitPrice: 1000, TaxRate: &fx.taxRateID, ProductID: &fx.productID},
		},
		SubTotal: 2000, TaxTotal: 400, Total: 2400,
		AmountReceived: 2400, PaymentMethod: "cash", BankAccountID: register.ID,
	})
	if err != nil {
		t.Fatalf("CreateCashSale (cash): %v", err)
	}

	settledLoan, err := d.CreateCashSale(CreateCashSaleRequest{
		OrganizationID: fx.orgID, ClientID: fx.clientID, Date: fx.date, Currency: "EUR",
		LineItems: []CreateInvoiceLineItemRequest{
			{Quantity: 2, UnitPrice: 1000, TaxRate: &fx.taxRateID, ProductID: &fx.productID},
		},
		SubTotal: 2000, TaxTotal: 400, Total: 2400,
		AmountReceived: 900, PaymentMethod: "cash", BankAccountID: register.ID,
	})
	if err != nil {
		t.Fatalf("CreateCashSale (settled loan): %v", err)
	}
	if _, err := d.CreatePayment(CreatePaymentRequest{
		OrganizationID: fx.orgID, Direction: "inbound", ClientID: &fx.clientID,
		BankAccountID: register.ID, Amount: 1500, Currency: "EUR", Date: fx.date + dayLaterMs, Method: "cash",
		Applications: []CreatePaymentApplicationRequest{
			{DocumentType: "invoice", DocumentID: settledLoan.Invoice.ID, Amount: 1500},
		},
	}); err != nil {
		t.Fatalf("CreatePayment (settle loan): %v", err)
	}

	openLoan, err := d.CreateCashSale(CreateCashSaleRequest{
		OrganizationID: fx.orgID, ClientID: fx.clientID, Date: fx.date, Currency: "EUR",
		LineItems: []CreateInvoiceLineItemRequest{
			{Quantity: 2, UnitPrice: 1000, TaxRate: &fx.taxRateID, ProductID: &fx.productID},
		},
		SubTotal: 2000, TaxTotal: 400, Total: 2400,
		AmountReceived: 400, PaymentMethod: "cash", BankAccountID: register.ID,
	})
	if err != nil {
		t.Fatalf("CreateCashSale (open loan): %v", err)
	}

	rows, err := d.GetLoanStatus(fx.orgID, "")
	if err != nil {
		t.Fatalf("GetLoanStatus: %v", err)
	}
	byInvoiceID := map[string]LoanStatusRow{}
	for _, r := range rows {
		byInvoiceID[r.InvoiceID] = r
	}

	if _, present := byInvoiceID[cashSale.Invoice.ID]; present {
		t.Fatalf("a pure cash sale should not appear in loan status")
	}
	settled, present := byInvoiceID[settledLoan.Invoice.ID]
	if !present {
		t.Fatalf("a since-settled loan should still appear in loan status")
	}
	if settled.Original != 2400 || settled.Paid != 2400 || settled.Outstanding != 0 {
		t.Fatalf("settled loan row = %+v, want original=2400 paid=2400 outstanding=0", settled)
	}
	open, present := byInvoiceID[openLoan.Invoice.ID]
	if !present {
		t.Fatalf("a still-open loan should appear in loan status")
	}
	if open.Original != 2400 || open.Paid != 400 || open.Outstanding != 2000 {
		t.Fatalf("open loan row = %+v, want original=2400 paid=400 outstanding=2000", open)
	}

	filtered, err := d.GetLoanStatus(fx.orgID, fx.clientID)
	if err != nil {
		t.Fatalf("GetLoanStatus (filtered): %v", err)
	}
	if len(filtered) != len(rows) {
		t.Fatalf("filtering by the only client changed the row count: %d vs %d", len(filtered), len(rows))
	}

	otherClient, err := d.CreateClient(CreateClientRequest{OrganizationID: fx.orgID, Name: ptr("Someone Else")})
	if err != nil {
		t.Fatalf("CreateClient: %v", err)
	}
	filteredOut, err := d.GetLoanStatus(fx.orgID, otherClient.ID)
	if err != nil {
		t.Fatalf("GetLoanStatus (other client): %v", err)
	}
	if len(filteredOut) != 0 {
		t.Fatalf("filtering by an unrelated client returned %d rows, want 0", len(filteredOut))
	}
}
