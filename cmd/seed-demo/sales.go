package main

import (
	"fmt"
	"time"

	"github.com/MaMissaoui/fatura-cloud/db"
)

// generateSalesForDay creates today's batch of direct sales invoices — the
// bulk of this tool's document volume. "Direct" means billed straight to
// the client with no order/delivery in front of it, the common case for a
// service-heavy small business; maybeStartOrder below covers the smaller
// stock-fulfillment path (order -> delivery -> invoice) that actually moves
// inventory.
func (s *Seeder) generateSalesForDay(day time.Time, weekend bool) error {
	rng := s.profile.InvoicesPerWeekday
	if weekend {
		rng = s.profile.InvoicesPerWeekendDay
	}
	count := s.rng.IntRange(rng[0], rng[1])
	for i := 0; i < count; i++ {
		if err := s.createDirectInvoice(day); err != nil {
			return err
		}
	}
	return nil
}

// createDirectInvoice creates one invoice, sends it, then schedules its
// eventual fate (paid in full, paid in two installments, left outstanding,
// or cancelled) via the scheduler — see the package doc comment on
// Scheduler for why this is the mechanism every multi-step flow uses.
func (s *Seeder) createDirectInvoice(day time.Time) error {
	client := Pick(s.rng, s.clients)
	lines := s.randomInvoiceLines(s.rng.IntRange(1, 5))

	subTotal, taxTotal, total := computeTotals(lines.totals)
	dueDate := midnightUTC(day.AddDate(0, 0, int(s.orgProfile.dueDays)))

	req := db.CreateInvoiceRequest{
		OrganizationID: s.orgID,
		Number:         s.invoiceNum.next(day.Year()),
		State:          "draft",
		ClientID:       client.id,
		Date:           midnightUTC(day),
		DueDate:        &dueDate,
		Currency:       s.cfg.Currency,
		Total:          total,
		TaxTotal:       taxTotal,
		SubTotal:       subTotal,
		LineItems:      lines.items,
		PaymentTerms:   strPtr(fmt.Sprintf("Net %d days", s.orgProfile.dueDays)),
	}
	var inv db.Invoice
	if err := s.c.Post("/api/invoices", req, &inv); err != nil {
		return fmt.Errorf("create invoice for %s: %w", client.name, err)
	}
	s.stats.Invoices++

	if err := s.c.Patch("/api/invoices/"+inv.ID+"/state", map[string]string{"state": "sent"}, nil); err != nil {
		return fmt.Errorf("send invoice %s: %w", inv.Number, err)
	}

	// Target shares: 3% cancelled, 80% paid in full, 13% paid via two
	// partials, ~4% that don't pay on the normal schedule — split between a
	// slow-paying client eventually collected well past due (~3.2%) and
	// genuine permanent bad debt (~0.8%). That last figure is deliberately
	// small and, unlike the slow-pay bucket, never resolves: every invoice
	// in it stays outstanding for the rest of the 18-month run, so even a
	// modest per-invoice rate compounds. (First tuned after generating a
	// full run at 17% never-paid and finding the 90+ bucket had grown to
	// several million euros; a later run at a flat 4% never-paid still put
	// ~38% of all outstanding AR value in the 90+ bucket — because paid
	// invoices keep leaving the outstanding set while a permanent bucket
	// never does, so it dominates the snapshot even at a small headline
	// rate. Splitting most of that 4% into "slow but eventually collected"
	// keeps the aging report's 31-90 buckets populated with real texture
	// instead of an ever-accumulating tail.) Each `case` below is a
	// conditional probability given every earlier case was false, chosen
	// so the unconditional shares work out to the targets above.
	switch {
	case s.rng.Chance(0.03):
		// A small fraction never go out at all — cancelled shortly after
		// issuing (a client-side change of mind, a duplicate caught late).
		return s.c.Patch("/api/invoices/"+inv.ID+"/state", map[string]string{"state": "cancelled"}, nil)

	case s.rng.Chance(0.80 / 0.97):
		// Paid in full, sometime between issue and typically a bit past the
		// due date (some early, most on time, some late) — a realistic AR
		// aging spread rather than everything paying exactly on day 14.
		payDay := businessDaysLater(day, s.rng.IntRange(3, 40))
		s.schedulePayment(payDay, inv.ID, total, client.id, "invoice")

	case s.rng.Chance(0.13 / (0.97 - 0.80)):
		// Two partial payments — exercises partial-payment/aging display.
		first := total / 2
		second := total - first
		firstDay := businessDaysLater(day, s.rng.IntRange(5, 20))
		secondDay := businessDaysLater(firstDay, s.rng.IntRange(5, 25))
		s.schedulePayment(firstDay, inv.ID, first, client.id, "invoice")
		s.schedulePayment(secondDay, inv.ID, second, client.id, "invoice")

	case s.rng.Chance(0.80):
		// A slow-paying client — genuinely collected eventually, just well
		// past due (60-150 business days), not written off. This is the
		// bucket that gives the 31-60/61-90/90+ aging buckets real content
		// without it accumulating forever the way the permanent-bad-debt
		// default case below does.
		payDay := businessDaysLater(day, s.rng.IntRange(60, 150))
		s.schedulePayment(payDay, inv.ID, total, client.id, "invoice")

	default:
		// Genuinely never paid — permanent bad debt/write-off, ~0.8% of all
		// invoices. Left outstanding for the rest of the run.
	}
	return nil
}

