package main

import (
	"log"
	"strconv"

	"github.com/xuri/excelize/v2"
)

// buildDeliveryTemplate generates db/templates/delivery_default.xlsx, the
// embedded default for outbound deliveries (delivery notes). Unlike every
// other document type, outbound_delivery_line_items has no price columns at
// all — a delivery note never shows prices — so this template has no totals
// block, no currency, and a 3-column item table (Description/Quantity/Unit)
// instead of the usual 5-6.
func buildDeliveryTemplate() {
	f := excelize.NewFile()
	sheet := "Delivery Note"
	f.SetSheetName(f.GetSheetName(0), sheet)
	styles := newTemplateStyles(f)

	set := func(cell, value string) { _ = f.SetCellStr(sheet, cell, value) }

	// Seller block (top-left) — the organization shipping the goods.
	set("A1", "{{organization.name}}")
	f.SetCellStyle(sheet, "A1", "A1", styles.bold)
	set("A2", "{{organization.street}} {{organization.houseNumber}}")
	set("A3", "{{organization.postalCode}} {{organization.city}}")
	set("A4", "VAT: {{organization.vatin}}")
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

	// Line item table header. Description spans A:C (merged, extra room since
	// there's no price column competing for space); the {{#lineItems}} marker
	// on the repeat row lives in column E, off to the right.
	headerRow := 13
	if err := f.MergeCell(sheet, "A"+strconv.Itoa(headerRow), "C"+strconv.Itoa(headerRow)); err != nil {
		log.Fatal(err)
	}
	set("A"+strconv.Itoa(headerRow), "Description")
	f.SetCellStyle(sheet, "A"+strconv.Itoa(headerRow), "C"+strconv.Itoa(headerRow), styles.header)
	set("D"+strconv.Itoa(headerRow), "Quantity")
	f.SetCellStyle(sheet, "D"+strconv.Itoa(headerRow), "D"+strconv.Itoa(headerRow), styles.header)

	// Repeat row: A:C merged carries the description, D the quantity+unit, E the marker.
	repeatRow := headerRow + 1
	if err := f.MergeCell(sheet, "A"+strconv.Itoa(repeatRow), "C"+strconv.Itoa(repeatRow)); err != nil {
		log.Fatal(err)
	}
	set("A"+strconv.Itoa(repeatRow), "{{lineItems.description}}")
	set("D"+strconv.Itoa(repeatRow), "{{lineItems.quantity}} {{lineItems.unit}}")
	set("E"+strconv.Itoa(repeatRow), "{{#lineItems}}")

	footerRow := repeatRow + 4
	set("A"+strconv.Itoa(footerRow), "Notes: {{delivery.notes}}")

	f.SetColWidth(sheet, "A", "C", 20)
	f.SetColWidth(sheet, "D", "D", 16)

	applyFitToPageWidth(f, sheet)
	addAvailableFieldsSheet(f, deliveryFieldRefs)
	finalizeWorkbook(f, sheet, "db/templates/gen/delivery_default.xlsx")
}

// deliveryFieldRefs groups every placeholder db.buildDeliveryScalarPlaceholders
// / db.buildDeliveryLineItemPlaceholders (db/xlsx_export_delivery.go) resolve.
var deliveryFieldRefs = []fieldRef{
	{"Header", "{{delivery.number}}", "Delivery number"},
	{"Header", "{{delivery.date}}", "Delivery date"},
	{"Header", "{{delivery.orderNumber}}", "Linked sales order number, blank if standalone"},
	{"Header", "{{delivery.shippingAddress}}", "Free-text shipping address, blank if unset"},
	{"Header", "{{delivery.trackingNumber}}", "Carrier tracking number, blank if unset"},
	{"Header", "{{organization.name}}", "Seller company name"},
	{"Header", "{{organization.vatin}}", "Seller VAT number"},
	{"Header", "{{organization.email}}", "Seller email"},
	{"Header", "{{organization.phone}}", "Seller phone"},
	{"Header", "{{organization.website}}", "Seller website"},
	{"Header", "{{organization.street}}", "Seller street"},
	{"Header", "{{organization.houseNumber}}", "Seller house number"},
	{"Header", "{{organization.postalCode}}", "Seller postal code"},
	{"Header", "{{organization.city}}", "Seller city"},
	{"Header", "{{client.name}}", "Customer name, blank on a walk-in/no-client delivery"},
	{"Header", "{{client.vatin}}", "Customer VAT number"},
	{"Header", "{{client.email}}", "Customer email"},
	{"Header", "{{client.phone}}", "Customer phone"},
	{"Header", "{{client.street}}", "Customer street"},
	{"Header", "{{client.houseNumber}}", "Customer house number"},
	{"Header", "{{client.postalCode}}", "Customer postal code"},
	{"Header", "{{client.city}}", "Customer city"},

	{"Item lines", "{{#lineItems}}", "Marker (not a value) — place alone in any one cell of the row to repeat once per line item; that whole cell is blanked in the output"},
	{"Item lines", "{{lineItems.description}}", "Line item description"},
	{"Item lines", "{{lineItems.quantity}}", "Quantity"},
	{"Item lines", "{{lineItems.unit}}", "Unit of measure (e.g. pcs, kg), blank if unset"},

	{"Footer", "{{delivery.notes}}", "Free-text notes, blank if unset"},
}
