package db

import "testing"

func TestFormatMoneyCents(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		cents   int64
		code    string
		minFrac *int64
		country *string
		want    string
	}{
		{"simple EUR", 123456, "EUR", nil, nil, "1,234.56 EUR"},
		{"negative", -500, "USD", nil, nil, "-5.00 USD"},
		{"zero", 0, "USD", nil, nil, "0.00 USD"},
		{"no currency code", 100, "", nil, nil, "1.00"},
		{"JPY has no minor unit, rounds", 1234, "JPY", nil, nil, "12 JPY"},  // 12.34 -> rounds to 12
		{"JPY rounds up", 1250, "JPY", nil, nil, "13 JPY"},                  // 12.50 -> rounds up (>=50)
		{"TND 3 decimals pads a zero", 1000, "TND", nil, nil, "10.000 TND"}, // storage has no 3rd decimal
		{"org override to 0 digits", 1250, "EUR", ptr(int64(0)), nil, "13 EUR"},
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
