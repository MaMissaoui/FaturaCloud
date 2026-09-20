package db

import (
	"strings"
	"testing"
)

func TestCreateCashSaleFullCashSaleMarksInvoicePaid(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-cash-sale-full")
	register := accountByCode(t, d, fx.orgID, "1010")

	result, err := d.CreateCashSale(CreateCashSaleRequest{
		OrganizationID: fx.orgID, ClientID: fx.clientID, Date: fx.date, Currency: "EUR",
		LineItems: []CreateInvoiceLineItemRequest{
			{Quantity: 2, UnitPrice: 1000, TaxRate: &fx.taxRateID, ProductID: &fx.productID},
		},
		SubTotal: 2000, TaxTotal: 400, Total: 2400,
		AmountReceived: 2400, PaymentMethod: "cash", BankAccountID: register.ID,
	})
	if err != nil {
		t.Fatalf("CreateCashSale: %v", err)
	}
	if result.Invoice.State != "paid" {
		t.Fatalf("invoice state = %q, want paid", result.Invoice.State)
	}
	if result.Payment == nil {
		t.Fatal("expected a payment to have been recorded")
	}

	invEntry, err := d.FindPostedEntryForSourceDocument("invoice", result.Invoice.ID)
	if err != nil || invEntry == nil {
		t.Fatalf("expected a posted entry for the invoice, err=%v entry=%v", err, invEntry)
	}
	payEntry, err := d.FindPostedEntryForSourceDocument("payment", result.Payment.ID)
	if err != nil || payEntry == nil {
		t.Fatalf("expected a posted entry for the payment, err=%v entry=%v", err, payEntry)
	}

	paid, err := d.GetInvoiceAmountPaid(result.Invoice.ID)
	if err != nil {
		t.Fatalf("GetInvoiceAmountPaid: %v", err)
	}
	if paid != 2400 {
		t.Fatalf("GetInvoiceAmountPaid = %d, want 2400", paid)
	}

	invLines, err := d.GetJournalEntryLines(invEntry.ID)
	if err != nil {
		t.Fatalf("GetJournalEntryLines(invoice): %v", err)
	}
	payLines, err := d.GetJournalEntryLines(payEntry.ID)
	if err != nil {
		t.Fatalf("GetJournalEntryLines(payment): %v", err)
	}
	arDebit, arCredit := sumLines(invLines, fx.arAccountID)
	payArDebit, payArCredit := sumLines(payLines, fx.arAccountID)
	if arDebit-arCredit+payArDebit-payArCredit != 0 {
		t.Fatalf("AR does not net to zero across both entries: invoice(%d,%d) payment(%d,%d)",
			arDebit, arCredit, payArDebit, payArCredit)
	}

	open, err := d.GetClientOpenInvoices(fx.clientID)
	if err != nil {
		t.Fatalf("GetClientOpenInvoices: %v", err)
	}
	for _, inv := range open {
		if inv.ID == result.Invoice.ID {
			t.Fatalf("fully-paid cash sale still appears in open invoices")
		}
	}
}

func TestCreateCashSaleLoanPartialPaymentStaysOpen(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-cash-sale-partial")
	register := accountByCode(t, d, fx.orgID, "1010")

	result, err := d.CreateCashSale(CreateCashSaleRequest{
		OrganizationID: fx.orgID, ClientID: fx.clientID, Date: fx.date, Currency: "EUR",
		LineItems: []CreateInvoiceLineItemRequest{
			{Quantity: 2, UnitPrice: 1000, TaxRate: &fx.taxRateID, ProductID: &fx.productID},
		},
		SubTotal: 2000, TaxTotal: 400, Total: 2400,
		AmountReceived: 900, PaymentMethod: "cash", BankAccountID: register.ID,
	})
	if err != nil {
		t.Fatalf("CreateCashSale: %v", err)
	}
	if result.Invoice.State != "sent" {
		t.Fatalf("invoice state = %q, want sent", result.Invoice.State)
	}
	if result.Payment == nil {
		t.Fatal("expected a partial payment to have been recorded")
	}

	paid, err := d.GetInvoiceAmountPaid(result.Invoice.ID)
	if err != nil {
		t.Fatalf("GetInvoiceAmountPaid: %v", err)
	}
	if paid != 900 {
		t.Fatalf("GetInvoiceAmountPaid = %d, want 900", paid)
	}

	open, err := d.GetClientOpenInvoices(fx.clientID)
	if err != nil {
		t.Fatalf("GetClientOpenInvoices: %v", err)
	}
	found := false
	for _, inv := range open {
		if inv.ID == result.Invoice.ID {
			found = true
			if inv.Total != 1500 {
				t.Fatalf("open balance = %d, want 1500", inv.Total)
			}
		}
	}
	if !found {
		t.Fatal("partially-paid loan sale should appear in open invoices")
	}
}

