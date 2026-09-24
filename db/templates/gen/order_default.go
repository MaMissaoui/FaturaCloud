package main

import (
	"strconv"

	"github.com/xuri/excelize/v2"
)

// buildOrderDefaultTemplate generates db/templates/order_default.xlsx, following
// invoice.go/purchase_order.go's layout conventions. A sales order carries
// prices (it's sent to the client, unlike an outbound delivery note) but,
// like purchase orders, has no server-validated stored totals — subtotal/
// tax/total are computed at export time (db/xlsx_export_order.go). Unlike
// purchase orders it has no per-line Unit or Tax Rate column at all
// (OrderLineItem has neither), so the item table is only 4 columns wide.
func buildOrderDefaultTemplate() {
	f := excelize.NewFile()
	sheet := "Order"
	f.SetSheetName(f.GetSheetName(0), sheet)
	styles := newTemplateStyles(f)
	darkHeader := newDarkHeaderStyle(f)

	set := func(cell, value string) { _ = f.SetCellStr(sheet, cell, value) }

	// Seller block (top-left) — the organization the order confirms with.
	set("A1", "{{organization.name}}")
	f.SetCellStyle(sheet, "A1", "A1", styles.bold)
	set("A2", "{{organization.street}} {{organization.houseNumber}}")
	set("A3", "{{organization.postalCode}} {{organization.city}}")
	set("A4", "Matricule fiscal: {{organization.vatin}}")
	set("A5", "{{organization.email}} | {{organization.phone}}")

	// Document title + metadata (top-right).
	set("E1", "ORDER CONFIRMATION")
	f.SetCellStyle(sheet, "E1", "E1", styles.title)
	set("E2", "Order number: {{order.number}}")
	set("E3", "Order date: {{order.orderDate}}")
	set("E4", "Delivery date: {{order.deliveryDate}}")
	set("E5", "Tracking number: {{order.trackingNumber}}")

	// Buyer block.
	set("A7", "Bill To")
	f.SetCellStyle(sheet, "A7", "A7", styles.bold)
	set("A8", "{{client.name}}")
	set("A9", "{{client.street}} {{client.houseNumber}}")
	set("A10", "{{client.postalCode}} {{client.city}}")
	set("A11", "Matricule fiscal: {{client.vatin}}")

	// Line item table header. Product (SKU) and Description (name) are two
	// separate columns, mirroring the web app's line-items table — see
	// invoice.go's matching comment for why. Quantity, Unit Price, Line
	// Total (C-E); the marker lives in column F, off to the right of the
	// visible table.
	headerRow := 13
	cols := []string{"A", "B", "C", "D", "E"}
	labels := []string{"Product", "Description", "Quantity", "Unit Price", "Line Total"}
	for i, col := range cols {
		cell := col + strconv.Itoa(headerRow)
		set(cell, labels[i])
		f.SetCellStyle(sheet, cell, cell, darkHeader)
	}

	// Repeat row: A the SKU, B the description, C-E the rest of the
	// per-item placeholders, F the marker.
	repeatRow := headerRow + 1
	set("A"+strconv.Itoa(repeatRow), "{{lineItems.sku}}")
	set("B"+strconv.Itoa(repeatRow), "{{lineItems.description}}")
	set("C"+strconv.Itoa(repeatRow), "{{lineItems.quantity}}")
	set("D"+strconv.Itoa(repeatRow), "{{lineItems.unitPrice}}")
	set("E"+strconv.Itoa(repeatRow), "{{lineItems.lineTotal}}")
	set("F"+strconv.Itoa(repeatRow), "{{#lineItems}}")
	// Quantity/Unit Price/Line Total are numeric — right-align on the repeat
	// row so DuplicateRowTo carries it to every expanded row.
	for _, col := range []string{"C", "D", "E"} {
		cell := col + strconv.Itoa(repeatRow)
		f.SetCellStyle(sheet, cell, cell, styles.right)
	}

	// Totals block, a few rows below the repeat row — located by the fill
	// engine via placeholder scan after row expansion, never a cached index.
	totalsRow := repeatRow + 3
	set("D"+strconv.Itoa(totalsRow), "Subtotal")
	f.SetCellStyle(sheet, "D"+strconv.Itoa(totalsRow), "D"+strconv.Itoa(totalsRow), styles.bold)
	set("E"+strconv.Itoa(totalsRow), "{{order.subTotal}}")
	f.SetCellStyle(sheet, "E"+strconv.Itoa(totalsRow), "E"+strconv.Itoa(totalsRow), styles.right)
	set("D"+strconv.Itoa(totalsRow+1), "Tax")
	f.SetCellStyle(sheet, "D"+strconv.Itoa(totalsRow+1), "D"+strconv.Itoa(totalsRow+1), styles.bold)
	set("E"+strconv.Itoa(totalsRow+1), "{{order.taxTotal}}")
	f.SetCellStyle(sheet, "E"+strconv.Itoa(totalsRow+1), "E"+strconv.Itoa(totalsRow+1), styles.right)
	set("D"+strconv.Itoa(totalsRow+2), "Total")
	f.SetCellStyle(sheet, "D"+strconv.Itoa(totalsRow+2), "D"+strconv.Itoa(totalsRow+2), styles.bold)
	set("E"+strconv.Itoa(totalsRow+2), "{{order.total}}")
	f.SetCellStyle(sheet, "E"+strconv.Itoa(totalsRow+2), "E"+strconv.Itoa(totalsRow+2), styles.right)

	footerRow := totalsRow + 5
	set("A"+strconv.Itoa(footerRow), "Shipping address: {{order.shippingAddress}}")
	set("A"+strconv.Itoa(footerRow+1), "Notes: {{order.notes}}")

	// A (Product/SKU) is 22, not the original 16 -- a longer SKU (e.g.
	// "WIDGET-XL-DELUXE") overflowed visibly into Description at 16, found
	// by actually rendering a PDF with a long SKU rather than the short
	// ones every other manual check happened to use.
	f.SetColWidth(sheet, "A", "A", 22)
	f.SetColWidth(sheet, "B", "B", 40)
	f.SetColWidth(sheet, "C", "D", 14)
	f.SetColWidth(sheet, "E", "E", 16)

	applyFitToPageWidth(f, sheet)
	applyRepeatingHeaderRows(f, sheet, headerRow)
	applyPageFooter(f, sheet)
	addAvailableFieldsSheet(f, orderFieldRefs)
	finalizeWorkbook(f, sheet, "db/templates/gen/order_default.xlsx")
}
