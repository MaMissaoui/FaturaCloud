package main

import (
	"strconv"

	"github.com/xuri/excelize/v2"
)

// buildInboundDeliveryTemplate generates
// db/templates/inbound_delivery_default.xlsx, the embedded default for
// goods receipts. Like purchase_order.go, the vendor is who this document
// came from — vendorId is nullable, same precedent. Unlike purchase orders,
// there is no tax rate and no aggregate totals block
// (inbound_delivery_line_items.unitCost feeds average cost, not a taxed
// customer price — see db/xlsx_export_inbound_delivery.go), so the item
// table carries unitCost/lineTotal per line but nothing is summed at the
// bottom.
func buildInboundDeliveryTemplate() {
	f := excelize.NewFile()
	sheet := "Goods Receipt"
	f.SetSheetName(f.GetSheetName(0), sheet)
	styles := newTemplateStyles(f)

	set := func(cell, value string) { _ = f.SetCellStr(sheet, cell, value) }

	// Vendor block (top-left) — who delivered this.
	set("A1", "{{vendor.name}}")
	f.SetCellStyle(sheet, "A1", "A1", styles.bold)
	set("A2", "{{vendor.street}} {{vendor.houseNumber}}")
	set("A3", "{{vendor.postalCode}} {{vendor.city}}")
	set("A4", "VAT: {{vendor.vatin}}")
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
	set("A11", "VAT: {{organization.vatin}}")

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
		f.SetCellStyle(sheet, cell, cell, styles.header)
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

	f.SetColWidth(sheet, "A", "A", 16)
	f.SetColWidth(sheet, "B", "B", 40)
	f.SetColWidth(sheet, "C", "E", 14)

	applyFitToPageWidth(f, sheet)
	applyRepeatingHeaderRows(f, sheet, headerRow)
	applyPageFooter(f, sheet)
	addAvailableFieldsSheet(f, inboundDeliveryFieldRefs)
	finalizeWorkbook(f, sheet, "db/templates/gen/inbound_delivery_default.xlsx")
}

// inboundDeliveryFieldRefs groups every placeholder
// db.buildInboundDeliveryScalarPlaceholders / db.buildInboundDeliveryLineItemPlaceholders
// (db/xlsx_export_inbound_delivery.go) resolve.
var inboundDeliveryFieldRefs = []fieldRef{
	{"Header", "{{inboundDelivery.number}}", "Receipt number"},
	{"Header", "{{inboundDelivery.date}}", "Receipt date"},
	{"Header", "{{inboundDelivery.currency}}", "Currency code (e.g. EUR)"},
	{"Header", "{{inboundDelivery.purchaseOrder}}", "Linked purchase order number, blank if unlinked"},
	{"Header", "{{inboundDelivery.vendorDeliveryNote}}", "Number on the vendor's own paperwork, blank if unset"},
	{"Header", "{{inboundDelivery.trackingNumber}}", "Carrier tracking number, blank if unset"},
	{"Header", "{{vendor.name}}", "Vendor company name, blank if unset"},
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

	{"Item lines", "{{#lineItems}}", "Marker (not a value) — place alone in any one cell of the row to repeat once per line item; that whole cell is blanked in the output"},
	{"Item lines", "{{lineItems.sku}}", "Linked product's SKU, blank on a free-text line or an unset SKU"},
	{"Item lines", "{{lineItems.description}}", "Line item description"},
	{"Item lines", "{{lineItems.quantity}}", "Quantity received"},
	{"Item lines", "{{lineItems.unit}}", "Unit of measure (e.g. pcs, kg), blank if unset"},
	{"Item lines", "{{lineItems.unitCost}}", "Unit cost, formatted with currency — feeds average cost, not a customer price"},
	{"Item lines", "{{lineItems.lineTotal}}", "Quantity x unit cost, formatted with currency"},

	{"Footer", "{{inboundDelivery.notes}}", "Free-text notes, blank if unset"},

	// Export info — when this specific file was generated, not any document
	// field. See mergeExportMetaPlaceholders (db/xlsx_export.go).
	{"Export info", "{{export.generatedDate}}", "Date this file was exported (not the receipt's own date), in the organization's date format"},
	{"Export info", "{{export.generatedTime}}", "Time this file was exported, 24-hour HH:MM, server time"},
	{"Export info", "&P (page number) / &N (total pages)", "Native Excel/LibreOffice codes, not a {{}} placeholder — only work inside this sheet's own Page Layout ▸ Header/Footer, never in a regular cell, since the page count isn't known until export. The default footer already shows \"Page &P of &N\" on every page; edit it in Excel/LibreOffice's own header/footer editor to move or restyle it"},
}
