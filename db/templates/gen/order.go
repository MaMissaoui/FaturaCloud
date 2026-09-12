package main

import (
	"strconv"

	"github.com/xuri/excelize/v2"
)

// buildOrderTemplate generates db/templates/order_default.xlsx, following
// invoice.go/purchase_order.go's layout conventions. A sales order carries
// prices (it's sent to the client, unlike an outbound delivery note) but,
// like purchase orders, has no server-validated stored totals — subtotal/
// tax/total are computed at export time (db/xlsx_export_order.go). Unlike
// purchase orders it has no per-line Unit or Tax Rate column at all
// (OrderLineItem has neither), so the item table is only 4 columns wide.
func buildOrderTemplate() {
	f := excelize.NewFile()
	sheet := "Order"
	f.SetSheetName(f.GetSheetName(0), sheet)
	styles := newTemplateStyles(f)

	set := func(cell, value string) { _ = f.SetCellStr(sheet, cell, value) }

	// Seller block (top-left) — the organization the order confirms with.
	set("A1", "{{organization.name}}")
	f.SetCellStyle(sheet, "A1", "A1", styles.bold)
	set("A2", "{{organization.street}} {{organization.houseNumber}}")
	set("A3", "{{organization.postalCode}} {{organization.city}}")
	set("A4", "VAT: {{organization.vatin}}")
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
	set("A11", "VAT: {{client.vatin}}")

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
		f.SetCellStyle(sheet, cell, cell, styles.header)
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

	f.SetColWidth(sheet, "A", "A", 16)
	f.SetColWidth(sheet, "B", "B", 40)
	f.SetColWidth(sheet, "C", "D", 14)
	f.SetColWidth(sheet, "E", "E", 16)

	applyFitToPageWidth(f, sheet)
	applyRepeatingHeaderRows(f, sheet, headerRow)
	applyPageFooter(f, sheet)
	addAvailableFieldsSheet(f, orderFieldRefs)
	finalizeWorkbook(f, sheet, "db/templates/gen/order_default.xlsx")
}

// orderFieldRefs groups every placeholder db.buildOrderScalarPlaceholders /
// db.buildOrderLineItemPlaceholders (db/xlsx_export_order.go) resolve, the
// same Header / Item lines / Footer grouping as invoiceFieldRefs.
var orderFieldRefs = []fieldRef{
	// Header — seller, order metadata, and customer info.
	{"Header", "{{order.number}}", "Order number"},
	{"Header", "{{order.orderDate}}", "Order date"},
	{"Header", "{{order.deliveryDate}}", "Expected delivery date, blank if unset"},
	{"Header", "{{order.currency}}", "Currency code (e.g. EUR)"},
	{"Header", "{{order.trackingNumber}}", "Shipment tracking number, blank if unset"},
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
	{"Item lines", "{{lineItems.sku}}", "Linked product's SKU, blank on a free-text line or an unset SKU"},
	{"Item lines", "{{lineItems.description}}", "Line item description"},
	{"Item lines", "{{lineItems.quantity}}", "Quantity"},
	{"Item lines", "{{lineItems.unitPrice}}", "Unit price, formatted with currency"},
	{"Item lines", "{{lineItems.lineTotal}}", "Quantity x unit price, formatted with currency"},

	// Footer — computed totals (no stored total columns on an order, unlike
	// invoices) and shipping/notes.
	{"Footer", "{{order.subTotal}}", "Subtotal before tax, computed from line items"},
	{"Footer", "{{order.taxTotal}}", "Total tax — always 0.00, order line items carry no tax rate"},
	{"Footer", "{{order.total}}", "Grand total, computed from line items"},
	{"Footer", "{{order.shippingAddress}}", "Free-text shipping address, blank if unset"},
	{"Footer", "{{order.notes}}", "Free-text notes, blank if unset"},

	// Export info — when this specific file was generated, not any document
	// field. See mergeExportMetaPlaceholders (db/xlsx_export.go).
	{"Export info", "{{export.generatedDate}}", "Date this file was exported (not the order's own date), in the organization's date format"},
	{"Export info", "{{export.generatedTime}}", "Time this file was exported, 24-hour HH:MM, server time"},
	{"Export info", "&P (page number) / &N (total pages)", "Native Excel/LibreOffice codes, not a {{}} placeholder — only work inside this sheet's own Page Layout ▸ Header/Footer, never in a regular cell, since the page count isn't known until export. The default footer already shows \"Page &P of &N\" on every page; edit it in Excel/LibreOffice's own header/footer editor to move or restyle it"},
}