func TestCreateCashSaleZeroPaymentLoanSaleHasNoPayment(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-cash-sale-zero")

	result, err := d.CreateCashSale(CreateCashSaleRequest{
		OrganizationID: fx.orgID, ClientID: fx.clientID, Date: fx.date, Currency: "EUR",
		LineItems: []CreateInvoiceLineItemRequest{
			{Quantity: 2, UnitPrice: 1000, TaxRate: &fx.taxRateID, ProductID: &fx.productID},
		},
		SubTotal: 2000, TaxTotal: 400, Total: 2400,
		AmountReceived: 0,
	})
	if err != nil {
		t.Fatalf("CreateCashSale: %v", err)
	}
	if result.Invoice.State != "sent" {
		t.Fatalf("invoice state = %q, want sent", result.Invoice.State)
	}
	if result.Payment != nil {
		t.Fatal("expected no payment to be created for a zero-upfront loan sale")
	}

	invEntry, err := d.FindPostedEntryForSourceDocument("invoice", result.Invoice.ID)
	if err != nil || invEntry == nil {
		t.Fatalf("expected the invoice's own entry to still be posted, err=%v entry=%v", err, invEntry)
	}

	open, err := d.GetClientOpenInvoices(fx.clientID)
	if err != nil {
		t.Fatalf("GetClientOpenInvoices: %v", err)
	}
	found := false
	for _, inv := range open {
		if inv.ID == result.Invoice.ID {
			found = true
			if inv.Total != 2400 {
				t.Fatalf("open balance = %d, want 2400", inv.Total)
			}
		}
	}
	if !found {
		t.Fatal("unpaid loan sale should appear in open invoices")
	}
}

func TestCreateCashSaleInlineClientCreation(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-cash-sale-newclient")
	register := accountByCode(t, d, fx.orgID, "1010")

	result, err := d.CreateCashSale(CreateCashSaleRequest{
		OrganizationID: fx.orgID,
		NewClient:      &CreateClientRequest{Name: ptr("Walk-in Customer"), Phone: ptr("20123456")},
		Date:           fx.date, Currency: "EUR",
		LineItems: []CreateInvoiceLineItemRequest{
			{Quantity: 1, UnitPrice: 1000, TaxRate: &fx.taxRateID, ProductID: &fx.productID},
		},
		SubTotal: 1000, TaxTotal: 200, Total: 1200,
		AmountReceived: 1200, BankAccountID: register.ID,
	})
	if err != nil {
		t.Fatalf("CreateCashSale: %v", err)
	}
	if result.Client.ID == "" {
		t.Fatal("expected a new client to have been created")
	}
	stored, err := d.GetClient(result.Client.ID)
	if err != nil {
		t.Fatalf("GetClient: %v", err)
	}
	if stored.Phone == nil || *stored.Phone != "20123456" {
		t.Fatalf("stored client phone = %v, want 20123456", stored.Phone)
	}
}

