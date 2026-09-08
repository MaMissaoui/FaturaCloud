// Command gen generates db/templates/invoice_default.xlsx — the embedded
// default template for issue #115's custom invoice export. A real Excel/
// LibreOffice session wasn't available in the environment this feature was
// built in, so this script builds an equivalent, correctly structured
// workbook programmatically via excelize instead of hand-authoring one.
// The output is still a real, valid .xlsx a user can open and edit like any
// other — TestEmbeddedDefaultInvoiceTemplatePlaceholdersAllResolve
// (db/xlsx_export_test.go) is what actually guards its placeholders against
// a typo, since the committed binary itself isn't diff-reviewable.
//
// Run from the repo root and copy the output over the committed default if
// the layout ever needs a deliberate change:
//
//	go run ./db/templates/gen && mv db/templates/gen/invoice_default.xlsx db/templates/invoice_default.xlsx
package main

import (
	"log"
	"strconv"

	"github.com/xuri/excelize/v2"
)

func main() {
	f := excelize.NewFile()
	sheet := "Invoice"
	f.SetSheetName(f.GetSheetName(0), sheet)

	bold, _ := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
	title, _ := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true, Size: 18}})
	header, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"1E293B"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center"},
	})
	right, _ := f.NewStyle(&excelize.Style{Alignment: &excelize.Alignment{Horizontal: "right"}})

	set := func(cell, value string) { _ = f.SetCellStr(sheet, cell, value) }

	// Seller block (top-left).
	set("A1", "{{organization.name}}")
	f.SetCellStyle(sheet, "A1", "A1", bold)
	set("A2", "{{organization.street}} {{organization.houseNumber}}")
	set("A3", "{{organization.postalCode}} {{organization.city}}")
	set("A4", "VAT: {{organization.vatin}}")
	set("A5", "{{organization.email}} | {{organization.phone}}")

	// Document title + metadata (top-right).
	set("E1", "INVOICE")
	f.SetCellStyle(sheet, "E1", "E1", title)
	set("E2", "Invoice number: {{invoice.number}}")
	set("E3", "Date: {{invoice.date}}")
	set("E4", "Due date: {{invoice.dueDate}}")
	set("E5", "Buyer reference: {{invoice.buyerReference}}")

	// Buyer block.
	set("A7", "Bill To")
	f.SetCellStyle(sheet, "A7", "A7", bold)
	set("A8", "{{client.name}}")
	set("A9", "{{client.street}} {{client.houseNumber}}")
	set("A10", "{{client.postalCode}} {{client.city}}")
	set("A11", "VAT: {{client.vatin}}")

	// Line item table header.
	headerRow := 13
	cols := []string{"A", "B", "C", "D", "E"}
	labels := []string{"Description", "Quantity", "Unit Price", "Tax Rate", "Line Total"}
	for i, col := range cols {
		cell := col + strconv.Itoa(headerRow)
		set(cell, labels[i])
		f.SetCellStyle(sheet, cell, cell, header)
	}

	// Repeat row: column A carries the {{#lineItems}} marker (cleared by the
	// fill engine before output), the remaining columns carry per-item
	// placeholders.
	repeatRow := headerRow + 1
	set("A"+strconv.Itoa(repeatRow), "{{#lineItems}}")
	set("B"+strconv.Itoa(repeatRow), "{{lineItems.description}}")
	set("C"+strconv.Itoa(repeatRow), "{{lineItems.quantity}}")
	set("D"+strconv.Itoa(repeatRow), "{{lineItems.unitPrice}}")
	set("E"+strconv.Itoa(repeatRow), "{{lineItems.taxRate}}")
	set("F"+strconv.Itoa(repeatRow), "{{lineItems.lineTotal}}")

	// Totals block, a few rows below the repeat row — located by the fill
	// engine via placeholder scan after row expansion, never a cached index.
	totalsRow := repeatRow + 3
	set("E"+strconv.Itoa(totalsRow), "Subtotal")
	f.SetCellStyle(sheet, "E"+strconv.Itoa(totalsRow), "E"+strconv.Itoa(totalsRow), bold)
	set("F"+strconv.Itoa(totalsRow), "{{invoice.subTotal}}")
	f.SetCellStyle(sheet, "F"+strconv.Itoa(totalsRow), "F"+strconv.Itoa(totalsRow), right)
	set("E"+strconv.Itoa(totalsRow+1), "Tax")
	f.SetCellStyle(sheet, "E"+strconv.Itoa(totalsRow+1), "E"+strconv.Itoa(totalsRow+1), bold)
	set("F"+strconv.Itoa(totalsRow+1), "{{invoice.taxTotal}}")
	f.SetCellStyle(sheet, "F"+strconv.Itoa(totalsRow+1), "F"+strconv.Itoa(totalsRow+1), right)
	set("E"+strconv.Itoa(totalsRow+2), "Total")
	f.SetCellStyle(sheet, "E"+strconv.Itoa(totalsRow+2), "E"+strconv.Itoa(totalsRow+2), bold)
	set("F"+strconv.Itoa(totalsRow+2), "{{invoice.total}}")
	f.SetCellStyle(sheet, "F"+strconv.Itoa(totalsRow+2), "F"+strconv.Itoa(totalsRow+2), right)

	footerRow := totalsRow + 5
	set("A"+strconv.Itoa(footerRow), "Payment terms: {{invoice.paymentTerms}}")
	set("A"+strconv.Itoa(footerRow+1), "IBAN: {{organization.iban}} | Bank: {{organization.bankName}}")

	f.SetColWidth(sheet, "A", "A", 28)
	f.SetColWidth(sheet, "B", "B", 28)
	f.SetColWidth(sheet, "C", "E", 14)
	f.SetColWidth(sheet, "F", "F", 16)

	if err := f.SaveAs("db/templates/gen/invoice_default.xlsx"); err != nil {
		log.Fatal(err)
	}
}
