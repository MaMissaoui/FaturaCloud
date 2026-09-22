package db

import "testing"

func TestFrenchNumber(t *testing.T) {
	cases := map[int64]string{
		0:       "zéro",
		1:       "un",
		16:      "seize",
		17:      "dix-sept",
		21:      "vingt-un",
		51:      "cinquante-un",
		70:      "soixante-dix",
		71:      "soixante-onze",
		79:      "soixante-dix-neuf",
		80:      "quatre-vingts",
		81:      "quatre-vingt-un",
		90:      "quatre-vingt-dix",
		91:      "quatre-vingt-onze",
		99:      "quatre-vingt-dix-neuf",
		100:     "cent",
		101:     "cent un",
		200:     "deux cents",
		201:     "deux cent un",
		999:     "neuf cent quatre-vingt-dix-neuf",
		1000:    "mille",
		1001:    "mille un",
		3051:    "trois mille cinquante-un",
		2000:    "deux mille",
		1000000: "un million",
		2000000: "deux millions",
		1234567: "un million deux cent trente-quatre mille cinq cent soixante-sept",
	}
	for n, want := range cases {
		if got := frenchNumber(n); got != want {
			t.Errorf("frenchNumber(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestAmountInWordsFrench(t *testing.T) {
	cases := []struct {
		cents    int64
		currency string
		want     string
	}{
		// The sample invoice: 3 051,000 DT.
		{305100, "TND", "Trois Mille Cinquante-Un Dinars"},
		{0, "TND", "Zéro Dinars"},
		{100, "TND", "Un Dinar"},
		{200, "TND", "Deux Dinars"},
		{100000, "TND", "Mille Dinars"},
		// 3051,500 DT = 3051 dinars and 500 millimes (50 cents × 10).
		{305150, "TND", "Trois Mille Cinquante-Un Dinars et Cinq Cents Millimes"},
		// Non-TND: currency code as the unit, "centimes" for the fraction.
		{8050, "EUR", "Quatre-Vingts EUR et Cinquante Centimes"},
		{100, "EUR", "Un EUR"},
	}
	for _, tc := range cases {
		if got := amountInWordsFrench(tc.cents, tc.currency); got != tc.want {
			t.Errorf("amountInWordsFrench(%d, %q) = %q, want %q", tc.cents, tc.currency, got, tc.want)
		}
	}
}

func TestAmountInWordsLine(t *testing.T) {
	if got := amountInWordsLine(305100, "TND", false); got != "" {
		t.Errorf("disabled line should be empty, got %q", got)
	}
	want := "Arrêtée la présente facture à la somme de : Trois Mille Cinquante-Un Dinars."
	if got := amountInWordsLine(305100, "TND", true); got != want {
		t.Errorf("amountInWordsLine = %q, want %q", got, want)
	}
}
