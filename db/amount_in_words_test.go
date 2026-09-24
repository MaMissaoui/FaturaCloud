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
		// "cents"/"quatre-vingts" lose their plural before "mille" (F144),
		// but keep it before "millions".
		80000:     "quatre-vingt mille",
		200000:    "deux cent mille",
		300080:    "trois cent mille quatre-vingts",
		280000:    "deux cent quatre-vingt mille",
		200000000: "deux cents millions",
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
	if got := amountInWordsLine(305100, "TND", false, DocumentLanguageFrench); got != "" {
		t.Errorf("disabled line should be empty, got %q", got)
	}
	want := "Arrêtée la présente facture à la somme de : Trois Mille Cinquante-Un Dinars."
	if got := amountInWordsLine(305100, "TND", true, DocumentLanguageFrench); got != want {
		t.Errorf("amountInWordsLine = %q, want %q", got, want)
	}
}

func TestEnglishNumber(t *testing.T) {
	cases := map[int64]string{
		0: "zero", 1: "one", 13: "thirteen", 21: "twenty-one", 40: "forty",
		105: "one hundred five", 999: "nine hundred ninety-nine",
		1000: "one thousand", 3051: "three thousand fifty-one",
		200_000: "two hundred thousand", 2_000_001: "two million one",
		987_654_321: "nine hundred eighty-seven million six hundred fifty-four thousand three hundred twenty-one",
	}
	for n, want := range cases {
		if got := englishNumber(n); got != want {
			t.Errorf("englishNumber(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestGermanNumber(t *testing.T) {
	cases := map[int64]string{
		0: "null", 1: "eins", 16: "sechzehn", 17: "siebzehn", 21: "einundzwanzig",
		30: "dreißig", 60: "sechzig", 70: "siebzig", 100: "einhundert",
		101: "einhunderteins", 1000: "eintausend", 1001: "eintausendeins",
		21_000: "einundzwanzigtausend", 101_000: "einhunderteintausend",
		987_654:       "neunhundertsiebenundachtzigtausendsechshundertvierundfünfzig",
		1_000_000:     "eine Million",
		2_000_000:     "zwei Millionen",
		1_000_000_000: "eine Milliarde",
		3_200_001:     "drei Millionen zweihunderttausendeins",
	}
	for n, want := range cases {
		if got := germanNumber(n, "eins"); got != want {
			t.Errorf("germanNumber(%d) = %q, want %q", n, got, want)
		}
	}
	// Before a currency noun a final standalone 1 is "ein".
	if got := germanNumber(1, "ein"); got != "ein" {
		t.Errorf(`germanNumber(1, "ein") = %q, want "ein"`, got)
	}
}

func TestAmountInWordsLineByLanguage(t *testing.T) {
	cases := []struct {
		cents              int64
		currency, language string
		want               string
	}{
		{305100, "TND", DocumentLanguageFrench, "Arrêtée la présente facture à la somme de : Trois Mille Cinquante-Un Dinars."},
		{305100, "TND", DocumentLanguageEnglish, "Amount in words: Three Thousand Fifty-One Dinars."},
		{100, "TND", DocumentLanguageEnglish, "Amount in words: One Dinar."},
		{125, "TND", DocumentLanguageEnglish, "Amount in words: One Dinar and Two Hundred Fifty Millimes."},
		{123401, "EUR", DocumentLanguageEnglish, "Amount in words: One Thousand Two Hundred Thirty-Four EUR and One Cent."},
		{50, "USD", DocumentLanguageEnglish, "Amount in words: Fifty Cents."},
		{305100, "TND", DocumentLanguageGerman, "Betrag in Worten: Dreitausendeinundfünfzig Dinar."},
		{125, "TND", DocumentLanguageGerman, "Betrag in Worten: Ein Dinar und zweihundertfünfzig Millimes."},
		{123401, "EUR", DocumentLanguageGerman, "Betrag in Worten: Eintausendzweihundertvierunddreißig EUR und ein Cent."},
		{100_000_050, "EUR", DocumentLanguageGerman, "Betrag in Worten: Eine Million EUR und fünfzig Cent."},
		{0, "EUR", DocumentLanguageGerman, "Betrag in Worten: Null EUR."},
	}
	for _, tc := range cases {
		if got := amountInWordsLine(tc.cents, tc.currency, true, tc.language); got != tc.want {
			t.Errorf("amountInWordsLine(%d, %s, %s) = %q, want %q", tc.cents, tc.currency, tc.language, got, tc.want)
		}
	}
}

func TestDocumentLanguageFor(t *testing.T) {
	tunisia, def := ptr(DocumentLayoutTunisia), ptr(DocumentLayoutDefault)
	cases := []struct {
		language, layout *string
		want             string
	}{
		{nil, nil, DocumentLanguageEnglish},
		{nil, def, DocumentLanguageEnglish},
		// A Tunisian organization that never picked a language keeps the
		// French line its layout's labels match.
		{nil, tunisia, DocumentLanguageFrench},
		{ptr(""), tunisia, DocumentLanguageFrench},
		{ptr("xx"), def, DocumentLanguageEnglish},
		{ptr("de"), tunisia, DocumentLanguageGerman},
		{ptr("fr"), def, DocumentLanguageFrench},
	}
	for _, tc := range cases {
		if got := documentLanguageFor(tc.language, tc.layout); got != tc.want {
			t.Errorf("documentLanguageFor(%v, %v) = %q, want %q", deref(tc.language), deref(tc.layout), got, tc.want)
		}
	}
}

func TestUpdateOrganizationDocumentLanguage(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-lang"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	updated, err := d.UpdateOrganization(org.ID, UpdateOrganizationRequest{DocumentLanguage: ptr("de")})
	if err != nil {
		t.Fatalf("UpdateOrganization(de): %v", err)
	}
	if deref(updated.DocumentLanguage) != "de" {
		t.Fatalf("documentLanguage = %s, want de", deref(updated.DocumentLanguage))
	}
	if _, err := d.UpdateOrganization(org.ID, UpdateOrganizationRequest{DocumentLanguage: ptr("es")}); err == nil {
		t.Fatal("UpdateOrganization(es) should be rejected")
	}
	// Omitted keeps it.
	updated, err = d.UpdateOrganization(org.ID, UpdateOrganizationRequest{Name: ptr("Renamed")})
	if err != nil || deref(updated.DocumentLanguage) != "de" {
		t.Fatalf("after an unrelated update: documentLanguage = %s, err %v; want de", deref(updated.DocumentLanguage), err)
	}
}
