package main

import (
	"log"
	"strconv"

	"github.com/xuri/excelize/v2"
)

// buildDeliveryDefaultTemplate generates db/templates/delivery_default.xlsx, the
// embedded default for outbound deliveries (delivery notes). Unlike every
// other document type, outbound_delivery_line_items has no price columns at
// all — a delivery note never shows prices — so this template has no totals
// block, no currency, and a 3-column item table (Description/Quantity/Unit)
// instead of the usual 5-6.
func buildDeliveryDefaultTemplate() {
	f := excelize.NewFile()
	sheet := "Delivery Note"
	f.SetSheetName(f.GetSheetName(0), sheet)
	styles := newTemplateStyles(f)
	darkHeader := newDarkHeaderStyle(f)

	set := func(cell, value string) { _ = f.SetCellStr(sheet, cell, value) }

	// Seller block (top-left) — the organization shipping the goods.
	set("A1", "{{organization.name}}")
	f.SetCellStyle(sheet, "A1", "A1", styles.bold)
	set("A2", "{{organization.street}} {{organization.houseNumber}}")
	set("A3", "{{organization.postalCode}} {{organization.city}}")
	set("A4", "Matricule fiscal: {{organization.vatin}}")
	set("A5", "{{organization.email}} | {{organization.phone}}")

	// Document title + metadata (top-right).
	set("E1", "DELIVERY NOTE")
	f.SetCellStyle(sheet, "E1", "E1", styles.title)
	set("E2", "Delivery number: {{delivery.number}}")
	set("E3", "Delivery date: {{delivery.date}}")
	set("E4", "Order: {{delivery.orderNumber}}")
	set("E5", "Tracking number: {{delivery.trackingNumber}}")

	// Buyer block.
	set("A7", "Deliver To")
	f.SetCellStyle(sheet, "A7", "A7", styles.bold)
	set("A8", "{{client.name}}")
	set("A9", "{{client.street}} {{client.houseNumber}}")
	set("A10", "{{client.postalCode}} {{client.city}}")
	set("A11", "Shipping address: {{delivery.shippingAddress}}")

	// Line item table header. Description spans A:B (merged, the same
	// invoice-style layout as every other template) with a dedicated SKU
	// column at C; the {{#lineItems}} marker on the repeat row lives in
	// column E, off to the right.
	headerRow := 13
	if err := f.MergeCell(sheet, "A"+strconv.Itoa(headerRow), "B"+strconv.Itoa(headerRow)); err != nil {
		log.Fatal(err)
	}
	cols := []string{"A", "C", "D"}
	labels := []string{"Description", "Product", "Quantity"}
	for i, col := range cols {
		cell := col + strconv.Itoa(headerRow)
		set(cell, labels[i])
		f.SetCellStyle(sheet, cell, cell, darkHeader)
	}
	f.SetCellStyle(sheet, "B"+strconv.Itoa(headerRow), "B"+strconv.Itoa(headerRow), darkHeader)

	// Repeat row: A:B merged carries the description, C the SKU, D the
	// quantity+unit, E the marker.
	repeatRow := headerRow + 1
	if err := f.MergeCell(sheet, "A"+strconv.Itoa(repeatRow), "B"+strconv.Itoa(repeatRow)); err != nil {
		log.Fatal(err)
	}
	set("A"+strconv.Itoa(repeatRow), "{{lineItems.description}}")
	set("C"+strconv.Itoa(repeatRow), "{{lineItems.sku}}")
	set("D"+strconv.Itoa(repeatRow), "{{lineItems.quantity}} {{lineItems.unit}}")
	set("E"+strconv.Itoa(repeatRow), "{{#lineItems}}")
	// Quantity is numeric — right-align on the repeat row so DuplicateRowTo
	// carries it to every expanded row. SKU stays left (an identifier/text).
	f.SetCellStyle(sheet, "D"+strconv.Itoa(repeatRow), "D"+strconv.Itoa(repeatRow), styles.right)

	footerRow := repeatRow + 4
	set("A"+strconv.Itoa(footerRow), "Notes: {{delivery.notes}}")

	f.SetColWidth(sheet, "A", "A", 24)
	f.SetColWidth(sheet, "B", "B", 16)
	// C (Product/SKU) is 22, not the original 16 -- see invoice.go's
	// matching comment for why.
	f.SetColWidth(sheet, "C", "C", 22)
	f.SetColWidth(sheet, "D", "D", 16)

	applyFitToPageWidth(f, sheet)
	applyRepeatingHeaderRows(f, sheet, headerRow)
	applyPageFooter(f, sheet)
	addAvailableFieldsSheet(f, deliveryFieldRefs)
	finalizeWorkbook(f, sheet, "db/templates/gen/delivery_default.xlsx")
}
