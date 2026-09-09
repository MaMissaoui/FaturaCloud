package main

import "math"

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
