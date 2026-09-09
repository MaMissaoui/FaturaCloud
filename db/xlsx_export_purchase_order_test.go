package db

import (
	"bytes"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func testPurchaseOrder() PurchaseOrder {
	return PurchaseOrder{
		OrderNumber: "PO-0001",
		OrderDate:   1772721000000, // 2026-03-05
		Currency:    ptr("EUR"),
	}
}

func testVendor() Vendor {
	return Vendor{Name: ptr("Acme Supplies")}
}

// buildPurchaseOrderFixtureTemplate mirrors buildFixtureTemplate but with
// purchaseOrder.* placeholders instead of invoice.* ones.
func buildPurchaseOrderFixtureTemplate(t *testing.T) []byte {
	t.Helper()
	f := excelize.NewFile()
	sheet := f.GetSheetName(0)
	_ = f.SetCellStr(sheet, "A1", "Order {{purchaseOrder.number}} for {{organization.name}}")
	_ = f.SetCellStr(sheet, "A3", "{{#lineItems}}")
	_ = f.SetCellStr(sheet, "B3", "{{lineItems.description}}")
	_ = f.SetCellStr(sheet, "C3", "{{lineItems.lineTotal}}")
	_ = f.SetCellStr(sheet, "B5", "Total: {{purchaseOrder.total}}")

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatalf("build fixture template: %v", err)
	}
	return buf.Bytes()
}

func TestFillPurchaseOrderTemplateResolvesScalarPlaceholders(t *testing.T) {
	tmpl := buildPurchaseOrderFixtureTemplate(t)
	order := testPurchaseOrder()
	lineItems := []PurchaseOrderLineItem{{Description: "Widget", Quantity: 2, UnitPrice: 500}}

	out, unresolved, err := FillPurchaseOrderTemplate(tmpl, order, lineItems, testOrg(), testVendor(), nil)
	if err != nil {
		t.Fatalf("FillPurchaseOrderTemplate: %v", err)
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
	if !strings.Contains(a1, "PO-0001") || !strings.Contains(a1, "ACME Corp") {
		t.Errorf("A1 = %q, want it to contain order number and organization name", a1)
	}
}

func TestFillPurchaseOrderTemplateComputesTotalsFromLineItems(t *testing.T) {
	f := excelize.NewFile()
	sheet := f.GetSheetName(0)
	_ = f.SetCellStr(sheet, "A3", "{{#lineItems}}")
	_ = f.SetCellStr(sheet, "B3", "{{lineItems.description}}")
	_ = f.SetCellStr(sheet, "B5", "Subtotal: {{purchaseOrder.subTotal}} Tax: {{purchaseOrder.taxTotal}} Total: {{purchaseOrder.total}}")
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatalf("build fixture: %v", err)
	}

	order := testPurchaseOrder()
	lineItems := []PurchaseOrderLineItem{
		{Description: "Widget", Quantity: 2, UnitPrice: 1000, TaxRate: ptr("rate1")},
	}
	taxRates := map[string]TaxRate{"rate1": {ID: "rate1", Percentage: 19}}

	out, _, err := FillPurchaseOrderTemplate(buf.Bytes(), order, lineItems, testOrg(), testVendor(), taxRates)
	if err != nil {
		t.Fatalf("FillPurchaseOrderTemplate: %v", err)
	}

	f2, err := excelize.OpenReader(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("open output: %v", err)
	}
	defer f2.Close()
	got, _ := f2.GetCellValue(f2.GetSheetName(0), "B5")

	// 2 x 1000 cents = 2000 subtotal, 19% tax = 380, total = 2380.
	want := "Subtotal: 20.00 EUR Tax: 3.80 EUR Total: 23.80 EUR"
	if got != want {
		t.Errorf("B5 = %q, want %q", got, want)
	}
}

// TestEmbeddedDefaultPurchaseOrderTemplatePlaceholdersAllResolve mirrors
// TestEmbeddedDefaultInvoiceTemplatePlaceholdersAllResolve — the only
// automated guard against a typo in the committed, non-diff-reviewable
// binary every organization without a custom template gets.
func TestEmbeddedDefaultPurchaseOrderTemplatePlaceholdersAllResolve(t *testing.T) {
	f, err := excelize.OpenReader(bytes.NewReader(purchaseOrderDefaultTemplate))
	if err != nil {
		t.Fatalf("open embedded default template: %v", err)
	}
	defer f.Close()
	sheet := f.GetSheetName(0)

	scalars := buildPurchaseOrderScalarPlaceholders(testPurchaseOrder(), testOrg(), testVendor(), "EUR", 0, 0, 0)
	lineItemKeys := map[string]bool{
		"lineItems.description": true, "lineItems.quantity": true, "lineItems.unit": true,
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
				t.Errorf("embedded default purchase order template has unresolvable placeholder {{%s}}", key)
			}
		}
	}
	if !sawMarker {
		t.Error("embedded default purchase order template has no {{#lineItems}} marker row")
	}
}
