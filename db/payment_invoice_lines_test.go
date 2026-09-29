package db

import (
	"reflect"
	"testing"
)

// A loan sale's upfront payment covers its invoice as a whole and a later
// Cash Book line payment settles one line: GetPayments names the products of
// each, and GetPaymentInvoiceLines returns the invoice's lines (invoice
// order, line amounts summing to the invoice total) with what that payment
// applied.
func TestPaymentInvoiceLinesAndProducts(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-payment-invoice-lines")
	register := accountByCode(t, d, fx.orgID, "1010")
	if _, err := d.UpdateOrganization(fx.orgID, UpdateOrganizationRequest{DefaultCashRegisterAccountID: &register.ID}); err != nil {
		t.Fatalf("UpdateOrganization: %v", err)
	}
	fridge, err := d.CreateProduct(CreateProductRequest{OrganizationID: fx.orgID, Name: "Fridge", Type: "service", Price: 3000})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}

	// Widget first on the invoice, the (more valuable) Fridge second — the
	// loan allocator orders by value, the panel must keep invoice order.
	sale, err := d.CreateCashSale(CreateCashSaleRequest{
		OrganizationID: fx.orgID, ClientID: fx.clientID, Date: fx.date, Currency: "EUR",
		LineItems: []CreateInvoiceLineItemRequest{
			{Quantity: 2, UnitPrice: 1000, TaxRate: &fx.taxRateID, ProductID: &fx.productID},
			{Quantity: 1, UnitPrice: 3000, TaxRate: &fx.taxRateID, ProductID: &fridge.ID},
		},
		SubTotal: 5000, TaxTotal: 1000, Total: 6000,
		AmountReceived: 1000, PaymentMethod: "cash", BankAccountID: register.ID,
	})
	if err != nil {
		t.Fatalf("CreateCashSale: %v", err)
	}
	lines, err := d.GetInvoiceLineItems(sale.Invoice.ID)
	if err != nil || len(lines) != 2 {
		t.Fatalf("GetInvoiceLineItems = %v, %v", lines, err)
	}
	linePayment, err := d.CreateCashSalePayment(CreateCashSalePaymentRequest{
		InvoiceID: sale.Invoice.ID, InvoiceLineItemID: lines[1].ID, Amount: 500, Date: fx.date,
	})
	if err != nil {
		t.Fatalf("CreateCashSalePayment: %v", err)
	}

	payments, err := d.GetPayments(fx.orgID)
	if err != nil {
		t.Fatalf("GetPayments: %v", err)
	}
	byID := map[string]Payment{}
	for _, p := range payments {
		byID[p.ID] = p
	}
	upfront, line := byID[sale.Payment.ID], byID[linePayment.Payment.ID]
	if !upfront.WholeInvoice || !reflect.DeepEqual(upfront.Products, []string{"Widget", "Fridge"}) {
		t.Errorf("upfront payment = wholeInvoice %v, products %v; want true, [Widget Fridge]", upfront.WholeInvoice, upfront.Products)
	}
	if line.WholeInvoice || !reflect.DeepEqual(line.Products, []string{"Fridge"}) {
		t.Errorf("line payment = wholeInvoice %v, products %v; want false, [Fridge]", line.WholeInvoice, line.Products)
	}

	details, err := d.GetPaymentInvoiceLines(sale.Payment.ID)
	if err != nil || len(details) != 1 {
		t.Fatalf("GetPaymentInvoiceLines(upfront) = %+v, %v", details, err)
	}
	det := details[0]
	if det.InvoiceNumber != sale.Invoice.Number || det.InvoiceTotal != 6000 || det.Applied != 1000 || !det.WholeInvoice {
		t.Errorf("upfront detail = %+v; want number %q, total 6000, applied 1000, whole invoice", det, sale.Invoice.Number)
	}
	if len(det.Lines) != 2 || det.Lines[0].ProductName != "Widget" || det.Lines[1].ProductName != "Fridge" {
		t.Fatalf("upfront lines = %+v; want Widget then Fridge", det.Lines)
	}
	if sum := det.Lines[0].Amount + det.Lines[1].Amount; sum != 6000 {
		t.Errorf("line amounts sum to %d, want the invoice total 6000", sum)
	}
	if det.Lines[0].Amount != 2400 || det.Lines[0].Quantity != 2 {
		t.Errorf("Widget line = %+v; want quantity 2, amount 2400 (tax included)", det.Lines[0])
	}
	if det.Lines[0].PaidByThisPayment != 0 || det.Lines[1].PaidByThisPayment != 0 {
		t.Errorf("a whole-invoice payment paid no line directly: %+v", det.Lines)
	}

	details, err = d.GetPaymentInvoiceLines(linePayment.Payment.ID)
	if err != nil || len(details) != 1 {
		t.Fatalf("GetPaymentInvoiceLines(line) = %+v, %v", details, err)
	}
	det = details[0]
	if det.WholeInvoice || det.Applied != 500 || det.Lines[1].PaidByThisPayment != 500 || det.Lines[0].PaidByThisPayment != 0 {
		t.Errorf("line payment detail = %+v; want 500 applied to the Fridge line only", det)
	}

	// A vendor payment has no sales invoices to show.
	if none, err := d.GetPaymentInvoiceLines("no-such-payment"); err != nil || len(none) != 0 {
		t.Errorf("GetPaymentInvoiceLines(unknown) = %v, %v; want empty", none, err)
	}
}
