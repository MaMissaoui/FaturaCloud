package db

import "fmt"

// FillOrderTemplate fills an uploaded or embedded-default order (sales
// order) template with an order's data, expanding {{#lineItems}} the same
// way FillInvoiceTemplate does — see fillTemplate in xlsx_export.go for the
// shared engine every FillXTemplate wraps. Like purchase orders, orders have
// no server-validated stored totals, so subtotal/tax/total are computed here
// via computeExportTotals. OrderLineItem has no taxRate column at all
// (unlike PurchaseOrderLineItem), so every exportLineItem here carries a nil
// TaxRate — computeExportTotals correctly treats that as untaxed rather than
// erroring, so the tax line is always 0 for this document type.
func FillOrderTemplate(
	templateBytes []byte,
	order Order,
	lineItems []OrderLineItem,
	org Organization,
	client Client,
	logo []byte,
	orientation string,
) ([]byte, []string, error) {
	currency := ""
	if order.Currency != nil {
		currency = *order.Currency
	} else if org.Currency != nil {
		currency = *org.Currency
	}

	exportItems := make([]exportLineItem, len(lineItems))
	for i, li := range lineItems {
		exportItems[i] = exportLineItem{Quantity: li.Quantity, UnitPrice: li.UnitPrice}
	}
	subTotal, taxTotal, total := computeExportTotals(exportItems, nil)

	scalars := buildOrderScalarPlaceholders(order, org, client, currency, subTotal, taxTotal, total)
	mergeExportMetaPlaceholders(scalars, org.DateFormat)
	lineRows := make([]map[string]string, len(lineItems))
	for i, li := range lineItems {
		lineRows[i] = buildOrderLineItemPlaceholders(li, currency, org.MinimumFractionDigits, org.CountryCode)
	}
	blocks := []repeatBlock{{marker: lineItemMarker, rows: lineRows, required: true}}
	return fillTemplate(templateBytes, scalars, blocks, logo, orientation)
}

// FetchOrderExportData gathers everything FillOrderTemplate needs for one
// order — mirrors FetchPurchaseOrderExportData's shape. Callers hold dbMu
// only around this call; the fill/convert step that follows must run
// lock-free (see api/document_templates.go's exportInvoiceDocument for why).
func (d *Database) FetchOrderExportData(orderID string) (*Order, []OrderLineItem, *Organization, *Client, []byte, []byte, string, error) {
	order, err := d.GetOrder(orderID)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, "", fmt.Errorf("fetch_order_export_data: get order: %w", err)
	}
	lineItems, err := d.GetOrderLineItems(orderID)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, "", fmt.Errorf("fetch_order_export_data: get line items: %w", err)
	}
	org, err := d.GetOrganization(order.OrganizationID)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, "", fmt.Errorf("fetch_order_export_data: get organization: %w", err)
	}

	// orders.clientId is nullable (ON DELETE SET NULL) — an order that's lost
	// its client just exports with every client.* placeholder blank.
	var client Client
	if order.ClientID != nil {
		c, err := d.GetClient(*order.ClientID)
		if err != nil {
			return nil, nil, nil, nil, nil, nil, "", fmt.Errorf("fetch_order_export_data: get client: %w", err)
		}
		client = *c
	}

	templateBytes, _, err := resolveTemplateBytes(d, order.OrganizationID, "order", org.DocumentLayout)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, "", fmt.Errorf("fetch_order_export_data: resolve template: %w", err)
	}
	orientation, err := d.GetDocumentTemplateOrientation(order.OrganizationID, "order")
	if err != nil {
		return nil, nil, nil, nil, nil, nil, "", fmt.Errorf("fetch_order_export_data: resolve orientation: %w", err)
	}
	// A missing logo is not an error — the template's logo cell just stays
	// empty (GetOrganizationLogo returns nil, nil for an org that never
	// uploaded one).
	logo, err := d.GetOrganizationLogo(order.OrganizationID)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, "", fmt.Errorf("fetch_order_export_data: get logo: %w", err)
	}

	return order, lineItems, org, &client, templateBytes, logo, orientation, nil
}

// buildOrderScalarPlaceholders is the fixed namespace->field allowlist for
// order/organization/client values — see buildScalarPlaceholders in
// xlsx_export.go for the invoice equivalent this mirrors.
func buildOrderScalarPlaceholders(order Order, org Organization, client Client, currency string, subTotal, taxTotal, total int64) map[string]string {
	return map[string]string{
		"order.number":          order.OrderNumber,
		"order.orderDate":       formatOrgDate(order.OrderDate, org.DateFormat),
		"order.deliveryDate":    formatOptionalOrgDate(order.DeliveryDate, org.DateFormat),
		"order.currency":        currency,
		"order.shippingAddress": derefString(order.ShippingAddress),
		"order.trackingNumber":  derefString(order.TrackingNumber),
		"order.notes":           derefString(order.Notes),
		"order.subTotal":        formatMoneyCents(subTotal, currency, org.MinimumFractionDigits, org.CountryCode),
		"order.taxTotal":        formatMoneyCents(taxTotal, currency, org.MinimumFractionDigits, org.CountryCode),
		"order.total":           formatMoneyCents(total, currency, org.MinimumFractionDigits, org.CountryCode),

		"organization.name":        derefString(org.Name),
		"organization.vatin":       derefString(org.Vatin),
		"organization.email":       derefString(org.Email),
		"organization.phone":       derefString(org.Phone),
		"organization.website":     derefString(org.Website),
		"organization.street":      derefString(org.Street),
		"organization.houseNumber": derefString(org.HouseNumber),
		"organization.postalCode":  derefString(org.PostalCode),
		"organization.city":        derefString(org.City),
		"organization.iban":        derefString(org.IBAN),
		"organization.bankName":    derefString(org.BankName),

		"client.name":           derefString(client.Name),
		"client.identityNumber": derefString(client.IdentityNumber),
		"client.address":        derefString(client.Address),
		"client.vatin":          derefString(client.Vatin),
		"client.email":          firstEmail(client.Emails),
		"client.phone":          derefString(client.Phone),
		"client.phone2":         derefString(client.Phone2),
		"client.phone3":         derefString(client.Phone3),
		"client.street":         derefString(client.Street),
		"client.houseNumber":    derefString(client.HouseNumber),
		"client.postalCode":     derefString(client.PostalCode),
		"client.city":           derefString(client.City),
	}
}

// buildOrderLineItemPlaceholders is the per-row namespace for the repeated
// line item block. No taxRate placeholder — OrderLineItem has no taxRate
// column at all (unlike PurchaseOrderLineItem/InvoiceLineItem).
func buildOrderLineItemPlaceholders(li OrderLineItem, currency string, minimumFractionDigits *int64, countryCode *string) map[string]string {
	lineTotal := lineTotalCents(li.Quantity, li.UnitPrice)
	return map[string]string{
		"lineItems.sku":         derefString(li.SKU),
		"lineItems.description": li.Description,
		"lineItems.quantity":    formatQuantity(li.Quantity),
		"lineItems.unitPrice":   formatMoneyCents(li.UnitPrice, currency, minimumFractionDigits, countryCode),
		"lineItems.lineTotal":   formatMoneyCents(lineTotal, currency, minimumFractionDigits, countryCode),
	}
}
