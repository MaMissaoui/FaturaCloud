package db

import "testing"

func invoiceState(t *testing.T, d *Database, id string) string {
	t.Helper()
	inv, err := d.GetInvoice(id)
	if err != nil {
		t.Fatalf("GetInvoice: %v", err)
	}
	return inv.State
}

// TestPaymentsKeepInvoicePaidStateInStep: a part payment leaves a sent invoice
// sent, the payment that clears it marks it paid, and voiding that payment
// moves it back to sent.
func TestPaymentsKeepInvoicePaidStateInStep(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-paid-state")
	inv := fx.createInvoice(t, d, "inv-1", 1, 1000) // total 1200
	if _, err := d.UpdateInvoiceState(inv.ID, "sent"); err != nil {
		t.Fatalf("UpdateInvoiceState(sent): %v", err)
	}
	bank := accountByCode(t, d, fx.orgID, "1020")
	pay := func(amount int64) *Payment {
		t.Helper()
		p, err := d.CreatePayment(CreatePaymentRequest{
			OrganizationID: fx.orgID, Direction: "inbound", ClientID: &fx.clientID,
			BankAccountID: bank.ID, Amount: amount, Currency: "EUR", Date: fx.date, Method: "bank_transfer",
			Applications: []CreatePaymentApplicationRequest{{DocumentType: "invoice", DocumentID: inv.ID, Amount: amount}},
		})
		if err != nil {
			t.Fatalf("CreatePayment(%d): %v", amount, err)
		}
		return p
	}

	first := pay(500)
	if got := invoiceState(t, d, inv.ID); got != "sent" {
		t.Fatalf("after a part payment state = %q, want sent", got)
	}
	last := pay(700)
	if got := invoiceState(t, d, inv.ID); got != "paid" {
		t.Fatalf("after the clearing payment state = %q, want paid", got)
	}

	if _, err := d.VoidPayment(last.ID, fx.date); err != nil {
		t.Fatalf("VoidPayment: %v", err)
	}
	if got := invoiceState(t, d, inv.ID); got != "sent" {
		t.Fatalf("after voiding the clearing payment state = %q, want sent", got)
	}
	// Voiding the remaining part payment changes nothing more.
	if _, err := d.VoidPayment(first.ID, fx.date); err != nil {
		t.Fatalf("VoidPayment first: %v", err)
	}
	if got := invoiceState(t, d, inv.ID); got != "sent" {
		t.Fatalf("after voiding the part payment state = %q, want sent", got)
	}
}

// TestVoidKeepsHandMarkedPaid: an invoice someone marked paid on a part
// payment stays paid when that payment is voided — its payments never covered
// it, so "paid" was their call, not the rule's.
func TestVoidKeepsHandMarkedPaid(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-paid-hand")
	inv := fx.createInvoice(t, d, "inv-1", 1, 1000) // total 1200
	if _, err := d.UpdateInvoiceState(inv.ID, "sent"); err != nil {
		t.Fatalf("UpdateInvoiceState(sent): %v", err)
	}
	bank := accountByCode(t, d, fx.orgID, "1020")
	p, err := d.CreatePayment(CreatePaymentRequest{
		OrganizationID: fx.orgID, Direction: "inbound", ClientID: &fx.clientID,
		BankAccountID: bank.ID, Amount: 500, Currency: "EUR", Date: fx.date, Method: "bank_transfer",
		Applications: []CreatePaymentApplicationRequest{{DocumentType: "invoice", DocumentID: inv.ID, Amount: 500}},
	})
	if err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}
	if _, err := d.UpdateInvoiceState(inv.ID, "paid"); err != nil {
		t.Fatalf("UpdateInvoiceState(paid): %v", err)
	}
	if _, err := d.VoidPayment(p.ID, fx.date); err != nil {
		t.Fatalf("VoidPayment: %v", err)
	}
	if got := invoiceState(t, d, inv.ID); got != "paid" {
		t.Fatalf("state = %q, want paid (marked by hand)", got)
	}
}

// TestPaymentMarksBillPaid: the purchases mirror — an approved bill becomes
// paid once paid in full, and approved again when that payment is voided.
func TestPaymentMarksBillPaid(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-paid-bill")
	bill, err := d.CreateIncomingInvoice(CreateIncomingInvoiceRequest{
		OrganizationID: fx.orgID, VendorID: fx.vendorID, VendorInvoiceNumber: "bill-1",
		Date: fx.date, Currency: "EUR", SubTotal: 10000, TaxTotal: 0, Total: 10000,
		LineItems: []CreateInvoiceLineItemRequest{{Quantity: 1, UnitPrice: 10000, ProductID: &fx.productID}},
	})
	if err != nil {
		t.Fatalf("CreateIncomingInvoice: %v", err)
	}
	if _, err := d.UpdateIncomingInvoiceState(bill.ID, "approved"); err != nil {
		t.Fatalf("UpdateIncomingInvoiceState(approved): %v", err)
	}
	bank := accountByCode(t, d, fx.orgID, "1020")
	p, err := d.CreatePayment(CreatePaymentRequest{
		OrganizationID: fx.orgID, Direction: "outbound", VendorID: &fx.vendorID,
		BankAccountID: bank.ID, Amount: 10000, Currency: "EUR", Date: fx.date, Method: "bank_transfer",
		Applications: []CreatePaymentApplicationRequest{{DocumentType: "incoming_invoice", DocumentID: bill.ID, Amount: 10000}},
	})
	if err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}
	billState := func() string {
		b, err := d.GetIncomingInvoice(bill.ID)
		if err != nil {
			t.Fatalf("GetIncomingInvoice: %v", err)
		}
		return b.State
	}
	if got := billState(); got != "paid" {
		t.Fatalf("after full payment state = %q, want paid", got)
	}
	if _, err := d.VoidPayment(p.ID, fx.date); err != nil {
		t.Fatalf("VoidPayment: %v", err)
	}
	if got := billState(); got != "approved" {
		t.Fatalf("after void state = %q, want approved", got)
	}
}