func TestCreateCashSaleDuplicatePhoneRejected(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-cash-sale-dupphone")

	if _, err := d.UpdateClient(fx.clientID, UpdateClientRequest{Name: ptr("Test Client"), Phone: ptr("99887766")}); err != nil {
		t.Fatalf("UpdateClient: %v", err)
	}

	_, err := d.CreateCashSale(CreateCashSaleRequest{
		OrganizationID: fx.orgID,
		NewClient:      &CreateClientRequest{Name: ptr("Another Person"), Phone: ptr("99887766")},
		Date:           fx.date, Currency: "EUR",
		LineItems: []CreateInvoiceLineItemRequest{
			{Quantity: 1, UnitPrice: 1000, TaxRate: &fx.taxRateID, ProductID: &fx.productID},
		},
		SubTotal: 1000, TaxTotal: 200, Total: 1200,
		AmountReceived: 1200,
	})
	if err == nil {
		t.Fatal("expected duplicate-phone client creation to be rejected")
	}
}

func TestCreateCashSaleClosedFiscalYearRejectsBeforeAnyWrite(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-cash-sale-noyear")

	clientsBefore, err := d.GetClients(fx.orgID)
	if err != nil {
		t.Fatalf("GetClients: %v", err)
	}

	// fx's fiscal year only covers 2025 — a 2030 date has no open year.
	farFutureDate := int64(1893456000000) // 2030-01-01
	_, err = d.CreateCashSale(CreateCashSaleRequest{
		OrganizationID: fx.orgID, ClientID: fx.clientID, Date: farFutureDate, Currency: "EUR",
		LineItems: []CreateInvoiceLineItemRequest{
			{Quantity: 1, UnitPrice: 1000, TaxRate: &fx.taxRateID, ProductID: &fx.productID},
		},
		SubTotal: 1000, TaxTotal: 200, Total: 1200,
		AmountReceived: 1200,
	})
	if err == nil {
		t.Fatal("expected a date outside any open fiscal year to be rejected")
	}

	clientsAfter, err := d.GetClients(fx.orgID)
	if err != nil {
		t.Fatalf("GetClients: %v", err)
	}
	if len(clientsAfter) != len(clientsBefore) {
		t.Fatalf("expected no client to be committed on rejection, before=%d after=%d", len(clientsBefore), len(clientsAfter))
	}
}

func TestCreateCashSaleSequentialInvoiceNumbersDoNotCollide(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-cash-sale-numbering")
	register := accountByCode(t, d, fx.orgID, "1010")
	// newGLPostingTestFixture's CreateOrganization call doesn't set a number
	// format (an explicit NULL in the INSERT, bypassing the schema's own
	// DEFAULT 'INV-{year}-{number}' — SQLite only applies a column DEFAULT
	// when the column is omitted from the statement entirely), so set one
	// explicitly to make this test meaningful.
	if _, err := d.UpdateOrganization(fx.orgID, UpdateOrganizationRequest{
		InvoiceNumberFormat: ptr("INV-{year}-{number}"),
	}); err != nil {
		t.Fatalf("UpdateOrganization: %v", err)
	}

	req := CreateCashSaleRequest{
		OrganizationID: fx.orgID, ClientID: fx.clientID, Date: fx.date, Currency: "EUR",
		LineItems: []CreateInvoiceLineItemRequest{
			{Quantity: 1, UnitPrice: 1000, TaxRate: &fx.taxRateID, ProductID: &fx.productID},
		},
		SubTotal: 1000, TaxTotal: 200, Total: 1200,
		AmountReceived: 1200, BankAccountID: register.ID,
	}

	first, err := d.CreateCashSale(req)
	if err != nil {
		t.Fatalf("CreateCashSale (first): %v", err)
	}
	second, err := d.CreateCashSale(req)
	if err != nil {
		t.Fatalf("CreateCashSale (second): %v", err)
	}

	if first.Invoice.Number == second.Invoice.Number {
		t.Fatalf("two consecutive cash sales produced the same invoice number: %q", first.Invoice.Number)
	}
	if first.Invoice.Number != "INV-2025-1" || second.Invoice.Number != "INV-2025-2" {
		t.Fatalf("got numbers %q, %q, want INV-2025-1, INV-2025-2", first.Invoice.Number, second.Invoice.Number)
	}
}

