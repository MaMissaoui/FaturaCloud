package main

// buildInvoiceTunisiaTemplate generates db/templates/invoice_tunisia.xlsx — the
// embedded default template for issue #115's custom invoice export. See
// tunisia_layout.go's buildTunisiaLayout for the shared frame every document
// type's Tunisian-layout default uses, and the package doc comment in
// main.go for why these are generated programmatically rather than
// hand-authored.
//
// TestEmbeddedDefaultInvoiceTemplatePlaceholdersAllResolve
// (db/xlsx_export_test.go) is what actually guards the placeholders against a
// typo, since the committed binary itself isn't diff-reviewable.
//
// The output has two sheets: "Facture" (the content sheet — sheet index 0,
// what db.FillInvoiceTemplate fills and returns) and "Available fields", a
// reference table of every placeholder the fill engine resolves.
func buildInvoiceTunisiaTemplate() {
	f := buildTunisiaLayout(tunisiaLayoutSpec{
		sheet: "Facture",
		title: "Facture N°: {{invoice.number}}",
		date:  "Date : {{invoice.date}}",
		columns: []tableColumn{
			{"Code", "{{lineItems.sku}}", false},
			{"Désignation", "{{lineItems.description}}", false},
			{"Quantité", "{{lineItems.quantity}}", true},
			{"P.U HT", "{{lineItems.unitPrice}}", true},
			{"TOTAL HT", "{{lineItems.lineTotal}}", true},
		},
		totals: []totalRow{
			{"Total Brut HTVA", "{{invoice.subTotal}}", false},
			{"Remise", "{{invoice.discountAmount}}", false},
			{"Total Net HTVA", "{{invoice.netTaxable}}", false},
			{"Total TVA", "{{invoice.taxTotal}}", false},
			{"D.Timbre", "{{invoice.fiscalStampAmount}}", false},
			{"Total TTC", "{{invoice.total}}", true},
		},
		hasVatRecap:   true,
		amountInWords: "{{invoice.amountInWords}}",
		footerLeft:    "IBAN : {{organization.iban}}",
		footerRight:   "{{organization.bankName}}",
		partyName:     "{{client.name}}",
		partyFields: []string{
			"{{client.address}}",
			"{{client.postalCode}} {{client.city}}",
			"Tel: {{client.phone}}",
			"CIN: {{client.identityNumber}}",
		},
	})
	addAvailableFieldsSheet(f, invoiceFieldRefs)
	finalizeWorkbook(f, "Facture", "db/templates/gen/invoice_tunisia.xlsx")
}

// invoiceFieldRefs groups every placeholder the invoice fill engine
// (db/xlsx_export.go's buildScalarPlaceholders / buildLineItemPlaceholders /
// buildTaxBreakdownRows) resolves.
var invoiceFieldRefs = []fieldRef{
	{"Header", "{{organization.logo}}", "Seller logo — place alone in a cell to anchor the uploaded image there (blank if the org has none)"},
	{"Header", "{{invoice.number}}", "Invoice number"},
	{"Header", "{{invoice.date}}", "Invoice date"},
	{"Header", "{{invoice.dueDate}}", "Payment due date"},
	{"Header", "{{invoice.currency}}", "Currency code (e.g. EUR)"},
	{"Header", "{{invoice.buyerReference}}", "Buyer reference (e.g. a Leitweg-ID)"},
	{"Header", "{{organization.name}}", "Seller company name"},
	{"Header", "{{organization.vatin}}", "Seller VAT / MF number"},
	{"Header", "{{organization.email}}", "Seller email"},
	{"Header", "{{organization.phone}}", "Seller phone"},
	{"Header", "{{organization.website}}", "Seller website"},
	{"Header", "{{organization.street}}", "Seller street"},
	{"Header", "{{organization.houseNumber}}", "Seller house number"},
	{"Header", "{{organization.postalCode}}", "Seller postal code"},
	{"Header", "{{organization.city}}", "Seller city"},
	{"Header", "{{client.name}}", "Customer name"},
	{"Header", "{{client.vatin}}", "Customer VAT / MF number"},
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

	{"Item lines", "{{#lineItems}}", "Marker (not a value) — place alone in any one cell of the row to repeat once per line item; that whole cell is blanked in the output"},
	{"Item lines", "{{lineItems.sku}}", "Linked product's SKU (the Code column)"},
	{"Item lines", "{{lineItems.description}}", "Line item description (Désignation)"},
	{"Item lines", "{{lineItems.quantity}}", "Quantity"},
	{"Item lines", "{{lineItems.unitPrice}}", "Unit price, formatted with currency"},
	{"Item lines", "{{lineItems.taxRate}}", "Tax rate percentage (e.g. 19.5%)"},
	{"Item lines", "{{lineItems.lineTotal}}", "Quantity x unit price, formatted with currency"},

	{"VAT recap", "{{#taxLines}}", "Marker (not a value) — place alone in any one cell of the row to repeat once per distinct tax rate"},
	{"VAT recap", "{{taxLines.rate}}", "Tax rate percentage (e.g. 19%)"},
	{"VAT recap", "{{taxLines.base}}", "Taxable base for this rate, net of its share of the discount"},
	{"VAT recap", "{{taxLines.amount}}", "VAT amount for this rate"},

	{"Footer", "{{invoice.subTotal}}", "Total Brut HTVA — subtotal before discount and tax"},
	{"Footer", "{{invoice.discountAmount}}", "Remise — flat pre-tax discount"},
	{"Footer", "{{invoice.netTaxable}}", "Total Net HTVA — subtotal minus discount"},
	{"Footer", "{{invoice.taxTotal}}", "Total TVA"},
	{"Footer", "{{invoice.total}}", "Total TTC — grand total"},
	{"Footer", "{{invoice.paymentTerms}}", "Payment terms text"},
	{"Footer", "{{invoice.fiscalStampAmount}}", "Tunisia timbre fiscal — flat duty, formatted with currency (0 if unused)"},
	{"Footer", "{{invoice.withholdingTaxLine}}", "Full \"Withholding tax (12%): 40.00 EUR\" line, entirely blank (if unset)"},
	{"Footer", "{{invoice.amountInWords}}", "Whole \"Arrêtée la présente facture à la somme de ...\" sentence, blank unless enabled in Formatting settings"},
	{"Footer", "{{organization.iban}}", "Seller IBAN"},
	{"Footer", "{{organization.bankName}}", "Seller bank name"},

	{"Export info", "{{export.generatedDate}}", "Date this file was exported, in the organization's date format"},
	{"Export info", "{{export.generatedTime}}", "Time this file was exported, 24-hour HH:MM, server time"},
	{"Export info", "&P (page number) / &N (total pages)", "Native Excel/LibreOffice codes — only work in Page Layout ▸ Header/Footer, never a regular cell"},
}
