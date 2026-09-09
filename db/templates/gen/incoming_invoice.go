package main

import (
	"log"
	"strconv"

	"github.com/xuri/excelize/v2"
)

// buildIncomingInvoiceTemplate generates
// db/templates/incoming_invoice_default.xlsx, following invoice.go's layout
// conventions. Unlike purchase orders/orders, an incoming invoice DOES have
// server-validated stored totals (db.FillIncomingInvoiceTemplate reads
// subTotal/taxTotal/total straight off the row — see
// db/xlsx_export_incoming_invoice.go), so this mirrors invoice.go's totals
// block rather than purchase_order.go's computed one. The "buyer" here is
// FaturaCloud's own organization (this is a bill it received, not one it
// issued) and the "seller" block is the vendor — the header layout is
// swapped relative to invoice.go for exactly that reason.
func buildIncomingInvoiceTemplate() {
	f := excelize.NewFile()
	sheet := "Incoming Invoice"
	f.SetSheetName(f.GetSheetName(0), sheet)
	styles := newTemplateStyles(f)

	set := func(cell, value string) { _ = f.SetCellStr(sheet, cell, value) }

	// Vendor block (top-left) — who billed this.
	set("A1", "{{vendor.name}}")
	f.SetCellStyle(sheet, "A1", "A1", styles.bold)
	set("A2", "{{vendor.street}} {{vendor.houseNumber}}")
	set("A3", "{{vendor.postalCode}} {{vendor.city}}")
	set("A4", "VAT: {{vendor.vatin}}")
	set("A5", "{{vendor.email}} | {{vendor.phone}}")

	// Document title + metadata (top-right).
	set("E1", "INCOMING INVOICE")
	f.SetCellStyle(sheet, "E1", "E1", styles.title)
	set("E2", "Vendor invoice number: {{incomingInvoice.number}}")
	set("E3", "Date: {{incomingInvoice.date}}")
	set("E4", "Due date: {{incomingInvoice.dueDate}}")
	set("E5", "Reference: {{incomingInvoice.reference}}")
	set("E6", "Purchase order: {{incomingInvoice.purchaseOrder}}")

	// Buyer block — FaturaCloud's own organization, receiving the bill.
	set("A7", "Billed To")
	f.SetCellStyle(sheet, "A7", "A7", styles.bold)
	set("A8", "{{organization.name}}")
	set("A9", "{{organization.street}} {{organization.houseNumber}}")
	set("A10", "{{organization.postalCode}} {{organization.city}}")
	set("A11", "VAT: {{organization.vatin}}")

	// Line item table header. Description spans A:B (merged), same reasoning
	// as invoice.go; the marker lives in column G, off to the right of the
	// visible 5-column table (db.findMarkerRow scans every cell, not just
	// column A).
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
	// per-item placeholders, G the marker.
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
	// These ARE the incoming invoice's own stored/validated totals, unlike
	// purchase_order.go's computed ones.
	totalsRow := repeatRow + 3
	set("E"+strconv.Itoa(totalsRow), "Subtotal")
	f.SetCellStyle(sheet, "E"+strconv.Itoa(totalsRow), "E"+strconv.Itoa(totalsRow), styles.bold)
	set("F"+strconv.Itoa(totalsRow), "{{incomingInvoice.subTotal}}")
	f.SetCellStyle(sheet, "F"+strconv.Itoa(totalsRow), "F"+strconv.Itoa(totalsRow), styles.right)
	set("E"+strconv.Itoa(totalsRow+1), "Tax")
	f.SetCellStyle(sheet, "E"+strconv.Itoa(totalsRow+1), "E"+strconv.Itoa(totalsRow+1), styles.bold)
	set("F"+strconv.Itoa(totalsRow+1), "{{incomingInvoice.taxTotal}}")
	f.SetCellStyle(sheet, "F"+strconv.Itoa(totalsRow+1), "F"+strconv.Itoa(totalsRow+1), styles.right)
	set("E"+strconv.Itoa(totalsRow+2), "Total")
	f.SetCellStyle(sheet, "E"+strconv.Itoa(totalsRow+2), "E"+strconv.Itoa(totalsRow+2), styles.bold)
	set("F"+strconv.Itoa(totalsRow+2), "{{incomingInvoice.total}}")
	f.SetCellStyle(sheet, "F"+strconv.Itoa(totalsRow+2), "F"+strconv.Itoa(totalsRow+2), styles.right)

	footerRow := totalsRow + 5
	set("A"+strconv.Itoa(footerRow), "Notes: {{incomingInvoice.notes}}")

	f.SetColWidth(sheet, "A", "A", 28)
	f.SetColWidth(sheet, "B", "B", 28)
	f.SetColWidth(sheet, "C", "E", 14)
	f.SetColWidth(sheet, "F", "F", 16)

	applyFitToPageWidth(f, sheet)
	applyRepeatingHeaderRows(f, sheet, headerRow)
	applyPageFooter(f, sheet)
	addAvailableFieldsSheet(f, incomingInvoiceFieldRefs)
	finalizeWorkbook(f, sheet, "db/templates/gen/incoming_invoice_default.xlsx")
}