func TestCreateCashSaleZeroTotalSkipsGLPosting(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-cash-sale-zerototal")

	result, err := d.CreateCashSale(CreateCashSaleRequest{
		OrganizationID: fx.orgID, ClientID: fx.clientID, Date: fx.date, Currency: "EUR",
		LineItems: []CreateInvoiceLineItemRequest{
			{Quantity: 1, UnitPrice: 0, ProductID: &fx.productID},
		},
		SubTotal: 0, TaxTotal: 0, Total: 0,
		AmountReceived: 0,
	})
	if err != nil {
		t.Fatalf("CreateCashSale: %v", err)
	}
	if result.Invoice.State != "paid" {
		t.Fatalf("invoice state = %q, want paid (nothing owed)", result.Invoice.State)
	}
	entry, err := d.FindPostedEntryForSourceDocument("invoice", result.Invoice.ID)
	if err != nil {
		t.Fatalf("FindPostedEntryForSourceDocument: %v", err)
	}
	if entry != nil {
		t.Fatal("expected no GL entry to be posted for a zero-total sale")
	}
}

func TestCreateCashSaleAmountReceivedExceedsTotalRejected(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-cash-sale-overpay")

	_, err := d.CreateCashSale(CreateCashSaleRequest{
		OrganizationID: fx.orgID, ClientID: fx.clientID, Date: fx.date, Currency: "EUR",
		LineItems: []CreateInvoiceLineItemRequest{
			{Quantity: 1, UnitPrice: 1000, TaxRate: &fx.taxRateID, ProductID: &fx.productID},
		},
		SubTotal: 1000, TaxTotal: 200, Total: 1200,
		AmountReceived: 1300,
	})
	if err == nil {
		t.Fatal("expected amount received exceeding the sale total to be rejected")
	}
}

func TestCreateCashSaleInvalidPaymentMethodRejected(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-cash-sale-badmethod")

	_, err := d.CreateCashSale(CreateCashSaleRequest{
		OrganizationID: fx.orgID, ClientID: fx.clientID, Date: fx.date, Currency: "EUR",
		LineItems: []CreateInvoiceLineItemRequest{
			{Quantity: 1, UnitPrice: 1000, TaxRate: &fx.taxRateID, ProductID: &fx.productID},
		},
		SubTotal: 1000, TaxTotal: 200, Total: 1200,
		AmountReceived: 1200, PaymentMethod: "bitcoin",
	})
	if err == nil {
		t.Fatal("expected an unrecognized payment method to be rejected")
	}
}

// F101: the counter account must be the dedicated register/till account, not
// defaultCashAccountId — every chart-of-accounts template wires the "cash"
// role to Bank (db/account.go), so falling back to it silently credited Bank
// while the Cash Book register balance/report watched the till. A sale with
// no register account configured and no explicit bankAccountId must be
// rejected with a clear error naming defaultCashRegisterAccountId, even when
// defaultCashAccountId *is* set (which it is by every seeded chart).
func TestCreateCashSaleNoRegisterAccountRejected(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-cash-sale-noregister")

	// The org is seeded with defaultCashAccountId wired to Bank. Leave it
	// set deliberately, and make sure no register account is configured —
	// "" clears a default*AccountId column (see UpdateOrganization's
	// three-way convention), unlike nil which means "leave it alone".
	if _, err := d.UpdateOrganization(fx.orgID, UpdateOrganizationRequest{
		DefaultCashRegisterAccountID: ptr(""),
	}); err != nil {
		t.Fatalf("UpdateOrganization: %v", err)
	}

	_, err := d.CreateCashSale(CreateCashSaleRequest{
		OrganizationID: fx.orgID, ClientID: fx.clientID, Date: fx.date, Currency: "EUR",
		LineItems: []CreateInvoiceLineItemRequest{
			{Quantity: 1, UnitPrice: 1000, TaxRate: &fx.taxRateID, ProductID: &fx.productID},
		},
		SubTotal: 1000, TaxTotal: 200, Total: 1200,
		AmountReceived: 1200, // BankAccountID deliberately left empty
	})
	if err == nil {
		t.Fatal("expected a cash sale with no register account configured to be rejected")
	}
	if !strings.Contains(err.Error(), "defaultCashRegisterAccountId") {
		t.Fatalf("error = %q, want it to name defaultCashRegisterAccountId", err.Error())
	}
}

