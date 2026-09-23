package main

// buildInboundDeliveryTunisiaTemplate generates
// db/templates/inbound_delivery_tunisia.xlsx via the shared Tunisian layout.
// A goods receipt (Bon de réception) is an internal document — it shows each
// line's unit cost and line total (the received value) but, like a delivery
// note, has no tax rates and no aggregate totals block.
func buildInboundDeliveryTunisiaTemplate() {
	f := buildTunisiaLayout(tunisiaLayoutSpec{
		sheet:     "Bon de réception",
		title:     "Bon de réception N°: {{inboundDelivery.number}}",
		date:      "Date : {{inboundDelivery.date}}",
		partyName: "{{vendor.name}}",
		partyFields: []string{
			"{{vendor.street}} {{vendor.houseNumber}}",
			"{{vendor.postalCode}} {{vendor.city}}",
			"Tel: {{vendor.phone}}",
			"MF: {{vendor.vatin}}",
		},
		columns: []tableColumn{
			{"Code", "{{lineItems.sku}}", false},
			{"Désignation", "{{lineItems.description}}", false},
			{"Quantité", "{{lineItems.quantity}}", true},
			{"Unité", "{{lineItems.unit}}", false},
			{"Coût U.", "{{lineItems.unitCost}}", true},
			{"TOTAL", "{{lineItems.lineTotal}}", true},
		},
		footerLeft:  "IBAN : {{organization.iban}}",
		footerRight: "{{organization.bankName}}",
	})
	addAvailableFieldsSheet(f, inboundDeliveryFieldRefs)
	finalizeWorkbook(f, "Bon de réception", "db/templates/gen/inbound_delivery_tunisia.xlsx")
}

// inboundDeliveryFieldRefs groups every placeholder
// db.buildInboundDeliveryScalarPlaceholders /
// db.buildInboundDeliveryLineItemPlaceholders (db/xlsx_export_inbound_delivery.go)
// resolve.
var inboundDeliveryFieldRefs = []fieldRef{
	{"Header", "{{organization.logo}}", "Buyer logo — place alone in a cell to anchor the uploaded image there"},
	{"Header", "{{inboundDelivery.number}}", "Goods receipt number"},
	{"Header", "{{inboundDelivery.date}}", "Receipt date"},
	{"Header", "{{inboundDelivery.currency}}", "Currency code (e.g. EUR)"},
	{"Header", "{{inboundDelivery.purchaseOrder}}", "Linked purchase order number, blank if unset"},
	{"Header", "{{inboundDelivery.vendorDeliveryNote}}", "Vendor's delivery note number, blank if unset"},
	{"Header", "{{inboundDelivery.trackingNumber}}", "Tracking number, blank if unset"},
	{"Header", "{{organization.name}}", "Buyer company name"},
	{"Header", "{{organization.vatin}}", "Buyer VAT number"},
	{"Header", "{{organization.email}}", "Buyer email"},
	{"Header", "{{organization.phone}}", "Buyer phone"},
	{"Header", "{{organization.website}}", "Buyer website"},
	{"Header", "{{organization.street}}", "Buyer street"},
	{"Header", "{{organization.houseNumber}}", "Buyer house number"},
	{"Header", "{{organization.postalCode}}", "Buyer postal code"},
	{"Header", "{{organization.city}}", "Buyer city"},
	{"Header", "{{organization.iban}}", "Buyer IBAN"},
	{"Header", "{{organization.bankName}}", "Buyer bank name"},
	{"Header", "{{vendor.name}}", "Vendor name"},
	{"Header", "{{vendor.vatin}}", "Vendor VAT number"},
	{"Header", "{{vendor.email}}", "Vendor email"},
	{"Header", "{{vendor.phone}}", "Vendor phone"},
	{"Header", "{{vendor.street}}", "Vendor street"},
	{"Header", "{{vendor.houseNumber}}", "Vendor house number"},
	{"Header", "{{vendor.postalCode}}", "Vendor postal code"},
	{"Header", "{{vendor.city}}", "Vendor city"},

	{"Item lines", "{{#lineItems}}", "Marker (not a value) — place alone in one cell of the row to repeat once per line item"},
	{"Item lines", "{{lineItems.sku}}", "Linked product's SKU"},
	{"Item lines", "{{lineItems.description}}", "Line item description"},
	{"Item lines", "{{lineItems.quantity}}", "Quantity"},
	{"Item lines", "{{lineItems.unit}}", "Unit of measure, blank if unset"},
	{"Item lines", "{{lineItems.unitCost}}", "Unit cost, formatted with currency"},
	{"Item lines", "{{lineItems.lineTotal}}", "Quantity x unit cost, formatted with currency"},

	{"Footer", "{{inboundDelivery.notes}}", "Free-text notes, blank if unset"},

	{"Export info", "{{export.generatedDate}}", "Date this file was exported, in the organization's date format"},
	{"Export info", "{{export.generatedTime}}", "Time this file was exported, 24-hour HH:MM, server time"},
	{"Export info", "&P (page number) / &N (total pages)", "Native Excel/LibreOffice codes, only work in Page Layout ▸ Header/Footer"},
}
