package db

import (
	"fmt"
	"math/big"
)

// Match statuses, per invoice line.
const (
	MatchMatched          = "matched"
	MatchUnlinked         = "unlinked"
	MatchQuantityVariance = "quantity_variance"
	MatchOverReceived     = "over_received"
	MatchPriceVariance    = "price_variance"
)

// MatchLine is one invoice line compared against what was ordered and what was
// actually received. It is computed on demand and never stored: a stored flag
// would go stale the moment a linked goods receipt is cancelled, the same
// reasoning that keeps stockQuantity and unitCost derived.
type MatchLine struct {
	LineItemID          string   `json:"lineItemId"`
	PurchaseOrderLineID *string  `json:"purchaseOrderLineItemId"`
	Description         string   `json:"description"`
	OrderedQuantity     *float64 `json:"orderedQuantity"`
	ReceivedQuantity    *float64 `json:"receivedQuantity"`
	PreviouslyInvoiced  float64  `json:"previouslyInvoicedQuantity"`
	InvoicedQuantity    float64  `json:"invoicedQuantity"`
	OrderedUnitPrice    *int64   `json:"orderedUnitPrice"`
	InvoicedUnitPrice   int64    `json:"invoicedUnitPrice"`
	Status              string   `json:"status"`
	Message             string   `json:"message"`
}

// GetIncomingInvoiceMatch performs the 3-way match: invoice against purchase
// order (price and quantity) and against goods actually received (quantity,
// counting what other invoices already billed for the same order line).
func (d *Database) GetIncomingInvoiceMatch(invoiceID string) ([]MatchLine, error) {
	invoice, err := d.GetIncomingInvoice(invoiceID)
	if err != nil {
		return nil, fmt.Errorf("get_incoming_invoice_match lookup: %w", err)
	}
	items, err := d.GetIncomingInvoiceLineItems(invoiceID)
	if err != nil {
		return nil, err
	}

	org, err := d.GetOrganization(invoice.OrganizationID)
	if err != nil {
		return nil, fmt.Errorf("get_incoming_invoice_match organization: %w", err)
	}
	priceTolerance := valueOrZero(org.MatchPriceTolerancePercent)
	quantityTolerance := valueOrZero(org.MatchQuantityTolerancePercent)

	lines := make([]MatchLine, 0, len(items))
	for _, item := range items {
		line := MatchLine{
			LineItemID:          item.ID,
			PurchaseOrderLineID: item.PurchaseOrderLineItemID,
			Description:         item.Description,
			InvoicedQuantity:    item.Quantity,
			InvoicedUnitPrice:   item.UnitPrice,
			Status:              MatchUnlinked,
		}

		if item.PurchaseOrderLineItemID == nil {
			line.Message = "not linked to a purchase order line"
			lines = append(lines, line)
			continue
		}
		poLineID := *item.PurchaseOrderLineItemID

		var ordered struct {
			Quantity  float64 `db:"quantity"`
			UnitPrice int64   `db:"unitPrice"`
		}
		if err := d.DB.Get(&ordered,
			`SELECT quantity, unitPrice FROM purchase_order_line_items WHERE id = ?`, poLineID,
		); err != nil {
			line.Message = "the linked purchase order line no longer exists"
			lines = append(lines, line)
			continue
		}
		line.OrderedQuantity = &ordered.Quantity
		line.OrderedUnitPrice = &ordered.UnitPrice

		var received float64
		if err := d.DB.Get(&received, `
			SELECT COALESCE(SUM(dli.quantity), 0)
			FROM inbound_delivery_line_items dli
			JOIN inbound_deliveries idl ON dli.deliveryId = idl.id
			WHERE dli.purchaseOrderLineItemId = ? AND idl.status = 'received'`,
			poLineID,
		); err != nil {
			return nil, fmt.Errorf("get_incoming_invoice_match received: %w", err)
		}
		line.ReceivedQuantity = &received

		// What other non-cancelled invoices already billed against this same
		// order line — without this, the same goods can be billed twice.
		var previouslyInvoiced float64
		if err := d.DB.Get(&previouslyInvoiced, `
			SELECT COALESCE(SUM(li.quantity), 0)
			FROM incoming_invoice_line_items li
			JOIN incoming_invoices inv ON li.incomingInvoiceId = inv.id
			WHERE li.purchaseOrderLineItemId = ?
			  AND inv.id != ?
			  AND inv.state != 'cancelled'`,
			poLineID, invoiceID,
		); err != nil {
			return nil, fmt.Errorf("get_incoming_invoice_match previously_invoiced: %w", err)
		}
		line.PreviouslyInvoiced = previouslyInvoiced

		line.Status, line.Message = classifyMatch(line, quantityTolerance, priceTolerance)
		lines = append(lines, line)
	}

	return lines, nil
}