// incomingInvoiceFieldRefs groups every placeholder
// db.buildIncomingInvoiceScalarPlaceholders / db.buildIncomingInvoiceLineItemPlaceholders
// (db/xlsx_export_incoming_invoice.go) resolve, the same Header / Item lines
// / Footer grouping as invoiceFieldRefs.
var incomingInvoiceFieldRefs = []fieldRef{
	// Header — vendor (who billed this), invoice metadata, and buyer (our own organization) info.
	{"Header", "{{incomingInvoice.number}}", "Vendor's own invoice number"},
	{"Header", "{{incomingInvoice.date}}", "Invoice date"},
	{"Header", "{{incomingInvoice.dueDate}}", "Payment due date, blank if unset"},
	{"Header", "{{incomingInvoice.currency}}", "Currency code (e.g. EUR)"},
	{"Header", "{{incomingInvoice.reference}}", "Free-text reference, blank if unset"},
	{"Header", "{{incomingInvoice.purchaseOrder}}", "Linked purchase order number, blank if unlinked"},
	{"Header", "{{vendor.name}}", "Vendor company name"},
	{"Header", "{{vendor.vatin}}", "Vendor VAT number"},
	{"Header", "{{vendor.email}}", "Vendor email"},
	{"Header", "{{vendor.phone}}", "Vendor phone"},
	{"Header", "{{vendor.street}}", "Vendor street"},
	{"Header", "{{vendor.houseNumber}}", "Vendor house number"},
	{"Header", "{{vendor.postalCode}}", "Vendor postal code"},
	{"Header", "{{vendor.city}}", "Vendor city"},
	{"Header", "{{organization.name}}", "Buyer (your own) company name"},
	{"Header", "{{organization.vatin}}", "Buyer VAT number"},
	{"Header", "{{organization.email}}", "Buyer email"},
	{"Header", "{{organization.phone}}", "Buyer phone"},
	{"Header", "{{organization.website}}", "Buyer website"},
	{"Header", "{{organization.street}}", "Buyer street"},
	{"Header", "{{organization.houseNumber}}", "Buyer house number"},
	{"Header", "{{organization.postalCode}}", "Buyer postal code"},
	{"Header", "{{organization.city}}", "Buyer city"},

	// Item lines — only meaningful inside the repeated {{#lineItems}} row.
	{"Item lines", "{{#lineItems}}", "Marker (not a value) — place alone in any one cell of the row to repeat once per line item; that whole cell is blanked in the output"},
	{"Item lines", "{{lineItems.description}}", "Line item description"},
	{"Item lines", "{{lineItems.quantity}}", "Quantity"},
	{"Item lines", "{{lineItems.unitPrice}}", "Unit price, formatted with currency"},
	{"Item lines", "{{lineItems.taxRate}}", "Tax rate percentage (e.g. 19.5%), blank if unset"},
	{"Item lines", "{{lineItems.lineTotal}}", "Quantity x unit price, formatted with currency"},

	// Footer — the invoice's own server-validated stored totals (unlike
	// purchase orders/orders, these are read directly, never recomputed).
	{"Footer", "{{incomingInvoice.subTotal}}", "Subtotal before tax"},
	{"Footer", "{{incomingInvoice.taxTotal}}", "Total tax"},
	{"Footer", "{{incomingInvoice.total}}", "Grand total"},
	{"Footer", "{{incomingInvoice.notes}}", "Free-text notes, blank if unset"},
}
