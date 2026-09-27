package main

import (
	"fmt"
	"sort"

	"github.com/MaMissaoui/fatura-cloud/db"
)

// Since v3.57.0 a Cash Book sale takes its stock-tracked lines out of stock
// server-side (db/invoice_stock.go). The retail scenario was written before
// that, when counter sales never touched stock, so it bought far less than
// it sold: a trial run left ~70% of products with negative stock. This file
// keeps the retail scenario's stock realistic, the way a real shop runs:
//
//   - setupRetailOpeningStock records an opening count for every stock
//     product at its catalog cost when the run starts.
//   - retailSaleLines only sells what the local on-hand mirror (stock.go)
//     says is on the shelf, and every sale takes it off the mirror.
//   - retailReorderLines makes retail purchase orders restock every product
//     at or below a reorder point (counting what's already on order), back
//     up to a target level, instead of random ones in moto-sized bulk.
//
// None of this applies to the moto scenario, whose sales go through
// deliveries that already respect on-hand stock (sales.go).

// retailOpeningStock is the opening count per stock product — a small
// appliance shop keeps a few units of each model on the floor/in the back.
var retailOpeningStock = [2]int{3, 8}

// retailRestockTarget is the level a reorder brings a product back up to.
var retailRestockTarget = [2]int{5, 10}

func (s *Seeder) setupRetailOpeningStock() error {
	reference := "Stock initial"
	note := "Opening stock (seed-demo)"
	for _, p := range s.stockProducts() {
		qty := float64(s.rng.IntRange(retailOpeningStock[0], retailOpeningStock[1]))
		req := db.CreateStockMovementRequest{
			OrganizationID: s.orgID,
			ProductID:      p.id,
			Type:           "count_addition",
			Quantity:       qty,
			UnitCost:       int64Ptr(p.costCents),
			Note:           &note,
			Reference:      &reference,
		}
		if err := s.c.Post("/api/stock-movements", req, nil); err != nil {
			return fmt.Errorf("opening stock for %s: %w", p.name, err)
		}
		s.adjustOnHand(p.id, qty)
	}
	return nil
}

// retailSaleLines builds n counter-sale lines from products actually on the
// shelf (services are always available). Stock products are reserved as
// lines are added, so one sale never takes more than is on hand. If the
// whole shop is sold out it falls back to any sellable product — the server
// allows a sale to take stock negative, but this keeps it rare.
func (s *Seeder) retailSaleLines(n int) invoiceLines {
	reserved := map[string]float64{}
	available := func(p productRef) float64 { return s.onHand(p.id) - reserved[p.id] }

	out := invoiceLines{}
	for i := 0; i < n; i++ {
		var candidates []productRef
		for _, p := range s.sellableProducts() {
			if !p.stockEnabled || available(p) >= 1 {
				candidates = append(candidates, p)
			}
		}
		if len(candidates) == 0 {
			candidates = s.sellableProducts()
		}
		p := Pick(s.rng, candidates)
		qty := s.quantityFor(p)
		if p.stockEnabled {
			if avail := available(p); avail >= 1 && qty > avail {
				qty = avail
			}
			reserved[p.id] += qty
		}
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

// takeRetailSaleStock mirrors a recorded sale's stock-out in the local
// on-hand estimate.
func (s *Seeder) takeRetailSaleStock(items []db.CreateInvoiceLineItemRequest) {
	for _, item := range items {
		if item.ProductID == nil {
			continue
		}
		if p, ok := s.productByID(*item.ProductID); ok && p.stockEnabled {
			s.adjustOnHand(p.id, -item.Quantity)
		}
	}
}

// retailReorderPoint is the (on hand + on order) level at or below which a
// product is restocked, and retailMaxReorderLines caps one purchase order.
const (
	retailReorderPoint    = 2
	retailMaxReorderLines = 10
)

// retailReorderLines returns every product at or below the reorder point
// (counting what's already on order), lowest first, capped at
// retailMaxReorderLines — the reorder-point restocking a real shop does.
// Empty means nothing needs ordering, and the caller places no order.
func (s *Seeder) retailReorderLines() []productRef {
	level := func(p productRef) float64 { return s.onHand(p.id) + s.onOrder[p.id] }
	var low []productRef
	for _, p := range s.stockProducts() {
		if level(p) <= retailReorderPoint {
			low = append(low, p)
		}
	}
	sort.SliceStable(low, func(i, j int) bool { return level(low[i]) < level(low[j]) })
	if len(low) > retailMaxReorderLines {
		low = low[:retailMaxReorderLines]
	}
	return low
}

func (s *Seeder) retailReorderQuantity(p productRef) float64 {
	target := float64(s.rng.IntRange(retailRestockTarget[0], retailRestockTarget[1]))
	qty := target - s.onHand(p.id) - s.onOrder[p.id]
	if qty < 2 {
		qty = 2
	}
	return qty
}