// F101's positive counterpart: a zero-deposit loan sale has no payment to
// post, so it must still work with no register account configured — the new
// requirement applies only to the cash-sale (amount received > 0) path.
func TestCreateCashSaleZeroDepositLoanNeedsNoRegisterAccount(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-cash-sale-noregister-loan")

	if _, err := d.UpdateOrganization(fx.orgID, UpdateOrganizationRequest{
		DefaultCashRegisterAccountID: ptr(""),
	}); err != nil {
		t.Fatalf("UpdateOrganization: %v", err)
	}

	result, err := d.CreateCashSale(CreateCashSaleRequest{
		OrganizationID: fx.orgID, ClientID: fx.clientID, Date: fx.date, Currency: "EUR",
		LineItems: []CreateInvoiceLineItemRequest{
			{Quantity: 1, UnitPrice: 1000, TaxRate: &fx.taxRateID, ProductID: &fx.productID},
		},
		SubTotal: 1000, TaxTotal: 200, Total: 1200,
		AmountReceived: 0,
	})
	if err != nil {
		t.Fatalf("CreateCashSale (zero-deposit loan): %v", err)
	}
	if result.Payment != nil {
		t.Fatal("expected no payment for a zero-deposit loan sale")
	}
}

func TestCreateCashSaleCrossOrgClientRejected(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-cash-sale-crossorg-a")
	other := newGLPostingTestFixture(t, d, "org-cash-sale-crossorg-b")
	register := accountByCode(t, d, fx.orgID, "1010")

	_, err := d.CreateCashSale(CreateCashSaleRequest{
		OrganizationID: fx.orgID, ClientID: other.clientID, Date: fx.date, Currency: "EUR",
		LineItems: []CreateInvoiceLineItemRequest{
			{Quantity: 1, UnitPrice: 1000, TaxRate: &fx.taxRateID, ProductID: &fx.productID},
		},
		SubTotal: 1000, TaxTotal: 200, Total: 1200,
		AmountReceived: 1200, BankAccountID: register.ID,
	})
	if err == nil {
		t.Fatal("expected a client belonging to a different organization to be rejected")
	}
}

func TestCreateCashSaleCrossOrgBankAccountRejected(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-cash-sale-crossorg-c")
	other := newGLPostingTestFixture(t, d, "org-cash-sale-crossorg-d")

	_, err := d.CreateCashSale(CreateCashSaleRequest{
		OrganizationID: fx.orgID, ClientID: fx.clientID, Date: fx.date, Currency: "EUR",
		LineItems: []CreateInvoiceLineItemRequest{
			{Quantity: 1, UnitPrice: 1000, TaxRate: &fx.taxRateID, ProductID: &fx.productID},
		},
		SubTotal: 1000, TaxTotal: 200, Total: 1200,
		AmountReceived: 1200, BankAccountID: other.arAccountID,
	})
	if err == nil {
		t.Fatal("expected a bank/cash account belonging to a different organization to be rejected")
	}
}

