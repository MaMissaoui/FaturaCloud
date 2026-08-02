package db

import (
	"database/sql"
	"fmt"
	"math/big"
)

// Every exchangeRate column in this schema (invoices, purchase_orders,
// incoming_invoices, inbound_deliveries) follows the same direction: it is
// the number of organization-currency units equal to 1 unit of the
// document's own currency. E.g. a USD invoice for a EUR-functional
// organization with exchangeRate 0.92 means 1 USD = 0.92 EUR, so
// amountInOrgCurrency = amountInDocCurrency * exchangeRate.
//
// The rate is captured once, at save time, and frozen — never re-derived on
// read. A posted document must not reprice itself because today's market
// rate moved; that's the opposite property from 3-way matching (computed on
// read for freshness) or the average-cost replay (recomputed for
// correctness). Here immutability is what's needed.
//
// Stored as a decimal string (via math/big), never float64 — consistent
// with the exact-rational arithmetic already used for invoice totals
// (invoice_totals.go) and 3-way match (incoming_invoice_match.go).

// orgCurrencyOrDefault returns the organization's configured currency, or
// "EUR" if it has never been set — matching the fallback every currency
// Select in the frontend already uses.
func orgCurrencyOrDefault(org *Organization) string {
	if org != nil && org.Currency != nil && *org.Currency != "" {
		return *org.Currency
	}
	return "EUR"
}

// resolveExchangeRateForSave decides what exchangeRate value (if any) a
// document should be saved with, and validates it:
//
//   - The effective currency (newCurrency if provided, else currentCurrency)
//     equals the organization's currency: no rate needed; returns nil,
//     clearing any previously stored rate.
//   - It differs, and a new rate was submitted with this request: the rate
//     must be a positive number; returns its canonical decimal-string form.
//   - It differs, no new rate was submitted, but the currency isn't
//     changing: the previously stored rate carries over unchanged.
//   - It differs, no new rate was submitted, and the currency *is*
//     changing: rejected — a rate captured for the old currency must never
//     be silently carried onto a new one.
func resolveExchangeRateForSave(
	orgCurrency string,
	currentCurrency string, currentRate *string,
	newCurrency *string, newRate *float64,
) (*string, error) {
	effectiveCurrency := currentCurrency
	currencyChanging := false
	if newCurrency != nil && *newCurrency != "" && *newCurrency != currentCurrency {
		effectiveCurrency = *newCurrency
		currencyChanging = true
	}

	if effectiveCurrency == "" || effectiveCurrency == orgCurrency {
		return nil, nil
	}

	if newRate != nil {
		r, err := floatToRat(*newRate)
		if err != nil || r.Sign() <= 0 {
			return nil, newValidationError("invalid exchange rate %v", *newRate)
		}
		s := r.FloatString(10)
		return &s, nil
	}

	if currencyChanging || currentRate == nil {
		return nil, newValidationError(
			"exchange rate is required: document currency %q differs from organization currency %q",
			effectiveCurrency, orgCurrency,
		)
	}

	return currentRate, nil
}

// GetLastExchangeRate returns the most recently used exchangeRate for the
// given currency within this organization — across invoices, purchase
// orders, and incoming invoices, whichever was saved most recently — purely
// as a prefill convenience for the next document in that currency. It is
// never used to auto-derive a rate that gets stored: the user still confirms
// it (see resolveExchangeRateForSave's "no live FX API" reasoning).
func (d *Database) GetLastExchangeRate(organizationID, currency string) (*float64, *int64, error) {
	var row struct {
		ExchangeRate     string `db:"exchangeRate"`
		ExchangeRateDate *int64 `db:"exchangeRateDate"`
	}
	// Ordered by exchangeRateDate alone (not createdAt): the three source
	// tables store createdAt with different column types (TEXT vs INTEGER),
	// which SQLite would sort by storage class rather than chronologically in
	// a UNION — a tie on exchangeRateDate is rare enough that losing the
	// tiebreak isn't worth that risk.
	err := d.DB.Get(&row, `
		SELECT exchangeRate, exchangeRateDate FROM (
			SELECT exchangeRate, exchangeRateDate FROM invoices
				WHERE organizationId = ? AND currency = ? AND exchangeRate IS NOT NULL
			UNION ALL
			SELECT exchangeRate, exchangeRateDate FROM purchase_orders
				WHERE organizationId = ? AND currency = ? AND exchangeRate IS NOT NULL
			UNION ALL
			SELECT exchangeRate, exchangeRateDate FROM incoming_invoices
				WHERE organizationId = ? AND currency = ? AND exchangeRate IS NOT NULL
		)
		ORDER BY COALESCE(exchangeRateDate, 0) DESC
		LIMIT 1`,
		organizationID, currency, organizationID, currency, organizationID, currency,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("get_last_exchange_rate: %w", err)
	}

	r, ok := new(big.Rat).SetString(row.ExchangeRate)
	if !ok {
		return nil, nil, newValidationError("invalid stored exchange rate %q", row.ExchangeRate)
	}
	rate, _ := new(big.Float).SetRat(r).Float64()
	return &rate, row.ExchangeRateDate, nil
}

// parseExchangeRate turns a stored exchangeRate column back into a rational
// multiplier, defaulting to 1 (same currency, no conversion) when nil.
func parseExchangeRate(rate *string) (*big.Rat, error) {
	if rate == nil {
		return big.NewRat(1, 1), nil
	}
	r, ok := new(big.Rat).SetString(*rate)
	if !ok {
		return nil, newValidationError("invalid stored exchange rate %q", *rate)
	}
	return r, nil
}

// convertCents converts a cents amount by a rate (see parseExchangeRate),
// rounding to the nearest whole cent — used to turn a goods receipt line's
// unitCost (in the receipt's own currency) into the organization-currency
// value that actually gets written to stockMovements/average cost.
func convertCents(cents int64, rate *big.Rat) int64 {
	scaled := new(big.Rat).Mul(new(big.Rat).SetInt64(cents), rate)
	return roundHalfUp(scaled, 0).Num().Int64()
}
