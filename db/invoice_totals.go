package db

import (
	"fmt"
	"math/big"
	"strconv"
)

// validateInvoiceTotals independently recomputes subtotal/tax/total from the
// line items and their tax rates, and rejects the request if they don't
// match what the client submitted (F18: totals are otherwise client-computed
// and stored verbatim).
//
// This mirrors src/routes/invoices/details.tsx and src/utils/currency.ts
// exactly: line totals are summed per tax rate, tax is rounded to 2 decimal
// places (half up) once per tax-rate group — not per line — and the grand
// total is subtotal + tax. It's done with exact rational arithmetic
// (math/big), not float64: a case as ordinary as a 3.33 unit price at 19.5%
// tax lands exactly on a rounding boundary (0.64935 -> 0.65), and float64's
// binary rounding error can flip which way that goes, which would reject
// perfectly legitimate invoices created through the normal UI.
func (d *Database) validateInvoiceTotals(lineItems []CreateInvoiceLineItemRequest, subTotal, taxTotal, total int64) error {
	subtotalUnits := new(big.Rat)
	groupSubtotals := map[string]*big.Rat{}
	var groupOrder []string

	for _, item := range lineItems {
		qty, err := floatToRat(item.Quantity)
		if err != nil {
			return newValidationError("invalid quantity")
		}
		price, err := floatToRat(item.UnitPrice)
		if err != nil {
			return newValidationError("invalid unit price")
		}
		priceUnits := new(big.Rat).Quo(price, hundred)
		lineTotal := new(big.Rat).Mul(qty, priceUnits)
		subtotalUnits.Add(subtotalUnits, lineTotal)

		if item.TaxRate != nil && *item.TaxRate != "" {
			if _, ok := groupSubtotals[*item.TaxRate]; !ok {
				groupSubtotals[*item.TaxRate] = new(big.Rat)
				groupOrder = append(groupOrder, *item.TaxRate)
			}
			groupSubtotals[*item.TaxRate].Add(groupSubtotals[*item.TaxRate], lineTotal)
		}
	}

	taxTotalUnits := new(big.Rat)
	for _, taxRateID := range groupOrder {
		rate, err := d.GetTaxRate(taxRateID)
		if err != nil {
			return newValidationError("unknown tax rate on invoice line item")
		}
		pct, err := floatToRat(rate.Percentage)
		if err != nil {
			return newValidationError("invalid tax rate percentage")
		}
		tax := new(big.Rat).Mul(groupSubtotals[taxRateID], pct)
		tax.Quo(tax, hundred)
		tax = roundHalfUp(tax, 2)
		taxTotalUnits.Add(taxTotalUnits, tax)
	}

	totalUnits := new(big.Rat).Add(subtotalUnits, taxTotalUnits)

	wantSubTotal := ratToCents(subtotalUnits)
	wantTaxTotal := ratToCents(taxTotalUnits)
	wantTotal := ratToCents(totalUnits)

	if wantSubTotal != subTotal || wantTaxTotal != taxTotal || wantTotal != total {
		return newValidationError(
			"invoice totals don't match line items: got subtotal=%d tax=%d total=%d, expected subtotal=%d tax=%d total=%d",
			subTotal, taxTotal, total, wantSubTotal, wantTaxTotal, wantTotal,
		)
	}
	return nil
}

var hundred = big.NewRat(100, 1)

// floatToRat converts a float64 to the exact rational value of its shortest
// round-trip decimal representation — i.e. what a human (or JS's decimal.js,
// which converts numbers via their string form) would read that number as —
// rather than the float's raw, often-irrational-looking binary fraction.
func floatToRat(f float64) (*big.Rat, error) {
	r, ok := new(big.Rat).SetString(strconv.FormatFloat(f, 'g', -1, 64))
	if !ok {
		return nil, fmt.Errorf("cannot represent %v as a rational", f)
	}
	return r, nil
}

// roundHalfUp rounds r to the given number of decimal places, half away from
// zero — matching decimal.js's ROUND_HALF_UP, which src/utils/currency.ts
// uses for calculateTax. All amounts in this domain are non-negative.
func roundHalfUp(r *big.Rat, places int) *big.Rat {
	scale := new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(places)), nil))
	scaled := new(big.Rat).Mul(r, scale)

	q, rem := new(big.Int).QuoRem(scaled.Num(), scaled.Denom(), new(big.Int))
	twiceRem := new(big.Int).Mul(rem, big.NewInt(2))
	if twiceRem.CmpAbs(scaled.Denom()) >= 0 {
		if scaled.Sign() >= 0 {
			q.Add(q, big.NewInt(1))
		} else {
			q.Sub(q, big.NewInt(1))
		}
	}

	return new(big.Rat).Quo(new(big.Rat).SetInt(q), scale)
}

// ratToCents rounds a currency-units amount to the nearest whole cent —
// matching src/utils/currency.ts's unitsToCents (Math.round(units * 100)).
func ratToCents(r *big.Rat) int64 {
	cents := roundHalfUp(new(big.Rat).Mul(r, hundred), 0)
	return cents.Num().Int64()
}
