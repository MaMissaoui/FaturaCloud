package db

import "testing"

// TestGetPaymentsFilters pins the payments list's optional filters (audit
// F172): the zero options list everything, direction and clientId narrow
// it, and a narrowed list still carries each payment's invoice numbers and
// products.
func TestGetPaymentsFilters(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-pl")
	invoiceID, _, _ := newTwoLineLoanSale(t, d, fx, 1000)
	invoice, err := d.GetInvoice(invoiceID)
	if err != nil {
		t.Fatalf("GetInvoice: %v", err)
	}

	other, err := d.CreateClient(CreateClientRequest{OrganizationID: fx.orgID, Name: ptr("Other customer")})
	if err != nil {
		t.Fatalf("CreateClient: %v", err)
	}
	register := accountByCode(t, d, fx.orgID, "1010")
	for _, row := range []struct{ id, direction, clientID, vendorID any }{
		{"pay-other", "inbound", other.ID, nil},
		{"pay-vendor", "outbound", nil, fx.vendorID},
	} {
		if _, err := d.DB.Exec(`
			INSERT INTO payments (id, organizationId, direction, clientId, vendorId, bankAccountId, amount, currency, date, method)
			VALUES (?, ?, ?, ?, ?, ?, 100, 'EUR', ?, 'cash')`,
			row.id, fx.orgID, row.direction, row.clientID, row.vendorID, register.ID, fx.date,
		); err != nil {
			t.Fatalf("insert payment: %v", err)
		}
	}

	ids := func(opts PaymentListOptions) map[string]Payment {
		t.Helper()
		payments, err := d.GetPayments(fx.orgID, opts)
		if err != nil {
			t.Fatalf("GetPayments(%+v): %v", opts, err)
		}
		out := map[string]Payment{}
		for _, p := range payments {
			out[p.ID] = p
		}
		return out
	}

	all := ids(PaymentListOptions{})
	if len(all) != 3 {
		t.Fatalf("all payments = %d, want 3", len(all))
	}
	if got := ids(PaymentListOptions{Direction: "outbound"}); len(got) != 1 || got["pay-vendor"].ID == "" {
		t.Fatalf("outbound = %v, want only pay-vendor", got)
	}
	inbound := ids(PaymentListOptions{Direction: "inbound"})
	if len(inbound) != 2 || inbound["pay-vendor"].ID != "" {
		t.Fatalf("inbound = %v, want the two client payments", inbound)
	}
	mine := ids(PaymentListOptions{Direction: "inbound", ClientID: fx.clientID})
	if len(mine) != 1 {
		t.Fatalf("the fixture client's inbound payments = %d, want 1", len(mine))
	}
	for _, p := range mine {
		if len(p.InvoiceNumbers) != 1 || p.InvoiceNumbers[0] != invoice.Number {
			t.Fatalf("invoice numbers = %v, want [%s]", p.InvoiceNumbers, invoice.Number)
		}
		if len(p.Products) == 0 {
			t.Fatal("a narrowed list lost the payment's products")
		}
	}
	if got := ids(PaymentListOptions{ClientID: other.ID}); len(got) != 1 || got["pay-other"].ID == "" {
		t.Fatalf("other client's payments = %v, want only pay-other", got)
	}
}
