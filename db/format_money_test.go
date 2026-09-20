package db

import "testing"

func TestFormatMoneyCents(t *testing.T) {
	t.Parallel()
	// Expected strings below (F121/F123) were captured from a real
	// Intl.NumberFormat(...).format run for the same value, currency and
	// minimumFractionDigits (en-US unless a country is given), then reduced
	// to this formatter's symbol-free " <CODE>" shape. Intl resolves
	// maximumFractionDigits to max(minimumFractionDigits, the currency's own
	// default digits) and strips trailing zeros above the minimum; this
	// formatter must agree on every digit/separator. The one deliberate
	// divergence is the sign on a sub-unit negative that rounds to zero
	// displayed digits, where Intl prints "-0" and this renders "0" (F123).
	tests := []struct {
		name    string
		cents   int64
		code    string
		minFrac *int64
		country *string
		want    string
	}{
		// Unchanged, no-override behavior (min == max == currency digits).
		{"simple EUR", 123456, "EUR", nil, nil, "1,234.56 EUR"},
		{"negative", -500, "USD", nil, nil, "-5.00 USD"},
		{"zero", 0, "USD", nil, nil, "0.00 USD"},
		{"no currency code", 100, "", nil, nil, "1.00"},
		{"JPY has no minor unit, rounds", 1234, "JPY", nil, nil, "12 JPY"},  // 12.34 -> rounds to 12
		{"JPY rounds up", 1250, "JPY", nil, nil, "13 JPY"},                  // 12.50 -> rounds up (>=50)
		{"TND 3 decimals pads a zero", 1000, "TND", nil, nil, "10.000 TND"}, // storage has no 3rd decimal
		{"USD unset stays 2 decimals", 1250, "USD", nil, nil, "12.50 USD"},

		// F121: minimumFractionDigits is a true minimum, not an exact count.
		{"USD min 0 strips the trailing zero", 1250, "USD", ptr(int64(0)), nil, "12.5 USD"},
		{"USD min 0 drops a zero fraction", 1200, "USD", ptr(int64(0)), nil, "12 USD"},
		{"USD min 1 pads to one digit", 1200, "USD", ptr(int64(1)), nil, "12.0 USD"},
		{"USD min 3 pads to three digits", 1250, "USD", ptr(int64(3)), nil, "12.500 USD"},
		{"TND min 3 pads to three digits", 1000, "TND", ptr(int64(3)), nil, "10.000 TND"},
		{"TND min 4 goes past the currency default", 1234, "TND", ptr(int64(4)), nil, "12.3400 TND"},
		{"JPY min 0 stays whole", 1234, "JPY", ptr(int64(0)), nil, "12 JPY"},
		{"JPY min 1 rounds to tenths", 1234, "JPY", ptr(int64(1)), nil, "12.3 JPY"},
		{"EUR min 0 keeps the tenths", 1250, "EUR", ptr(int64(0)), nil, "12.5 EUR"},
		// A sub-unit negative at 0 displayed digits must not render "-0"
		// (Intl itself prints "-0" here; this is the F123 fix).
		{"sub-unit negative at 0 digits is not -0", -40, "JPY", ptr(int64(0)), nil, "0 JPY"},
		{"negative cent at 0 digits is not -0", -1, "JPY", nil, nil, "0 JPY"},
		{"negative rounds away from zero", -150, "JPY", nil, nil, "-2 JPY"},

		// F123: AT (ICU de-AT groups with "." like de-DE, not a space).
		{"Austria: period-thousands, comma-decimal", 123456789, "EUR", nil, ptr("AT"), "1.234.567,89 EUR"},
		{"large amount groups thousands", 123456789, "EUR", nil, nil, "1,234,567.89 EUR"},
		{"Tunisia: space-thousands, comma-decimal", 123456789, "TND", nil, ptr("TN"), "1 234 567,890 TND"}, // TND is a 3-decimal currency, see currencyThreeDecimalDigits
		{"Germany: period-thousands, comma-decimal", 123456789, "EUR", nil, ptr("DE"), "1.234.567,89 EUR"},
		{"unlisted country falls back to the default", 123456789, "EUR", nil, ptr("ZZ"), "1,234,567.89 EUR"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := formatMoneyCents(tc.cents, tc.code, tc.minFrac, tc.country)
			if got != tc.want {
				t.Fatalf("formatMoneyCents(%d, %q, %v, %v) = %q, want %q", tc.cents, tc.code, tc.minFrac, tc.country, got, tc.want)
			}
		})
	}
}

func TestFormatQuantity(t *testing.T) {
	t.Parallel()
	if got := formatQuantity(2); got != "2" {
		t.Fatalf("got %q, want 2", got)
	}
	if got := formatQuantity(2.5); got != "2.5" {
		t.Fatalf("got %q, want 2.5", got)
	}
}
