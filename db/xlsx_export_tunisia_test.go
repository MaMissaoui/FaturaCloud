package db

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

// TestTunisiaLayoutFillsEndToEnd exercises the new default layout as a whole:
// line-item expansion, the per-tax-rate VAT recap, the amount-in-words line,
// the discount (Remise) column and a logo image anchor all survive one fill.
func TestTunisiaLayoutFillsEndToEnd(t *testing.T) {
	org := testOrg()
	org.AmountInWordsEnabled = ptr(int64(1))
	if org.Currency == nil {
		org.Currency = ptr("TND")
	}
	*org.Currency = "TND"
	client := testClient()
	client.IdentityNumber = ptr("02254406")

	taxA := TaxRate{ID: "t1", Name: "TVA 19", Percentage: 19}
	taxB := TaxRate{ID: "t2", Name: "TVA 7", Percentage: 7}
	taxRates := map[string]TaxRate{"t1": taxA, "t2": taxB}

	invoice := testInvoice()
	invoice.Currency = "TND"
	invoice.DiscountAmount = 1000 // 10.00 DT remise

	rateA, rateB := "t1", "t2"
	lineItems := []InvoiceLineItem{
		{Description: ptr("Super Diamant 125"), SKU: ptr("006"), Quantity: 1, UnitPrice: 100000, TaxRate: &rateA},
		{Description: ptr("Petit diamant"), SKU: ptr("007"), Quantity: 2, UnitPrice: 5000, TaxRate: &rateB},
	}

	// A real 1x1 PNG so the logo anchor path is actually exercised.
	logo, err := base64.StdEncoding.DecodeString(
		"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==",
	)
	if err != nil {
		t.Fatalf("decode png: %v", err)
	}

	filled, unresolved, err := FillInvoiceTemplate(invoiceDefaultTemplate, invoice, lineItems, org, client, taxRates, logo, "")
	if err != nil {
		t.Fatalf("fill: %v", err)
	}
	if len(unresolved) > 0 {
		t.Errorf("unresolved placeholders: %v", unresolved)
	}

	f, err := excelize.OpenReader(bytes.NewReader(filled))
	if err != nil {
		t.Fatalf("reopen filled: %v", err)
	}
	defer f.Close()
	sheet := f.GetSheetName(0)
	rows, _ := f.GetRows(sheet)

	var flat []string
	for _, r := range rows {
		flat = append(flat, strings.Join(r, "\x00"))
	}
	all := strings.Join(flat, "\n")

	for _, want := range []string{"Facture N°:", "Total Brut HTVA", "Remise", "Total Net HTVA", "Total TTC", "CIN: 02254406", "Arrêtée la présente facture"} {
		if !strings.Contains(all, want) {
			t.Errorf("filled output missing %q", want)
		}
	}
	if strings.Contains(all, "{{") {
		t.Errorf("filled output still contains a placeholder: %s", all)
	}
	// The VAT recap should have expanded to two rate rows (19% and 7%).
	if !strings.Contains(all, "19%") || !strings.Contains(all, "7%") {
		t.Errorf("VAT recap rows not expanded: %s", all)
	}
	// The logo image must actually be embedded (libreoffice/excel picture part).
	if pics, _ := f.GetPictureCells(sheet); len(pics) == 0 {
		t.Error("expected an embedded logo picture")
	}
}
