package db

import "testing"

// An organization created without a format gets the same default the New
// Organization form sends, instead of an explicit NULL overriding the
// column default.
func TestCreateOrganizationDefaultsInvoiceNumberFormat(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	for _, format := range []*string{nil, ptr(""), ptr("  ")} {
		org, err := d.CreateOrganization(CreateOrganizationRequest{InvoiceNumberFormat: format})
		if err != nil {
			t.Fatalf("CreateOrganization: %v", err)
		}
		if org.InvoiceNumberFormat == nil || *org.InvoiceNumberFormat != DefaultInvoiceNumberFormat {
			t.Fatalf("invoice number format = %v, want %q", org.InvoiceNumberFormat, DefaultInvoiceNumberFormat)
		}
	}
}

// A Cash Book sale has no number field, so an organization stored without
// a format (NULL from before the default, or "" cleared through the API)
// still numbers its sales with the default.
func TestCashSaleNumbersWithDefaultWhenFormatMissing(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-cash-sale-noformat")

	sell := func() string {
		t.Helper()
		res, err := d.CreateCashSale(CreateCashSaleRequest{
			OrganizationID: fx.orgID, ClientID: fx.clientID, Date: fx.date, Currency: "EUR",
			LineItems: []CreateInvoiceLineItemRequest{{Quantity: 1, UnitPrice: 1000, ProductID: &fx.productID}},
			SubTotal: 1000, Total: 1000,
		})
		if err != nil {
			t.Fatalf("CreateCashSale: %v", err)
		}
		return res.Invoice.Number
	}

	if _, err := d.DB.Exec(`UPDATE organizations SET invoice_number_format = NULL, invoice_number_counter = 0 WHERE id = ?`, fx.orgID); err != nil {
		t.Fatalf("clear format: %v", err)
	}
	if got := sell(); got != "#1" {
		t.Fatalf("number with NULL format = %q, want #1", got)
	}

	if _, err := d.UpdateOrganization(fx.orgID, UpdateOrganizationRequest{InvoiceNumberFormat: ptr("")}); err != nil {
		t.Fatalf("UpdateOrganization(\"\"): %v", err)
	}
	if got := sell(); got != "#2" {
		t.Fatalf("number with empty format = %q, want #2", got)
	}

	if _, err := d.UpdateOrganization(fx.orgID, UpdateOrganizationRequest{InvoiceNumberFormat: ptr("CB-{number}")}); err != nil {
		t.Fatalf("UpdateOrganization(format): %v", err)
	}
	if got := sell(); got != "CB-3" {
		t.Fatalf("number with own format = %q, want CB-3", got)
	}
}
