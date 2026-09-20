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

	// The invoice and its linked purchase order each carry their own
	// currency+exchangeRate independently (nothing forces them to match).
	// Comparing raw cent amounts across two different currencies would be
	// comparing apples to oranges, so the price check below converts both
	// sides to organization-currency terms first — see db/exchange_rate.go.
	invoiceRate, err := parseExchangeRate(invoice.ExchangeRate)
	if err != nil {
		return nil, fmt.Errorf("get_incoming_invoice_match invoice rate: %w", err)
	}

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
			Quantity     float64 `db:"quantity"`
			UnitPrice    int64   `db:"unitPrice"`
			ExchangeRate *string `db:"exchangeRate"`
		}
		if err := d.DB.Get(&ordered, `
			SELECT poli.quantity AS quantity, poli.unitPrice AS unitPrice, po.exchangeRate AS exchangeRate
			FROM purchase_order_line_items poli
			JOIN purchase_orders po ON poli.purchaseOrderId = po.id
			WHERE poli.id = ?`, poLineID,
		); err != nil {
			line.Message = "the linked purchase order line no longer exists"
			lines = append(lines, line)
			continue
		}
		line.OrderedQuantity = &ordered.Quantity
		line.OrderedUnitPrice = &ordered.UnitPrice
		orderedRate, err := parseExchangeRate(ordered.ExchangeRate)
		if err != nil {
			return nil, fmt.Errorf("get_incoming_invoice_match order rate: %w", err)
		}

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

		// What other invoices already billed against this same order line —
		// without this, the same goods can be billed twice. Scoped to
		// state IN ('approved', 'paid'), matching grniClearedQtyForPOLine
		// (db/gl_posting.go) rather than != 'cancelled': a draft bill has no
		// AP obligation and hasn't touched GRNI yet, so counting it here
		// only produced a spurious variance against a draft that might be
		// edited or deleted before ever being approved. The actual
		// double-billing guard still holds at the point it matters — the
		// draft→approved transition itself re-evaluates this match, so a
		// second bill can't reach "approved" while a first one is already
		// there for the same goods (F53, 2026-08-13 audit).
		var previouslyInvoiced float64
		if err := d.DB.Get(&previouslyInvoiced, `
			SELECT COALESCE(SUM(li.quantity), 0)
			FROM incoming_invoice_line_items li
			JOIN incoming_invoices inv ON li.incomingInvoiceId = inv.id
			WHERE li.purchaseOrderLineItemId = ?
			  AND inv.id != ?
			  AND inv.state IN ('approved', 'paid')`,
			poLineID, invoiceID,
		); err != nil {
			return nil, fmt.Errorf("get_incoming_invoice_match previously_invoiced: %w", err)
		}
		line.PreviouslyInvoiced = previouslyInvoiced

		line.Status, line.Message = classifyMatch(line, quantityTolerance, priceTolerance, orderedRate, invoiceRate)
		lines = append(lines, line)
	}

	return lines, nil
}