// classifyMatch applies the tolerances. Comparisons use exact rationals rather
// than float64 so a quantity like 0.1+0.2 can't trip a zero tolerance.
func classifyMatch(line MatchLine, quantityTolerance, priceTolerance float64) (string, string) {
	invoiced, err := floatToRat(line.InvoicedQuantity)
	if err != nil {
		return MatchQuantityVariance, "invalid invoiced quantity"
	}

	// Over-billing against goods actually received, counting other invoices.
	if line.ReceivedQuantity != nil {
		received, err1 := floatToRat(*line.ReceivedQuantity)
		previous, err2 := floatToRat(line.PreviouslyInvoiced)
		if err1 == nil && err2 == nil {
			billed := new(big.Rat).Add(previous, invoiced)
			allowed := withTolerance(received, quantityTolerance)
			if billed.Cmp(allowed) > 0 {
				return MatchOverReceived, fmt.Sprintf(
					"billing %s of %q but only %s received (%s already invoiced)",
					ratStr(invoiced), line.Description, ratStr(received), ratStr(previous),
				)
			}
		}
	}

	// Billing more than was ordered.
	if line.OrderedQuantity != nil {
		if ordered, err := floatToRat(*line.OrderedQuantity); err == nil {
			if invoiced.Cmp(withTolerance(ordered, quantityTolerance)) > 0 {
				return MatchQuantityVariance, fmt.Sprintf(
					"billing %s of %q but only %s ordered",
					ratStr(invoiced), line.Description, ratStr(ordered),
				)
			}
		}
	}

	// Unit price drift against the order.
	if line.OrderedUnitPrice != nil {
		ordered := new(big.Rat).SetInt64(*line.OrderedUnitPrice)
		billed := new(big.Rat).SetInt64(line.InvoicedUnitPrice)
		diff := new(big.Rat).Sub(billed, ordered)
		diff.Abs(diff)
		allowed := new(big.Rat).Mul(ordered, big.NewRat(int64(priceTolerance*1e6), 100*1e6))
		allowed.Abs(allowed)
		if diff.Cmp(allowed) > 0 {
			return MatchPriceVariance, fmt.Sprintf(
				"%q billed at %d but ordered at %d",
				line.Description, line.InvoicedUnitPrice, *line.OrderedUnitPrice,
			)
		}
	}

	return MatchMatched, ""
}

// withTolerance returns base scaled up by percent (e.g. 2 -> base * 1.02).
func withTolerance(base *big.Rat, percent float64) *big.Rat {
	if percent == 0 {
		return base
	}
	factor := new(big.Rat).Add(big.NewRat(1, 1), big.NewRat(int64(percent*1e6), 100*1e6))
	return new(big.Rat).Mul(base, factor)
}

func ratStr(r *big.Rat) string {
	return r.FloatString(2)
}

// unmatchedLines returns a human-readable message per line in variance —
// `unlinked` is informational only and never blocks.
func unmatchedLines(lines []MatchLine) []string {
	messages := []string{}
	for _, line := range lines {
		switch line.Status {
		case MatchMatched, MatchUnlinked:
			continue
		default:
			messages = append(messages, line.Message)
		}
	}
	return messages
}

func valueOrZero(v *float64) float64 {
	if v == nil {
		return 0
	}
	return *v
}
