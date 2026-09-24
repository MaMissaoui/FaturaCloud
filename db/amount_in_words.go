package db

import (
	"strings"
)

// amount in words (montant en lettres) — the "Arrêtée la présente facture à
// la somme de ..." line on the Tunisian document layout. French, because
// that's the language of the printed format this feature reproduces; the
// wording is deliberately not translated (the whole template's static labels
// are French too). Enabled per organization by amountInWordsEnabled
// (migration 0088); a disabled organization's placeholder resolves to "" so
// the template line disappears entirely rather than leaving a dangling
// "à la somme de :".
//
// Storage is 2-decimal cents everywhere in this app, but Tunisia's dinar
// subdivides into 1000 millimes (3 decimals) — so the fractional part is
// expressed in millimes at ten times the stored cent value (1 cent = 10
// millimes), and is always a multiple of ten. This is the same
// "2-decimal storage can't represent an arbitrary millime" limitation
// documented on the Tunisia invoice support notes; round-number totals (which
// is every amount this app can actually store to a whole millime boundary)
// are unaffected.

var frenchUnits = []string{
	"zéro", "un", "deux", "trois", "quatre", "cinq", "six", "sept", "huit",
	"neuf", "dix", "onze", "douze", "treize", "quatorze", "quinze", "seize",
	"dix-sept", "dix-huit", "dix-neuf",
}

// frenchBelow100 spells 0..99. The 70s/90s are the classic French irregulars
// (soixante-dix, quatre-vingt-dix) built from the teens; 80 takes a plural
// "s" only when it stands alone. Everything is hyphen-joined, matching the
// format of the sample invoice this reproduces ("Cinquante-Un", not
// "cinquante et un").
func frenchBelow100(n int) string {
	if n < 20 {
		return frenchUnits[n]
	}
	tens, unit := n/10, n%10
	switch tens {
	case 7, 9:
		base := "soixante"
		if tens == 9 {
			base = "quatre-vingt"
		}
		return base + "-" + frenchUnits[10+unit]
	case 8:
		if unit == 0 {
			return "quatre-vingts"
		}
		return "quatre-vingt-" + frenchUnits[unit]
	default:
		base := []string{"", "", "vingt", "trente", "quarante", "cinquante", "soixante"}[tens]
		if unit == 0 {
			return base
		}
		return base + "-" + frenchUnits[unit]
	}
}

// frenchBelow1000 spells 0..999. "cent" is plural only when it ends the
// number (deux cents) and singular when followed by another part
// (deux cent un); hundreds and the remaining part are space-separated, the
// traditional French spelling the sample invoice uses ("Cinquante-Un" keeps
// its hyphen inside the tens/unit pair, but "deux cent" is two words).
func frenchBelow1000(n int) string {
	hundreds, rest := n/100, n%100
	if hundreds == 0 {
		return frenchBelow100(n)
	}
	var s string
	if hundreds == 1 {
		s = "cent"
	} else {
		s = frenchUnits[hundreds] + " cent"
	}
	if rest == 0 {
		if hundreds > 1 {
			s += "s"
		}
		return s
	}
	return s + " " + frenchBelow100(rest)
}

// invariableBeforeMille drops the plural "s" that "cents" and
// "quatre-vingts" take when they end a number: "mille" is an adjective, so
// before it they stay invariable (deux cent mille, quatre-vingt mille —
// audit F144). Before "million"/"milliard", which are nouns, the plural
// stays (deux cents millions), so only the thousands group goes through this.
func invariableBeforeMille(s string) string {
	if strings.HasSuffix(s, "cents") || strings.HasSuffix(s, "quatre-vingts") {
		return strings.TrimSuffix(s, "s")
	}
	return s
}

// frenchNumber spells any non-negative int64. "mille" is invariable
// (deux mille, not deux milles); million/milliard take an "s" in the plural.
// Scale words are space-separated.
func frenchNumber(n int64) string {
	if n == 0 {
		return "zéro"
	}
	var parts []string
	groups := []struct {
		value int64
		name  string
	}{
		{1_000_000_000_000, "billion"},
		{1_000_000_000, "milliard"},
		{1_000_000, "million"},
	}
	for _, g := range groups {
		if count := n / g.value; count > 0 {
			s := frenchBelow1000(int(count)) + " " + g.name
			if count > 1 {
				s += "s"
			}
			parts = append(parts, s)
			n %= g.value
		}
	}
	if thousands := n / 1000; thousands > 0 {
		if thousands == 1 {
			parts = append(parts, "mille")
		} else {
			parts = append(parts, invariableBeforeMille(frenchBelow1000(int(thousands)))+" mille")
		}
		n %= 1000
	}
	if n > 0 {
		parts = append(parts, frenchBelow1000(int(n)))
	}
	return strings.Join(parts, " ")
}

// capitalizeWords upper-cases the first letter of the whole string and of
// every word after a space or hyphen, matching the sample's "Trois Mille
// Cinquante-Un" (separators preserved).
func capitalizeWords(s string) string {
	out := []rune(s)
	capitalizeNext := true
	for i, r := range out {
		if capitalizeNext {
			out[i] = []rune(strings.ToUpper(string(r)))[0]
		}
		capitalizeNext = r == ' ' || r == '-'
	}
	return string(out)
}

// amountInWordsFrench spells a cents amount in French with the currency's
// unit names: TND uses Dinar(s)/Millime(s) (the fractional part at 10x, see
// the file comment); every other currency uses its code as the unit and
// "centimes" for the fraction.
func amountInWordsFrench(totalCents int64, currencyCode string) string {
	if totalCents < 0 {
		totalCents = -totalCents
	}
	units := totalCents / 100
	cents := totalCents % 100

	unitName, fractionName, fractionValue := currencyCode, "Centimes", cents
	if currencyCode == "TND" {
		unitName, fractionName, fractionValue = "Dinars", "Millimes", cents*10
	}

	words := []string{}
	if units > 0 || cents == 0 {
		name := unitName
		if units == 1 {
			if currencyCode == "TND" {
				name = "Dinar"
			}
		}
		words = append(words, capitalizeWords(frenchNumber(units))+" "+name)
	}
	if fractionValue > 0 {
		words = append(words, capitalizeWords(frenchNumber(fractionValue))+" "+fractionName)
	}
	return strings.Join(words, " et ")
}

// amountInWordsLine builds the whole sentence the template prints, or "" when
// the organization hasn't enabled the feature (so the line vanishes rather
// than leaving a label with nothing after it — the same shape
// invoice.withholdingTaxLine uses). The label text is intentionally French,
// matching the rest of the Tunisian layout.
func amountInWordsLine(totalCents int64, currencyCode string, enabled bool) string {
	if !enabled {
		return ""
	}
	return "Arrêtée la présente facture à la somme de : " +
		amountInWordsFrench(totalCents, currencyCode) + "."
}