// TestMigration0101BackfillsSettledDocuments: the one-time catch-up moves a
// sent invoice and an approved bill that payments fully cover to paid, and
// leaves a part-paid invoice and one whose clearing payment was voided open.
// The documents are put back in their pre-rule state by hand, as if paid
// before payments kept the state in step.
func TestMigration0101BackfillsSettledDocuments(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-paid-backfill")
	bank := accountByCode(t, d, fx.orgID, "1020")
	payInvoice := func(id string, amount int64) *Payment {
		t.Helper()
		p, err := d.CreatePayment(CreatePaymentRequest{
			OrganizationID: fx.orgID, Direction: "inbound", ClientID: &fx.clientID,
			BankAccountID: bank.ID, Amount: amount, Currency: "EUR", Date: fx.date, Method: "bank_transfer",
			Applications: []CreatePaymentApplicationRequest{{DocumentType: "invoice", DocumentID: id, Amount: amount}},
		})
		if err != nil {
			t.Fatalf("CreatePayment(%s, %d): %v", id, amount, err)
		}
		return p
	}
	sentInvoice := func(number string) *Invoice {
		t.Helper()
		inv := fx.createInvoice(t, d, number, 1, 1000) // total 1200
		if _, err := d.UpdateInvoiceState(inv.ID, "sent"); err != nil {
			t.Fatalf("UpdateInvoiceState(sent): %v", err)
		}
		return inv
	}

	settled := sentInvoice("inv-settled")
	payInvoice(settled.ID, 1200)
	partPaid := sentInvoice("inv-part")
	payInvoice(partPaid.ID, 500)
	voided := sentInvoice("inv-voided")
	if _, err := d.VoidPayment(payInvoice(voided.ID, 1200).ID, fx.date); err != nil {
		t.Fatalf("VoidPayment: %v", err)
	}

	bill, err := d.CreateIncomingInvoice(CreateIncomingInvoiceRequest{
		OrganizationID: fx.orgID, VendorID: fx.vendorID, VendorInvoiceNumber: "bill-settled",
		Date: fx.date, Currency: "EUR", SubTotal: 10000, TaxTotal: 0, Total: 10000,
		LineItems: []CreateInvoiceLineItemRequest{{Quantity: 1, UnitPrice: 10000, ProductID: &fx.productID}},
	})
	if err != nil {
		t.Fatalf("CreateIncomingInvoice: %v", err)
	}
	if _, err := d.UpdateIncomingInvoiceState(bill.ID, "approved"); err != nil {
		t.Fatalf("UpdateIncomingInvoiceState(approved): %v", err)
	}
	if _, err := d.CreatePayment(CreatePaymentRequest{
		OrganizationID: fx.orgID, Direction: "outbound", VendorID: &fx.vendorID,
		BankAccountID: bank.ID, Amount: 10000, Currency: "EUR", Date: fx.date, Method: "bank_transfer",
		Applications: []CreatePaymentApplicationRequest{{DocumentType: "incoming_invoice", DocumentID: bill.ID, Amount: 10000}},
	}); err != nil {
		t.Fatalf("CreatePayment(bill): %v", err)
	}

	// Back to the state a document paid before the rule kept.
	d.DB.MustExec(`UPDATE invoices SET state = 'sent' WHERE id = ?`, settled.ID)
	d.DB.MustExec(`UPDATE incoming_invoices SET state = 'approved' WHERE id = ?`, bill.ID)

	sql, err := migrationsFS.ReadFile("migrations/0101_backfill_paid_state.up.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	d.DB.MustExec(string(sql))

	for _, c := range []struct {
		name, id, want string
	}{
		{"settled", settled.ID, "paid"},
		{"part paid", partPaid.ID, "sent"},
		{"clearing payment voided", voided.ID, "sent"},
	} {
		if got := invoiceState(t, d, c.id); got != c.want {
			t.Errorf("%s invoice: state = %q, want %q", c.name, got, c.want)
		}
	}
	b, err := d.GetIncomingInvoice(bill.ID)
	if err != nil {
		t.Fatalf("GetIncomingInvoice: %v", err)
	}
	if b.State != "paid" {
		t.Errorf("settled bill: state = %q, want paid", b.State)
	}
}
