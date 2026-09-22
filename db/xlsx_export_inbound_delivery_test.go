package db

import (
	"bytes"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func testInboundDelivery() InboundDelivery {
	return InboundDelivery{
		DeliveryNumber: "GR-0001",
		DeliveryDate:   1772721000000, // 2026-03-05
		Currency:       ptr("EUR"),
	}
}

// buildInboundDeliveryFixtureTemplate mirrors buildPurchaseOrderFixtureTemplate
// but with inboundDelivery.* placeholders and no tax/total placeholders —
// inbound_delivery_line_items has no tax rate and this document type has no
// aggregate totals block.
func buildInboundDeliveryFixtureTemplate(t *testing.T) []byte {
	t.Helper()
	f := excelize.NewFile()
	sheet := f.GetSheetName(0)
	_ = f.SetCellStr(sheet, "A1", "Receipt {{inboundDelivery.number}} from {{vendor.name}}")
	_ = f.SetCellStr(sheet, "A3", "{{#lineItems}}")
	_ = f.SetCellStr(sheet, "B3", "{{lineItems.description}}")
	_ = f.SetCellStr(sheet, "C3", "{{lineItems.lineTotal}}")

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatalf("build fixture template: %v", err)
	}
	return buf.Bytes()
}

func TestFillInboundDeliveryTemplateResolvesScalarPlaceholders(t *testing.T) {
	t.Parallel()
	tmpl := buildInboundDeliveryFixtureTemplate(t)
	delivery := testInboundDelivery()
	lineItems := []InboundDeliveryLineItem{{Description: "Widget", Quantity: 2, UnitCost: ptr(int64(500))}}

	out, unresolved, err := FillInboundDeliveryTemplate(tmpl, delivery, lineItems, testOrg(), testVendor(), nil, "")
	if err != nil {
		t.Fatalf("FillInboundDeliveryTemplate: %v", err)
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
	if !strings.Contains(a1, "GR-0001") || !strings.Contains(a1, "Acme Supplies") {
		t.Errorf("A1 = %q, want it to contain receipt number and vendor name", a1)
	}
}

// TestFillInboundDeliveryTemplateComputesLineTotalFromCost confirms each
// line's lineTotal is quantity x unitCost, the same lineTotalCents idiom
// every other line-item namespace uses — no aggregate footer to check here
// since this document type deliberately has none.
func TestFillInboundDeliveryTemplateComputesLineTotalFromCost(t *testing.T) {
	t.Parallel()
	tmpl := buildInboundDeliveryFixtureTemplate(t)
	delivery := testInboundDelivery()
	lineItems := []InboundDeliveryLineItem{{Description: "Widget", Quantity: 2, UnitCost: ptr(int64(1000))}}

	out, _, err := FillInboundDeliveryTemplate(tmpl, delivery, lineItems, testOrg(), testVendor(), nil, "")
	if err != nil {
		t.Fatalf("FillInboundDeliveryTemplate: %v", err)
	}

	f, err := excelize.OpenReader(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("open output: %v", err)
	}
	defer f.Close()
	got, _ := f.GetCellValue(f.GetSheetName(0), "C3")

	// 2 x 1000 cents = 2000 cents = 20.00 EUR.
	want := "20.00 EUR"
	if got != want {
		t.Errorf("C3 = %q, want %q", got, want)
	}
}

// TestEmbeddedDefaultInboundDeliveryTemplatePlaceholdersAllResolve mirrors
// TestEmbeddedDefaultPurchaseOrderTemplatePlaceholdersAllResolve — the only
// automated guard against a typo in the committed, non-diff-reviewable
// binary every organization without a custom template gets.
func TestEmbeddedDefaultInboundDeliveryTemplatePlaceholdersAllResolve(t *testing.T) {
	t.Parallel()
	assertEmbeddedTemplatePlaceholdersResolve(t, inboundDeliveryDefaultTemplate, "inbound delivery",
		buildInboundDeliveryScalarPlaceholders(testInboundDelivery(), testOrg(), testVendor(), "EUR"),
		map[string]bool{
			"lineItems.sku": true, "lineItems.description": true, "lineItems.quantity": true,
			"lineItems.unit": true, "lineItems.unitCost": true, "lineItems.lineTotal": true,
		})
}
