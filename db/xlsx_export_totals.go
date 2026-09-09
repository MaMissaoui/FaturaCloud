package db

import "math/big"

// exportLineItem is the minimal shape computeExportTotals needs from any
// document type's line item: quantity, unit price in cents (every
// unitPrice/unitCost column's convention), and an optional tax rate id.
type exportLineItem struct {
	Quantity  float64
	UnitPrice int64
	TaxRate   *string
}

// computeExportTotals independently derives subtotal/tax/total in cents for
// a document type with no server-validated stored totals of its own (unlike
// invoices — see validateInvoiceTotals) — purchase orders and the other
// document types this feeds don't have subTotal/taxTotal/total columns to
// read back, so an export has to compute them the same way a from-scratch
// invoice would. Uses the identical exact-rational, per-tax-rate-group
// rounding idiom (line totals summed per rate, tax rounded to 2 decimal
// places once per group, never per line) so a purchase order's exported
// total agrees with what creating an invoice from the same line items would
// produce. taxRates must already contain every distinct rate referenced —
// one query per distinct rate, not per line, same precedent as
// FetchInvoiceExportData; a referenced-but-missing rate is treated as
// untaxed rather than failing the whole export.
func computeExportTotals(items []exportLineItem, taxRates map[string]TaxRate) (subTotal, taxTotal, total int64) {
	subtotalUnits := new(big.Rat)
	groupSubtotals := map[string]*big.Rat{}
	var groupOrder []string

	for _, item := range items {
		qty, err := floatToRat(item.Quantity)
		if err != nil {
			continue
		}
		price, err := floatToRat(float64(item.UnitPrice))
		if err != nil {
			continue
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
		rate, ok := taxRates[taxRateID]
		if !ok {
			continue
		}
		pct, err := floatToRat(rate.Percentage)
		if err != nil {
			continue
		}
		tax := new(big.Rat).Mul(groupSubtotals[taxRateID], pct)
		tax.Quo(tax, hundred)
		tax = roundHalfUp(tax, 2)
		taxTotalUnits.Add(taxTotalUnits, tax)
	}

	totalUnits := new(big.Rat).Add(subtotalUnits, taxTotalUnits)
	return ratToCents(subtotalUnits), ratToCents(taxTotalUnits), ratToCents(totalUnits)
}