// classifyMatch applies the tolerances. Comparisons use exact rationals rather
// than float64 so a quantity like 0.1+0.2 can't trip a zero tolerance.
//
// orderedRate/invoicedRate convert the order's and the invoice's own unit
// prices to organization-currency terms before the price check — the two
// documents each carry an independent currency, so comparing raw cents would
// silently compare two different currencies whenever they don't match.
func classifyMatch(
	line MatchLine, quantityTolerance, priceTolerance float64, orderedRate, invoicedRate *big.Rat,
) (string, string) {
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

	// Unit price drift against the order, compared in organization-currency
	// terms so a PO and invoice booked in different currencies aren't
	// compared as if they were the same one.
	if line.OrderedUnitPrice != nil {
		ordered := new(big.Rat).Mul(new(big.Rat).SetInt64(*line.OrderedUnitPrice), orderedRate)
		billed := new(big.Rat).Mul(new(big.Rat).SetInt64(line.InvoicedUnitPrice), invoicedRate)
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

// hasBlockingVariance mirrors src/types/incoming-invoice.ts's function of the
// same name — unlinked is informational only and never blocks.
func hasBlockingVariance(lines []MatchLine) bool {
	for _, line := range lines {
		if line.Status != MatchMatched && line.Status != MatchUnlinked {
			return true
		}
	}
	return false
}

// GetIncomingInvoiceMatchSummaries reports, for every purchase-order-linked
// incoming invoice in the organization, whether it currently has a blocking
// 3-way-match variance — the list page's only way to surface this without
// opening every invoice individually. Scoped to PO-linked invoices only (an
// invoice with no purchase order has every line "unlinked", which is
// informational and never a variance).
//
// F122: this is now set-based. It used to load every PO-linked invoice id and
// call GetIncomingInvoiceMatch once per invoice — and that function runs ~2
// queries plus ~3 per linked line, so the list page (which calls this on every
// load) paid ≈ N·(2+3L) serialized round-trips. It now fetches every invoice's
// lines plus the received/previously-invoiced aggregates for the whole
// organization in a small constant number of queries, then reuses the exact
// same classifyMatch/hasBlockingVariance helpers the per-invoice path uses, so
// each invoice's boolean is identical to GetIncomingInvoiceMatch's own result
// (see TestGetIncomingInvoiceMatchSummariesMatchesPerInvoice). GetIncomingInvoiceMatch
// itself is untouched — the single-invoice view still uses it.
func (d *Database) GetIncomingInvoiceMatchSummaries(organizationID string) (map[string]bool, error) {
	org, err := d.GetOrganization(organizationID)
	if err != nil {
		return nil, fmt.Errorf("get_incoming_invoice_match_summaries organization: %w", err)
	}
	priceTolerance := valueOrZero(org.MatchPriceTolerancePercent)
	quantityTolerance := valueOrZero(org.MatchQuantityTolerancePercent)

	// Query 1: one row per PO-linked invoice. The vendors join mirrors
	// GetIncomingInvoice's own select (an invoice is only ever returned, and
	// so only ever matched, if its vendor still exists).
	var invoices []struct {
		ID           string  `db:"id"`
		State        string  `db:"state"`
		ExchangeRate *string `db:"exchangeRate"`
	}
	if err := d.DB.Select(&invoices, `
		SELECT ii.id AS id, ii.state AS state, ii.exchangeRate AS exchangeRate
		FROM incoming_invoices ii
		INNER JOIN vendors v ON ii.vendorId = v.id
		WHERE ii.organizationId = ? AND ii.purchaseOrderId IS NOT NULL`,
		organizationID,
	); err != nil {
		return nil, fmt.Errorf("get_incoming_invoice_match_summaries invoices: %w", err)
	}

	// Query 2: every line of those invoices, left-joined to its linked PO line
	// and PO. A line whose PO line (or PO) is missing comes back with
	// poLineFound = 0 — the same "the linked purchase order line no longer
	// exists" unlinked result the per-invoice lookup produces for that case.
	type summaryLineRow struct {
		InvoiceID               string   `db:"invoiceId"`
		LineItemID              string   `db:"lineItemId"`
		PurchaseOrderLineItemID *string  `db:"purchaseOrderLineItemId"`
		Description             string   `db:"description"`
		Quantity                float64  `db:"quantity"`
		UnitPrice               int64    `db:"unitPrice"`
		OrderedQuantity         *float64 `db:"orderedQuantity"`
		OrderedUnitPrice        *int64   `db:"orderedUnitPrice"`
		OrderExchangeRate       *string  `db:"orderExchangeRate"`
		POLineFound             int      `db:"poLineFound"`
	}
	var lineRows []summaryLineRow
	if err := d.DB.Select(&lineRows, `
		SELECT li.incomingInvoiceId AS invoiceId,
		       li.id AS lineItemId,
		       li.purchaseOrderLineItemId AS purchaseOrderLineItemId,
		       li.description AS description,
		       li.quantity AS quantity,
		       li.unitPrice AS unitPrice,
		       poli.quantity AS orderedQuantity,
		       poli.unitPrice AS orderedUnitPrice,
		       po.exchangeRate AS orderExchangeRate,
		       CASE WHEN poli.id IS NULL OR po.id IS NULL THEN 0 ELSE 1 END AS poLineFound
		FROM incoming_invoice_line_items li
		JOIN incoming_invoices inv ON li.incomingInvoiceId = inv.id
		LEFT JOIN purchase_order_line_items poli ON li.purchaseOrderLineItemId = poli.id
		LEFT JOIN purchase_orders po ON poli.purchaseOrderId = po.id
		WHERE inv.organizationId = ? AND inv.purchaseOrderId IS NOT NULL
		ORDER BY li.incomingInvoiceId, li.position ASC`,
		organizationID,
	); err != nil {
		return nil, fmt.Errorf("get_incoming_invoice_match_summaries lines: %w", err)
	}

	// Query 3: what has actually been received, per order line.
	var receivedRows []struct {
		POLineID string  `db:"poLineId"`
		Quantity float64 `db:"quantity"`
	}
	if err := d.DB.Select(&receivedRows, `
		SELECT dli.purchaseOrderLineItemId AS poLineId, COALESCE(SUM(dli.quantity), 0) AS quantity
		FROM inbound_delivery_line_items dli
		JOIN inbound_deliveries idl ON dli.deliveryId = idl.id
		WHERE dli.purchaseOrderLineItemId IS NOT NULL AND idl.status = 'received'
		  AND idl.organizationId = ?
		GROUP BY dli.purchaseOrderLineItemId`,
		organizationID,
	); err != nil {
		return nil, fmt.Errorf("get_incoming_invoice_match_summaries received: %w", err)
	}
	receivedByPOLine := make(map[string]float64, len(receivedRows))
	for _, r := range receivedRows {
		receivedByPOLine[r.POLineID] = r.Quantity
	}

	// Query 4: what every approved/paid bill has already billed per order
	// line. The per-invoice path excludes the invoice under test with
	// `inv.id != ?`; that subtraction happens in Go below, where each
	// invoice's own billed quantities are already in hand.
	var billedRows []struct {
		POLineID string  `db:"poLineId"`
		Quantity float64 `db:"quantity"`
	}
	if err := d.DB.Select(&billedRows, `
		SELECT li.purchaseOrderLineItemId AS poLineId, COALESCE(SUM(li.quantity), 0) AS quantity
		FROM incoming_invoice_line_items li
		JOIN incoming_invoices inv ON li.incomingInvoiceId = inv.id
		WHERE li.purchaseOrderLineItemId IS NOT NULL AND inv.state IN ('approved', 'paid')
		  AND inv.organizationId = ?
		GROUP BY li.purchaseOrderLineItemId`,
		organizationID,
	); err != nil {
		return nil, fmt.Errorf("get_incoming_invoice_match_summaries previously_invoiced: %w", err)
	}
	billedByPOLine := make(map[string]float64, len(billedRows))
	for _, r := range billedRows {
		billedByPOLine[r.POLineID] = r.Quantity
	}

	// Group the line rows by invoice, and tally each invoice's own billed
	// quantity per order line (the term Query 4 can't exclude, since it isn't
	// scoped to one invoice).
	linesByInvoice := make(map[string][]summaryLineRow, len(invoices))
	ownQtyByInvoice := make(map[string]map[string]float64, len(invoices))
	for _, r := range lineRows {
		linesByInvoice[r.InvoiceID] = append(linesByInvoice[r.InvoiceID], r)
		if r.PurchaseOrderLineItemID != nil {
			own := ownQtyByInvoice[r.InvoiceID]
			if own == nil {
				own = map[string]float64{}
				ownQtyByInvoice[r.InvoiceID] = own
			}
			own[*r.PurchaseOrderLineItemID] += r.Quantity
		}
	}

	summaries := make(map[string]bool, len(invoices))
	for _, inv := range invoices {
		// Parsed for every invoice even when it has no lines, matching
		// GetIncomingInvoiceMatch's own unconditional parse.
		invoiceRate, err := parseExchangeRate(inv.ExchangeRate)
		if err != nil {
			return nil, fmt.Errorf("get_incoming_invoice_match_summaries invoice rate: %w", err)
		}

		rows := linesByInvoice[inv.ID]
		lines := make([]MatchLine, 0, len(rows))
		for _, r := range rows {
			line := MatchLine{
				LineItemID:          r.LineItemID,
				PurchaseOrderLineID: r.PurchaseOrderLineItemID,
				Description:         r.Description,
				InvoicedQuantity:    r.Quantity,
				InvoicedUnitPrice:   r.UnitPrice,
				Status:              MatchUnlinked,
			}

			if r.PurchaseOrderLineItemID == nil {
				line.Message = "not linked to a purchase order line"
				lines = append(lines, line)
				continue
			}
			if r.POLineFound == 0 {
				line.Message = "the linked purchase order line no longer exists"
				lines = append(lines, line)
				continue
			}
			poLineID := *r.PurchaseOrderLineItemID

			orderedRate, err := parseExchangeRate(r.OrderExchangeRate)
			if err != nil {
				return nil, fmt.Errorf("get_incoming_invoice_match_summaries order rate: %w", err)
			}
			orderedQuantity := *r.OrderedQuantity
			orderedUnitPrice := *r.OrderedUnitPrice
			line.OrderedQuantity = &orderedQuantity
			line.OrderedUnitPrice = &orderedUnitPrice

			received := receivedByPOLine[poLineID]
			line.ReceivedQuantity = &received

			previouslyInvoiced := billedByPOLine[poLineID]
			if inv.State == "approved" || inv.State == "paid" {
				previouslyInvoiced -= ownQtyByInvoice[inv.ID][poLineID]
			}
			line.PreviouslyInvoiced = previouslyInvoiced

			line.Status, line.Message = classifyMatch(line, quantityTolerance, priceTolerance, orderedRate, invoiceRate)
			lines = append(lines, line)
		}
		summaries[inv.ID] = hasBlockingVariance(lines)
	}
	return summaries, nil
}
