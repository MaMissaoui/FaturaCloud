package main

// buildPurchaseOrderTunisiaTemplate generates db/templates/purchase_order_tunisia.xlsx
// via the shared Tunisian layout (tunisia_layout.go), so every document type
// looks like one designed system. A purchase order is buyer-side outward, so
// the boxed second-party block shows the vendor.
func buildPurchaseOrderTunisiaTemplate() {
	f := buildTunisiaLayout(tunisiaLayoutSpec{
		sheet:     "Bon de commande",
		title:     "Bon de commande N°: {{purchaseOrder.number}}",
		date:      "Date : {{purchaseOrder.orderDate}}",
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
			{"P.U HT", "{{lineItems.unitPrice}}", true},
			{"TOTAL HT", "{{lineItems.lineTotal}}", true},
		},
		totals: []totalRow{
			{"Total Brut HTVA", "{{purchaseOrder.subTotal}}", false},
			{"Total TVA", "{{purchaseOrder.taxTotal}}", false},
			{"Total TTC", "{{purchaseOrder.total}}", true},
		},
		hasVatRecap: true,
		footerLeft:  "IBAN : {{organization.iban}}",
		footerRight: "{{organization.bankName}}",
	})
	addAvailableFieldsSheet(f, purchaseOrderFieldRefs)
	finalizeWorkbook(f, "Bon de commande", "db/templates/gen/purchase_order_tunisia.xlsx")
}

// purchaseOrderFieldRefs groups every placeholder
// db.buildPurchaseOrderScalarPlaceholders / db.buildPurchaseOrderLineItemPlaceholders
// (db/xlsx_export_purchase_order.go) resolve.
var purchaseOrderFieldRefs = []fieldRef{
	{"Header", "{{organization.logo}}", "Buyer logo — place alone in a cell to anchor the uploaded image there"},
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
	{"Item lines", "{{lineItems.sku}}", "Linked product's SKU, blank on a free-text line or an unset SKU"},
	{"Item lines", "{{lineItems.description}}", "Line item description"},
	{"Item lines", "{{lineItems.quantity}}", "Quantity"},
	{"Item lines", "{{lineItems.unit}}", "Unit of measure (e.g. pcs, kg), blank if unset"},
	{"Item lines", "{{lineItems.unitPrice}}", "Unit price, formatted with currency"},
	{"Item lines", "{{lineItems.taxRate}}", "Tax rate percentage (e.g. 19.5%), blank if unset"},
	{"Item lines", "{{lineItems.lineTotal}}", "Quantity x unit price, formatted with currency"},

	{"VAT recap", "{{#taxLines}}", "Marker (not a value) — place alone in one cell of the row to repeat once per distinct tax rate"},
	{"VAT recap", "{{taxLines.rate}}", "Tax rate percentage (e.g. 19%)"},
	{"VAT recap", "{{taxLines.base}}", "Taxable base for this rate"},
	{"VAT recap", "{{taxLines.amount}}", "VAT amount for this rate"},

	{"Footer", "{{purchaseOrder.subTotal}}", "Subtotal before tax, computed from line items"},
	{"Footer", "{{purchaseOrder.taxTotal}}", "Total tax, computed from line items"},
	{"Footer", "{{purchaseOrder.total}}", "Grand total, computed from line items"},
	{"Footer", "{{purchaseOrder.notes}}", "Free-text notes, blank if unset"},

	{"Export info", "{{export.generatedDate}}", "Date this file was exported, in the organization's date format"},
	{"Export info", "{{export.generatedTime}}", "Time this file was exported, 24-hour HH:MM, server time"},
	{"Export info", "&P (page number) / &N (total pages)", "Native Excel/LibreOffice codes, only work in Page Layout ▸ Header/Footer"},
}
