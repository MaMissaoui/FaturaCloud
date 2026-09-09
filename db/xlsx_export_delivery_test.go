package db

import (
	"bytes"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func testOutboundDelivery() OutboundDelivery {
	return OutboundDelivery{
		DeliveryNumber: "DEL-0001",
		DeliveryDate:   1772721000000, // 2026-03-05
	}
}

// buildDeliveryFixtureTemplate mirrors buildPurchaseOrderFixtureTemplate but
// with delivery.* placeholders and no price/tax/total placeholders at all —
// outbound_delivery_line_items has no price columns.
func buildDeliveryFixtureTemplate(t *testing.T) []byte {
	t.Helper()
	f := excelize.NewFile()
	sheet := f.GetSheetName(0)
	_ = f.SetCellStr(sheet, "A1", "Delivery {{delivery.number}} from {{organization.name}} to {{client.name}}")
	_ = f.SetCellStr(sheet, "A3", "{{#lineItems}}")
	_ = f.SetCellStr(sheet, "B3", "{{lineItems.description}}")
	_ = f.SetCellStr(sheet, "C3", "{{lineItems.quantity}} {{lineItems.unit}}")

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatalf("build fixture template: %v", err)
	}
	return buf.Bytes()
}

func TestFillDeliveryTemplateResolvesScalarPlaceholders(t *testing.T) {
	tmpl := buildDeliveryFixtureTemplate(t)
	delivery := testOutboundDelivery()
	lineItems := []OutboundDeliveryLineItem{{Description: "Widget", Quantity: 3, Unit: ptr("pcs")}}

	out, unresolved, err := FillDeliveryTemplate(tmpl, delivery, lineItems, testOrg(), testClient())
	if err != nil {
		t.Fatalf("FillDeliveryTemplate: %v", err)
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
	if !strings.Contains(a1, "DEL-0001") || !strings.Contains(a1, "ACME Corp") ||
		!strings.Contains(a1, "Test Client GmbH") {
		t.Errorf("A1 = %q, want it to contain delivery number, organization name, and client name", a1)
	}
}

// TestFillDeliveryTemplateIncludesSKU guards against a regression found via
// advisor review: the old client-side DeliveryNotePDF button showed each
// line's product SKU, and the first cut of this server template dropped it.
// buildDeliveryLineItemPlaceholders' "lineItems.sku" (joined from products
// via GetDeliveryLineItems) restores it — blank for a free-text line, same
// convention as every other optional field here.
func TestFillDeliveryTemplateIncludesSKU(t *testing.T) {
	f := excelize.NewFile()
	sheet := f.GetSheetName(0)
	_ = f.SetCellStr(sheet, "A3", "{{#lineItems}}")
	_ = f.SetCellStr(sheet, "B3", "{{lineItems.sku}}")
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatalf("build fixture: %v", err)
	}

	delivery := testOutboundDelivery()
	lineItems := []OutboundDeliveryLineItem{{Description: "Widget", Quantity: 1, SKU: ptr("WID-001")}}

	out, _, err := FillDeliveryTemplate(buf.Bytes(), delivery, lineItems, testOrg(), testClient())
	if err != nil {
		t.Fatalf("FillDeliveryTemplate: %v", err)
	}
	f2, err := excelize.OpenReader(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("open output: %v", err)
	}
	defer f2.Close()
	got, _ := f2.GetCellValue(f2.GetSheetName(0), "B3")
	if got != "WID-001" {
		t.Errorf("B3 = %q, want %q", got, "WID-001")
	}
}

// TestFillDeliveryTemplateBlankClientOnStandaloneDelivery confirms a
// walk-in/no-client delivery (ClientID nil, so the caller passes a zero
// Client) exports with client.* placeholders blank rather than failing —
// the same precedent as purchase orders' vendor-less export.
func TestFillDeliveryTemplateBlankClientOnStandaloneDelivery(t *testing.T) {
	tmpl := buildDeliveryFixtureTemplate(t)
	delivery := testOutboundDelivery()
	lineItems := []OutboundDeliveryLineItem{{Description: "Widget", Quantity: 1}}

	out, unresolved, err := FillDeliveryTemplate(tmpl, delivery, lineItems, testOrg(), Client{})
	if err != nil {
		t.Fatalf("FillDeliveryTemplate: %v", err)
	}
	if len(unresolved) != 0 {
		t.Errorf("unexpected unresolved placeholders: %v", unresolved)
	}

	f, err := excelize.OpenReader(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("open output: %v", err)
	}
	defer f.Close()
	a1, _ := f.GetCellValue(f.GetSheetName(0), "A1")
	if !strings.Contains(a1, "DEL-0001") {
		t.Errorf("A1 = %q, want it to still contain the delivery number", a1)
	}
}

// TestEmbeddedDefaultDeliveryTemplatePlaceholdersAllResolve mirrors
// TestEmbeddedDefaultPurchaseOrderTemplatePlaceholdersAllResolve — the only
// automated guard against a typo in the committed, non-diff-reviewable
// binary every organization without a custom template gets.
func TestEmbeddedDefaultDeliveryTemplatePlaceholdersAllResolve(t *testing.T) {
	f, err := excelize.OpenReader(bytes.NewReader(deliveryDefaultTemplate))
	if err != nil {
		t.Fatalf("open embedded default template: %v", err)
	}
	defer f.Close()
	sheet := f.GetSheetName(0)

	scalars := buildDeliveryScalarPlaceholders(testOutboundDelivery(), testOrg(), testClient())
	lineItemKeys := map[string]bool{
		"lineItems.description": true, "lineItems.sku": true,
		"lineItems.quantity": true, "lineItems.unit": true,
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
				t.Errorf("embedded default delivery template has unresolvable placeholder {{%s}}", key)
			}
		}
	}
	if !sawMarker {
		t.Error("embedded default delivery template has no {{#lineItems}} marker row")
	}
}
