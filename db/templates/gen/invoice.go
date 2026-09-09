package main

import (
	"log"
	"strconv"

	"github.com/xuri/excelize/v2"
)

// buildInvoiceTemplate generates db/templates/invoice_default.xlsx — the
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
func buildInvoiceTemplate() {
	f := excelize.NewFile()
	sheet := "Invoice"
	f.SetSheetName(f.GetSheetName(0), sheet)
	styles := newTemplateStyles(f)

	set := func(cell, value string) { _ = f.SetCellStr(sheet, cell, value) }

	// Seller block (top-left).
	set("A1", "{{organization.name}}")
	f.SetCellStyle(sheet, "A1", "A1", styles.bold)
	set("A2", "{{organization.street}} {{organization.houseNumber}}")
	set("A3", "{{organization.postalCode}} {{organization.city}}")
	set("A4", "VAT: {{organization.vatin}}")
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
	set("A11", "VAT: {{client.vatin}}")

	// Line item table header. Description spans A:B (merged) for extra room —
	// a description is the one field on this row long enough to need it.
	// The {{#lineItems}} marker on the repeat row below lives in column G,
	// off to the right of the visible table: db.findMarkerRow scans every
	// cell for it, not just column A, specifically so the marker never has
	// to consume a real content column the way it used to.
	headerRow := 13
	if err := f.MergeCell(sheet, "A"+strconv.Itoa(headerRow), "B"+strconv.Itoa(headerRow)); err != nil {
		log.Fatal(err)
	}
	cols := []string{"A", "C", "D", "E", "F"}
	labels := []string{"Description", "Quantity", "Unit Price", "Tax Rate", "Line Total"}
	for i, col := range cols {
		cell := col + strconv.Itoa(headerRow)
		set(cell, labels[i])
		f.SetCellStyle(sheet, cell, cell, styles.header)
	}
	f.SetCellStyle(sheet, "B"+strconv.Itoa(headerRow), "B"+strconv.Itoa(headerRow), styles.header)

	// Repeat row: A:B merged carries the description, C-F the rest of the
	// per-item placeholders, G the marker (DuplicateRowTo preserves both the
	// merge and the marker across every expanded line item row).
	repeatRow := headerRow + 1
	if err := f.MergeCell(sheet, "A"+strconv.Itoa(repeatRow), "B"+strconv.Itoa(repeatRow)); err != nil {
		log.Fatal(err)
	}
	set("A"+strconv.Itoa(repeatRow), "{{lineItems.description}}")
	set("C"+strconv.Itoa(repeatRow), "{{lineItems.quantity}}")
	set("D"+strconv.Itoa(repeatRow), "{{lineItems.unitPrice}}")
	set("E"+strconv.Itoa(repeatRow), "{{lineItems.taxRate}}")
	set("F"+strconv.Itoa(repeatRow), "{{lineItems.lineTotal}}")
	set("G"+strconv.Itoa(repeatRow), "{{#lineItems}}")

	// Totals block, a few rows below the repeat row — located by the fill
	// engine via placeholder scan after row expansion, never a cached index.
	totalsRow := repeatRow + 3
	set("E"+strconv.Itoa(totalsRow), "Subtotal")
	f.SetCellStyle(sheet, "E"+strconv.Itoa(totalsRow), "E"+strconv.Itoa(totalsRow), styles.bold)
	set("F"+strconv.Itoa(totalsRow), "{{invoice.subTotal}}")
	f.SetCellStyle(sheet, "F"+strconv.Itoa(totalsRow), "F"+strconv.Itoa(totalsRow), styles.right)
	set("E"+strconv.Itoa(totalsRow+1), "Tax")
	f.SetCellStyle(sheet, "E"+strconv.Itoa(totalsRow+1), "E"+strconv.Itoa(totalsRow+1), styles.bold)
	set("F"+strconv.Itoa(totalsRow+1), "{{invoice.taxTotal}}")
	f.SetCellStyle(sheet, "F"+strconv.Itoa(totalsRow+1), "F"+strconv.Itoa(totalsRow+1), styles.right)
	set("E"+strconv.Itoa(totalsRow+2), "Total")
	f.SetCellStyle(sheet, "E"+strconv.Itoa(totalsRow+2), "E"+strconv.Itoa(totalsRow+2), styles.bold)
	set("F"+strconv.Itoa(totalsRow+2), "{{invoice.total}}")
	f.SetCellStyle(sheet, "F"+strconv.Itoa(totalsRow+2), "F"+strconv.Itoa(totalsRow+2), styles.right)

	footerRow := totalsRow + 5
	set("A"+strconv.Itoa(footerRow), "Payment terms: {{invoice.paymentTerms}}")
	set("A"+strconv.Itoa(footerRow+1), "IBAN: {{organization.iban}} | Bank: {{organization.bankName}}")
	// Tunisia invoice support: shown unconditionally, blank/zero value and all,
	// the same convention as VAT/buyer reference above — this is now the ONLY
	// place a fiscal-stamp org's stamp duty and withholding tax reach the PDF
	// (pdf-tunisia.tsx no longer renders once PDF always follows this template).
	set("A"+strconv.Itoa(footerRow+2), "Fiscal stamp: {{invoice.fiscalStampAmount}}")
	set("A"+strconv.Itoa(footerRow+3), "Withholding tax ({{invoice.withholdingTaxRate}}): {{invoice.withholdingTaxAmount}}")

	f.SetColWidth(sheet, "A", "A", 28)
	f.SetColWidth(sheet, "B", "B", 28)
	f.SetColWidth(sheet, "C", "E", 14)
	f.SetColWidth(sheet, "F", "F", 16)

	applyFitToPageWidth(f, sheet)
	applyRepeatingHeaderRows(f, sheet, headerRow)
	applyPageFooter(f, sheet)
	addAvailableFieldsSheet(f, invoiceFieldRefs)
	finalizeWorkbook(f, sheet, "db/templates/gen/invoice_default.xlsx")
}

