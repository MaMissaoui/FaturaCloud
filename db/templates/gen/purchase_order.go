package main

import (
	"log"
	"strconv"

	"github.com/xuri/excelize/v2"
)

// buildPurchaseOrderTemplate generates db/templates/purchase_order_default.xlsx,
// following invoice.go's layout conventions (seller/buyer blocks, merged
// description column, marker off to the right, "Available fields" reference
// sheet) so every document type in this app looks like one designed system.
// Unlike invoices, a purchase order has no server-validated stored totals
// (db.FillPurchaseOrderTemplate computes subtotal/tax/total from line items
// at export time — see db/xlsx_export_purchase_order.go) and adds a per-line
// Unit column (db.PurchaseOrderLineItem.Unit has no invoice-line
// equivalent), so the item table is 6 columns wide instead of 5.
func buildPurchaseOrderTemplate() {
	f := excelize.NewFile()
	sheet := "Purchase Order"
	f.SetSheetName(f.GetSheetName(0), sheet)
	styles := newTemplateStyles(f)

	set := func(cell, value string) { _ = f.SetCellStr(sheet, cell, value) }

	// Buyer block (top-left) — the organization issuing the order.
	set("A1", "{{organization.name}}")
	f.SetCellStyle(sheet, "A1", "A1", styles.bold)
	set("A2", "{{organization.street}} {{organization.houseNumber}}")
	set("A3", "{{organization.postalCode}} {{organization.city}}")
	set("A4", "VAT: {{organization.vatin}}")
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
	set("A11", "VAT: {{vendor.vatin}}")

	// Line item table header. Description spans A:B (merged), same reasoning
	// as the invoice template; the marker lives in column H, off to the
	// right of the visible 6-column table (db.findMarkerRow scans every
	// cell, not just column A).
	headerRow := 13
	if err := f.MergeCell(sheet, "A"+strconv.Itoa(headerRow), "B"+strconv.Itoa(headerRow)); err != nil {
		log.Fatal(err)
	}
	cols := []string{"A", "C", "D", "E", "F", "G"}
	labels := []string{"Description", "Quantity", "Unit", "Unit Price", "Tax Rate", "Line Total"}
	for i, col := range cols {
		cell := col + strconv.Itoa(headerRow)
		set(cell, labels[i])
		f.SetCellStyle(sheet, cell, cell, styles.header)
	}
	f.SetCellStyle(sheet, "B"+strconv.Itoa(headerRow), "B"+strconv.Itoa(headerRow), styles.header)

	// Repeat row: A:B merged carries the description, C-G the rest of the
	// per-item placeholders, H the marker.
	repeatRow := headerRow + 1
	if err := f.MergeCell(sheet, "A"+strconv.Itoa(repeatRow), "B"+strconv.Itoa(repeatRow)); err != nil {
		log.Fatal(err)
	}
	set("A"+strconv.Itoa(repeatRow), "{{lineItems.description}}")
	set("C"+strconv.Itoa(repeatRow), "{{lineItems.quantity}}")
	set("D"+strconv.Itoa(repeatRow), "{{lineItems.unit}}")
	set("E"+strconv.Itoa(repeatRow), "{{lineItems.unitPrice}}")
	set("F"+strconv.Itoa(repeatRow), "{{lineItems.taxRate}}")
	set("G"+strconv.Itoa(repeatRow), "{{lineItems.lineTotal}}")
	set("H"+strconv.Itoa(repeatRow), "{{#lineItems}}")

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

	f.SetColWidth(sheet, "A", "A", 28)
	f.SetColWidth(sheet, "B", "B", 28)
	f.SetColWidth(sheet, "C", "F", 13)
	f.SetColWidth(sheet, "G", "G", 16)

	applyFitToPageWidth(f, sheet)
	addAvailableFieldsSheet(f, purchaseOrderFieldRefs)
	finalizeWorkbook(f, sheet, "db/templates/gen/purchase_order_default.xlsx")
}

// purchaseOrderFieldRefs groups every placeholder
// db.buildPurchaseOrderScalarPlaceholders / db.buildPurchaseOrderLineItemPlaceholders
// (db/xlsx_export_purchase_order.go) resolve, the same Header / Item lines /
// Footer grouping as invoiceFieldRefs.
var purchaseOrderFieldRefs = []fieldRef{
	// Header — buyer, order metadata, and vendor info.
	{"Header", "{{purchaseOrder.number}}", "Purchase order number"},
	{"Header", "{{purchaseOrder.orderDate}}", "Order date"},
	{"Header", "{{purchaseOrder.expectedDate}}", "Expected delivery date, blank if unset"},
	{"Header", "{{purchaseOrder.currency}}", "Currency code (e.g. EUR)"},
	{"Header", "{{purchaseOrder.deliveryAddress}}", "Free-text delivery address, blank if unset"},
	{"Header", "{{organization.name}}", "Buyer company name"},
	{"Header", "{{organization.vatin}}", "Buyer VAT number"},
	{"Header", "{{organization.email}}", "Buyer email"},
	{"Header", "{{organization.phone}}", "Buyer phone"},
	{"Header", "{{organization.website}}", "Buyer website"},
	{"Header", "{{organization.street}}", "Buyer street"},
	{"Header", "{{organization.houseNumber}}", "Buyer house number"},
	{"Header", "{{organization.postalCode}}", "Buyer postal code"},
	{"Header", "{{organization.city}}", "Buyer city"},
	{"Header", "{{vendor.name}}", "Vendor name"},
	{"Header", "{{vendor.vatin}}", "Vendor VAT number"},
	{"Header", "{{vendor.email}}", "Vendor email"},
	{"Header", "{{vendor.phone}}", "Vendor phone"},
	{"Header", "{{vendor.street}}", "Vendor street"},
	{"Header", "{{vendor.houseNumber}}", "Vendor house number"},
	{"Header", "{{vendor.postalCode}}", "Vendor postal code"},
	{"Header", "{{vendor.city}}", "Vendor city"},

	// Item lines — only meaningful inside the repeated {{#lineItems}} row.
	{"Item lines", "{{#lineItems}}", "Marker (not a value) — place alone in any one cell of the row to repeat once per line item; that whole cell is blanked in the output"},
	{"Item lines", "{{lineItems.description}}", "Line item description"},
	{"Item lines", "{{lineItems.quantity}}", "Quantity"},
	{"Item lines", "{{lineItems.unit}}", "Unit of measure (e.g. pcs, kg), blank if unset"},
	{"Item lines", "{{lineItems.unitPrice}}", "Unit price, formatted with currency"},
	{"Item lines", "{{lineItems.taxRate}}", "Tax rate percentage (e.g. 19.5%), blank if unset"},
	{"Item lines", "{{lineItems.lineTotal}}", "Quantity x unit price, formatted with currency"},

	// Footer — computed totals (no stored total columns on a purchase
	// order, unlike invoices — see db/xlsx_export_purchase_order.go) and notes.
	{"Footer", "{{purchaseOrder.subTotal}}", "Subtotal before tax, computed from line items"},
	{"Footer", "{{purchaseOrder.taxTotal}}", "Total tax, computed from line items"},
	{"Footer", "{{purchaseOrder.total}}", "Grand total, computed from line items"},
	{"Footer", "{{purchaseOrder.notes}}", "Free-text notes, blank if unset"},
}
