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

	if err := d.validateInvoiceTotals(items, 1000000, 0, 1000100, 100); err != nil {
		t.Fatalf("validateInvoiceTotals with a 100-cent stamp: %v", err)
	}
	if err := d.validateInvoiceTotals(items, 1000000, 0, 1010000, 100); err == nil {
		t.Fatalf("validateInvoiceTotals should reject a total computed as if the stamp were 100x larger")
	}
	if err := d.validateInvoiceTotals(items, 1000000, 0, 1000000, 0); err != nil {
		t.Fatalf("validateInvoiceTotals with no stamp (0) must behave exactly as before: %v", err)
	}
}
