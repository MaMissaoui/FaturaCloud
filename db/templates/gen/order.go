package main

// buildOrderTunisiaTemplate generates db/templates/order_tunisia.xlsx via the shared
// Tunisian layout. An order is sales-side outward (a "Bon de commande" the
// customer places, or a confirmation), so the boxed block shows the client.
// Orders have no per-line tax rate at all (db.OrderLineItem), so the VAT recap
// is a single zero-tax row and Total TVA is always zero.
func buildOrderTunisiaTemplate() {
	f := buildTunisiaLayout(tunisiaLayoutSpec{
		sheet:     "Commande",
		title:     "Commande N°: {{order.number}}",
		date:      "Date : {{order.orderDate}}",
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
			{"P.U HT", "{{lineItems.unitPrice}}", true},
			{"TOTAL HT", "{{lineItems.lineTotal}}", true},
		},
		totals: []totalRow{
			{"Total Brut HTVA", "{{order.subTotal}}", false},
			{"Total TVA", "{{order.taxTotal}}", false},
			{"Total TTC", "{{order.total}}", true},
		},
		footerLeft:  "IBAN : {{organization.iban}}",
		footerRight: "{{organization.bankName}}",
	})
	addAvailableFieldsSheet(f, orderFieldRefs)
	finalizeWorkbook(f, "Commande", "db/templates/gen/order_tunisia.xlsx")
}

// orderFieldRefs groups every placeholder
// db.buildOrderScalarPlaceholders / db.buildOrderLineItemPlaceholders
// (db/xlsx_export_order.go) resolve.
var orderFieldRefs = []fieldRef{
	{"Header", "{{organization.logo}}", "Seller logo — place alone in a cell to anchor the uploaded image there"},
	{"Header", "{{order.number}}", "Order number"},
	{"Header", "{{order.orderDate}}", "Order date"},
	{"Header", "{{order.deliveryDate}}", "Delivery date, blank if unset"},
	{"Header", "{{order.currency}}", "Currency code (e.g. EUR)"},
	{"Header", "{{order.shippingAddress}}", "Free-text shipping address, blank if unset"},
	{"Header", "{{order.trackingNumber}}", "Tracking number, blank if unset"},
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
	{"Item lines", "{{lineItems.unitPrice}}", "Unit price, formatted with currency"},
	{"Item lines", "{{lineItems.lineTotal}}", "Quantity x unit price, formatted with currency"},

	{"Footer", "{{order.subTotal}}", "Subtotal before tax, computed from line items"},
	{"Footer", "{{order.taxTotal}}", "Total tax (always zero — orders carry no per-line tax)"},
	{"Footer", "{{order.total}}", "Grand total, computed from line items"},
	{"Footer", "{{order.notes}}", "Free-text notes, blank if unset"},

	{"Export info", "{{export.generatedDate}}", "Date this file was exported, in the organization's date format"},
	{"Export info", "{{export.generatedTime}}", "Time this file was exported, 24-hour HH:MM, server time"},
	{"Export info", "&P (page number) / &N (total pages)", "Native Excel/LibreOffice codes, only work in Page Layout ▸ Header/Footer"},
}