// schedulePayment queues a CreatePayment call for payDay if payDay is still
// within the simulated range — a payment that would fall after --end-date
// is simply never made, leaving the document genuinely outstanding as of
// "today" rather than back-dating a payment into the future.
func (s *Seeder) schedulePayment(payDay time.Time, documentID string, amount int64, partnerID, documentType string) {
	if payDay.After(s.cfg.EndDate) {
		return
	}
	s.sched.Schedule(payDay, func() error {
		direction, clientID, vendorID := "inbound", &partnerID, (*string)(nil)
		if documentType == "incoming_invoice" {
			direction, clientID, vendorID = "outbound", nil, &partnerID
		}
		req := db.CreatePaymentRequest{
			OrganizationID: s.orgID,
			Direction:      direction,
			ClientID:       clientID,
			VendorID:       vendorID,
			BankAccountID:  s.cashAccountID,
			Amount:         amount,
			Currency:       s.cfg.Currency,
			Date:           midnightUTC(payDay),
			Method:         Pick(s.rng, paymentMethods),
			Applications: []db.CreatePaymentApplicationRequest{
				{DocumentType: documentType, DocumentID: documentID, Amount: amount},
			},
		}
		if err := s.c.Post("/api/payments", req, nil); err != nil {
			return fmt.Errorf("payment for %s %s: %w", documentType, documentID, err)
		}
		s.stats.Payments++
		return nil
	})
}

var paymentMethods = []string{"bank_transfer", "bank_transfer", "bank_transfer", "card", "direct_debit", "cash"}

// invoiceLines bundles the API request shape (db types, cents) with the
// plain lineItem shape money.go's computeTotals needs — built together so
// the two can never drift out of sync with each other.
type invoiceLines struct {
	items  []db.CreateInvoiceLineItemRequest
	totals []lineItem
}

// orderLineSet is CreateOrderRequest's own line-item request type — no
// totals alongside it, since CreateOrderRequest has no Total/SubTotal/
// TaxTotal fields to validate against in the first place (orders compute
// their totals only at export time — see db/xlsx_export_order.go). The
// eventual shipped-order invoice computes its own totals independently in
// shipOrder, since OrderLineItem carries no tax rate to reuse.
type orderLineSet struct {
	items []db.CreateOrderLineItemRequest
}

