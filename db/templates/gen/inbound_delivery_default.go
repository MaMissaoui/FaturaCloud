package main

import (
	"strconv"

	"github.com/xuri/excelize/v2"
)

// buildInboundDeliveryDefaultTemplate generates
// db/templates/inbound_delivery_default.xlsx, the embedded default for
// goods receipts. Like purchase_order.go, the vendor is who this document
// came from — vendorId is nullable, same precedent. Unlike purchase orders,
// there is no tax rate and no aggregate totals block
// (inbound_delivery_line_items.unitCost feeds average cost, not a taxed
// customer price — see db/xlsx_export_inbound_delivery.go), so the item
// table carries unitCost/lineTotal per line but nothing is summed at the
// bottom.
func buildInboundDeliveryDefaultTemplate() {
	f := excelize.NewFile()
	sheet := "Goods Receipt"
	f.SetSheetName(f.GetSheetName(0), sheet)
	styles := newTemplateStyles(f)
	darkHeader := newDarkHeaderStyle(f)

	set := func(cell, value string) { _ = f.SetCellStr(sheet, cell, value) }

	// Vendor block (top-left) — who delivered this.
	set("A1", "{{vendor.name}}")
	f.SetCellStyle(sheet, "A1", "A1", styles.bold)
	set("A2", "{{vendor.street}} {{vendor.houseNumber}}")
	set("A3", "{{vendor.postalCode}} {{vendor.city}}")
	set("A4", "Matricule fiscal: {{vendor.vatin}}")
	set("A5", "{{vendor.email}} | {{vendor.phone}}")

	// Document title + metadata (top-right).
	set("E1", "GOODS RECEIPT")
	f.SetCellStyle(sheet, "E1", "E1", styles.title)
	set("E2", "Receipt number: {{inboundDelivery.number}}")
	set("E3", "Receipt date: {{inboundDelivery.date}}")
	set("E4", "Purchase order: {{inboundDelivery.purchaseOrder}}")
	set("E5", "Vendor delivery note: {{inboundDelivery.vendorDeliveryNote}}")
	set("E6", "Tracking number: {{inboundDelivery.trackingNumber}}")

	// Buyer block — FaturaCloud's own organization, receiving the goods.
	set("A7", "Received By")
	f.SetCellStyle(sheet, "A7", "A7", styles.bold)
	set("A8", "{{organization.name}}")
	set("A9", "{{organization.street}} {{organization.houseNumber}}")
	set("A10", "{{organization.postalCode}} {{organization.city}}")
	set("A11", "Matricule fiscal: {{organization.vatin}}")

	// Line item table header. Product (SKU) and Description (name) are two
	// separate columns, mirroring the web app's line-items table — see
	// invoice.go's matching comment for why. The marker lives in column F,
	// off to the right of the visible 6-column table.
	headerRow := 13
	cols := []string{"A", "B", "C", "D", "E"}
	labels := []string{"Product", "Description", "Quantity", "Unit Cost", "Line Total"}
	for i, col := range cols {
		cell := col + strconv.Itoa(headerRow)
		set(cell, labels[i])
		f.SetCellStyle(sheet, cell, cell, darkHeader)
	}

	// Repeat row: A the SKU, B the description, C-E the rest.
	repeatRow := headerRow + 1
	set("A"+strconv.Itoa(repeatRow), "{{lineItems.sku}}")
	set("B"+strconv.Itoa(repeatRow), "{{lineItems.description}}")
	set("C"+strconv.Itoa(repeatRow), "{{lineItems.quantity}} {{lineItems.unit}}")
	set("D"+strconv.Itoa(repeatRow), "{{lineItems.unitCost}}")
	set("E"+strconv.Itoa(repeatRow), "{{lineItems.lineTotal}}")
	set("F"+strconv.Itoa(repeatRow), "{{#lineItems}}")
	// Quantity/Unit Cost/Line Total are numeric — right-align on the repeat
	// row so DuplicateRowTo carries it to every expanded row.
	for _, col := range []string{"C", "D", "E"} {
		cell := col + strconv.Itoa(repeatRow)
		f.SetCellStyle(sheet, cell, cell, styles.right)
	}

	footerRow := repeatRow + 4
	set("A"+strconv.Itoa(footerRow), "Notes: {{inboundDelivery.notes}}")

	// A (Product/SKU) is 22, not the original 16 -- a longer SKU (e.g.
	// "WIDGET-XL-DELUXE") overflowed visibly into Description at 16, found
	// by actually rendering a PDF with a long SKU rather than the short
	// ones every other manual check happened to use.
	f.SetColWidth(sheet, "A", "A", 22)
	f.SetColWidth(sheet, "B", "B", 40)
	f.SetColWidth(sheet, "C", "E", 14)

	applyFitToPageWidth(f, sheet)
	applyRepeatingHeaderRows(f, sheet, headerRow)
	applyPageFooter(f, sheet)
	addAvailableFieldsSheet(f, inboundDeliveryFieldRefs)
	finalizeWorkbook(f, sheet, "db/templates/gen/inbound_delivery_default.xlsx")
}
