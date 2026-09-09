package db

import (
	"bytes"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func testIncomingInvoice() IncomingInvoice {
	return IncomingInvoice{
		VendorInvoiceNumber: "VINV-0001",
		Date:                1772721000000, // 2026-03-05
		Currency:            "EUR",
		Total:               11900,
		TaxTotal:            1900,
		SubTotal:            10000,
	}
}

// buildIncomingInvoiceFixtureTemplate mirrors buildPurchaseOrderFixtureTemplate
// but with incomingInvoice.* placeholders instead of purchaseOrder.* ones.
func buildIncomingInvoiceFixtureTemplate(t *testing.T) []byte {
	t.Helper()
	f := excelize.NewFile()
	sheet := f.GetSheetName(0)
	_ = f.SetCellStr(sheet, "A1", "Bill {{incomingInvoice.number}} from {{vendor.name}}")
	_ = f.SetCellStr(sheet, "A3", "{{#lineItems}}")
	_ = f.SetCellStr(sheet, "B3", "{{lineItems.description}}")
	_ = f.SetCellStr(sheet, "C3", "{{lineItems.lineTotal}}")
	_ = f.SetCellStr(sheet, "B5", "Subtotal: {{incomingInvoice.subTotal}} Tax: {{incomingInvoice.taxTotal}} Total: {{incomingInvoice.total}}")

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatalf("build fixture template: %v", err)
	}
	return buf.Bytes()
}

func TestFillIncomingInvoiceTemplateResolvesScalarPlaceholders(t *testing.T) {
	t.Parallel()
	tmpl := buildIncomingInvoiceFixtureTemplate(t)
	invoice := testIncomingInvoice()
	lineItems := []IncomingInvoiceLineItem{{Description: "Widget", Quantity: 2, UnitPrice: 500}}

	out, unresolved, err := FillIncomingInvoiceTemplate(tmpl, invoice, lineItems, testOrg(), testVendor(), nil)
	if err != nil {
		t.Fatalf("FillIncomingInvoiceTemplate: %v", err)
	}
	if len(unresolved) != 0 {
		t.Errorf("unexpected unresolved placeholders: %v", unresolved)
	}

	f, err := excelize.OpenReader(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("open output: %v", err)
	}
	defer f.Close()
	sheet := f.GetSheetName(0)

	a1, _ := f.GetCellValue(sheet, "A1")
	if !strings.Contains(a1, "VINV-0001") || !strings.Contains(a1, "Acme Supplies") {
		t.Errorf("A1 = %q, want it to contain vendor invoice number and vendor name", a1)
	}
}

// TestFillIncomingInvoiceTemplateUsesStoredTotals is the deliberate divergence
// from purchase orders/orders: unlike those two, an incoming invoice has
// server-validated stored totals (db.validateInvoiceTotals via the shared
// CreateInvoiceLineItemRequest path) and must read them directly rather than
// recomputing from line items — a line item total here would give the wrong
// answer if it were used, confirming the fill path never calls
// computeExportTotals for this document type.
func TestFillIncomingInvoiceTemplateUsesStoredTotals(t *testing.T) {
	t.Parallel()
	tmpl := buildIncomingInvoiceFixtureTemplate(t)
	invoice := testIncomingInvoice() // subTotal=10000, taxTotal=1900, total=11900

	// Line items sum to something entirely different (2 x 100 = 200 cents) —
	// if the fill path recomputed totals from these, the output would not
	// match the invoice's own stored totals.
	lineItems := []IncomingInvoiceLineItem{{Description: "Widget", Quantity: 2, UnitPrice: 100}}

	out, _, err := FillIncomingInvoiceTemplate(tmpl, invoice, lineItems, testOrg(), testVendor(), nil)
	if err != nil {
		t.Fatalf("FillIncomingInvoiceTemplate: %v", err)
	}

	f, err := excelize.OpenReader(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("open output: %v", err)
	}
	defer f.Close()
	got, _ := f.GetCellValue(f.GetSheetName(0), "B5")

	want := "Subtotal: 100.00 EUR Tax: 19.00 EUR Total: 119.00 EUR"
	if got != want {
		t.Errorf("B5 = %q, want %q (stored totals, not recomputed from line items)", got, want)
	}
}

// TestEmbeddedDefaultIncomingInvoiceTemplatePlaceholdersAllResolve mirrors
// TestEmbeddedDefaultPurchaseOrderTemplatePlaceholdersAllResolve — the only
// automated guard against a typo in the committed, non-diff-reviewable
// binary every organization without a custom template gets.
func TestEmbeddedDefaultIncomingInvoiceTemplatePlaceholdersAllResolve(t *testing.T) {
	t.Parallel()
	f, err := excelize.OpenReader(bytes.NewReader(incomingInvoiceDefaultTemplate))
	if err != nil {
		t.Fatalf("open embedded default template: %v", err)
	}
	defer f.Close()
	sheet := f.GetSheetName(0)

	scalars := buildIncomingInvoiceScalarPlaceholders(testIncomingInvoice(), testOrg(), testVendor(), "EUR")
	lineItemKeys := map[string]bool{
		"lineItems.description": true, "lineItems.quantity": true,
		"lineItems.unitPrice": true, "lineItems.taxRate": true, "lineItems.lineTotal": true,
	}

	rows, err := f.GetRows(sheet)
	if err != nil {
		t.Fatalf("read rows: %v", err)
	}

	sawMarker := false
	for _, row := range rows {
		for _, cell := range row {
			if strings.TrimSpace(cell) == lineItemMarker {
				sawMarker = true
				continue
			}
			for _, match := range placeholderPattern.FindAllStringSubmatch(cell, -1) {
				key := match[1]
				if _, ok := scalars[key]; ok {
					continue
				}
				if lineItemKeys[key] {
					continue
				}
				t.Errorf("embedded default incoming invoice template has unresolvable placeholder {{%s}}", key)
			}
		}
	}
	if !sawMarker {
		t.Error("embedded default incoming invoice template has no {{#lineItems}} marker row")
	}
}