// sellableProducts excludes "component" products (catalog.go) — bought
// from vendors to assemble a finished good, never sold directly to a
// client. Services (category == "") and "finished" goods both remain
// sellable. The purchasing-side mirror is stockProducts (purchasing.go),
// which excludes "finished" instead.
func (s *Seeder) sellableProducts() []productRef {
	out := make([]productRef, 0, len(s.products))
	for _, p := range s.products {
		if p.category != "component" {
			out = append(out, p)
		}
	}
	return out
}

// randomInvoiceLines picks n distinct-ish products (repeats are fine and
// realistic — "2 line items of the same consulting rate" happens) and
// random quantities appropriate to whether it's a per-hour service or a
// physical unit.
func (s *Seeder) randomInvoiceLines(n int) invoiceLines {
	sellable := s.sellableProducts()
	out := invoiceLines{}
	for i := 0; i < n; i++ {
		p := Pick(s.rng, sellable)
		qty := s.quantityFor(p)
		taxPercent := s.taxPercentFor(p.taxRateID)

		out.items = append(out.items, db.CreateInvoiceLineItemRequest{
			Description: strPtr(p.name),
			Quantity:    qty,
			UnitPrice:   float64(p.priceCents),
			TaxRate:     strPtr(p.taxRateID),
			ProductID:   strPtr(p.id),
		})
		out.totals = append(out.totals, lineItem{quantity: qty, unitPriceCents: p.priceCents, taxPercent: taxPercent})
	}
	return out
}

func (s *Seeder) quantityFor(p productRef) float64 {
	if p.qtyHi > 0 {
		return float64(s.rng.IntRange(p.qtyLo, p.qtyHi))
	}
	if p.stockEnabled {
		return float64(s.rng.IntRange(1, 12)) // catalog.go's stock entries don't set qtyLo/qtyHi — low per-unit prices make this range realistic as-is
	}
	return float64(s.rng.IntRange(1, 24))
}

func (s *Seeder) taxPercentFor(taxRateID string) float64 {
	switch taxRateID {
	case s.standardTax.id:
		return s.standardTax.percent
	case s.reducedTax.id:
		return s.reducedTax.percent
	default:
		return 0
	}
}

// --- order -> delivery -> invoice (the stock-moving sales path) ----------

// maybeStartOrder decides, once per week (Monday), how many sales orders to
// place that week and spreads them across the week's remaining business
// days via the scheduler — the same "decide the week's volume on Monday,
// let the scheduler fan it out" shape maybeStartPurchaseOrder uses.
func (s *Seeder) maybeStartOrder(day time.Time) error {
	if day.Weekday() != time.Monday {
		return nil
	}
	count := s.rng.IntRange(s.profile.OrdersPerWeek[0], s.profile.OrdersPerWeek[1])
	for i := 0; i < count; i++ {
		startDay := businessDaysLater(day, s.rng.IntRange(0, 4))
		if startDay.After(s.cfg.EndDate) {
			continue
		}
		s.sched.Schedule(startDay, func() error { return s.createOrder(startDay) })
	}
	return nil
}

