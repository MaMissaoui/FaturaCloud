package db

import "testing"

func TestFormatMoneyCents(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		cents   int64
		code    string
		minFrac *int64
		want    string
	}{
		{"simple EUR", 123456, "EUR", nil, "1,234.56 EUR"},
		{"negative", -500, "USD", nil, "-5.00 USD"},
		{"zero", 0, "USD", nil, "0.00 USD"},
		{"no currency code", 100, "", nil, "1.00"},
		{"JPY has no minor unit, rounds", 1234, "JPY", nil, "12 JPY"},  // 12.34 -> rounds to 12
		{"JPY rounds up", 1250, "JPY", nil, "13 JPY"},                  // 12.50 -> rounds up (>=50)
		{"TND 3 decimals pads a zero", 1000, "TND", nil, "10.000 TND"}, // storage has no 3rd decimal
		{"org override to 0 digits", 1250, "EUR", ptr(int64(0)), "13 EUR"},
		{"large amount groups thousands", 123456789, "EUR", nil, "1,234,567.89 EUR"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := formatMoneyCents(tc.cents, tc.code, tc.minFrac)
			if got != tc.want {
				t.Fatalf("formatMoneyCents(%d, %q, %v) = %q, want %q", tc.cents, tc.code, tc.minFrac, got, tc.want)
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
