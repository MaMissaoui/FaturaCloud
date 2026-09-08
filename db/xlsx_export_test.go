package db

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func testInvoice() Invoice {
	return Invoice{
		Number:   "INV-0001",
		Date:     1772721000000, // 2026-03-05
		Currency: "EUR",
		Total:    11900,
		TaxTotal: 1900,
		SubTotal: 10000,
	}
}

func testOrg() Organization {
	return Organization{
		Name:       ptr("ACME Corp"),
		Currency:   ptr("EUR"),
		DateFormat: ptr("YYYY-MM-DD"),
	}
}

func testClient() Client {
	return Client{Name: ptr("Test Client GmbH")}
}

// buildFixtureTemplate returns a minimal in-memory .xlsx with one scalar
// placeholder, a marker row for line items, and a footer total placeholder
// below it — enough to exercise expansion, scalar substitution, and the
// footer-after-expansion ordering rule.
func buildFixtureTemplate(t *testing.T) []byte {
	t.Helper()
	f := excelize.NewFile()
	sheet := f.GetSheetName(0)
	_ = f.SetCellStr(sheet, "A1", "Invoice {{invoice.number}} for {{organization.name}}")
	_ = f.SetCellStr(sheet, "A3", "{{#lineItems}}")
	_ = f.SetCellStr(sheet, "B3", "{{lineItems.description}}")
	_ = f.SetCellStr(sheet, "C3", "{{lineItems.lineTotal}}")
	_ = f.SetCellStr(sheet, "B5", "Total: {{invoice.total}}")

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatalf("build fixture template: %v", err)
	}
	return buf.Bytes()
}

func TestFillInvoiceTemplateResolvesScalarPlaceholders(t *testing.T) {
	tmpl := buildFixtureTemplate(t)
	lineItems := []InvoiceLineItem{
		{Description: ptr("Widget"), Quantity: 1, UnitPrice: 10000},
	}

	out, unresolved, err := FillInvoiceTemplate(tmpl, testInvoice(), lineItems, testOrg(), testClient(), nil)
	if err != nil {
		t.Fatalf("FillInvoiceTemplate: %v", err)
	}
	if len(unresolved) != 0 {
		t.Fatalf("expected no unresolved placeholders, got %v", unresolved)
	}

	f, err := excelize.OpenReader(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("open filled output: %v", err)
	}
	defer f.Close()
	sheet := f.GetSheetName(0)

	a1, _ := f.GetCellValue(sheet, "A1")
	if want := "Invoice INV-0001 for ACME Corp"; a1 != want {
		t.Fatalf("A1 = %q, want %q", a1, want)
	}
}

func TestFillInvoiceTemplateExpandsLineItemRows(t *testing.T) {
	tmpl := buildFixtureTemplate(t)
	lineItems := []InvoiceLineItem{
		{Description: ptr("Widget"), Quantity: 2, UnitPrice: 2500},
		{Description: ptr("Gadget"), Quantity: 1, UnitPrice: 5000},
		{Description: ptr("Gizmo"), Quantity: 3, UnitPrice: 1000},
	}

	out, unresolved, err := FillInvoiceTemplate(tmpl, testInvoice(), lineItems, testOrg(), testClient(), nil)
	if err != nil {
		t.Fatalf("FillInvoiceTemplate: %v", err)
	}
	if len(unresolved) != 0 {
		t.Fatalf("expected no unresolved placeholders, got %v", unresolved)
	}

	f, err := excelize.OpenReader(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("open filled output: %v", err)
	}
	defer f.Close()
	sheet := f.GetSheetName(0)

	// 3 line items -> marker row (3) duplicated to rows 3,4,5; the footer,
	// originally on row 5, must have shifted to row 7.
	wantDescriptions := []struct {
		row  string
		desc string
		tot  string
	}{
		{"3", "Widget", "50.00 EUR"}, // 2 * 2500 cents = 5000 cents = 50.00
		{"4", "Gadget", "50.00 EUR"}, // 1 * 5000 cents = 5000 cents = 50.00
		{"5", "Gizmo", "30.00 EUR"},  // 3 * 1000 cents = 3000 cents = 30.00
	}
	for _, wd := range wantDescriptions {
		desc, _ := f.GetCellValue(sheet, "B"+wd.row)
		if desc != wd.desc {
			t.Fatalf("B%s = %q, want %q", wd.row, desc, wd.desc)
		}
		total, _ := f.GetCellValue(sheet, "C"+wd.row)
		if total != wd.tot {
			t.Fatalf("C%s = %q, want %q", wd.row, total, wd.tot)
		}
		// The marker cell in column A of every repeated row must be cleared,
		// never left as literal "{{#lineItems}}" text.
		markerCell, _ := f.GetCellValue(sheet, "A"+wd.row)
		if markerCell != "" {
			t.Fatalf("A%s (marker cell) = %q, want empty", wd.row, markerCell)
		}
	}

	// Footer row, originally row 5 in the 1-line-item template, must have
	// shifted down to row 7 (marker row 3 + 3 repeated rows + ... = row
	// 3+3-1=5 is the last repeated row, footer was 2 rows below the marker
	// in the original template, so it's now 2 rows below the last repeated
	// row: 5+2=7) and still resolve correctly — proves the footer was
	// located by content after expansion, not a stale cached row index.
	footer, _ := f.GetCellValue(sheet, "B7")
	if want := "Total: 119.00 EUR"; footer != want {
		t.Fatalf("B7 (footer) = %q, want %q", footer, want)
	}
}