// invoiceFieldRefs groups every placeholder db.buildScalarPlaceholders /
// db.buildLineItemPlaceholders (db/xlsx_export.go) resolve, the way a
// template author encounters them: Header (seller/invoice/customer info at
// the top), Item lines (only valid inside the repeated {{#lineItems}} row),
// and Footer (totals and payment/banking details).
var invoiceFieldRefs = []fieldRef{
	// Header — seller, invoice metadata, and customer info.
	{"Header", "{{invoice.number}}", "Invoice number"},
	{"Header", "{{invoice.date}}", "Invoice date"},
	{"Header", "{{invoice.dueDate}}", "Payment due date"},
	{"Header", "{{invoice.currency}}", "Currency code (e.g. EUR)"},
	{"Header", "{{invoice.buyerReference}}", "Buyer reference (e.g. a Leitweg-ID)"},
	{"Header", "{{organization.name}}", "Seller company name"},
	{"Header", "{{organization.vatin}}", "Seller VAT number"},
	{"Header", "{{organization.email}}", "Seller email"},
	{"Header", "{{organization.phone}}", "Seller phone"},
	{"Header", "{{organization.website}}", "Seller website"},
	{"Header", "{{organization.street}}", "Seller street"},
	{"Header", "{{organization.houseNumber}}", "Seller house number"},
	{"Header", "{{organization.postalCode}}", "Seller postal code"},
	{"Header", "{{organization.city}}", "Seller city"},
	{"Header", "{{client.name}}", "Customer name"},
	{"Header", "{{client.vatin}}", "Customer VAT number"},
	{"Header", "{{client.email}}", "Customer email"},
	{"Header", "{{client.phone}}", "Customer phone"},
	{"Header", "{{client.street}}", "Customer street"},
	{"Header", "{{client.houseNumber}}", "Customer house number"},
	{"Header", "{{client.postalCode}}", "Customer postal code"},
	{"Header", "{{client.city}}", "Customer city"},

	// Item lines — only meaningful inside the repeated {{#lineItems}} row.
	{"Item lines", "{{#lineItems}}", "Marker (not a value) — place alone in any one cell of the row to repeat once per line item; that whole cell is blanked in the output"},
	{"Item lines", "{{lineItems.description}}", "Line item description"},
	{"Item lines", "{{lineItems.quantity}}", "Quantity"},
	{"Item lines", "{{lineItems.unitPrice}}", "Unit price, formatted with currency"},
	{"Item lines", "{{lineItems.taxRate}}", "Tax rate percentage (e.g. 19.5%)"},
	{"Item lines", "{{lineItems.lineTotal}}", "Quantity x unit price, formatted with currency"},

	// Footer — totals and payment/banking details.
	{"Footer", "{{invoice.subTotal}}", "Subtotal before tax"},
	{"Footer", "{{invoice.taxTotal}}", "Total tax"},
	{"Footer", "{{invoice.total}}", "Grand total"},
	{"Footer", "{{invoice.paymentTerms}}", "Payment terms text"},
	{"Footer", "{{invoice.fiscalStampAmount}}", "Tunisia timbre fiscal — flat duty, formatted with currency (0 if unused)"},
	{"Footer", "{{invoice.withholdingTaxRate}}", "Withholding tax rate percentage, blank if unset"},
	{"Footer", "{{invoice.withholdingTaxAmount}}", "Withholding tax amount, formatted with currency, blank if unset"},
	{"Footer", "{{organization.iban}}", "Seller IBAN"},
	{"Footer", "{{organization.bankName}}", "Seller bank name"},

	// Export info — when this specific file was generated, not any document
	// field. See mergeExportMetaPlaceholders (db/xlsx_export.go).
	{"Export info", "{{export.generatedDate}}", "Date this file was exported (not the invoice's own date), in the organization's date format"},
	{"Export info", "{{export.generatedTime}}", "Time this file was exported, 24-hour HH:MM, server time"},
	{"Export info", "&P (page number) / &N (total pages)", "Native Excel/LibreOffice codes, not a {{}} placeholder — only work inside this sheet's own Page Layout ▸ Header/Footer, never in a regular cell, since the page count isn't known until export. The default footer already shows \"Page &P of &N\" on every page; edit it in Excel/LibreOffice's own header/footer editor to move or restyle it"},
}
