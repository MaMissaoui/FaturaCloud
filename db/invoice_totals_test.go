package db

import "testing"

// TestRoundCents is F54's unit-level regression test: roundCents must round
// half away from zero rather than truncate toward zero, matching the
// ROUND_HALF_UP semantics roundHalfUp uses for tax.
func TestRoundCents(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   float64
		want int64
	}{
		{1000, 1000},
		{1000.4, 1000},
		{1000.5, 1001},
		{1000.9999999998, 1001}, // a float round-trip landing just under a whole cent
		{0.5, 1},
		{-0.5, -1},
	}
	for _, c := range cases {
		if got := roundCents(c.in); got != c.want {
			t.Errorf("roundCents(%v) = %d, want %d", c.in, got, c.want)
		}
	}
}

// TestIncomingInvoiceLineItemRoundsFractionalUnitPrice is F54's integration
// regression test: a fractional-cent unitPrice (e.g. arriving through JSON
// after a division) must round to the nearest cent when stored, not
// truncate toward zero — int64(1000.9) used to store 1000, silently a cent
// short of what a client requesting 1000.9 actually meant.
func TestIncomingInvoiceLineItemRoundsFractionalUnitPrice(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-f54-round"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	vendor, err := d.CreateVendor(CreateVendorRequest{OrganizationID: org.ID, Name: ptr("Vendor")})
	if err != nil {
		t.Fatalf("CreateVendor: %v", err)
	}

	inv, err := d.CreateIncomingInvoice(CreateIncomingInvoiceRequest{
		OrganizationID: org.ID, VendorID: vendor.ID, VendorInvoiceNumber: "BILL-1",
		Date: 1700000000000, Currency: "EUR",
		SubTotal: 1001, Total: 1001,
		LineItems: []CreateInvoiceLineItemRequest{
			{Quantity: 1, UnitPrice: 1000.9999999998}, // truncates to 1000, rounds to 1001
		},
	})
	if err != nil {
		t.Fatalf("CreateIncomingInvoice: %v", err)
	}

	items, err := d.GetIncomingInvoiceLineItems(inv.ID)
	if err != nil || len(items) != 1 {
		t.Fatalf("GetIncomingInvoiceLineItems: err=%v, len=%d", err, len(items))
	}
	if items[0].UnitPrice != 1001 {
		t.Fatalf("unitPrice = %d, want 1001 (rounded, not truncated)", items[0].UnitPrice)
	}
}

// TestValidateInvoiceTotalsFiscalStampAmount is a regression test for a bug
// caught in manual verification: fiscalStampAmount arrives in cents (like
// every other Invoice total column) but was added directly into a
// currency-units accumulator, overstating its contribution to the expected
// total 100x. 100 cents of stamp must add exactly 100 cents to the expected
// total, not 10000.
func TestValidateInvoiceTotalsFiscalStampAmount(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	items := []CreateInvoiceLineItemRequest{{Quantity: 1, UnitPrice: 1000000}}

	if err := d.validateInvoiceTotals(items, 1000000, 0, 1000100, 100, 0); err != nil {
		t.Fatalf("validateInvoiceTotals with a 100-cent stamp: %v", err)
	}
	if err := d.validateInvoiceTotals(items, 1000000, 0, 1010000, 100, 0); err == nil {
		t.Fatalf("validateInvoiceTotals should reject a total computed as if the stamp were 100x larger")
	}
	if err := d.validateInvoiceTotals(items, 1000000, 0, 1000000, 0, 0); err != nil {
		t.Fatalf("validateInvoiceTotals with no stamp (0) must behave exactly as before: %v", err)
	}
}

// TestValidateInvoiceTotalsDiscount covers the remise path: the discount
// reduces the taxable base (tax + total), subTotal stays the gross sum, and a
// discount larger than the subtotal is rejected.
func TestValidateInvoiceTotalsDiscount(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-discount"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	rateID := "rate-19"
	if _, err := d.CreateTaxRate(CreateTaxRateRequest{ID: rateID, OrganizationID: org.ID, Name: "TVA 19", Percentage: 19}); err != nil {
		t.Fatalf("CreateTaxRate: %v", err)
	}

	items := []CreateInvoiceLineItemRequest{
		{Quantity: 1, UnitPrice: 100000, TaxRate: &rateID}, // 1000.00 @ 19%
	}

	// subTotal = 1000.00 (gross); discount 100.00 -> net base 900.00,
	// tax 171.00, total 1071.00.
	if err := d.validateInvoiceTotals(items, 100000, 17100, 107100, 0, 10000); err != nil {
		t.Fatalf("discount validation: %v", err)
	}
	// The pre-discount totals must now be rejected (they no longer match).
	if err := d.validateInvoiceTotals(items, 100000, 19000, 119000, 0, 10000); err == nil {
		t.Error("discount should make the un-discounted totals invalid")
	}
	// A discount exceeding the subtotal is refused.
	if err := d.validateInvoiceTotals(items, 100000, 0, 0, 0, 200000); err == nil {
		t.Error("discount larger than the subtotal must be rejected")
	}
	// A negative discount is refused.
	if err := d.validateInvoiceTotals(items, 100000, 19000, 119000, 0, -1); err == nil {
		t.Error("negative discount must be rejected")
	}
}

