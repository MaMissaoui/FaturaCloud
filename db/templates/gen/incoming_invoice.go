package main

// buildIncomingInvoiceTunisiaTemplate generates
// db/templates/incoming_invoice_tunisia.xlsx via the shared Tunisian layout.
// A vendor bill (Facture fournisseur) is inward — the boxed second-party
// block shows the vendor (who issued it), while the seller block is the
// organization receiving the bill. It has real server-validated stored totals
// (unlike purchase orders/orders), so the totals block reads them directly.
func buildIncomingInvoiceTunisiaTemplate() {
	f := buildTunisiaLayout(tunisiaLayoutSpec{
		sheet:     "Facture fournisseur",
		title:     "Facture N°: {{incomingInvoice.number}}",
		date:      "Date : {{incomingInvoice.date}}",
		partyName: "{{vendor.name}}",
		partyFields: []string{
			"{{vendor.street}} {{vendor.houseNumber}}",
			"{{vendor.postalCode}} {{vendor.city}}",
			"Tel: {{vendor.phone}}",
			"MF: {{vendor.vatin}}",
		},
		columns: []tableColumn{
			// A Code column like every other Tunisia-layout type: the frame's
			// column widths (and the VAT recap beneath) assume Code in A and
			// Désignation in the wide B, so without it Désignation wrapped
			// into the narrow A column.
			{"Code", "{{lineItems.sku}}", false},
			{"Désignation", "{{lineItems.description}}", false},
			{"Quantité", "{{lineItems.quantity}}", true},
			{"P.U HT", "{{lineItems.unitPrice}}", true},
			{"TOTAL HT", "{{lineItems.lineTotal}}", true},
		},
		totals: []totalRow{
			{"Total Brut HTVA", "{{incomingInvoice.subTotal}}", false},
			{"Total TVA", "{{incomingInvoice.taxTotal}}", false},
			{"Total TTC", "{{incomingInvoice.total}}", true},
		},
		hasVatRecap: true,
		footerLeft:  "IBAN : {{organization.iban}}",
		footerRight: "{{organization.bankName}}",
	})
	addAvailableFieldsSheet(f, incomingInvoiceFieldRefs)
	finalizeWorkbook(f, "Facture fournisseur", "db/templates/gen/incoming_invoice_tunisia.xlsx")
}

// incomingInvoiceFieldRefs groups every placeholder
// db.buildIncomingInvoiceScalarPlaceholders /
// db.buildIncomingInvoiceLineItemPlaceholders (db/xlsx_export_incoming_invoice.go)
// resolve.
var incomingInvoiceFieldRefs = []fieldRef{
	{"Header", "{{organization.logo}}", "Buyer logo — place alone in a cell to anchor the uploaded image there"},
	{"Header", "{{incomingInvoice.number}}", "Vendor's invoice number"},
	{"Header", "{{incomingInvoice.date}}", "Bill date"},
	{"Header", "{{incomingInvoice.dueDate}}", "Payment due date, blank if unset"},
	{"Header", "{{incomingInvoice.currency}}", "Currency code (e.g. EUR)"},
	{"Header", "{{incomingInvoice.reference}}", "Free-text reference, blank if unset"},
	{"Header", "{{incomingInvoice.purchaseOrder}}", "Linked purchase order number, blank if unset"},
	{"Header", "{{organization.name}}", "Buyer (bill recipient) company name"},
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
	{"Item lines", "{{lineItems.sku}}", "Linked product's SKU (the Code column), blank on a free-text line"},
	{"Item lines", "{{lineItems.description}}", "Line item description"},
	{"Item lines", "{{lineItems.quantity}}", "Quantity"},
	{"Item lines", "{{lineItems.unitPrice}}", "Unit price, formatted with currency"},
	{"Item lines", "{{lineItems.taxRate}}", "Tax rate percentage (e.g. 19.5%), blank if unset"},
	{"Item lines", "{{lineItems.lineTotal}}", "Quantity x unit price, formatted with currency"},

	{"VAT recap", "{{#taxLines}}", "Marker (not a value) — place alone in one cell of the row to repeat once per distinct tax rate"},
	{"VAT recap", "{{taxLines.rate}}", "Tax rate percentage (e.g. 19%)"},
	{"VAT recap", "{{taxLines.base}}", "Taxable base for this rate"},
	{"VAT recap", "{{taxLines.amount}}", "VAT amount for this rate"},

	{"Footer", "{{incomingInvoice.subTotal}}", "Subtotal before tax (stored, server-validated)"},
	{"Footer", "{{incomingInvoice.taxTotal}}", "Total tax (stored, server-validated)"},
	{"Footer", "{{incomingInvoice.total}}", "Grand total (stored, server-validated)"},
	{"Footer", "{{incomingInvoice.notes}}", "Free-text notes, blank if unset"},

	{"Export info", "{{export.generatedDate}}", "Date this file was exported, in the organization's date format"},
	{"Export info", "{{export.generatedTime}}", "Time this file was exported, 24-hour HH:MM, server time"},
	{"Export info", "&P (page number) / &N (total pages)", "Native Excel/LibreOffice codes, only work in Page Layout ▸ Header/Footer"},
}
