package main

// buildDeliveryTemplate generates db/templates/delivery_default.xlsx via the
// shared Tunisian layout. A delivery note (Bon de livraison) never shows
// prices, so it has no VAT recap and no totals block — a pure line-item list.
func buildDeliveryTemplate() {
	f := buildTunisiaLayout(tunisiaLayoutSpec{
		sheet:     "Bon de livraison",
		title:     "Bon de livraison N°: {{delivery.number}}",
		date:      "Date : {{delivery.date}}",
		partyName: "{{client.name}}",
		partyFields: []string{
			"{{client.street}} {{client.houseNumber}}",
			"{{client.postalCode}} {{client.city}}",
			"Tel: {{client.phone}}",
			"CIN: {{client.identityNumber}}",
		},
		columns: []tableColumn{
			{"Code", "{{lineItems.sku}}", false},
			{"Désignation", "{{lineItems.description}}", false},
			{"Quantité", "{{lineItems.quantity}}", true},
			{"Unité", "{{lineItems.unit}}", false},
		},
		footerLeft:  "IBAN : {{organization.iban}}",
		footerRight: "{{organization.bankName}}",
	})
	addAvailableFieldsSheet(f, deliveryFieldRefs)
	finalizeWorkbook(f, "Bon de livraison", "db/templates/gen/delivery_default.xlsx")
}

// deliveryFieldRefs groups every placeholder
// db.buildDeliveryScalarPlaceholders / db.buildDeliveryLineItemPlaceholders
// (db/xlsx_export_delivery.go) resolve.
var deliveryFieldRefs = []fieldRef{
	{"Header", "{{organization.logo}}", "Seller logo — place alone in a cell to anchor the uploaded image there"},
	{"Header", "{{delivery.number}}", "Delivery number"},
	{"Header", "{{delivery.date}}", "Delivery date"},
	{"Header", "{{delivery.orderNumber}}", "Linked order number, blank on a standalone delivery"},
	{"Header", "{{delivery.shippingAddress}}", "Free-text shipping address, blank if unset"},
	{"Header", "{{delivery.trackingNumber}}", "Tracking number, blank if unset"},
	{"Header", "{{organization.name}}", "Seller company name"},
	{"Header", "{{organization.vatin}}", "Seller VAT number"},
	{"Header", "{{organization.email}}", "Seller email"},
	{"Header", "{{organization.phone}}", "Seller phone"},
	{"Header", "{{organization.website}}", "Seller website"},
	{"Header", "{{organization.street}}", "Seller street"},
	{"Header", "{{organization.houseNumber}}", "Seller house number"},
	{"Header", "{{organization.postalCode}}", "Seller postal code"},
	{"Header", "{{organization.city}}", "Seller city"},
	{"Header", "{{organization.iban}}", "Seller IBAN"},
	{"Header", "{{organization.bankName}}", "Seller bank name"},
	{"Header", "{{client.name}}", "Customer name"},
	{"Header", "{{client.vatin}}", "Customer VAT number"},
	{"Header", "{{client.identityNumber}}", "Customer national ID / CIN number"},
	{"Header", "{{client.address}}", "Customer single-line address"},
	{"Header", "{{client.email}}", "Customer email"},
	{"Header", "{{client.phone}}", "Customer phone"},
	{"Header", "{{client.phone2}}", "Customer phone 2"},
	{"Header", "{{client.phone3}}", "Customer phone 3"},
	{"Header", "{{client.street}}", "Customer street"},
	{"Header", "{{client.houseNumber}}", "Customer house number"},
	{"Header", "{{client.postalCode}}", "Customer postal code"},
	{"Header", "{{client.city}}", "Customer city"},

	{"Item lines", "{{#lineItems}}", "Marker (not a value) — place alone in one cell of the row to repeat once per line item"},
	{"Item lines", "{{lineItems.sku}}", "Linked product's SKU"},
	{"Item lines", "{{lineItems.description}}", "Line item description"},
	{"Item lines", "{{lineItems.quantity}}", "Quantity"},
	{"Item lines", "{{lineItems.unit}}", "Unit of measure, blank if unset"},

	{"Footer", "{{delivery.notes}}", "Free-text notes, blank if unset"},

	{"Export info", "{{export.generatedDate}}", "Date this file was exported, in the organization's date format"},
	{"Export info", "{{export.generatedTime}}", "Time this file was exported, 24-hour HH:MM, server time"},
	{"Export info", "&P (page number) / &N (total pages)", "Native Excel/LibreOffice codes, only work in Page Layout ▸ Header/Footer"},
}
