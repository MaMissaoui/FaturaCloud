package db

import (
	"strconv"
	"strings"
)

// currencyZeroDecimalDigits/currencyThreeDecimalDigits are the common ISO
// 4217 minor-unit exceptions (JPY has no minor unit, several Gulf-region
// currencies use 3 decimals). Deliberately small and explicit rather than a
// full ISO 4217 table — there's no ICU-equivalent bundled with Go's standard
// library the way the frontend's getDefaultFractionDigits reads from Intl,
// and organizations.minimum_fraction_digits already lets an org override
// this per-currency in the common case.
var currencyZeroDecimalDigits = map[string]bool{"JPY": true, "KRW": true, "VND": true, "CLP": true}
var currencyThreeDecimalDigits = map[string]bool{"BHD": true, "KWD": true, "OMR": true, "JOD": true, "TND": true}

func currencyDefaultDigits(currencyCode string) int {
	switch {
	case currencyZeroDecimalDigits[currencyCode]:
		return 0
	case currencyThreeDecimalDigits[currencyCode]:
		return 3
	default:
		return 2
	}
}

// formatMoneyCents is the one canonical cents-to-string formatter every
// money placeholder in the fill engine funnels through — replacing the four
// inconsistent approaches found across the existing React-PDF components
// (raw cents into a shared helper in some, a local /100 float division in
// others, mismatched rounding). Output is grouped, fixed-decimal, and
// currency-code-suffixed (e.g. "1,234.56 EUR") rather than symbol-prefixed:
// there's no per-export UI locale to key a symbol/placement choice off on
// the server, unlike the frontend's Intl.NumberFormat(locale, ...) calls.
// Distinct from einvoice.go's formatCents, a plain unlocalized 2-decimal
// string for XML numeric fields — a different domain with no grouping/
// currency-suffix/configurable-digits needs.
//
// cents is always stored as exact hundredths regardless of what the
// currency's own minor unit is (see CLAUDE.md's Database section on TND's
// millime) — digits beyond that stored precision are zero-padding, not
// invented precision.
func formatMoneyCents(cents int64, currencyCode string, minimumFractionDigits *int64) string {
	digits := currencyDefaultDigits(currencyCode)
	if minimumFractionDigits != nil && *minimumFractionDigits >= 0 {
		digits = int(*minimumFractionDigits)
	}

	negative := cents < 0
	if negative {
		cents = -cents
	}
	whole := cents / 100
	frac2 := cents % 100 // exact hundredths; storage never carries finer precision

	var fracDigits string
	switch {
	case digits <= 0:
		digits = 0
		if frac2 >= 50 {
			whole++
		}
	case digits == 1:
		tenths := (frac2 + 5) / 10
		if tenths == 10 {
			tenths = 0
			whole++
		}
		fracDigits = strconv.FormatInt(tenths, 10)
	default:
		fracDigits = pad2(frac2) + strings.Repeat("0", digits-2)
	}

	out := groupThousands(strconv.FormatInt(whole, 10))
	if digits > 0 {
		out += "." + fracDigits
	}
	if negative && (whole != 0 || frac2 != 0) {
		out = "-" + out
	}
	if currencyCode != "" {
		out += " " + currencyCode
	}
	return out
}

func pad2(n int64) string {
	s := strconv.FormatInt(n, 10)
	if len(s) < 2 {
		return "0" + s
	}
	return s
}

// groupThousands inserts "," every 3 digits from the right of a non-negative
// integer string (sign is handled by the caller).
func groupThousands(s string) string {
	n := len(s)
	if n <= 3 {
		return s
	}
	var b strings.Builder
	rem := n % 3
	if rem > 0 {
		b.WriteString(s[:rem])
		b.WriteString(",")
	}
	for i := rem; i < n; i += 3 {
		b.WriteString(s[i : i+3])
		if i+3 < n {
			b.WriteString(",")
		}
	}
	return b.String()
}

// formatQuantity renders a line-item quantity (a float64 that supports
// fractional units) in its shortest round-tripping decimal form — "2" for a
// whole quantity, "2.5" for a fractional one — rather than always padding to
// a fixed number of decimals.
func formatQuantity(q float64) string {
	return strconv.FormatFloat(q, 'f', -1, 64)
}