func TestCreateCashSaleForeignCurrencyRejected(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-cash-sale-forex")
	register := accountByCode(t, d, fx.orgID, "1010")

	// newGLPostingTestFixture's org never sets a currency, so
	// orgCurrencyOrDefault falls back to "EUR" (db/exchange_rate.go) — any
	// other currency must be rejected outright, since CreateCashSaleRequest
	// has no field to supply an exchange rate for it.
	_, err := d.CreateCashSale(CreateCashSaleRequest{
		OrganizationID: fx.orgID, ClientID: fx.clientID, Date: fx.date, Currency: "USD",
		LineItems: []CreateInvoiceLineItemRequest{
			{Quantity: 1, UnitPrice: 1000, TaxRate: &fx.taxRateID, ProductID: &fx.productID},
		},
		SubTotal: 1000, TaxTotal: 200, Total: 1200,
		AmountReceived: 1200, BankAccountID: register.ID,
	})
	if err == nil {
		t.Fatal("expected a non-organization currency to be rejected")
	}
}

// The ordinary Invoices screen computes its own number frontend-side
// (src/utils/invoice.ts) and CreateInvoice bumps organizations.
// invoice_number_counter by 1 regardless of what Number it was given — the
// same counter column CreateCashSale reads and bumps inside its own
// transaction (see generateDocumentNumber's doc comment). A cash sale
// created right after an ordinary invoice must pick up where that bump left
// off, not recompute a number the ordinary invoice already used.
func TestCreateCashSaleInvoiceNumberDoesNotCollideWithCreateInvoice(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-cash-sale-numbering-cross")
	register := accountByCode(t, d, fx.orgID, "1010")
	if _, err := d.UpdateOrganization(fx.orgID, UpdateOrganizationRequest{
		InvoiceNumberFormat: ptr("INV-{year}-{number}"),
	}); err != nil {
		t.Fatalf("UpdateOrganization: %v", err)
	}

	ordinary, err := d.CreateInvoice(CreateInvoiceRequest{
		OrganizationID: fx.orgID, Number: "INV-2025-1", State: "draft", ClientID: fx.clientID,
		Date: fx.date, Currency: "EUR",
		LineItems: []CreateInvoiceLineItemRequest{
			{Quantity: 1, UnitPrice: 1000, TaxRate: &fx.taxRateID, ProductID: &fx.productID},
		},
		SubTotal: 1000, TaxTotal: 200, Total: 1200,
	})
	if err != nil {
		t.Fatalf("CreateInvoice: %v", err)
	}

	result, err := d.CreateCashSale(CreateCashSaleRequest{
		OrganizationID: fx.orgID, ClientID: fx.clientID, Date: fx.date, Currency: "EUR",
		LineItems: []CreateInvoiceLineItemRequest{
			{Quantity: 1, UnitPrice: 1000, TaxRate: &fx.taxRateID, ProductID: &fx.productID},
		},
		SubTotal: 1000, TaxTotal: 200, Total: 1200,
		AmountReceived: 1200, BankAccountID: register.ID,
	})
	if err != nil {
		t.Fatalf("CreateCashSale: %v", err)
	}

	if result.Invoice.Number == ordinary.Number {
		t.Fatalf("cash sale reused the ordinary invoice's number %q", ordinary.Number)
	}
	if result.Invoice.Number != "INV-2025-2" {
		t.Fatalf("cash sale invoice number = %q, want INV-2025-2", result.Invoice.Number)
	}
}

