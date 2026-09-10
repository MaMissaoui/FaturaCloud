package main

import (
	"fmt"
	"math"
	"time"

	"github.com/MaMissaoui/fatura-cloud/db"
)

// purchaseLine is this tool's own record of one PO line, carrying the
// server-assigned PurchaseOrderLineItemID (fetched back after creation —
// CreatePurchaseOrder's response doesn't embed line items, see README.md)
// forward through receiving and billing, which is how the 3-way match
// links a receipt and a bill back to the same ordered line.
type purchaseLine struct {
	poLineID    string
	productID   string
	productName string
	unit        string
	quantity    float64
	unitCost    int64 // cents, what the vendor charges
}

// maybeStartPurchaseOrder mirrors maybeStartOrder's shape exactly: decide
// the week's volume on Monday, fan the individual POs out across the week's
// business days via the scheduler.
func (s *Seeder) maybeStartPurchaseOrder(day time.Time) error {
	if day.Weekday() != time.Monday {
		return nil
	}
	count := s.rng.IntRange(s.profile.PurchaseOrdersPerWeek[0], s.profile.PurchaseOrdersPerWeek[1])
	for i := 0; i < count; i++ {
		placeDay := businessDaysLater(day, s.rng.IntRange(0, 4))
		if placeDay.After(s.cfg.EndDate) {
			continue
		}
		s.sched.Schedule(placeDay, func() error { return s.createPurchaseOrder(placeDay) })
	}
	return nil
}

// createPurchaseOrder places and confirms one PO for 1-4 stock-enabled
// products (purchasing restocks inventory — a service has no purchasing
// side in this demo, same as the sales-side split in catalog.go), then
// schedules its eventual receipt.
func (s *Seeder) createPurchaseOrder(day time.Time) error {
	vendor := Pick(s.rng, s.vendors)
	stockProducts := s.stockProducts()
	if len(stockProducts) == 0 {
		return nil
	}

	n := s.rng.IntRange(1, 4)
	var reqLines []db.CreatePurchaseOrderLineItemRequest
	var localLines []purchaseLine
	for i := 0; i < n; i++ {
		p := Pick(s.rng, stockProducts)
		qty := float64(s.rng.IntRange(20, 150)) // restocking bulk, not a single-unit sale
		reqLines = append(reqLines, db.CreatePurchaseOrderLineItemRequest{
			ProductID:   strPtr(p.id),
			Description: p.name,
			Quantity:    qty,
			UnitPrice:   float64(p.costCents),
			Unit:        strPtr(p.unit),
		})
		localLines = append(localLines, purchaseLine{productID: p.id, productName: p.name, unit: p.unit, quantity: qty, unitCost: p.costCents})
	}

	req := db.CreatePurchaseOrderRequest{
		OrganizationID: s.orgID,
		VendorID:       &vendor.id,
		OrderNumber:    s.poNum.next(day.Year()),
		Status:         "draft",
		OrderDate:      midnightUTC(day),
		LineItems:      reqLines,
	}
	var po db.PurchaseOrder
	if err := s.c.Post("/api/purchase-orders", req, &po); err != nil {
		return fmt.Errorf("create PO for %s: %w", vendor.name, err)
	}
	s.stats.PurchaseOrders++
	if err := s.c.Patch("/api/purchase-orders/"+po.ID+"/status", map[string]string{"status": "confirmed"}, nil); err != nil {
		return fmt.Errorf("confirm PO %s: %w", po.OrderNumber, err)
	}

	var serverLines []db.PurchaseOrderLineItem
	if err := s.c.Get("/api/purchase-orders/"+po.ID+"/line-items", &serverLines); err != nil {
		return fmt.Errorf("read back PO %s line items: %w", po.OrderNumber, err)
	}
	if len(serverLines) != len(localLines) {
		return fmt.Errorf("PO %s: expected %d line items back, got %d", po.OrderNumber, len(localLines), len(serverLines))
	}
	for i := range localLines {
		localLines[i].poLineID = serverLines[i].ID
	}

	receiveDay := businessDaysLater(day, s.rng.IntRange(3, 14))
	if receiveDay.After(s.cfg.EndDate) {
		return nil // would arrive in the future — PO stays confirmed, not received
	}
	s.sched.Schedule(receiveDay, func() error { return s.receivePurchaseOrder(receiveDay, po, vendor, localLines) })
	return nil
}

