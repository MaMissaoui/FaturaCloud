package db

import "fmt"

// FillInboundDeliveryTemplate fills an uploaded or embedded-default inbound
// delivery (goods receipt) template with a receipt's data, expanding
// {{#lineItems}} the same way FillInvoiceTemplate does — see fillTemplate in
// xlsx_export.go for the shared engine every FillXTemplate wraps.
// inbound_delivery_line_items.unitCost is a *cost* feeding average cost
// (db.recomputeAverageCostTx), not a customer-facing price, so there is no
// tax and no aggregate totals block — each line's own cost/lineTotal is
// still useful on the printed receipt (what was this shipment worth), just
// not summed into a subtotal/tax/total footer the way a bill would be.
func FillInboundDeliveryTemplate(
	templateBytes []byte,
	delivery InboundDelivery,
	lineItems []InboundDeliveryLineItem,
	org Organization,
	vendor Vendor,
) ([]byte, []string, error) {
	currency := ""
	if delivery.Currency != nil {
		currency = *delivery.Currency
	} else if org.Currency != nil {
		currency = *org.Currency
	}

	scalars := buildInboundDeliveryScalarPlaceholders(delivery, org, vendor, currency)
	lineRows := make([]map[string]string, len(lineItems))
	for i, li := range lineItems {
		lineRows[i] = buildInboundDeliveryLineItemPlaceholders(li, currency, org.MinimumFractionDigits)
	}
	return fillTemplate(templateBytes, scalars, lineRows)
}

// FetchInboundDeliveryExportData gathers everything
// FillInboundDeliveryTemplate needs for one goods receipt — mirrors
// FetchPurchaseOrderExportData's shape, including its vendor-less fallback
// (inbound_deliveries.vendorId is nullable, same as purchase_orders.vendorId).
// Callers hold dbMu only around this call; the fill/convert step that
// follows must run lock-free (see api/document_templates.go's
// exportInvoiceDocument for why).
func (d *Database) FetchInboundDeliveryExportData(deliveryID string) (*InboundDelivery, []InboundDeliveryLineItem, *Organization, *Vendor, []byte, error) {
	delivery, err := d.GetInboundDelivery(deliveryID)
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("fetch_inbound_delivery_export_data: get inbound delivery: %w", err)
	}
	lineItems, err := d.GetInboundDeliveryLineItems(deliveryID)
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("fetch_inbound_delivery_export_data: get line items: %w", err)
	}
	org, err := d.GetOrganization(delivery.OrganizationID)
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("fetch_inbound_delivery_export_data: get organization: %w", err)
	}

	var vendor Vendor
	if delivery.VendorID != nil {
		v, err := d.GetVendor(*delivery.VendorID)
		if err != nil {
			return nil, nil, nil, nil, nil, fmt.Errorf("fetch_inbound_delivery_export_data: get vendor: %w", err)
		}
		vendor = *v
	}

	templateBytes, _, err := resolveTemplateBytes(d, delivery.OrganizationID, "inbound_delivery")
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("fetch_inbound_delivery_export_data: resolve template: %w", err)
	}

	return delivery, lineItems, org, &vendor, templateBytes, nil
}

// buildInboundDeliveryScalarPlaceholders is the fixed namespace->field
// allowlist for inboundDelivery/organization/vendor values.
func buildInboundDeliveryScalarPlaceholders(delivery InboundDelivery, org Organization, vendor Vendor, currency string) map[string]string {
	return map[string]string{
		"inboundDelivery.number":             delivery.DeliveryNumber,
		"inboundDelivery.date":               formatOrgDate(delivery.DeliveryDate, org.DateFormat),
		"inboundDelivery.currency":           currency,
		"inboundDelivery.purchaseOrder":      derefString(delivery.OrderNumber),
		"inboundDelivery.vendorDeliveryNote": derefString(delivery.VendorDeliveryNote),
		"inboundDelivery.trackingNumber":     derefString(delivery.TrackingNumber),
		"inboundDelivery.notes":              derefString(delivery.Notes),

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

// buildInboundDeliveryLineItemPlaceholders is the per-row namespace for the
// repeated line item block. unitCost/lineTotal are shown (this shipment's
// received value is useful on the printed receipt) but there is
// deliberately no lineItems.taxRate — a receipt's cost has no tax rate of
// its own, unlike a bill.
func buildInboundDeliveryLineItemPlaceholders(li InboundDeliveryLineItem, currency string, minimumFractionDigits *int64) map[string]string {
	var unitCost int64
	if li.UnitCost != nil {
		unitCost = *li.UnitCost
	}
	lineTotal := lineTotalCents(li.Quantity, unitCost)
	return map[string]string{
		"lineItems.description": li.Description,
		"lineItems.quantity":    formatQuantity(li.Quantity),
		"lineItems.unit":        derefString(li.Unit),
		"lineItems.unitCost":    formatMoneyCents(unitCost, currency, minimumFractionDigits),
		"lineItems.lineTotal":   formatMoneyCents(lineTotal, currency, minimumFractionDigits),
	}
}
