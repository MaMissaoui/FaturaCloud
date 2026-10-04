package main

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/MaMissaoui/fatura-cloud/db"
)

// Since v3.57.0 a Cash Book sale takes its stock-tracked lines out of stock
// server-side (db/invoice_stock.go). The retail scenario was written before
// that, when counter sales never touched stock, so it bought far less than
// it sold: a trial run left ~70% of products with negative stock. This file
// keeps the retail scenario's stock realistic, the way a real shop runs:
//
//   - setupRetailOpeningStock buys the opening stock with dated purchase
//     orders on the run's first day.
//   - retailSaleLines only sells what the local on-hand mirror (stock.go)
//     says is on the shelf, and every sale takes it off the mirror.
//   - retailReorderLines makes retail purchase orders restock every product
//     at or below a reorder point (counting what's already on order), back
//     up to a target level, instead of random ones in moto-sized bulk.
//
// None of this applies to the moto scenario, whose sales go through
// deliveries that already respect on-hand stock (sales.go).

// retailOpeningStock is the opening quantity per stock product — a small
// appliance shop keeps a few units of each model on the floor/in the back.
var retailOpeningStock = [2]int{3, 8}

// retailRestockTarget is the level a reorder brings a product back up to,
// before the season's weight for the product (retailSeasonalTarget).
var retailRestockTarget = [2]int{5, 10}

// setupRetailOpeningStock stocks the shop on the run's first day: the owner
// puts capital into the bank (an opening journal entry) and buys the opening
// stock from the vendors with ordinary purchase orders, received that day and
// billed and paid like any other. Buying it through dated documents, rather
// than an opening stock count (which the server posts at the real current
// time), keeps the Inventory account right on every past date: the sales of
// the first months take stock out of stock that was already there.
func (s *Seeder) setupRetailOpeningStock(day time.Time) error {
	products := s.stockProducts()
	if len(products) == 0 || len(s.vendors) == 0 {
		return nil
	}
	// Each vendor supplies a share of the catalog, by appliance kind.
	byVendor := map[int][]productRef{}
	kinds := map[string]int{}
	for _, p := range products {
		v, ok := kinds[p.kind]
		if !ok {
			v = len(kinds) % len(s.vendors)
			kinds[p.kind] = v
		}
		byVendor[v] = append(byVendor[v], p)
	}

	var value int64
	type opening struct {
		vendor vendorRef
		lines  []purchaseLine
	}
	var orders []opening
	for v := 0; v < len(s.vendors); v++ {
		if len(byVendor[v]) == 0 {
			continue
		}
		o := opening{vendor: s.vendors[v]}
		for _, p := range byVendor[v] {
			qty := math.Round(float64(s.rng.IntRange(retailOpeningStock[0], retailOpeningStock[1])) * math.Min(kindWeight(p.kind, day), 2))
			if p.serialized {
				qty = float64(s.rng.IntRange(2, 4))
			}
			if qty < 1 {
				qty = 1
			}
			o.lines = append(o.lines, purchaseLine{productID: p.id, productName: p.name, unit: p.unit, quantity: qty, unitCost: p.costCents})
			value += int64(qty) * p.costCents
		}
		orders = append(orders, o)
	}

	// The capital covers the stock with a margin for the first months'
	// running costs, rounded to a thousand dinars.
	capital := (value*12/10 + 5000000) / 100000 * 100000
	if err := s.recordCapitalContribution(day, capital); err != nil {
		return fmt.Errorf("opening capital: %w", err)
	}
	for _, o := range orders {
		po, err := s.placePurchaseOrder(day, o.vendor, o.lines, "Stock d'ouverture du magasin")
		if err != nil {
			return fmt.Errorf("opening stock order: %w", err)
		}
		if err := s.receivePurchaseOrder(day, po.order, o.vendor, po.lines); err != nil {
			return fmt.Errorf("opening stock receipt: %w", err)
		}
	}
	return nil
}

// retailSaleLines builds n counter-sale lines from products actually on the
// shelf (services are always available), each picked with its appliance
// kind's weight for the season (retail_season.go's kindWeight). Stock
// products are reserved as lines are added, so one sale never takes more than
// is on hand. If the whole shop is sold out it falls back to any sellable
// product — the server allows a sale to take stock negative, but this keeps
// it rare.
func (s *Seeder) retailSaleLines(day time.Time, n int) invoiceLines {
	reserved := map[string]float64{}
	available := func(p productRef) float64 { return s.onHand(p.id) - reserved[p.id] }

	out := invoiceLines{}
	for i := 0; i < n; i++ {
		var candidates []productRef
		var weights []float64
		for _, p := range s.counterProducts() {
			if !p.stockEnabled || available(p) >= 1 {
				candidates = append(candidates, p)
				w := 0.5 // a service rides along with an appliance now and then
				if p.stockEnabled {
					w = kindWeight(p.kind, day)
				}
				weights = append(weights, w)
			}
		}
		var p productRef
		if len(candidates) == 0 {
			p = Pick(s.rng, s.counterProducts())
		} else {
			p = candidates[s.weightedIndex(weights)]
		}
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

// counterProducts is what the Cash Book can sell: every sellable product
// except a serialized one, which a cash sale refuses until the counter has a
// serial picker.
func (s *Seeder) counterProducts() []productRef {
	var out []productRef
	for _, p := range s.sellableProducts() {
		if !p.serialized {
			out = append(out, p)
		}
	}
	return out
}

// weightedIndex picks an index of weights with probability proportional to
// its weight.
func (s *Seeder) weightedIndex(weights []float64) int {
	var sum float64
	for _, w := range weights {
		sum += w
	}
	x := s.rng.Float64Range(0, sum)
	for i, w := range weights {
		if x < w {
			return i
		}
		x -= w
	}
	return len(weights) - 1
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
	retailMaxReorderLines = 12
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

// retailReorderQuantity restocks p up to a target that follows the season a
// few weeks ahead (an order takes up to three weeks to arrive): air
// conditioners are bought in before the summer, not during it.
func (s *Seeder) retailReorderQuantity(day time.Time, p productRef) float64 {
	weight := math.Max(0.5, math.Min(3, kindWeight(p.kind, day.AddDate(0, 0, 21))))
	target := math.Round(float64(s.rng.IntRange(retailRestockTarget[0], retailRestockTarget[1])) * weight)
	qty := target - s.onHand(p.id) - s.onOrder[p.id]
	if qty < 2 {
		qty = 2
	}
	return qty
}