func TestFillInvoiceTemplateBlanksUnknownPlaceholder(t *testing.T) {
	f := excelize.NewFile()
	sheet := f.GetSheetName(0)
	_ = f.SetCellStr(sheet, "A1", "{{invoice.nubmer}}") // typo, not a real placeholder
	_ = f.SetCellStr(sheet, "A3", "{{#lineItems}}")
	_ = f.SetCellStr(sheet, "B3", "{{lineItems.description}}")
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatalf("build fixture: %v", err)
	}

	lineItems := []InvoiceLineItem{{Description: ptr("Widget"), Quantity: 1, UnitPrice: 1000}}
	out, unresolved, err := FillInvoiceTemplate(buf.Bytes(), testInvoice(), lineItems, testOrg(), testClient(), nil)
	if err != nil {
		t.Fatalf("FillInvoiceTemplate should not error on an unknown placeholder: %v", err)
	}
	if len(unresolved) != 1 || unresolved[0] != "invoice.nubmer" {
		t.Fatalf("expected unresolved=[invoice.nubmer], got %v", unresolved)
	}

	out2, err := excelize.OpenReader(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("open filled output: %v", err)
	}
	defer out2.Close()
	a1, _ := out2.GetCellValue(f.GetSheetName(0), "A1")
	if a1 != "" {
		t.Fatalf("A1 should be blanked for an unresolved placeholder, got %q", a1)
	}
}

func TestFillInvoiceTemplateRequiresMarkerRow(t *testing.T) {
	f := excelize.NewFile()
	_ = f.SetCellStr(f.GetSheetName(0), "A1", "{{invoice.number}}") // no marker row anywhere
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatalf("build fixture: %v", err)
	}

	_, _, err := FillInvoiceTemplate(buf.Bytes(), testInvoice(), nil, testOrg(), testClient(), nil)
	if err == nil {
		t.Fatal("expected an error for a template with no {{#lineItems}} marker row")
	}
}

