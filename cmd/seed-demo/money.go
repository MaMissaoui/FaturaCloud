package main

import (
	"math"
	"math/big"
)

// lineItem is the minimal shape computeTotals needs — used for both sales
// invoices and incoming (vendor) invoices, which share the exact same
// server-side validation (db.validateInvoiceTotals).
type lineItem struct {
	quantity       float64
	unitPriceCents int64
	taxPercent     float64 // 0 for untaxed
}

// computeTotals replicates db/invoice_totals.go's validateInvoiceTotals math
// closely enough to always agree with it: line totals summed per tax-rate
// group, tax rounded once per group (never per line), never a running
// float64 total. The server uses math/big exact rationals; every amount
// here is a whole number of cents times an integer-percent tax rate, so
// plain float64 has no precision to lose at these magnitudes — but the
// *rounding rule* (half away from zero, grouped by rate) has to match
// exactly or CreateInvoice/CreateIncomingInvoice comes back 409.
func computeTotals(items []lineItem) (subTotal, taxTotal, total int64) {
	byRate := map[float64]int64{} // taxPercent -> summed line totals in that group
	var rates []float64
	seen := map[float64]bool{}

	for _, li := range items {
		lineTotal := roundHalfUp(li.quantity * float64(li.unitPriceCents))
		subTotal += lineTotal
		if !seen[li.taxPercent] {
			seen[li.taxPercent] = true
			rates = append(rates, li.taxPercent)
		}
		byRate[li.taxPercent] += lineTotal
	}

	for _, rate := range rates {
		if rate == 0 {
			continue
		}
		groupSubtotal := byRate[rate]
		taxTotal += roundHalfUp(float64(groupSubtotal) * rate / 100.0)
	}

	total = subTotal + taxTotal
	return subTotal, taxTotal, total
}

func roundHalfUp(x float64) int64 {
	if x >= 0 {
		return int64(math.Floor(x + 0.5))
	}
	return -int64(math.Floor(-x + 0.5))
}

// computeTotalsWithDiscount is computeTotals for an invoice with a flat
// pre-tax discount (remise) and a fiscal stamp (timbre fiscal), matching
// db/invoice_totals.go: each tax group's base is its subtotal less its
// proportional share of the discount, its tax rounded once, and the total is
// the discounted net plus tax plus the stamp. subTotal stays the gross line
// total. Exact rationals, like the server, since the proportional share isn't
// a whole number of cents.
func computeTotalsWithDiscount(items []lineItem, discount, stamp int64) (subTotal, taxTotal, total int64) {
	byRate := map[float64]*big.Rat{}
	var rates []float64
	sub := new(big.Rat)
	for _, li := range items {
		line := new(big.Rat).Mul(new(big.Rat).SetFloat64(li.quantity), new(big.Rat).SetInt64(li.unitPriceCents))
		sub.Add(sub, line)
		if _, ok := byRate[li.taxPercent]; !ok {
			byRate[li.taxPercent] = new(big.Rat)
			rates = append(rates, li.taxPercent)
		}
		byRate[li.taxPercent].Add(byRate[li.taxPercent], line)
	}
	disc := new(big.Rat).SetInt64(discount)
	tax := new(big.Rat)
	for _, rate := range rates {
		if rate == 0 || sub.Sign() == 0 {
			continue
		}
		group := new(big.Rat).Set(byRate[rate])
		share := new(big.Rat).Mul(disc, byRate[rate])
		share.Quo(share, sub)
		group.Sub(group, share)
		t := new(big.Rat).Mul(group, new(big.Rat).SetFloat64(rate))
		t.Quo(t, big.NewRat(100, 1))
		tax.Add(tax, new(big.Rat).SetInt64(ratRoundHalfUp(t)))
	}
	subTotal = ratRoundHalfUp(sub)
	taxTotal = ratRoundHalfUp(tax)
	net := new(big.Rat).Sub(sub, disc)
	net.Add(net, tax)
	net.Add(net, new(big.Rat).SetInt64(stamp))
	total = ratRoundHalfUp(net)
	return subTotal, taxTotal, total
}

// ratRoundHalfUp rounds r to the nearest integer, halves away from zero.
func ratRoundHalfUp(r *big.Rat) int64 {
	neg := r.Sign() < 0
	a := new(big.Rat).Abs(r)
	a.Add(a, big.NewRat(1, 2))
	q := new(big.Int).Quo(a.Num(), a.Denom())
	if neg {
		q.Neg(q)
	}
	return q.Int64()
}
