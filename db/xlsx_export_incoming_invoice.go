package db

import "fmt"

// FillIncomingInvoiceTemplate fills an uploaded or embedded-default incoming
// invoice template with a vendor bill's data, expanding {{#lineItems}} the
// same way FillInvoiceTemplate does — see fillTemplate in xlsx_export.go for
// the shared engine both wrap. Unlike purchase orders/orders, an incoming
// invoice DOES have server-validated stored totals (it reuses
// CreateInvoiceLineItemRequest and goes through validateInvoiceTotals, same
// as a sales invoice — see CLAUDE.md's incoming_invoices.state note), so
// subTotal/taxTotal/total are read directly off the row, never recomputed
// via computeExportTotals.
func FillIncomingInvoiceTemplate(
	templateBytes []byte,
	invoice IncomingInvoice,
	lineItems []IncomingInvoiceLineItem,
	org Organization,
	vendor Vendor,
	taxRates map[string]TaxRate,
) ([]byte, []string, error) {
	currency := invoice.Currency
	if currency == "" && org.Currency != nil {
		currency = *org.Currency
	}

	scalars := buildIncomingInvoiceScalarPlaceholders(invoice, org, vendor, currency)
	lineRows := make([]map[string]string, len(lineItems))
	for i, li := range lineItems {
		lineRows[i] = buildIncomingInvoiceLineItemPlaceholders(li, currency, org.MinimumFractionDigits, resolveTaxRatePercent(taxRates, li.TaxRate))
	}
	return fillTemplate(templateBytes, scalars, lineRows)
}

// FetchIncomingInvoiceExportData gathers everything FillIncomingInvoiceTemplate
// needs for one incoming invoice — mirrors FetchInvoiceExportData's shape,
// including its "one query per distinct tax rate, not per line" precedent.
// Callers hold dbMu only around this call; the fill/convert step that
// follows must run lock-free (see api/document_templates.go's
// exportInvoiceDocument for why).
func (d *Database) FetchIncomingInvoiceExportData(invoiceID string) (*IncomingInvoice, []IncomingInvoiceLineItem, *Organization, *Vendor, []byte, map[string]TaxRate, error) {
	invoice, err := d.GetIncomingInvoice(invoiceID)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, fmt.Errorf("fetch_incoming_invoice_export_data: get incoming invoice: %w", err)
	}
	lineItems, err := d.GetIncomingInvoiceLineItems(invoiceID)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, fmt.Errorf("fetch_incoming_invoice_export_data: get line items: %w", err)
	}
	org, err := d.GetOrganization(invoice.OrganizationID)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, fmt.Errorf("fetch_incoming_invoice_export_data: get organization: %w", err)
	}

	// Unlike purchase_orders.vendorId, incoming_invoices.vendorId is a
	// required, non-nullable column — see db/incoming_invoice.go's struct —
	// so no vendor-less fallback is needed here.
	vendor, err := d.GetVendor(invoice.VendorID)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, fmt.Errorf("fetch_incoming_invoice_export_data: get vendor: %w", err)
	}

	templateBytes, _, err := resolveTemplateBytes(d, invoice.OrganizationID, "incoming_invoice")
	if err != nil {
		return nil, nil, nil, nil, nil, nil, fmt.Errorf("fetch_incoming_invoice_export_data: resolve template: %w", err)
	}

	taxRates := map[string]TaxRate{}
	for _, li := range lineItems {
		if li.TaxRate == nil || *li.TaxRate == "" {
			continue
		}
		if _, ok := taxRates[*li.TaxRate]; ok {
			continue
		}
		rate, err := d.GetTaxRate(*li.TaxRate)
		if err != nil {
			continue // an unresolvable tax rate just leaves lineItems.taxRate blank, not a failed export
		}
		taxRates[*li.TaxRate] = *rate
	}

	return invoice, lineItems, org, vendor, templateBytes, taxRates, nil
}

// buildIncomingInvoiceScalarPlaceholders is the fixed namespace->field
// allowlist for incomingInvoice/organization/vendor values — see
// buildScalarPlaceholders in xlsx_export.go for the invoice equivalent this
// mirrors. There's no clientId/buyerReference here (this is a bill FaturaCloud
// received, not one it issued) and no fiscal-stamp/withholding-tax fields
// (those are Tunisia sales-invoice specific, with no incoming-invoice column).
func buildIncomingInvoiceScalarPlaceholders(invoice IncomingInvoice, org Organization, vendor Vendor, currency string) map[string]string {
	return map[string]string{
		"incomingInvoice.number":        invoice.VendorInvoiceNumber,
		"incomingInvoice.date":          formatOrgDate(invoice.Date, org.DateFormat),
		"incomingInvoice.dueDate":       formatOptionalOrgDate(invoice.DueDate, org.DateFormat),
		"incomingInvoice.currency":      currency,
		"incomingInvoice.reference":     derefString(invoice.Reference),
		"incomingInvoice.purchaseOrder": derefString(invoice.OrderNumber),
		"incomingInvoice.notes":         derefString(invoice.Notes),
		"incomingInvoice.subTotal":      formatMoneyCents(invoice.SubTotal, currency, org.MinimumFractionDigits),
		"incomingInvoice.taxTotal":      formatMoneyCents(invoice.TaxTotal, currency, org.MinimumFractionDigits),
		"incomingInvoice.total":         formatMoneyCents(invoice.Total, currency, org.MinimumFractionDigits),

		"organization.name":        derefString(org.Name),
		"organization.vatin":       derefString(org.Vatin),
		"organization.email":       derefString(org.Email),
		"organization.phone":       derefString(org.Phone),
		"organization.website":     derefString(org.Website),
		"organization.street":      derefString(org.Street),
		"organization.houseNumber": derefString(org.HouseNumber),
		"organization.postalCode":  derefString(org.PostalCode),
		"organization.city":        derefString(org.City),

		"vendor.name":        derefString(vendor.Name),
		"vendor.vatin":       derefString(vendor.Vatin),
		"vendor.email":       firstEmail(vendor.Emails),
		"vendor.phone":       derefString(vendor.Phone),
		"vendor.street":      derefString(vendor.Street),
		"vendor.houseNumber": derefString(vendor.HouseNumber),
		"vendor.postalCode":  derefString(vendor.PostalCode),
		"vendor.city":        derefString(vendor.City),
	}
}

// buildIncomingInvoiceLineItemPlaceholders is the per-row namespace for the
// repeated line item block. Description is a plain string on
// IncomingInvoiceLineItem (unlike InvoiceLineItem's *string), same as
// PurchaseOrderLineItem — no derefString here.
func buildIncomingInvoiceLineItemPlaceholders(li IncomingInvoiceLineItem, currency string, minimumFractionDigits *int64, taxRatePercent string) map[string]string {
	lineTotal := lineTotalCents(li.Quantity, li.UnitPrice)
	return map[string]string{
		"lineItems.description": li.Description,
		"lineItems.quantity":    formatQuantity(li.Quantity),
		"lineItems.unitPrice":   formatMoneyCents(li.UnitPrice, currency, minimumFractionDigits),
		"lineItems.taxRate":     taxRatePercent,
		"lineItems.lineTotal":   formatMoneyCents(lineTotal, currency, minimumFractionDigits),
	}
}
