package main

import (
	"strconv"

	"github.com/xuri/excelize/v2"
)

// buildInvoiceDefaultTemplate generates db/templates/invoice_default.xlsx — the
// embedded default template for issue #115's custom invoice export. See the
// package doc comment in main.go for why this is generated programmatically
// rather than hand-authored. TestEmbeddedDefaultInvoiceTemplatePlaceholdersAllResolve
// (db/xlsx_export_test.go) is what actually guards its placeholders against
// a typo, since the committed binary itself isn't diff-reviewable.
//
// The output has two sheets: "Invoice" (the content sheet — sheet index 0,
// what db.FillInvoiceTemplate fills and returns) and "Available fields", a
// reference table of every placeholder db.buildScalarPlaceholders /
// db.buildLineItemPlaceholders resolve.
func buildInvoiceDefaultTemplate() {
	f := excelize.NewFile()
	sheet := "Invoice"
	f.SetSheetName(f.GetSheetName(0), sheet)
	styles := newTemplateStyles(f)
	darkHeader := newDarkHeaderStyle(f)

	set := func(cell, value string) { _ = f.SetCellStr(sheet, cell, value) }

	// Seller block (top-left).
	set("A1", "{{organization.name}}")
	f.SetCellStyle(sheet, "A1", "A1", styles.bold)
	set("A2", "{{organization.street}} {{organization.houseNumber}}")
	set("A3", "{{organization.postalCode}} {{organization.city}}")
	set("A4", "Matricule fiscal: {{organization.vatin}}")
	set("A5", "{{organization.email}} | {{organization.phone}}")

	// Document title + metadata (top-right).
	set("E1", "INVOICE")
	f.SetCellStyle(sheet, "E1", "E1", styles.title)
	set("E2", "Invoice number: {{invoice.number}}")
	set("E3", "Date: {{invoice.date}}")
	set("E4", "Due date: {{invoice.dueDate}}")
	set("E5", "Buyer reference: {{invoice.buyerReference}}")

	// Buyer block.
	set("A7", "Bill To")
	f.SetCellStyle(sheet, "A7", "A7", styles.bold)
	set("A8", "{{client.name}}")
	set("A9", "{{client.street}} {{client.houseNumber}}")
	set("A10", "{{client.postalCode}} {{client.city}}")
	set("A11", "Matricule fiscal: {{client.vatin}}")

	// Line item table header. Product (the SKU) and Description (the name)
	// are two separate columns, mirroring the web app's line-items table —
	// a document's "Description" field is filled with the product's name at
	// selection time, so a single merged column here would just duplicate
	// what the app itself keeps apart. The {{#lineItems}} marker on the
	// repeat row below lives in column G, off to the right of the visible
	// table: db.findMarkerRow scans every cell for it, not just column A,
	// specifically so the marker never has to consume a real content column.
	headerRow := 13
	cols := []string{"A", "B", "C", "D", "E", "F"}
	labels := []string{"Product", "Description", "Quantity", "Unit Price", "Tax Rate", "Line Total"}
	for i, col := range cols {
		cell := col + strconv.Itoa(headerRow)
		set(cell, labels[i])
		f.SetCellStyle(sheet, cell, cell, darkHeader)
	}

	// Repeat row: A the SKU, B the description, C-F the rest of the
	// per-item placeholders, G the marker.
	repeatRow := headerRow + 1
	set("A"+strconv.Itoa(repeatRow), "{{lineItems.sku}}")
	set("B"+strconv.Itoa(repeatRow), "{{lineItems.description}}")
	set("C"+strconv.Itoa(repeatRow), "{{lineItems.quantity}}")
	set("D"+strconv.Itoa(repeatRow), "{{lineItems.unitPrice}}")
	set("E"+strconv.Itoa(repeatRow), "{{lineItems.taxRate}}")
	set("F"+strconv.Itoa(repeatRow), "{{lineItems.lineTotal}}")
	set("G"+strconv.Itoa(repeatRow), "{{#lineItems}}")
	// Quantity/Unit Price/Tax Rate/Line Total are numeric, so the repeat row
	// itself carries the right-align style — DuplicateRowTo (db/xlsx_export.go)
	// preserves it across every expanded line-item row, the same precedent
	// the totals block below already uses. Product/Description stay left (text).
	for _, col := range []string{"C", "D", "E", "F"} {
		cell := col + strconv.Itoa(repeatRow)
		f.SetCellStyle(sheet, cell, cell, styles.right)
	}

	// Totals block, a few rows below the repeat row — located by the fill
	// engine via placeholder scan after row expansion, never a cached index.
	// Discount (remise, migration 0088) sits between Subtotal (gross) and Tax
	// so the block still adds up for an invoice that carries one; the generic
	// layout predates the discount field and didn't show it.
	totalsRow := repeatRow + 3
	totals := []struct{ label, value string }{
		{"Subtotal", "{{invoice.subTotal}}"},
		{"Discount", "{{invoice.discountAmount}}"},
		{"Tax", "{{invoice.taxTotal}}"},
		{"Total", "{{invoice.total}}"},
	}
	for i, row := range totals {
		label, value := "E"+strconv.Itoa(totalsRow+i), "F"+strconv.Itoa(totalsRow+i)
		set(label, row.label)
		f.SetCellStyle(sheet, label, label, styles.bold)
		set(value, row.value)
		f.SetCellStyle(sheet, value, value, styles.right)
	}

	footerRow := totalsRow + 6
	set("A"+strconv.Itoa(footerRow), "Payment terms: {{invoice.paymentTerms}}")
	set("A"+strconv.Itoa(footerRow+1), "IBAN: {{organization.iban}} | Bank: {{organization.bankName}}")
	// Tunisia invoice support: fiscalStampAmount is shown unconditionally,
	// blank/zero value and all, the same convention as VAT/buyer reference
	// above — this is now the ONLY place a fiscal-stamp org's stamp duty
	// reaches the PDF (pdf-tunisia.tsx no longer renders once PDF always
	// follows this template). Withholding tax is different: unlike the
	// stamp (NOT NULL DEFAULT 0, always a real amount), rate/amount are
	// nullable and blank for every organization that doesn't use the
	// feature — virtually all of them — so building "Withholding tax (…): …"
	// as static template text around two placeholders left a broken-looking
	// "Withholding tax (): " on every invoice PDF this app generates. The
	// whole line is one Go-computed placeholder instead (buildScalarPlaceholders
	// in db/xlsx_export.go), blank entirely when unset rather than a label
	// with nothing after it.
	set("A"+strconv.Itoa(footerRow+2), "Fiscal stamp: {{invoice.fiscalStampAmount}}")
	set("A"+strconv.Itoa(footerRow+3), "{{invoice.withholdingTaxLine}}")
	// The whole "Arrêtée la présente facture à la somme de …" sentence — blank
	// unless the organization enables it in Formatting, so it costs nothing on
	// an invoice that doesn't use it.
	set("A"+strconv.Itoa(footerRow+4), "{{invoice.amountInWords}}")

	// A (Product/SKU) is 22, not the original 16 -- a longer SKU (e.g.
	// "WIDGET-XL-DELUXE") overflowed visibly into Description at 16, found
	// by actually rendering a PDF with a long SKU rather than the short
	// ones every other manual check happened to use.
	f.SetColWidth(sheet, "A", "A", 22)
	f.SetColWidth(sheet, "B", "B", 40)
	f.SetColWidth(sheet, "C", "E", 14)
	f.SetColWidth(sheet, "F", "F", 16)

	applyFitToPageWidth(f, sheet)
	applyRepeatingHeaderRows(f, sheet, headerRow)
	applyPageFooter(f, sheet)
	addAvailableFieldsSheet(f, invoiceFieldRefs)
	finalizeWorkbook(f, sheet, "db/templates/gen/invoice_default.xlsx")
}