// TestValidateInvoiceTotalsDiscountMultiRate checks the proportional
// allocation across two tax groups: a 100.00 discount on a 1000.00+1000.00
// subtotal splits 50/50, halving each group's base.
func TestValidateInvoiceTotalsDiscountMultiRate(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-discount-multi"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	aID, bID := "rate-19", "rate-7"
	if _, err := d.CreateTaxRate(CreateTaxRateRequest{ID: aID, OrganizationID: org.ID, Name: "TVA 19", Percentage: 19}); err != nil {
		t.Fatalf("CreateTaxRate(a): %v", err)
	}
	if _, err := d.CreateTaxRate(CreateTaxRateRequest{ID: bID, OrganizationID: org.ID, Name: "TVA 7", Percentage: 7}); err != nil {
		t.Fatalf("CreateTaxRate(b): %v", err)
	}

	items := []CreateInvoiceLineItemRequest{
		{Quantity: 1, UnitPrice: 100000, TaxRate: &aID}, // 1000.00 @ 19%
		{Quantity: 1, UnitPrice: 100000, TaxRate: &bID}, // 1000.00 @ 7%
	}
	// Discount 100.00 -> 50.00 off each group: bases 950.00 and 950.00,
	// tax = 950*0.19=180.50 + 950*0.07=66.50 = 247.00, net 1900.00,
	// total 2147.00.
	if err := d.validateInvoiceTotals(items, 200000, 24700, 214700, 0, 10000); err != nil {
		t.Fatalf("multi-rate discount validation: %v", err)
	}
}

// TestUpdateInvoiceDiscountOnlyEditRevalidates guards the "effective state"
// fallback in UpdateInvoice: sending only a new discount (no line items, no
// other totals) must revalidate against the stored line items and totals, not
// silently accept a discount the stored total doesn't reflect.
func TestUpdateInvoiceDiscountOnlyEditRevalidates(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-upd-discount"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	if _, err := d.CreateFiscalYear(CreateFiscalYearRequest{
		OrganizationID: org.ID, Name: "2025", StartDate: 1735689600000, EndDate: 1767225599000,
	}); err != nil {
		t.Fatalf("CreateFiscalYear: %v", err)
	}
	client, err := d.CreateClient(CreateClientRequest{OrganizationID: org.ID, Name: ptr("C")})
	if err != nil {
		t.Fatalf("CreateClient: %v", err)
	}
	rateID := "rate-19"
	if _, err := d.CreateTaxRate(CreateTaxRateRequest{ID: rateID, OrganizationID: org.ID, Name: "TVA 19", Percentage: 19}); err != nil {
		t.Fatalf("CreateTaxRate: %v", err)
	}
	date := int64(1738368000000)
	inv, err := d.CreateInvoice(CreateInvoiceRequest{
		OrganizationID: org.ID, Number: "inv-disc", ClientID: client.ID, Date: date, Currency: "EUR",
		SubTotal: 100000, TaxTotal: 19000, Total: 119000, DiscountAmount: 0,
		LineItems: []CreateInvoiceLineItemRequest{{Quantity: 1, UnitPrice: 100000, TaxRate: &rateID}},
	})
	if err != nil {
		t.Fatalf("CreateInvoice: %v", err)
	}

	// A header-only edit that adds a discount but leaves the stored totals
	// must be rejected — the resulting state wouldn't balance.
	bad := int64(10000)
	if _, err := d.UpdateInvoice(inv.ID, UpdateInvoiceRequest{DiscountAmount: &bad}); err == nil {
		t.Fatal("expected a discount-only edit with stale totals to be rejected")
	}

	// The same edit with matching new totals is accepted.
	netSub, tax, total := int64(100000), int64(17100), int64(107100)
	updated, err := d.UpdateInvoice(inv.ID, UpdateInvoiceRequest{
		DiscountAmount: &bad, SubTotal: &netSub, TaxTotal: &tax, Total: &total,
	})
	if err != nil {
		t.Fatalf("UpdateInvoice with matching totals: %v", err)
	}
	if updated.DiscountAmount != 10000 {
		t.Fatalf("discountAmount = %d, want 10000", updated.DiscountAmount)
	}
	if updated.Total != 107100 {
		t.Fatalf("total = %d, want 107100", updated.Total)
	}
}
