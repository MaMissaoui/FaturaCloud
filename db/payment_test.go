package db

import (
	"testing"
)

// accountByCode is a small test helper shared across payment tests to look
// up a seeded account (e.g. "1020" Bank, "4200" FX Gain) by its chart-of-
// accounts code.
func accountByCode(t *testing.T, d *Database, orgID, code string) Account {
	t.Helper()
	accounts, err := d.GetAccounts(orgID)
	if err != nil {
		t.Fatalf("GetAccounts: %v", err)
	}
	for _, a := range accounts {
		if a.Code == code {
			return a
		}
	}
	t.Fatalf("account with code %q not found", code)
	return Account{}
}

func TestCreatePaymentFullySettlesInvoiceSameCurrency(t *testing.T) {
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-pay-full")
	inv := fx.createInvoice(t, d, "inv-1", 2, 1000) // total 2400 (2000 + 400 tax)
	if _, err := d.UpdateInvoiceState(inv.ID, "sent"); err != nil {
		t.Fatalf("UpdateInvoiceState(sent): %v", err)
	}
	bank := accountByCode(t, d, fx.orgID, "1020")

	payment, err := d.CreatePayment(CreatePaymentRequest{
		OrganizationID: fx.orgID, Direction: "inbound", ClientID: &fx.clientID,
		BankAccountID: bank.ID, Amount: 2400, Currency: "EUR", Date: fx.date, Method: "bank_transfer",
		Applications: []CreatePaymentApplicationRequest{{DocumentType: "invoice", DocumentID: inv.ID, Amount: 2400}},
	})
	if err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}
	if payment.JournalEntryID == nil {
		t.Fatal("expected payment to have a journal entry")
	}

	lines, err := d.GetJournalEntryLines(*payment.JournalEntryID)
	if err != nil {
		t.Fatalf("GetJournalEntryLines: %v", err)
	}
	bankDebit, bankCredit := sumLines(lines, bank.ID)
	if bankDebit != 2400 || bankCredit != 0 {
		t.Fatalf("bank line = debit %d credit %d, want debit 2400 credit 0", bankDebit, bankCredit)
	}
	arDebit, arCredit := sumLines(lines, fx.arAccountID)
	if arDebit != 0 || arCredit != 2400 {
		t.Fatalf("AR line = debit %d credit %d, want debit 0 credit 2400", arDebit, arCredit)
	}
	var totalDebit, totalCredit int64
	for _, l := range lines {
		totalDebit += l.Debit
		totalCredit += l.Credit
	}
	if totalDebit != totalCredit {
		t.Fatalf("entry does not balance: debit %d credit %d", totalDebit, totalCredit)
	}

	paid, err := d.GetInvoiceAmountPaid(inv.ID)
	if err != nil {
		t.Fatalf("GetInvoiceAmountPaid: %v", err)
	}
	if paid != 2400 {
		t.Fatalf("GetInvoiceAmountPaid = %d, want 2400", paid)
	}
}

func TestCreatePaymentPartialThenOverpayRejected(t *testing.T) {
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-pay-partial")
	inv := fx.createInvoice(t, d, "inv-1", 1, 1000) // total 1200
	if _, err := d.UpdateInvoiceState(inv.ID, "sent"); err != nil {
		t.Fatalf("UpdateInvoiceState(sent): %v", err)
	}
	bank := accountByCode(t, d, fx.orgID, "1020")

	if _, err := d.CreatePayment(CreatePaymentRequest{
		OrganizationID: fx.orgID, Direction: "inbound", ClientID: &fx.clientID,
		BankAccountID: bank.ID, Amount: 700, Currency: "EUR", Date: fx.date, Method: "bank_transfer",
		Applications: []CreatePaymentApplicationRequest{{DocumentType: "invoice", DocumentID: inv.ID, Amount: 700}},
	}); err != nil {
		t.Fatalf("CreatePayment (partial): %v", err)
	}

	paid, err := d.GetInvoiceAmountPaid(inv.ID)
	if err != nil {
		t.Fatalf("GetInvoiceAmountPaid: %v", err)
	}
	if paid != 700 {
		t.Fatalf("GetInvoiceAmountPaid = %d, want 700", paid)
	}

	// Remaining balance is 500 — attempting to apply 600 must be rejected.
	_, err = d.CreatePayment(CreatePaymentRequest{
		OrganizationID: fx.orgID, Direction: "inbound", ClientID: &fx.clientID,
		BankAccountID: bank.ID, Amount: 600, Currency: "EUR", Date: fx.date, Method: "bank_transfer",
		Applications: []CreatePaymentApplicationRequest{{DocumentType: "invoice", DocumentID: inv.ID, Amount: 600}},
	})
	if err == nil {
		t.Fatal("expected overpayment to be rejected")
	}

	// The remaining 500 settles cleanly.
	if _, err := d.CreatePayment(CreatePaymentRequest{
		OrganizationID: fx.orgID, Direction: "inbound", ClientID: &fx.clientID,
		BankAccountID: bank.ID, Amount: 500, Currency: "EUR", Date: fx.date, Method: "bank_transfer",
		Applications: []CreatePaymentApplicationRequest{{DocumentType: "invoice", DocumentID: inv.ID, Amount: 500}},
	}); err != nil {
		t.Fatalf("CreatePayment (remaining): %v", err)
	}
	paid, err = d.GetInvoiceAmountPaid(inv.ID)
	if err != nil {
		t.Fatalf("GetInvoiceAmountPaid: %v", err)
	}
	if paid != 1200 {
		t.Fatalf("GetInvoiceAmountPaid = %d, want 1200", paid)
	}
}

