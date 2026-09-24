package main

import (
	"strconv"

	"github.com/xuri/excelize/v2"
)

// buildPurchaseOrderDefaultTemplate generates db/templates/purchase_order_default.xlsx,
// following invoice.go's layout conventions (seller/buyer blocks, merged
// description column, marker off to the right, "Available fields" reference
// sheet) so every document type in this app looks like one designed system.
// Unlike invoices, a purchase order has no server-validated stored totals
// (db.FillPurchaseOrderTemplate computes subtotal/tax/total from line items
// at export time — see db/xlsx_export_purchase_order.go) and adds a per-line
// Unit column (db.PurchaseOrderLineItem.Unit has no invoice-line
// equivalent), so the item table is 6 columns wide instead of 5.
func buildPurchaseOrderDefaultTemplate() {
	f := excelize.NewFile()
	sheet := "Purchase Order"
	f.SetSheetName(f.GetSheetName(0), sheet)
	styles := newTemplateStyles(f)
	darkHeader := newDarkHeaderStyle(f)

	set := func(cell, value string) { _ = f.SetCellStr(sheet, cell, value) }

	// Buyer block (top-left) — the organization issuing the order.
	set("A1", "{{organization.name}}")
	f.SetCellStyle(sheet, "A1", "A1", styles.bold)
	set("A2", "{{organization.street}} {{organization.houseNumber}}")
	set("A3", "{{organization.postalCode}} {{organization.city}}")
	set("A4", "Matricule fiscal: {{organization.vatin}}")
	set("A5", "{{organization.email}} | {{organization.phone}}")

	// Document title + metadata (top-right).
	set("E1", "PURCHASE ORDER")
	f.SetCellStyle(sheet, "E1", "E1", styles.title)
	set("E2", "Order number: {{purchaseOrder.number}}")
	set("E3", "Order date: {{purchaseOrder.orderDate}}")
	set("E4", "Expected date: {{purchaseOrder.expectedDate}}")
	set("E5", "Deliver to: {{purchaseOrder.deliveryAddress}}")

	// Vendor block — who the order is sent to.
	set("A7", "Vendor")
	f.SetCellStyle(sheet, "A7", "A7", styles.bold)
	set("A8", "{{vendor.name}}")
	set("A9", "{{vendor.street}} {{vendor.houseNumber}}")
	set("A10", "{{vendor.postalCode}} {{vendor.city}}")
	set("A11", "Matricule fiscal: {{vendor.vatin}}")

	// Line item table header. Product (SKU) and Description (name) are two
	// separate columns, mirroring the web app's line-items table — see
	// invoice.go's matching comment for why. The marker lives in column H,
	// off to the right of the visible 7-column table (db.findMarkerRow
	// scans every cell, not just column A).
	headerRow := 13
	cols := []string{"A", "B", "C", "D", "E", "F", "G"}
	labels := []string{"Product", "Description", "Quantity", "Unit", "Unit Price", "Tax Rate", "Line Total"}
	for i, col := range cols {
		cell := col + strconv.Itoa(headerRow)
		set(cell, labels[i])
		f.SetCellStyle(sheet, cell, cell, darkHeader)
	}

	// Repeat row: A the SKU, B the description, C-G the rest of the
	// per-item placeholders, H the marker.
	repeatRow := headerRow + 1
	set("A"+strconv.Itoa(repeatRow), "{{lineItems.sku}}")
	set("B"+strconv.Itoa(repeatRow), "{{lineItems.description}}")
	set("C"+strconv.Itoa(repeatRow), "{{lineItems.quantity}}")
	set("D"+strconv.Itoa(repeatRow), "{{lineItems.unit}}")
	set("E"+strconv.Itoa(repeatRow), "{{lineItems.unitPrice}}")
	set("F"+strconv.Itoa(repeatRow), "{{lineItems.taxRate}}")
	set("G"+strconv.Itoa(repeatRow), "{{lineItems.lineTotal}}")
	set("H"+strconv.Itoa(repeatRow), "{{#lineItems}}")
	// Quantity/Unit Price/Tax Rate/Line Total are numeric — right-align on
	// the repeat row so DuplicateRowTo carries it to every expanded row.
	// Description and Unit (a uom label like "pcs") stay left as text.
	for _, col := range []string{"C", "E", "F", "G"} {
		cell := col + strconv.Itoa(repeatRow)
		f.SetCellStyle(sheet, cell, cell, styles.right)
	}

	// Totals block, a few rows below the repeat row — located by the fill
	// engine via placeholder scan after row expansion, never a cached index.
	totalsRow := repeatRow + 3
	set("F"+strconv.Itoa(totalsRow), "Subtotal")
	f.SetCellStyle(sheet, "F"+strconv.Itoa(totalsRow), "F"+strconv.Itoa(totalsRow), styles.bold)
	set("G"+strconv.Itoa(totalsRow), "{{purchaseOrder.subTotal}}")
	f.SetCellStyle(sheet, "G"+strconv.Itoa(totalsRow), "G"+strconv.Itoa(totalsRow), styles.right)
	set("F"+strconv.Itoa(totalsRow+1), "Tax")
	f.SetCellStyle(sheet, "F"+strconv.Itoa(totalsRow+1), "F"+strconv.Itoa(totalsRow+1), styles.bold)
	set("G"+strconv.Itoa(totalsRow+1), "{{purchaseOrder.taxTotal}}")
	f.SetCellStyle(sheet, "G"+strconv.Itoa(totalsRow+1), "G"+strconv.Itoa(totalsRow+1), styles.right)
	set("F"+strconv.Itoa(totalsRow+2), "Total")
	f.SetCellStyle(sheet, "F"+strconv.Itoa(totalsRow+2), "F"+strconv.Itoa(totalsRow+2), styles.bold)
	set("G"+strconv.Itoa(totalsRow+2), "{{purchaseOrder.total}}")
	f.SetCellStyle(sheet, "G"+strconv.Itoa(totalsRow+2), "G"+strconv.Itoa(totalsRow+2), styles.right)

	footerRow := totalsRow + 5
	set("A"+strconv.Itoa(footerRow), "Notes: {{purchaseOrder.notes}}")

	// A (Product/SKU) is 22, not the original 16 -- a longer SKU (e.g.
	// "WIDGET-XL-DELUXE") overflowed visibly into Description at 16, found
	// by actually rendering a PDF with a long SKU rather than the short
	// ones every other manual check happened to use.
	f.SetColWidth(sheet, "A", "A", 22)
	f.SetColWidth(sheet, "B", "B", 32)
	f.SetColWidth(sheet, "C", "F", 13)
	f.SetColWidth(sheet, "G", "G", 16)

	applyFitToPageWidth(f, sheet)
	applyRepeatingHeaderRows(f, sheet, headerRow)
	applyPageFooter(f, sheet)
	addAvailableFieldsSheet(f, purchaseOrderFieldRefs)
	finalizeWorkbook(f, sheet, "db/templates/gen/purchase_order_default.xlsx")
}
