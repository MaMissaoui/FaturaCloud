package main

// This tool keeps its own running estimate of each stock-enabled product's
// on-hand quantity, updated in lockstep with every receipt (purchasing.go)
// and shipment (sales.go) it creates — a local mirror of products.stockQuantity
// good enough to avoid ever asking the server to ship more than it has.
// It's an estimate, not a second source of truth: it's seeded at zero and
// only reflects what *this run* has done, so it can't account for stock a
// previous run (or a person, via the UI) already added — fine for a v1
// framework driving one fresh demo organization end to end; see README.md's
// "what this deliberately doesn't do" for the same caveat spelled out for
// users of the tool.

func (s *Seeder) onHand(productID string) float64 {
	for i := range s.products {
		if s.products[i].id == productID {
			return s.products[i].onHand
		}
	}
	return 0
}

func (s *Seeder) adjustOnHand(productID string, delta float64) {
	for i := range s.products {
		if s.products[i].id == productID {
			s.products[i].onHand += delta
			if s.products[i].onHand < 0 {
				s.products[i].onHand = 0
			}
			return
		}
	}
}

// productByID looks up a catalog entry by its server-assigned id — used
// wherever a response only carries productId (an order/delivery line item)
// and a caller needs the product's price/tax rate/stock flag back.
func (s *Seeder) productByID(id string) (productRef, bool) {
	for _, p := range s.products {
		if p.id == id {
			return p, true
		}
	}
	return productRef{}, false
}
