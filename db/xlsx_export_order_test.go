package db

import (
	"bytes"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func testOrder() Order {
	return Order{
		OrderNumber: "ORD-0001",
		OrderDate:   1772721000000, // 2026-03-05
		Currency:    ptr("EUR"),
	}
}

// buildOrderFixtureTemplate mirrors buildFixtureTemplate but with order.*
// placeholders instead of invoice.* ones.
func buildOrderFixtureTemplate(t *testing.T) []byte {
	t.Helper()
	f := excelize.NewFile()
	sheet := f.GetSheetName(0)
	_ = f.SetCellStr(sheet, "A1", "Order {{order.number}} for {{organization.name}}")
	_ = f.SetCellStr(sheet, "A3", "{{#lineItems}}")
	_ = f.SetCellStr(sheet, "B3", "{{lineItems.description}}")
	_ = f.SetCellStr(sheet, "C3", "{{lineItems.lineTotal}}")
	_ = f.SetCellStr(sheet, "B5", "Total: {{order.total}}")

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatalf("build fixture template: %v", err)
	}
	return buf.Bytes()
}

func TestFillOrderTemplateResolvesScalarPlaceholders(t *testing.T) {
	t.Parallel()
	tmpl := buildOrderFixtureTemplate(t)
	order := testOrder()
	lineItems := []OrderLineItem{{Description: "Widget", Quantity: 2, UnitPrice: 500}}

	out, unresolved, err := FillOrderTemplate(tmpl, order, lineItems, testOrg(), testClient())
	if err != nil {
		t.Fatalf("FillOrderTemplate: %v", err)
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
	if !strings.Contains(a1, "ORD-0001") || !strings.Contains(a1, "ACME Corp") {
		t.Errorf("A1 = %q, want it to contain order number and organization name", a1)
	}
}

func TestFillOrderTemplateComputesTotalsFromLineItemsWithNoTax(t *testing.T) {
	t.Parallel()
	f := excelize.NewFile()
	sheet := f.GetSheetName(0)
	_ = f.SetCellStr(sheet, "A3", "{{#lineItems}}")
	_ = f.SetCellStr(sheet, "B3", "{{lineItems.description}}")
	_ = f.SetCellStr(sheet, "B5", "Subtotal: {{order.subTotal}} Tax: {{order.taxTotal}} Total: {{order.total}}")
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatalf("build fixture: %v", err)
	}

	order := testOrder()
	lineItems := []OrderLineItem{
		{Description: "Widget", Quantity: 2, UnitPrice: 1000},
	}

	out, _, err := FillOrderTemplate(buf.Bytes(), order, lineItems, testOrg(), testClient())
	if err != nil {
		t.Fatalf("FillOrderTemplate: %v", err)
	}

	f2, err := excelize.OpenReader(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("open output: %v", err)
	}
	defer f2.Close()
	got, _ := f2.GetCellValue(f2.GetSheetName(0), "B5")

	// 2 x 1000 cents = 2000 subtotal, no tax rate on order line items -> 0 tax.
	want := "Subtotal: 20.00 EUR Tax: 0.00 EUR Total: 20.00 EUR"
	if got != want {
		t.Errorf("B5 = %q, want %q", got, want)
	}
}

// TestEmbeddedDefaultOrderTemplatePlaceholdersAllResolve mirrors
// TestEmbeddedDefaultInvoiceTemplatePlaceholdersAllResolve — the only
// automated guard against a typo in the committed, non-diff-reviewable
// binary every organization without a custom template gets.
func TestEmbeddedDefaultOrderTemplatePlaceholdersAllResolve(t *testing.T) {
	t.Parallel()
	f, err := excelize.OpenReader(bytes.NewReader(orderDefaultTemplate))
	if err != nil {
		t.Fatalf("open embedded default template: %v", err)
	}
	defer f.Close()
	sheet := f.GetSheetName(0)

	scalars := buildOrderScalarPlaceholders(testOrder(), testOrg(), testClient(), "EUR", 0, 0, 0)
	lineItemKeys := map[string]bool{
		"lineItems.description": true, "lineItems.quantity": true,
		"lineItems.unitPrice": true, "lineItems.lineTotal": true,
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
				t.Errorf("embedded default order template has unresolvable placeholder {{%s}}", key)
			}
		}
	}
	if !sawMarker {
		t.Error("embedded default order template has no {{#lineItems}} marker row")
	}
}