// createOrder runs the whole order -> confirm -> deliver -> invoice chain.
// Line items are restricted to products with enough tracked on-hand stock
// (see productRef.onHand, kept in step with purchasing.go's receipts) so a
// delivery's UpdateDeliveryStatus("shipped") call — which really does check
// stockQuantity server-side — never gets asked to ship more than exists.
// Early in the simulated range, before purchasing has built up any stock,
// this naturally falls back to service lines instead.
func (s *Seeder) createOrder(day time.Time) error {
	client := Pick(s.rng, s.clients)
	items := s.orderableLines(s.rng.IntRange(1, 4))
	if len(items.items) == 0 {
		return nil // nothing shippable or billable yet this early — skip, not an error
	}

	req := db.CreateOrderRequest{
		OrganizationID: s.orgID,
		ClientID:       &client.id,
		OrderNumber:    s.orderNum.next(day.Year()),
		Status:         "draft",
		OrderDate:      midnightUTC(day),
		LineItems:      items.items,
	}
	var order db.Order
	if err := s.c.Post("/api/orders", req, &order); err != nil {
		return fmt.Errorf("create order for %s: %w", client.name, err)
	}
	s.stats.Orders++
	if err := s.c.Patch("/api/orders/"+order.ID+"/status", map[string]string{"status": "confirmed"}, nil); err != nil {
		return fmt.Errorf("confirm order %s: %w", order.OrderNumber, err)
	}

	var orderLines []db.OrderLineItem
	if err := s.c.Get("/api/orders/"+order.ID+"/line-items", &orderLines); err != nil {
		return fmt.Errorf("read back order %s line items: %w", order.OrderNumber, err)
	}

	shipDay := businessDaysLater(day, s.rng.IntRange(1, 6))
	if shipDay.After(s.cfg.EndDate) {
		return nil // would ship in the future — leave the order confirmed, not shipped
	}
	s.sched.Schedule(shipDay, func() error {
		return s.shipOrder(shipDay, order, orderLines, client)
	})
	return nil
}

func (s *Seeder) shipOrder(day time.Time, order db.Order, orderLines []db.OrderLineItem, client clientRef) error {
	deliveryItems := make([]db.CreateDeliveryLineItemRequest, len(orderLines))
	for i, ol := range orderLines {
		deliveryItems[i] = db.CreateDeliveryLineItemRequest{
			OrderLineItemID: &ol.ID,
			ProductID:       ol.ProductID,
			Description:     ol.Description,
			Quantity:        ol.Quantity,
		}
	}
	req := db.CreateDeliveryRequest{
		OrganizationID: s.orgID,
		OrderID:        &order.ID,
		DeliveryNumber: s.deliveryNum.next(day.Year()),
		DeliveryDate:   midnightUTC(day),
		LineItems:      deliveryItems,
	}
	var delivery db.OutboundDelivery
	if err := s.c.Post("/api/deliveries", req, &delivery); err != nil {
		return fmt.Errorf("create delivery for order %s: %w", order.OrderNumber, err)
	}
	s.stats.Deliveries++
	if err := s.c.Patch("/api/deliveries/"+delivery.ID+"/status", map[string]string{"status": "shipped"}, nil); err != nil {
		return fmt.Errorf("ship delivery %s: %w", delivery.DeliveryNumber, err)
	}
	// Stock was already reserved in our own estimate at order creation
	// (orderableLines) — not decremented again here, or a stock-enabled
	// line would be double-counted against onHand.

	// Close the loop financially: bill the client for what was just shipped.
	// There's no structural order->invoice link in this schema (sales
	// invoice line items only carry productId/purchaseOrderLineItemId, no
	// orderLineItemId — see cmd/seed-demo/README.md), so this is a normal,
	// independent CreateInvoice call for the same lines. Tax is resolved
	// fresh from each product here — OrderLineItem itself carries no tax
	// rate at all (CLAUDE.md: "order.taxTotal — always 0.00"), but the
	// invoice that actually bills the client should still charge VAT like
	// every other sales invoice does.
	var totals []lineItem
	invLines := make([]db.CreateInvoiceLineItemRequest, len(orderLines))
	for i, ol := range orderLines {
		taxRateID, taxPercent := "", 0.0
		if ol.ProductID != nil {
			if p, ok := s.productByID(*ol.ProductID); ok {
				taxRateID, taxPercent = p.taxRateID, s.taxPercentFor(p.taxRateID)
			}
		}
		invLines[i] = db.CreateInvoiceLineItemRequest{
			Description: strPtr(ol.Description),
			Quantity:    ol.Quantity,
			UnitPrice:   float64(ol.UnitPrice),
			ProductID:   ol.ProductID,
		}
		if taxRateID != "" {
			invLines[i].TaxRate = strPtr(taxRateID)
		}
		totals = append(totals, lineItem{quantity: ol.Quantity, unitPriceCents: ol.UnitPrice, taxPercent: taxPercent})
	}
	subTotal, taxTotal, total := computeTotals(totals)
	dueDate := midnightUTC(day.AddDate(0, 0, int(s.orgProfile.dueDays)))
	invReq := db.CreateInvoiceRequest{
		OrganizationID: s.orgID,
		Number:         s.invoiceNum.next(day.Year()),
		State:          "draft",
		ClientID:       client.id,
		Date:           midnightUTC(day),
		DueDate:        &dueDate,
		Currency:       s.cfg.Currency,
		Total:          total,
		TaxTotal:       taxTotal,
		SubTotal:       subTotal,
		LineItems:      invLines,
		PaymentTerms:   strPtr(fmt.Sprintf("Net %d days", s.orgProfile.dueDays)),
	}
	var inv db.Invoice
	if err := s.c.Post("/api/invoices", invReq, &inv); err != nil {
		return fmt.Errorf("invoice for shipped order %s: %w", order.OrderNumber, err)
	}
	s.stats.Invoices++
	if err := s.c.Patch("/api/invoices/"+inv.ID+"/state", map[string]string{"state": "sent"}, nil); err != nil {
		return fmt.Errorf("send order invoice %s: %w", inv.Number, err)
	}
	switch {
	case s.rng.Chance(0.95):
		payDay := businessDaysLater(day, s.rng.IntRange(5, 35))
		s.schedulePayment(payDay, inv.ID, total, client.id, "invoice")
	case s.rng.Chance(0.8):
		// Slow-paying client, eventually collected — see createDirectInvoice's
		// comment on why this bucket exists instead of a flat never-paid share.
		payDay := businessDaysLater(day, s.rng.IntRange(60, 150))
		s.schedulePayment(payDay, inv.ID, total, client.id, "invoice")
	default:
		// Genuinely never paid (~1% of shipped-order invoices).
	}
	return nil
}