// TestCreatePaymentForeignCurrencyRealizesGain works a concrete example so
// the FX sign convention is checked against real numbers rather than
// reasoned about in prose: a $100.00 USD invoice booked at rate 0.90 (AR
// recorded at €90.00) collected in full once USD has strengthened to rate
// 1.00 (bank receives €100.00). The seller ends up €10.00 better off than
// the AR was booked for — a real realized gain — which must land as a
// credit to the FX Gain account, not a debit.
func TestCreatePaymentForeignCurrencyRealizesGain(t *testing.T) {
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-pay-fx-gain")

	inv, err := d.CreateInvoice(CreateInvoiceRequest{
		OrganizationID: fx.orgID, Number: "inv-usd-1", ClientID: fx.clientID,
		Date: fx.date, Currency: "USD", ExchangeRate: ptr(0.90),
		SubTotal: 10000, TaxTotal: 0, Total: 10000,
		LineItems: []CreateInvoiceLineItemRequest{{Quantity: 1, UnitPrice: 10000, ProductID: &fx.productID}},
	})
	if err != nil {
		t.Fatalf("CreateInvoice: %v", err)
	}
	if _, err := d.UpdateInvoiceState(inv.ID, "sent"); err != nil {
		t.Fatalf("UpdateInvoiceState(sent): %v", err)
	}
	original, err := d.FindPostedEntryForSourceDocument("invoice", inv.ID)
	if err != nil || original == nil {
		t.Fatalf("expected a posted entry, err=%v entry=%v", err, original)
	}
	originalLines, err := d.GetJournalEntryLines(original.ID)
	if err != nil {
		t.Fatalf("GetJournalEntryLines(original): %v", err)
	}
	arDebit, _ := sumLines(originalLines, fx.arAccountID)
	if arDebit != 9000 {
		t.Fatalf("original AR debit = %d, want 9000 (10000 USD @ 0.90)", arDebit)
	}

	bank := accountByCode(t, d, fx.orgID, "1020")
	fxGain := accountByCode(t, d, fx.orgID, "4200")

	payment, err := d.CreatePayment(CreatePaymentRequest{
		OrganizationID: fx.orgID, Direction: "inbound", ClientID: &fx.clientID,
		BankAccountID: bank.ID, Amount: 10000, Currency: "USD", ExchangeRate: ptr(1.00),
		Date: fx.date, Method: "bank_transfer",
		Applications: []CreatePaymentApplicationRequest{{DocumentType: "invoice", DocumentID: inv.ID, Amount: 10000}},
	})
	if err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}

	lines, err := d.GetJournalEntryLines(*payment.JournalEntryID)
	if err != nil {
		t.Fatalf("GetJournalEntryLines: %v", err)
	}
	bankDebit, _ := sumLines(lines, bank.ID)
	if bankDebit != 10000 {
		t.Fatalf("bank debit = %d, want 10000 (10000 USD @ 1.00)", bankDebit)
	}
	_, arCredit := sumLines(lines, fx.arAccountID)
	if arCredit != 9000 {
		t.Fatalf("AR credit = %d, want 9000 (settled at the invoice's own 0.90 rate)", arCredit)
	}
	gainDebit, gainCredit := sumLines(lines, fxGain.ID)
	if gainDebit != 0 || gainCredit != 1000 {
		t.Fatalf("FX gain line = debit %d credit %d, want debit 0 credit 1000", gainDebit, gainCredit)
	}
	var totalDebit, totalCredit int64
	for _, l := range lines {
		totalDebit += l.Debit
		totalCredit += l.Credit
	}
	if totalDebit != totalCredit {
		t.Fatalf("entry does not balance: debit %d credit %d", totalDebit, totalCredit)
	}
}