// Mirrors the Cash Book screen's "pay off an existing loan sale" flow end to
// end: CreateCashSale with no upfront payment, then a separate CreatePayment
// call (what PaymentPanel does) settling the full balance, then the follow-up
// UpdateInvoiceState("paid") call the screen itself makes — see
// db/cash_sale.go's CreateCashSale doc comment and
// src/routes/cash-book.tsx's handleSettled. The invoice must leave the open
// invoices list once all three steps have run.
func TestCreateCashSaleLoanSaleFullRoundTripSettlesToPaid(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-cash-sale-roundtrip")
	cash := accountByCode(t, d, fx.orgID, "1010")

	result, err := d.CreateCashSale(CreateCashSaleRequest{
		OrganizationID: fx.orgID, ClientID: fx.clientID, Date: fx.date, Currency: "EUR",
		LineItems: []CreateInvoiceLineItemRequest{
			{Quantity: 2, UnitPrice: 1000, TaxRate: &fx.taxRateID, ProductID: &fx.productID},
		},
		SubTotal: 2000, TaxTotal: 400, Total: 2400,
		AmountReceived: 0,
	})
	if err != nil {
		t.Fatalf("CreateCashSale: %v", err)
	}
	if result.Invoice.State != "sent" {
		t.Fatalf("invoice state = %q, want sent", result.Invoice.State)
	}

	if _, err := d.CreatePayment(CreatePaymentRequest{
		OrganizationID: fx.orgID, Direction: "inbound", ClientID: &fx.clientID,
		BankAccountID: cash.ID, Amount: 2400, Currency: "EUR", Date: fx.date, Method: "cash",
		Applications: []CreatePaymentApplicationRequest{
			{DocumentType: "invoice", DocumentID: result.Invoice.ID, Amount: 2400},
		},
	}); err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}

	paid, err := d.GetInvoiceAmountPaid(result.Invoice.ID)
	if err != nil {
		t.Fatalf("GetInvoiceAmountPaid: %v", err)
	}
	if paid != 2400 {
		t.Fatalf("GetInvoiceAmountPaid = %d, want 2400", paid)
	}
	// Invoice state itself is a manual flag everywhere except this screen's
	// own explicit follow-up call (see CreateCashSale's doc comment) — it's
	// still "sent" until that call runs, even though the balance is zero.
	if inv, err := d.GetInvoice(result.Invoice.ID); err != nil {
		t.Fatalf("GetInvoice: %v", err)
	} else if inv.State != "sent" {
		t.Fatalf("invoice state before the follow-up call = %q, want sent", inv.State)
	}

	if _, err := d.UpdateInvoiceState(result.Invoice.ID, "paid"); err != nil {
		t.Fatalf("UpdateInvoiceState: %v", err)
	}

	final, err := d.GetInvoice(result.Invoice.ID)
	if err != nil {
		t.Fatalf("GetInvoice: %v", err)
	}
	if final.State != "paid" {
		t.Fatalf("final invoice state = %q, want paid", final.State)
	}

	open, err := d.GetClientOpenInvoices(fx.clientID)
	if err != nil {
		t.Fatalf("GetClientOpenInvoices: %v", err)
	}
	for _, inv := range open {
		if inv.ID == result.Invoice.ID {
			t.Fatal("fully-settled loan sale should no longer appear in open invoices")
		}
	}
}

// A client can already have a draft invoice from the ordinary Invoices
// screen (CreateInvoice defaults to state "draft") with a nonzero balance.
// GetClientOpenInvoices must not surface it: a draft has no posted GL entry
// (needsInvoiceGLPresence requires sent/paid), so Cash Book's "Pay" button
// would otherwise open PaymentPanel against a document CreatePayment always
// rejects.
func TestCreateCashSaleGetClientOpenInvoicesExcludesDrafts(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-cash-sale-drafts")

	draft, err := d.CreateInvoice(CreateInvoiceRequest{
		OrganizationID: fx.orgID, State: "draft", ClientID: fx.clientID, Date: fx.date, Currency: "EUR",
		LineItems: []CreateInvoiceLineItemRequest{
			{Quantity: 1, UnitPrice: 1000, TaxRate: &fx.taxRateID, ProductID: &fx.productID},
		},
		SubTotal: 1000, TaxTotal: 200, Total: 1200,
	})
	if err != nil {
		t.Fatalf("CreateInvoice: %v", err)
	}

	open, err := d.GetClientOpenInvoices(fx.clientID)
	if err != nil {
		t.Fatalf("GetClientOpenInvoices: %v", err)
	}
	for _, inv := range open {
		if inv.ID == draft.ID {
			t.Fatal("a draft invoice with no posted GL entry must not appear as payable in Cash Book")
		}
	}
}