// orderableLines picks n lines from stock-enabled products that currently
// have on-hand quantity, falling back to service products when nothing
// does (see createOrder's doc comment).
func (s *Seeder) orderableLines(n int) orderLineSet {
	sellable := s.sellableProducts()
	out := orderLineSet{}
	var inStock []productRef
	for _, p := range sellable {
		if p.stockEnabled && p.onHand >= 1 {
			inStock = append(inStock, p)
		}
	}
	for i := 0; i < n; i++ {
		var p productRef
		if len(inStock) > 0 {
			p = Pick(s.rng, inStock)
		} else {
			var services []productRef
			for _, sp := range sellable {
				if !sp.stockEnabled {
					services = append(services, sp)
				}
			}
			if len(services) == 0 {
				continue
			}
			p = Pick(s.rng, services)
		}
		qty := s.quantityFor(p)
		if p.stockEnabled {
			max := s.onHand(p.id)
			if max < 1 {
				continue
			}
			if qty > max {
				qty = max
			}
			// Reserve now, at order creation, not at shipOrder's eventual
			// ship day — two orders placed on the same or nearby days
			// against the same low-stock finished good (see production.go)
			// would otherwise both see the same pre-reservation onHand and
			// both get confirmed for it, and only the first to actually
			// ship would succeed; the real server has no reservation
			// concept either (an order can be confirmed for more than is
			// in stock — only shipping enforces it), so this is this
			// tool's own bookkeeping catching a race the real app doesn't
			// prevent, not a mismatch with how orders actually behave.
			s.adjustOnHand(p.id, -qty)
		}
		out.items = append(out.items, db.CreateOrderLineItemRequest{
			ProductID:   strPtr(p.id),
			Description: p.name,
			Quantity:    qty,
			UnitPrice:   float64(p.priceCents),
		})
	}
	return out
}
