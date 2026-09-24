package db

import (
	"strings"
)

// The amount-in-words line in the organization's document language
// (organizations.documentLanguage, migration 0091). French is the original
// Tunisian "Arrêtée la présente facture …" wording in amount_in_words.go;
// English and German are added here with the same unit conventions: TND
// spells dinars and millimes (the fraction at ten times the stored cents),
// every other currency names its code and a cent fraction.

// Supported document languages.
const (
	DocumentLanguageEnglish = "en"
	DocumentLanguageGerman  = "de"
	DocumentLanguageFrench  = "fr"
)

var documentLanguages = map[string]bool{
	DocumentLanguageEnglish: true,
	DocumentLanguageGerman:  true,
	DocumentLanguageFrench:  true,
}

// documentLanguageFor resolves the language an organization's documents are
// written in: its explicit documentLanguage when that's a supported value,
// otherwise French on the Tunisian layout (the format the line reproduces,
// whose static labels are French too) and English on the default layout.
// The fallback keys off the layout, itself an explicit setting, never off
// the organization's country.
func documentLanguageFor(language, layout *string) string {
	if language != nil && documentLanguages[*language] {
		return *language
	}
	if normalizeDocumentLayout(layout) == DocumentLayoutTunisia {
		return DocumentLanguageFrench
	}
	return DocumentLanguageEnglish
}

// --- English ---------------------------------------------------------------

var englishUnits = []string{
	"zero", "one", "two", "three", "four", "five", "six", "seven", "eight",
	"nine", "ten", "eleven", "twelve", "thirteen", "fourteen", "fifteen",
	"sixteen", "seventeen", "eighteen", "nineteen",
}

var englishTens = []string{
	"", "", "twenty", "thirty", "forty", "fifty", "sixty", "seventy", "eighty", "ninety",
}

// englishBelow1000 spells 1..999 without "and" inside the number (the US
// convention: "one hundred five"), hyphenating 21–99.
func englishBelow1000(n int) string {
	var parts []string
	if hundreds := n / 100; hundreds > 0 {
		parts = append(parts, englishUnits[hundreds]+" hundred")
	}
	switch rest := n % 100; {
	case rest == 0:
	case rest < 20:
		parts = append(parts, englishUnits[rest])
	case rest%10 == 0:
		parts = append(parts, englishTens[rest/10])
	default:
		parts = append(parts, englishTens[rest/10]+"-"+englishUnits[rest%10])
	}
	return strings.Join(parts, " ")
}

// englishNumber spells any non-negative int64. Scale words never take a
// plural ("two million").
func englishNumber(n int64) string {
	if n == 0 {
		return "zero"
	}
	var parts []string
	for _, g := range []struct {
		value int64
		name  string
	}{
		{1_000_000_000_000, "trillion"},
		{1_000_000_000, "billion"},
		{1_000_000, "million"},
		{1_000, "thousand"},
	} {
		if count := n / g.value; count > 0 {
			parts = append(parts, englishBelow1000(int(count))+" "+g.name)
			n %= g.value
		}
	}
	if n > 0 {
		parts = append(parts, englishBelow1000(int(n)))
	}
	return strings.Join(parts, " ")
}

func amountInWordsEnglish(totalCents int64, currencyCode string) string {
	units, fraction := splitAmount(totalCents, currencyCode)
	unitName, unitSingular, fractionName, fractionSingular := currencyCode, currencyCode, "Cents", "Cent"
	if currencyCode == "TND" {
		unitName, unitSingular, fractionName, fractionSingular = "Dinars", "Dinar", "Millimes", "Millime"
	}
	words := []string{}
	if units > 0 || fraction == 0 {
		words = append(words, capitalizeWords(englishNumber(units))+" "+pick(units == 1, unitSingular, unitName))
	}
	if fraction > 0 {
		words = append(words, capitalizeWords(englishNumber(fraction))+" "+pick(fraction == 1, fractionSingular, fractionName))
	}
	return strings.Join(words, " and ")
}

// --- German ----------------------------------------------------------------

var germanUnits = []string{
	"null", "eins", "zwei", "drei", "vier", "fünf", "sechs", "sieben", "acht",
	"neun", "zehn", "elf", "zwölf", "dreizehn", "vierzehn", "fünfzehn",
	"sechzehn", "siebzehn", "achtzehn", "neunzehn",
}