// receivePurchaseOrder creates the goods receipt (usually for the full
// ordered quantity, sometimes a partial shipment — real vendors don't
// always ship everything at once), marks it received (which posts the GRNI
// accrual and raises stock server-side), updates this tool's own onHand
// mirror, and schedules the vendor's bill.
func (s *Seeder) receivePurchaseOrder(day time.Time, po db.PurchaseOrder, vendor vendorRef, lines []purchaseLine) error {
	var reqLines []db.CreateInboundDeliveryLineItemRequest
	received := make([]purchaseLine, len(lines))
	for i, l := range lines {
		qty := l.quantity
		if s.rng.Chance(0.15) {
			// Whole units only — every product here is "pcs"/"m"/"roll"
			// bulk stock, never fractional, and a whole quantity also
			// keeps this exactly reproducible against the server's own
			// math/big line-total rounding (see money.go's computeTotals
			// doc comment); a fractional float64 quantity hit a rounding
			// tie against the server on a couple of orders in a full
			// 18-month run before this fix.
			qty = math.Round(l.quantity * s.rng.Float64Range(0.7, 0.95))
			if qty < 1 {
				qty = 1
			}
		}
		received[i] = l
		received[i].quantity = qty
		reqLines = append(reqLines, db.CreateInboundDeliveryLineItemRequest{
			PurchaseOrderLineItemID: strPtr(l.poLineID),
			ProductID:               strPtr(l.productID),
			Description:             l.productName,
			Quantity:                qty,
			UnitCost:                float64Ptr(float64(l.unitCost)),
			Unit:                    strPtr(l.unit),
		})
	}

	req := db.CreateInboundDeliveryRequest{
		OrganizationID:  s.orgID,
		PurchaseOrderID: &po.ID,
		VendorID:        &vendor.id,
		DeliveryNumber:  s.inboundNum.next(day.Year()),
		DeliveryDate:    midnightUTC(day),
		LineItems:       reqLines,
	}
	var delivery db.InboundDelivery
	if err := s.c.Post("/api/inbound-deliveries", req, &delivery); err != nil {
		return fmt.Errorf("create receipt for PO %s: %w", po.OrderNumber, err)
	}
	s.stats.InboundDeliveries++
	if err := s.c.Patch("/api/inbound-deliveries/"+delivery.ID+"/status", map[string]string{"status": "received"}, nil); err != nil {
		return fmt.Errorf("mark receipt %s received: %w", delivery.DeliveryNumber, err)
	}
	if err := s.c.Patch("/api/purchase-orders/"+po.ID+"/status", map[string]string{"status": "received"}, nil); err != nil {
		return fmt.Errorf("mark PO %s received: %w", po.OrderNumber, err)
	}
	for _, l := range received {
		s.adjustOnHand(l.productID, l.quantity)
	}

	billDay := businessDaysLater(day, s.rng.IntRange(0, 7))
	if billDay.After(s.cfg.EndDate) {
		return nil
	}
	s.sched.Schedule(billDay, func() error { return s.billPurchaseOrder(billDay, po, vendor, received) })
	return nil
}

// billPurchaseOrder creates the vendor's incoming invoice for what was
// actually received, approves it (posting GL and clearing GRNI), and
// schedules its eventual payment. ~12% of bills get a deliberate price or
// quantity variance against the PO/receipt — enough to exercise the 3-way
// match panel without making every single bill an exception.
func (s *Seeder) billPurchaseOrder(day time.Time, po db.PurchaseOrder, vendor vendorRef, lines []purchaseLine) error {
	variance := s.rng.Chance(0.12)
	var items []lineItem
	var reqLines []db.CreateInvoiceLineItemRequest
	for _, l := range lines {
		qty := l.quantity
		unitCost := l.unitCost
		if variance {
			if s.rng.Chance(0.5) {
				unitCost = int64(float64(unitCost) * s.rng.Float64Range(1.03, 1.08))
			} else if qty > 1 {
				qty--
			}
		}
		reqLines = append(reqLines, db.CreateInvoiceLineItemRequest{
			Description:             strPtr(l.productName),
			Quantity:                qty,
			UnitPrice:               float64(unitCost),
			PurchaseOrderLineItemID: strPtr(l.poLineID),
			ProductID:               strPtr(l.productID),
		})
		items = append(items, lineItem{quantity: qty, unitPriceCents: unitCost})
	}
	subTotal, taxTotal, total := computeTotals(items)
	dueDate := midnightUTC(day.AddDate(0, 0, 30))

	req := db.CreateIncomingInvoiceRequest{
		OrganizationID:      s.orgID,
		VendorID:            vendor.id,
		PurchaseOrderID:     &po.ID,
		VendorInvoiceNumber: s.incomingNum.next(day.Year()),
		State:               "draft",
		Date:                midnightUTC(day),
		DueDate:             &dueDate,
		Currency:            "EUR",
		Total:               total,
		TaxTotal:            taxTotal,
		SubTotal:            subTotal,
		LineItems:           reqLines,
	}
	if variance {
		req.MatchOverride = 1
		req.MatchOverrideReason = strPtr("Seed data: accepted a routine PO/receipt variance from this vendor.")
	}

	var bill db.IncomingInvoice
	if err := s.c.Post("/api/incoming-invoices", req, &bill); err != nil {
		return fmt.Errorf("create bill for PO %s: %w", po.OrderNumber, err)
	}
	s.stats.IncomingInvoices++

	// matchOverride/matchOverrideReason are read from the row itself (set
	// above at creation), not from this PATCH's body — see
	// api/incoming_invoices.go's updateIncomingInvoiceState, which only
	// reads {state}.
	if err := s.c.Patch("/api/incoming-invoices/"+bill.ID+"/state", map[string]string{"state": "approved"}, nil); err != nil {
		return fmt.Errorf("approve bill %s: %w", bill.VendorInvoiceNumber, err)
	}

	if s.rng.Chance(0.95) {
		// See sales.go's createDirectInvoice comment on why the unpaid
		// share is kept small — it compounds across the whole run.
		payDay := businessDaysLater(day, s.rng.IntRange(5, 30))
		s.schedulePayment(payDay, bill.ID, total, vendor.id, "incoming_invoice")
	}
	return nil
}

// stockProducts excludes "finished" goods (catalog.go) — assembled, not
// bought from a vendor. The sales-side mirror is sellableProducts
// (sales.go), which excludes "component" instead.
func (s *Seeder) stockProducts() []productRef {
	var out []productRef
	for _, p := range s.products {
		if p.stockEnabled && p.category != "finished" {
			out = append(out, p)
		}
	}
	return out
}