// TestCreatePaymentOutboundRealizesLoss is the outbound mirror: a $100.00
// USD bill booked at rate 0.90 (AP recorded at €90.00) paid in full once
// USD has strengthened to rate 1.00 (€100.00 cash out). The org paid €10.00
// more than the liability was booked for — a real realized loss — which
// must land as a debit to the FX Loss account.
func TestCreatePaymentOutboundRealizesLoss(t *testing.T) {
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-pay-fx-loss")

	bill, err := d.CreateIncomingInvoice(CreateIncomingInvoiceRequest{
		OrganizationID: fx.orgID, VendorID: fx.vendorID, VendorInvoiceNumber: "bill-usd-1",
		Date: fx.date, Currency: "USD", ExchangeRate: ptr(0.90),
		SubTotal: 10000, TaxTotal: 0, Total: 10000,
		LineItems: []CreateInvoiceLineItemRequest{{Quantity: 1, UnitPrice: 10000, ProductID: &fx.productID}},
	})
	if err != nil {
		t.Fatalf("CreateIncomingInvoice: %v", err)
	}
	if _, err := d.UpdateIncomingInvoiceState(bill.ID, "approved"); err != nil {
		t.Fatalf("UpdateIncomingInvoiceState(approved): %v", err)
	}

	bank := accountByCode(t, d, fx.orgID, "1020")
	fxLoss := accountByCode(t, d, fx.orgID, "5200")

	payment, err := d.CreatePayment(CreatePaymentRequest{
		OrganizationID: fx.orgID, Direction: "outbound", VendorID: &fx.vendorID,
		BankAccountID: bank.ID, Amount: 10000, Currency: "USD", ExchangeRate: ptr(1.00),
		Date: fx.date, Method: "bank_transfer",
		Applications: []CreatePaymentApplicationRequest{{DocumentType: "incoming_invoice", DocumentID: bill.ID, Amount: 10000}},
	})
	if err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}

	lines, err := d.GetJournalEntryLines(*payment.JournalEntryID)
	if err != nil {
		t.Fatalf("GetJournalEntryLines: %v", err)
	}
	_, bankCredit := sumLines(lines, bank.ID)
	if bankCredit != 10000 {
		t.Fatalf("bank credit = %d, want 10000", bankCredit)
	}
	apDebit, _ := sumLines(lines, fx.apAccountID)
	if apDebit != 9000 {
		t.Fatalf("AP debit = %d, want 9000 (settled at the bill's own 0.90 rate)", apDebit)
	}
	lossDebit, lossCredit := sumLines(lines, fxLoss.ID)
	if lossDebit != 1000 || lossCredit != 0 {
		t.Fatalf("FX loss line = debit %d credit %d, want debit 1000 credit 0", lossDebit, lossCredit)
	}
	var totalDebit, totalCredit int64
	for _, l := range lines {
		totalDebit += l.Debit
		totalCredit += l.Credit
	}
	if totalDebit != totalCredit {
		t.Fatalf("entry does not balance: debit %d credit %d", totalDebit, totalCredit)
	}
}

