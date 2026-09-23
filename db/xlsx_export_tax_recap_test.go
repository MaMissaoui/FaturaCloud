package db

import (
	"bytes"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

// filledCellText opens a filled workbook and returns every non-empty cell of
// its content sheet as one newline-joined string, for substring assertions.
func filledCellText(t *testing.T, filled []byte) string {
	t.Helper()
	f, err := excelize.OpenReader(bytes.NewReader(filled))
	if err != nil {
		t.Fatalf("reopen filled: %v", err)
	}
	defer f.Close()
	rows, err := f.GetRows(f.GetSheetName(0))
	if err != nil {
		t.Fatalf("read rows: %v", err)
	}
	var b strings.Builder
	for _, row := range rows {
		for _, cell := range row {
			if cell != "" {
				b.WriteString(cell)
				b.WriteByte('\n')
			}
		}
	}
	return b.String()
}

// The Tunisia purchase order and incoming invoice templates carry a
// {{#taxLines}} VAT recap. Their fills must expand it (one row per rate in
// use) and clear the marker — before this, only invoices passed the block,
// so every Tunisia PO/vendor bill printed an empty recap and a literal
// "{{#taxLines}}" in the margin.

func TestTunisiaPurchaseOrderFillsTaxRecap(t *testing.T) {
	t.Parallel()
	taxRates := map[string]TaxRate{
		"t19": {ID: "t19", Name: "TVA 19", Percentage: 19},
		"t7":  {ID: "t7", Name: "TVA 7", Percentage: 7},
	}
	lineItems := []PurchaseOrderLineItem{
		{Description: "Widget", Quantity: 2, UnitPrice: 50000, TaxRate: ptr("t19")},
		{Description: "Gadget", Quantity: 1, UnitPrice: 10000, TaxRate: ptr("t7")},
	}
	filled, unresolved, err := FillPurchaseOrderTemplate(purchaseOrderTunisiaTemplate, testPurchaseOrder(), lineItems, testOrg(), testVendor(), taxRates, nil, "")
	if err != nil {
		t.Fatalf("fill: %v", err)
	}
	if len(unresolved) > 0 {
		t.Errorf("unresolved placeholders: %v", unresolved)
	}
	text := filledCellText(t, filled)
	if strings.Contains(text, "{{") {
		t.Errorf("filled PO still contains a placeholder/marker:\n%s", text)
	}
	for _, want := range []string{"19%", "7%", "1,000.00 EUR", "190.00 EUR", "100.00 EUR", "7.00 EUR"} {
		if !strings.Contains(text, want) {
			t.Errorf("expected recap value %q in filled PO:\n%s", want, text)
		}
	}
}

func TestTunisiaIncomingInvoiceFillsTaxRecap(t *testing.T) {
	t.Parallel()
	taxRates := map[string]TaxRate{"t19": {ID: "t19", Name: "TVA 19", Percentage: 19}}
	lineItems := []IncomingInvoiceLineItem{
		{Description: "Widget", Quantity: 2, UnitPrice: 50000, TaxRate: ptr("t19")},
	}
	filled, unresolved, err := FillIncomingInvoiceTemplate(incomingInvoiceTunisiaTemplate, testIncomingInvoice(), lineItems, testOrg(), testVendor(), taxRates, nil, "")
	if err != nil {
		t.Fatalf("fill: %v", err)
	}
	if len(unresolved) > 0 {
		t.Errorf("unresolved placeholders: %v", unresolved)
	}
	text := filledCellText(t, filled)
	if strings.Contains(text, "{{") {
		t.Errorf("filled incoming invoice still contains a placeholder/marker:\n%s", text)
	}
	for _, want := range []string{"19%", "1,000.00 EUR", "190.00 EUR"} {
		if !strings.Contains(text, want) {
			t.Errorf("expected recap value %q in filled incoming invoice:\n%s", want, text)
		}
	}
}

// An untaxed purchase order has no recap rows: the marker must still be
// cleared rather than printed.
func TestTunisiaPurchaseOrderWithoutTaxClearsRecapMarker(t *testing.T) {
	t.Parallel()
	lineItems := []PurchaseOrderLineItem{{Description: "Widget", Quantity: 31, UnitPrice: 16090}}
	filled, _, err := FillPurchaseOrderTemplate(purchaseOrderTunisiaTemplate, testPurchaseOrder(), lineItems, testOrg(), testVendor(), map[string]TaxRate{}, nil, "")
	if err != nil {
		t.Fatalf("fill: %v", err)
	}
	if text := filledCellText(t, filled); strings.Contains(text, "{{") {
		t.Errorf("untaxed PO still contains a placeholder/marker:\n%s", text)
	}
}