// A cash sale has no invoicing due-date concept (it's paid or settled via
// its own loan mechanism, not chased on terms) — dueDate must default to
// the sale's own date rather than staying nil, which the Invoices list
// otherwise renders as a bare, unexplained "-".
func TestCreateCashSaleDueDateDefaultsToSaleDate(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-cash-sale-due-date")
	register := accountByCode(t, d, fx.orgID, "1010")

	result, err := d.CreateCashSale(CreateCashSaleRequest{
		OrganizationID: fx.orgID, ClientID: fx.clientID, Date: fx.date, Currency: "EUR",
		LineItems: []CreateInvoiceLineItemRequest{
			{Quantity: 1, UnitPrice: 1000, TaxRate: &fx.taxRateID, ProductID: &fx.productID},
		},
		SubTotal: 1000, TaxTotal: 200, Total: 1200,
		AmountReceived: 1200, PaymentMethod: "cash", BankAccountID: register.ID,
	})
	if err != nil {
		t.Fatalf("CreateCashSale: %v", err)
	}
	if result.Invoice.DueDate == nil || *result.Invoice.DueDate != fx.date {
		t.Fatalf("invoice DueDate = %v, want %d (the sale's own date)", result.Invoice.DueDate, fx.date)
	}
}

// defaultCashAccountId is wired to Bank in every chart-of-accounts template
// (see CLAUDE.md's cash register account note) — a cash sale with no
// explicit bankAccountId must still land in the register when one is
// configured, not silently default to Bank the way it did before this was
// fixed (the exact bug behind a real production report: the balance widget
// watched the register account while every sale credited Bank instead).
func TestCreateCashSaleDefaultsToRegisterAccountNotBank(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-cash-sale-register-default")

	accounts, err := d.GetAccounts(fx.orgID)
	if err != nil {
		t.Fatalf("GetAccounts: %v", err)
	}
	var registerAccountID string
	for _, a := range accounts {
		if a.Code == "1010" { // "Cash" — the generic chart's till account
			registerAccountID = a.ID
		}
	}
	if registerAccountID == "" {
		t.Fatal("expected a seeded account with code 1010 (Cash)")
	}
	if _, err := d.UpdateOrganization(fx.orgID, UpdateOrganizationRequest{
		DefaultCashRegisterAccountID: ptr(registerAccountID),
	}); err != nil {
		t.Fatalf("UpdateOrganization: %v", err)
	}

	result, err := d.CreateCashSale(CreateCashSaleRequest{
		OrganizationID: fx.orgID, ClientID: fx.clientID, Date: fx.date, Currency: "EUR",
		LineItems: []CreateInvoiceLineItemRequest{
			{Quantity: 1, UnitPrice: 1000, TaxRate: &fx.taxRateID, ProductID: &fx.productID},
		},
		SubTotal: 1000, TaxTotal: 200, Total: 1200,
		AmountReceived: 1200, // BankAccountID deliberately left empty
	})
	if err != nil {
		t.Fatalf("CreateCashSale: %v", err)
	}
	if result.Payment.BankAccountID != registerAccountID {
		t.Fatalf("payment posted to account %q, want the register account %q (not Bank)", result.Payment.BankAccountID, registerAccountID)
	}

	// The GL settlement entry must actually debit the register, not just
	// carry it on the payments row — this is the number the register
	// balance/report is built from (GetAccountBalance), so a mismatch here
	// is exactly the production bug F101 names.
	payEntry, err := d.FindPostedEntryForSourceDocument("payment", result.Payment.ID)
	if err != nil || payEntry == nil {
		t.Fatalf("expected a posted entry for the payment, err=%v entry=%v", err, payEntry)
	}
	payLines, err := d.GetJournalEntryLines(payEntry.ID)
	if err != nil {
		t.Fatalf("GetJournalEntryLines: %v", err)
	}
	registerDebit, registerCredit := sumLines(payLines, registerAccountID)
	if registerDebit-registerCredit != 1200 {
		t.Fatalf("register net debit = %d, want 1200 (Dr register on a cash sale)", registerDebit-registerCredit)
	}
}