func TestVoidPaymentReversesAndRestoresBalance(t *testing.T) {
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-pay-void")
	inv := fx.createInvoice(t, d, "inv-1", 1, 1000) // total 1200
	if _, err := d.UpdateInvoiceState(inv.ID, "sent"); err != nil {
		t.Fatalf("UpdateInvoiceState(sent): %v", err)
	}
	bank := accountByCode(t, d, fx.orgID, "1020")

	payment, err := d.CreatePayment(CreatePaymentRequest{
		OrganizationID: fx.orgID, Direction: "inbound", ClientID: &fx.clientID,
		BankAccountID: bank.ID, Amount: 1200, Currency: "EUR", Date: fx.date, Method: "bank_transfer",
		Applications: []CreatePaymentApplicationRequest{{DocumentType: "invoice", DocumentID: inv.ID, Amount: 1200}},
	})
	if err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}

	voided, err := d.VoidPayment(payment.ID, fx.date)
	if err != nil {
		t.Fatalf("VoidPayment: %v", err)
	}
	if voided.Status != "voided" {
		t.Fatalf("payment status = %q, want voided", voided.Status)
	}
	if voided.VoidingEntryID == nil {
		t.Fatal("expected a voiding entry id")
	}

	originalEntry, err := d.GetJournalEntry(*payment.JournalEntryID)
	if err != nil {
		t.Fatalf("GetJournalEntry: %v", err)
	}
	if originalEntry.Status != "reversed" {
		t.Fatalf("original payment entry status = %q, want reversed", originalEntry.Status)
	}

	paid, err := d.GetInvoiceAmountPaid(inv.ID)
	if err != nil {
		t.Fatalf("GetInvoiceAmountPaid: %v", err)
	}
	if paid != 0 {
		t.Fatalf("GetInvoiceAmountPaid after void = %d, want 0", paid)
	}

	// The full balance is payable again.
	if _, err := d.CreatePayment(CreatePaymentRequest{
		OrganizationID: fx.orgID, Direction: "inbound", ClientID: &fx.clientID,
		BankAccountID: bank.ID, Amount: 1200, Currency: "EUR", Date: fx.date, Method: "bank_transfer",
		Applications: []CreatePaymentApplicationRequest{{DocumentType: "invoice", DocumentID: inv.ID, Amount: 1200}},
	}); err != nil {
		t.Fatalf("CreatePayment (after void): %v", err)
	}
}

func TestCreatePaymentRejectedWithoutPostedEntry(t *testing.T) {
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-pay-no-entry")
	inv := fx.createInvoice(t, d, "inv-1", 1, 1000) // still draft — never sent
	bank := accountByCode(t, d, fx.orgID, "1020")

	_, err := d.CreatePayment(CreatePaymentRequest{
		OrganizationID: fx.orgID, Direction: "inbound", ClientID: &fx.clientID,
		BankAccountID: bank.ID, Amount: 1200, Currency: "EUR", Date: fx.date, Method: "bank_transfer",
		Applications: []CreatePaymentApplicationRequest{{DocumentType: "invoice", DocumentID: inv.ID, Amount: 1200}},
	})
	if err == nil {
		t.Fatal("expected payment against a draft (unposted) invoice to be rejected")
	}
}

func TestUpdateInvoiceStateBlockedWhilePaymentOpen(t *testing.T) {
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-pay-state-guard")
	inv := fx.createInvoice(t, d, "inv-1", 1, 1000) // total 1200
	if _, err := d.UpdateInvoiceState(inv.ID, "sent"); err != nil {
		t.Fatalf("UpdateInvoiceState(sent): %v", err)
	}
	bank := accountByCode(t, d, fx.orgID, "1020")

	payment, err := d.CreatePayment(CreatePaymentRequest{
		OrganizationID: fx.orgID, Direction: "inbound", ClientID: &fx.clientID,
		BankAccountID: bank.ID, Amount: 500, Currency: "EUR", Date: fx.date, Method: "bank_transfer",
		Applications: []CreatePaymentApplicationRequest{{DocumentType: "invoice", DocumentID: inv.ID, Amount: 500}},
	})
	if err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}

	// Bouncing back to draft would reverse the invoice's posted entry while
	// the payment's settlement line still points at it — must be rejected.
	if _, err := d.UpdateInvoiceState(inv.ID, "draft"); err == nil {
		t.Fatal("expected state change to be rejected while a payment is still applied")
	}
	if _, err := d.UpdateInvoiceState(inv.ID, "cancelled"); err == nil {
		t.Fatal("expected cancellation to be rejected while a payment is still applied")
	}

	if _, err := d.VoidPayment(payment.ID, fx.date); err != nil {
		t.Fatalf("VoidPayment: %v", err)
	}
	if _, err := d.UpdateInvoiceState(inv.ID, "draft"); err != nil {
		t.Fatalf("UpdateInvoiceState(draft) after void: %v", err)
	}
}
