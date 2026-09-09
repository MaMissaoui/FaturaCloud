package db

import "fmt"

// FillPurchaseOrderTemplate fills an uploaded or embedded-default purchase
// order template with an order's data, expanding {{#lineItems}} the same way
// FillInvoiceTemplate does — see fillTemplate in xlsx_export.go for the
// shared engine both wrap. Purchase orders have no server-validated stored
// totals (unlike invoices — see db/invoice_totals.go), so subtotal/tax/total
// are computed here via computeExportTotals rather than read off the order
// row.
func FillPurchaseOrderTemplate(
	templateBytes []byte,
	order PurchaseOrder,
	lineItems []PurchaseOrderLineItem,
	org Organization,
	vendor Vendor,
	taxRates map[string]TaxRate,
) ([]byte, []string, error) {
	currency := ""
	if order.Currency != nil {
		currency = *order.Currency
	} else if org.Currency != nil {
		currency = *org.Currency
	}

	exportItems := make([]exportLineItem, len(lineItems))
	for i, li := range lineItems {
		exportItems[i] = exportLineItem{Quantity: li.Quantity, UnitPrice: li.UnitPrice, TaxRate: li.TaxRate}
	}
	subTotal, taxTotal, total := computeExportTotals(exportItems, taxRates)

	scalars := buildPurchaseOrderScalarPlaceholders(order, org, vendor, currency, subTotal, taxTotal, total)
	mergeExportMetaPlaceholders(scalars, org.DateFormat)
	lineRows := make([]map[string]string, len(lineItems))
	for i, li := range lineItems {
		lineRows[i] = buildPurchaseOrderLineItemPlaceholders(li, currency, org.MinimumFractionDigits, resolveTaxRatePercent(taxRates, li.TaxRate))
	}
	return fillTemplate(templateBytes, scalars, lineRows)
}

// FetchPurchaseOrderExportData gathers everything FillPurchaseOrderTemplate
// needs for one purchase order — mirrors FetchInvoiceExportData's shape,
// including its "one query per distinct tax rate, not per line" precedent.
// Callers hold dbMu only around this call; the fill/convert step that
// follows must run lock-free (see api/document_templates.go's
// exportInvoiceDocument for why).
func (d *Database) FetchPurchaseOrderExportData(orderID string) (*PurchaseOrder, []PurchaseOrderLineItem, *Organization, *Vendor, []byte, map[string]TaxRate, error) {
	order, err := d.GetPurchaseOrder(orderID)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, fmt.Errorf("fetch_purchase_order_export_data: get purchase order: %w", err)
	}
	lineItems, err := d.GetPurchaseOrderLineItems(orderID)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, fmt.Errorf("fetch_purchase_order_export_data: get line items: %w", err)
	}
	org, err := d.GetOrganization(order.OrganizationID)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, fmt.Errorf("fetch_purchase_order_export_data: get organization: %w", err)
	}

	// A purchase order's vendorId has no ON DELETE clause and no NOT NULL
	// requirement (see CLAUDE.md's db/vendor.go note) — an unset vendor just
	// leaves every vendor.* placeholder blank rather than failing the export.
	var vendor Vendor
	if order.VendorID != nil {
		v, err := d.GetVendor(*order.VendorID)
		if err != nil {
			return nil, nil, nil, nil, nil, nil, fmt.Errorf("fetch_purchase_order_export_data: get vendor: %w", err)
		}
		vendor = *v
	}

	templateBytes, _, err := resolveTemplateBytes(d, order.OrganizationID, "purchase_order")
	if err != nil {
		return nil, nil, nil, nil, nil, nil, fmt.Errorf("fetch_purchase_order_export_data: resolve template: %w", err)
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

	return order, lineItems, org, &vendor, templateBytes, taxRates, nil
}

// buildPurchaseOrderScalarPlaceholders is the fixed namespace->field
// allowlist for purchaseOrder/organization/vendor values — see
// buildScalarPlaceholders in xlsx_export.go for the invoice equivalent this
// mirrors.
func buildPurchaseOrderScalarPlaceholders(order PurchaseOrder, org Organization, vendor Vendor, currency string, subTotal, taxTotal, total int64) map[string]string {
	return map[string]string{
		"purchaseOrder.number":          order.OrderNumber,
		"purchaseOrder.orderDate":       formatOrgDate(order.OrderDate, org.DateFormat),
		"purchaseOrder.expectedDate":    formatOptionalOrgDate(order.ExpectedDate, org.DateFormat),
		"purchaseOrder.currency":        currency,
		"purchaseOrder.deliveryAddress": derefString(order.DeliveryAddress),
		"purchaseOrder.notes":           derefString(order.Notes),
		"purchaseOrder.subTotal":        formatMoneyCents(subTotal, currency, org.MinimumFractionDigits),
		"purchaseOrder.taxTotal":        formatMoneyCents(taxTotal, currency, org.MinimumFractionDigits),
		"purchaseOrder.total":           formatMoneyCents(total, currency, org.MinimumFractionDigits),

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

// buildPurchaseOrderLineItemPlaceholders is the per-row namespace for the
// repeated line item block. Description is a plain string on
// PurchaseOrderLineItem (unlike InvoiceLineItem's *string), so no
// derefString here.
func buildPurchaseOrderLineItemPlaceholders(li PurchaseOrderLineItem, currency string, minimumFractionDigits *int64, taxRatePercent string) map[string]string {
	lineTotal := lineTotalCents(li.Quantity, li.UnitPrice)
	return map[string]string{
		"lineItems.description": li.Description,
		"lineItems.quantity":    formatQuantity(li.Quantity),
		"lineItems.unit":        derefString(li.Unit),
		"lineItems.unitPrice":   formatMoneyCents(li.UnitPrice, currency, minimumFractionDigits),
		"lineItems.taxRate":     taxRatePercent,
		"lineItems.lineTotal":   formatMoneyCents(lineTotal, currency, minimumFractionDigits),
	}
}
