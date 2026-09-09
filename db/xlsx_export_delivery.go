package db

import "fmt"

// FillDeliveryTemplate fills an uploaded or embedded-default outbound
// delivery (delivery note) template with a delivery's data, expanding
// {{#lineItems}} the same way FillInvoiceTemplate does — see fillTemplate in
// xlsx_export.go for the shared engine every FillXTemplate wraps.
// outbound_delivery_line_items has no price columns at all (CLAUDE.md: "a
// delivery note never shows prices"), so there is no totals block and no
// currency placeholder — this is a line-item list only.
func FillDeliveryTemplate(
	templateBytes []byte,
	delivery OutboundDelivery,
	lineItems []OutboundDeliveryLineItem,
	org Organization,
	client Client,
) ([]byte, []string, error) {
	scalars := buildDeliveryScalarPlaceholders(delivery, org, client)
	lineRows := make([]map[string]string, len(lineItems))
	for i, li := range lineItems {
		lineRows[i] = buildDeliveryLineItemPlaceholders(li)
	}
	return fillTemplate(templateBytes, scalars, lineRows)
}

// FetchDeliveryExportData gathers everything FillDeliveryTemplate needs for
// one outbound delivery — mirrors FetchPurchaseOrderExportData's shape.
// Callers hold dbMu only around this call; the fill/convert step that
// follows must run lock-free (see api/document_templates.go's
// exportInvoiceDocument for why).
func (d *Database) FetchDeliveryExportData(deliveryID string) (*OutboundDelivery, []OutboundDeliveryLineItem, *Organization, *Client, []byte, error) {
	delivery, err := d.GetDelivery(deliveryID)
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("fetch_delivery_export_data: get delivery: %w", err)
	}
	lineItems, err := d.GetDeliveryLineItems(deliveryID)
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("fetch_delivery_export_data: get line items: %w", err)
	}
	org, err := d.GetOrganization(delivery.OrganizationID)
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("fetch_delivery_export_data: get organization: %w", err)
	}

	// ClientID is the *effective* client (order's, or the delivery's own —
	// see outboundDeliverySelect) and is nullable: a walk-in/no-client
	// standalone delivery exports with every client.* placeholder blank
	// rather than failing, the same precedent as purchase orders' vendor.
	var client Client
	if delivery.ClientID != nil {
		c, err := d.GetClient(*delivery.ClientID)
		if err != nil {
			return nil, nil, nil, nil, nil, fmt.Errorf("fetch_delivery_export_data: get client: %w", err)
		}
		client = *c
	}

	templateBytes, _, err := resolveTemplateBytes(d, delivery.OrganizationID, "delivery")
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("fetch_delivery_export_data: resolve template: %w", err)
	}

	return delivery, lineItems, org, &client, templateBytes, nil
}

// buildDeliveryScalarPlaceholders is the fixed namespace->field allowlist
// for delivery/organization/client values. No currency or money fields —
// this document type never carries a price.
func buildDeliveryScalarPlaceholders(delivery OutboundDelivery, org Organization, client Client) map[string]string {
	return map[string]string{
		"delivery.number":          delivery.DeliveryNumber,
		"delivery.date":            formatOrgDate(delivery.DeliveryDate, org.DateFormat),
		"delivery.orderNumber":     derefString(delivery.OrderNumber),
		"delivery.shippingAddress": derefString(delivery.ShippingAddress),
		"delivery.trackingNumber":  derefString(delivery.TrackingNumber),
		"delivery.notes":           derefString(delivery.Notes),

		"organization.name":        derefString(org.Name),
		"organization.vatin":       derefString(org.Vatin),
		"organization.email":       derefString(org.Email),
		"organization.phone":       derefString(org.Phone),
		"organization.website":     derefString(org.Website),
		"organization.street":      derefString(org.Street),
		"organization.houseNumber": derefString(org.HouseNumber),
		"organization.postalCode":  derefString(org.PostalCode),
		"organization.city":        derefString(org.City),

		"client.name":        derefString(client.Name),
		"client.vatin":       derefString(client.Vatin),
		"client.email":       firstEmail(client.Emails),
		"client.phone":       derefString(client.Phone),
		"client.street":      derefString(client.Street),
		"client.houseNumber": derefString(client.HouseNumber),
		"client.postalCode":  derefString(client.PostalCode),
		"client.city":        derefString(client.City),
	}
}

// buildDeliveryLineItemPlaceholders is the per-row namespace for the
// repeated line item block — description, quantity, unit, and the linked
// product's SKU, no price or line total, matching
// outbound_delivery_line_items' own columns. sku is a joined column (nil
// for a free-text line, or a line whose product has none), the same
// precedent as stockEnabled/serialized. Description is a plain string
// (unlike InvoiceLineItem's *string), same as PurchaseOrderLineItem — no
// derefString here.
func buildDeliveryLineItemPlaceholders(li OutboundDeliveryLineItem) map[string]string {
	return map[string]string{
		"lineItems.description": li.Description,
		"lineItems.sku":         derefString(li.SKU),
		"lineItems.quantity":    formatQuantity(li.Quantity),
		"lineItems.unit":        derefString(li.Unit),
	}
}