var germanTens = []string{
	"", "", "zwanzig", "dreißig", "vierzig", "fünfzig", "sechzig", "siebzig", "achtzig", "neunzig",
}

// germanBelow1000 spells 1..999 as one word. one is how a trailing,
// standalone 1 is written, which depends on what follows the number: "eins"
// when nothing does, "ein" before tausend or a masculine/neuter noun, "eine"
// before Million/Milliarde/Billion. Inside a compound 1 is always "ein"
// (einhundert, einundzwanzig).
func germanBelow1000(n int, one string) string {
	var s string
	if hundreds := n / 100; hundreds > 0 {
		if hundreds == 1 {
			s = "einhundert"
		} else {
			s = germanUnits[hundreds] + "hundert"
		}
	}
	switch rest := n % 100; {
	case rest == 0:
	case rest == 1:
		s += one
	case rest < 20:
		s += germanUnits[rest]
	case rest%10 == 0:
		s += germanTens[rest/10]
	default:
		unit := germanUnits[rest%10]
		if rest%10 == 1 {
			unit = "ein"
		}
		s += unit + "und" + germanTens[rest/10]
	}
	return s
}

// germanNumber spells any non-negative int64. Everything below a million is
// one word (zweitausenddreihundertvierundfünfzig); Million, Milliarde and
// Billion are separate nouns with their own plural (eine Million, zwei
// Millionen). one is the form of a final standalone 1, as in
// germanBelow1000: "eins" for a bare number, "ein" before a currency noun.
func germanNumber(n int64, one string) string {
	if n == 0 {
		return "null"
	}
	var parts []string
	for _, g := range []struct {
		value            int64
		singular, plural string
	}{
		{1_000_000_000_000, "Billion", "Billionen"},
		{1_000_000_000, "Milliarde", "Milliarden"},
		{1_000_000, "Million", "Millionen"},
	} {
		if count := n / g.value; count > 0 {
			parts = append(parts, germanBelow1000(int(count), "eine")+" "+pick(count == 1, g.singular, g.plural))
			n %= g.value
		}
	}
	var below string
	if thousands := n / 1000; thousands > 0 {
		below = germanBelow1000(int(thousands), "ein") + "tausend"
	}
	if rest := n % 1000; rest > 0 {
		below += germanBelow1000(int(rest), one)
	}
	if below != "" {
		parts = append(parts, below)
	}
	return strings.Join(parts, " ")
}

// capitalizeFirst upper-cases only the first letter: German capitalizes the
// start of the phrase, while the Million/Milliarde nouns are already
// capitalized and the number words stay lowercase.
func capitalizeFirst(s string) string {
	r := []rune(s)
	if len(r) == 0 {
		return s
	}
	r[0] = []rune(strings.ToUpper(string(r[0])))[0]
	return string(r)
}

func amountInWordsGerman(totalCents int64, currencyCode string) string {
	units, fraction := splitAmount(totalCents, currencyCode)
	unitName, fractionName, fractionSingular := currencyCode, "Cent", "Cent"
	if currencyCode == "TND" {
		unitName, fractionName, fractionSingular = "Dinar", "Millimes", "Millime"
	}
	words := []string{}
	if units > 0 || fraction == 0 {
		words = append(words, capitalizeFirst(germanNumber(units, "ein"))+" "+unitName)
	}
	if fraction > 0 {
		words = append(words, germanNumber(fraction, "ein")+" "+pick(fraction == 1, fractionSingular, fractionName))
	}
	return strings.Join(words, " und ")
}

// --- shared ----------------------------------------------------------------

// splitAmount splits an absolute cents amount into whole units and the
// fraction as spelled: cents, or millimes (ten times the cents) for TND.
func splitAmount(totalCents int64, currencyCode string) (units, fraction int64) {
	if totalCents < 0 {
		totalCents = -totalCents
	}
	units, fraction = totalCents/100, totalCents%100
	if currencyCode == "TND" {
		fraction *= 10
	}
	return units, fraction
}

func pick(cond bool, ifTrue, ifFalse string) string {
	if cond {
		return ifTrue
	}
	return ifFalse
}