// TestFillInvoiceTemplateMarkerCanBeInAnyColumn confirms an org that
// downloads the template, rearranges it in Excel, and uploads it back isn't
// stuck with the marker in column A — db.findMarkerRow scans every cell, so
// a custom layout can put {{#lineItems}} anywhere (here, column D, with the
// real content in A-C) and still expand correctly.
func TestFillInvoiceTemplateMarkerCanBeInAnyColumn(t *testing.T) {
	f := excelize.NewFile()
	sheet := f.GetSheetName(0)
	_ = f.SetCellStr(sheet, "A1", "{{invoice.number}}")
	_ = f.SetCellStr(sheet, "A3", "{{lineItems.description}}")
	_ = f.SetCellStr(sheet, "B3", "{{lineItems.quantity}}")
	_ = f.SetCellStr(sheet, "C3", "{{lineItems.lineTotal}}")
	_ = f.SetCellStr(sheet, "D3", "{{#lineItems}}")
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatalf("build fixture: %v", err)
	}

	lineItems := []InvoiceLineItem{
		{Description: ptr("Widget"), Quantity: 2, UnitPrice: 500},
		{Description: ptr("Gadget"), Quantity: 1, UnitPrice: 1000},
	}
	out, unresolved, err := FillInvoiceTemplate(buf.Bytes(), testInvoice(), lineItems, testOrg(), testClient(), nil)
	if err != nil {
		t.Fatalf("FillInvoiceTemplate: %v", err)
	}
	if len(unresolved) != 0 {
		t.Fatalf("expected no unresolved placeholders, got %v", unresolved)
	}

	out2, err := excelize.OpenReader(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("open filled output: %v", err)
	}
	defer out2.Close()
	a3, _ := out2.GetCellValue(sheet, "A3")
	if a3 != "Widget" {
		t.Fatalf("A3 = %q, want %q", a3, "Widget")
	}
	a4, _ := out2.GetCellValue(sheet, "A4")
	if a4 != "Gadget" {
		t.Fatalf("A4 = %q, want %q", a4, "Gadget")
	}
	d3, _ := out2.GetCellValue(sheet, "D3")
	if d3 != "" {
		t.Fatalf("D3 (marker cell) should be blanked, got %q", d3)
	}
}

// TestFillInvoiceTemplateStripsExtraSheets guards the embedded default
// template's "Available fields" reference tab (and any template author's own
// notes/reference sheet) from ever reaching an actual invoice export — it's
// documentation for editing the template, not part of the document a
// customer receives.
func TestFillInvoiceTemplateStripsExtraSheets(t *testing.T) {
	f := excelize.NewFile()
	sheet := f.GetSheetName(0)
	_ = f.SetCellStr(sheet, "A1", "{{invoice.number}}")
	_ = f.SetCellStr(sheet, "A3", "{{#lineItems}}")
	_ = f.SetCellStr(sheet, "B3", "{{lineItems.description}}")
	if _, err := f.NewSheet("Notes"); err != nil {
		t.Fatalf("add extra sheet: %v", err)
	}
	_ = f.SetCellStr("Notes", "A1", "just a reference sheet for the template author")
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatalf("build fixture: %v", err)
	}

	lineItems := []InvoiceLineItem{{Description: ptr("Widget"), Quantity: 1, UnitPrice: 1000}}
	out, _, err := FillInvoiceTemplate(buf.Bytes(), testInvoice(), lineItems, testOrg(), testClient(), nil)
	if err != nil {
		t.Fatalf("FillInvoiceTemplate: %v", err)
	}

	out2, err := excelize.OpenReader(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("open filled output: %v", err)
	}
	defer out2.Close()
	if sheets := out2.GetSheetList(); len(sheets) != 1 || sheets[0] != sheet {
		t.Fatalf("expected output to have only the content sheet %q, got %v", sheet, sheets)
	}
}

// TestEmbeddedDefaultInvoiceTemplatePlaceholdersAllResolve opens the real
// shipped default template and confirms every {{...}} in it is a placeholder
// the fill engine actually recognizes. Load-bearing: this committed .xlsx
// binary isn't diff-reviewable, so this is the only automated guard against
// a typo in the file every customer receives.
func TestEmbeddedDefaultInvoiceTemplatePlaceholdersAllResolve(t *testing.T) {
	f, err := excelize.OpenReader(bytes.NewReader(invoiceDefaultTemplate))
	if err != nil {
		t.Fatalf("open embedded default template: %v", err)
	}
	defer f.Close()
	sheet := f.GetSheetName(0)

	scalars := buildScalarPlaceholders(testInvoice(), testOrg(), testClient())
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
				t.Errorf("embedded default template has unresolvable placeholder {{%s}}", key)
			}
		}
	}
	if !sawMarker {
		t.Error("embedded default template has no {{#lineItems}} marker row")
	}
}

