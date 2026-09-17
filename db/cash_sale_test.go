package db

import "testing"

func TestCreateCashSaleFullCashSaleMarksInvoicePaid(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-cash-sale-full")

	result, err := d.CreateCashSale(CreateCashSaleRequest{
		OrganizationID: fx.orgID, ClientID: fx.clientID, Date: fx.date, Currency: "EUR",
		LineItems: []CreateInvoiceLineItemRequest{
			{Quantity: 2, UnitPrice: 1000, TaxRate: &fx.taxRateID, ProductID: &fx.productID},
		},
		SubTotal: 2000, TaxTotal: 400, Total: 2400,
		AmountReceived: 2400, PaymentMethod: "cash",
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

	result, err := d.CreateCashSale(CreateCashSaleRequest{
		OrganizationID: fx.orgID, ClientID: fx.clientID, Date: fx.date, Currency: "EUR",
		LineItems: []CreateInvoiceLineItemRequest{
			{Quantity: 2, UnitPrice: 1000, TaxRate: &fx.taxRateID, ProductID: &fx.productID},
		},
		SubTotal: 2000, TaxTotal: 400, Total: 2400,
		AmountReceived: 900, PaymentMethod: "cash",
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

	result, err := d.CreateCashSale(CreateCashSaleRequest{
		OrganizationID: fx.orgID,
		NewClient:      &CreateClientRequest{Name: ptr("Walk-in Customer"), Phone: ptr("20123456")},
		Date:           fx.date, Currency: "EUR",
		LineItems: []CreateInvoiceLineItemRequest{
			{Quantity: 1, UnitPrice: 1000, TaxRate: &fx.taxRateID, ProductID: &fx.productID},
		},
		SubTotal: 1000, TaxTotal: 200, Total: 1200,
		AmountReceived: 1200,
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
		AmountReceived: 1200,
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