// TestEmbeddedDefaultInvoiceTemplateLineItemHeaderAlignsWithData guards
// against the header row (e.g. "Description", "Quantity", ...) drifting out
// of alignment with the data row again. The {{#lineItems}} marker lives off
// in its own column (not one of the visible A-F content columns — see
// db.findMarkerRow), so "Description" gets the full merged A:B cell in both
// the header and data rows, with C-F carrying Quantity/Unit Price/Tax
// Rate/Line Total in both — column-for-column, not shifted by one.
func TestEmbeddedDefaultInvoiceTemplateLineItemHeaderAlignsWithData(t *testing.T) {
	f, err := excelize.OpenReader(bytes.NewReader(invoiceDefaultTemplate))
	if err != nil {
		t.Fatalf("open embedded default template: %v", err)
	}
	defer f.Close()
	sheet := f.GetSheetName(0)

	markerRow, _, err := findMarkerRow(f, sheet)
	if err != nil {
		t.Fatalf("find marker row: %v", err)
	}
	headerRowNum := markerRow - 1 // markerRow is 1-indexed; the header sits directly above it

	rows, err := f.GetRows(sheet)
	if err != nil {
		t.Fatalf("read rows: %v", err)
	}
	headerRow := rows[headerRowNum-1]
	dataRow := rows[markerRow-1]

	if len(headerRow) < 1 || strings.TrimSpace(headerRow[0]) != "Description" {
		t.Fatalf("header row column A should be \"Description\", got %v", headerRow)
	}
	if len(dataRow) < 1 || !strings.Contains(dataRow[0], "{{lineItems.description}}") {
		t.Fatalf("data row column A should carry {{lineItems.description}}, got %v", dataRow)
	}

	merges, err := f.GetMergeCells(sheet)
	if err != nil {
		t.Fatalf("get merge cells: %v", err)
	}
	wantMerged := map[string]bool{
		fmt.Sprintf("A%d:B%d", headerRowNum, headerRowNum): false,
		fmt.Sprintf("A%d:B%d", markerRow, markerRow):        false,
	}
	for _, m := range merges {
		key := m.GetStartAxis() + ":" + m.GetEndAxis()
		if _, ok := wantMerged[key]; ok {
			wantMerged[key] = true
		}
	}
	for want, found := range wantMerged {
		if !found {
			t.Errorf("expected merged range %s (description spanning A:B), got merges %v", want, merges)
		}
	}
}

func TestUploadDocumentTemplateRejectsInvalidXLSX(t *testing.T) {
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-1"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}

	err = d.UploadDocumentTemplate(org.ID, "invoice", "not-really.xlsx", []byte("not a real xlsx file"))
	if err == nil {
		t.Fatal("expected uploading non-xlsx bytes to be rejected")
	}
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected a *ValidationError, got %T: %v", err, err)
	}
}

func TestUploadAndResolveDocumentTemplateRoundTrips(t *testing.T) {
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-1"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}

	// No override yet: resolves to the embedded default.
	content, filename, err := d.ExportDocumentTemplateBytes(org.ID, "invoice")
	if err != nil {
		t.Fatalf("ExportDocumentTemplateBytes (default): %v", err)
	}
	if filename != "invoice_default.xlsx" {
		t.Fatalf("got filename %q, want invoice_default.xlsx", filename)
	}
	if !bytes.Equal(content, invoiceDefaultTemplate) {
		t.Fatal("expected default content to match the embedded template")
	}

	custom := buildFixtureTemplate(t)
	if err := d.UploadDocumentTemplate(org.ID, "invoice", "my-template.xlsx", custom); err != nil {
		t.Fatalf("UploadDocumentTemplate: %v", err)
	}

	content, filename, err = d.ExportDocumentTemplateBytes(org.ID, "invoice")
	if err != nil {
		t.Fatalf("ExportDocumentTemplateBytes (override): %v", err)
	}
	if filename != "my-template.xlsx" {
		t.Fatalf("got filename %q, want my-template.xlsx", filename)
	}
	if !bytes.Equal(content, custom) {
		t.Fatal("expected override content to match what was uploaded")
	}

	// Delete reverts to the default.
	ok, err := d.DeleteDocumentTemplate(org.ID, "invoice")
	if err != nil || !ok {
		t.Fatalf("DeleteDocumentTemplate: ok=%v err=%v", ok, err)
	}
	_, filename, err = d.ExportDocumentTemplateBytes(org.ID, "invoice")
	if err != nil || filename != "invoice_default.xlsx" {
		t.Fatalf("expected revert to default after delete: filename=%q err=%v", filename, err)
	}
}
