package db

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
)

// This exporter's column layout, header structure, and field encoding rules
// are cross-verified against two independently maintained open-source
// implementations (ledermann/datev, a Ruby gem, and ameax/datev-extf, a PHP
// library) plus a real EXTF_Buchungsstapel.csv example file from the
// former — not guessed, unlike the earlier "no validator available" stance
// this codebase took before researching the format. Both implementations
// agree on: 31 header-row fields, 125 data-row columns in the same order
// for format category 21 ("Buchungsstapel") / format version 13, Windows-1252
// encoding, semicolon separation, CRLF line endings, comma decimal
// separators, and the quoting rule (numeric fields bare, string fields
// quoted when non-empty, empty fields bare with no quotes).
var datevColumns = []string{
	"Umsatz (ohne Soll/Haben-Kz)", "Soll/Haben-Kennzeichen", "WKZ Umsatz", "Kurs", "Basisumsatz", "WKZ Basisumsatz",
	"Konto", "Gegenkonto (ohne BU-Schlüssel)", "BU-Schlüssel", "Belegdatum", "Belegfeld 1", "Belegfeld 2",
	"Skonto", "Buchungstext", "Postensperre", "Diverse Adressnummer", "Geschäftspartnerbank", "Sachverhalt",
	"Zinssperre", "Beleglink",
	"Beleginfo – Art 1", "Beleginfo – Inhalt 1", "Beleginfo – Art 2", "Beleginfo – Inhalt 2",
	"Beleginfo – Art 3", "Beleginfo – Inhalt 3", "Beleginfo – Art 4", "Beleginfo – Inhalt 4",
	"Beleginfo – Art 5", "Beleginfo – Inhalt 5", "Beleginfo – Art 6", "Beleginfo – Inhalt 6",
	"Beleginfo – Art 7", "Beleginfo – Inhalt 7", "Beleginfo – Art 8", "Beleginfo – Inhalt 8",
	"KOST1 – Kostenstelle", "KOST2 – Kostenstelle", "Kost Menge", "EU-Land u. USt-IdNr.", "EU-Steuersatz",
	"Abw. Versteuerungsart", "Sachverhalt L+L", "Funktionsergänzung L+L", "BU 49 Hauptfunktionstyp",
	"BU 49 Hauptfunktionsnummer", "BU 49 Funktionsergänzung",
	"Zusatzinformation – Art 1", "Zusatzinformation – Inhalt 1", "Zusatzinformation – Art 2", "Zusatzinformation – Inhalt 2",
	"Zusatzinformation – Art 3", "Zusatzinformation – Inhalt 3", "Zusatzinformation – Art 4", "Zusatzinformation – Inhalt 4",
	"Zusatzinformation – Art 5", "Zusatzinformation – Inhalt 5", "Zusatzinformation – Art 6", "Zusatzinformation – Inhalt 6",
	"Zusatzinformation – Art 7", "Zusatzinformation – Inhalt 7", "Zusatzinformation – Art 8", "Zusatzinformation – Inhalt 8",
	"Zusatzinformation – Art 9", "Zusatzinformation – Inhalt 9", "Zusatzinformation – Art 10", "Zusatzinformation – Inhalt 10",
	"Zusatzinformation – Art 11", "Zusatzinformation – Inhalt 11", "Zusatzinformation – Art 12", "Zusatzinformation – Inhalt 12",
	"Zusatzinformation – Art 13", "Zusatzinformation – Inhalt 13", "Zusatzinformation – Art 14", "Zusatzinformation – Inhalt 14",
	"Zusatzinformation – Art 15", "Zusatzinformation – Inhalt 15", "Zusatzinformation – Art 16", "Zusatzinformation – Inhalt 16",
	"Zusatzinformation – Art 17", "Zusatzinformation – Inhalt 17", "Zusatzinformation – Art 18", "Zusatzinformation – Inhalt 18",
	"Zusatzinformation – Art 19", "Zusatzinformation – Inhalt 19", "Zusatzinformation – Art 20", "Zusatzinformation – Inhalt 20",
	"Stück", "Gewicht", "Zahlweise", "Forderungsart", "Veranlagungsjahr", "Zugeordnete Fälligkeit", "Skontotyp",
	"Auftragsnummer", "Buchungstyp", "USt-Schlüssel (Anzahlungen)", "EU-Mitgliedstaat (Anzahlungen)",
	"Sachverhalt L+L (Anzahlungen)", "EU-Steuersatz (Anzahlungen)", "Erlöskonto (Anzahlungen)", "Herkunft-Kz",
	"Leerfeld", "KOST-Datum", "SEPA-Mandatsreferenz", "Skontosperre", "Gesellschaftername", "Beteiligtennummer",
	"Identifikationsnummer", "Zeichnernummer", "Postensperre bis", "Bezeichnung", "Kennzeichen", "Festschreibung",
	"Leistungsdatum", "Datum Zuord.", "Fälligkeit", "Generalumkehr", "Steuersatz", "Land", "Abrechnungsreferent",
	"BVV-Position", "EU-Mitgliedstaat u. UStID (Ursprung)", "EU-Steuersatz (Ursprung)", "Abw. Skontokonto",
}

// datevLineRow is one journal_lines row joined with what a DATEV row needs
// from its account and (if any) tax rate.
type datevLineRow struct {
	EntryID            string  `db:"entryId"`
	Date               int64   `db:"date"`
	Reference          *string `db:"reference"`
	EntryDescription   string  `db:"entryDescription"`
	AccountID          string  `db:"accountId"`
	AccountCode        string  `db:"accountCode"`
	AccountName        string  `db:"accountName"`
	DATEVAccountNumber *string `db:"datevAccountNumber"`
	LineDescription    *string `db:"lineDescription"`
	Debit              int64   `db:"debit"`
	Credit             int64   `db:"credit"`
	DatevBuKey         *string `db:"datevBuKey"`
}

// GenerateDATEV renders every posted/reversed journal line in a fiscal year
// as a DATEV Buchungsstapel EXTF file (Windows-1252, semicolon-separated,
// CRLF). Returns the rendered content and a suggested filename together, so
// the caller doesn't need a second lookup just to name the download.
//
// DATEV's Buchungsstapel is fundamentally a Konto/Gegenkonto (account/
// counter-account) format: one row is one elementary two-sided booking, not
// one row per journal_lines row like FEC. An entry with more than two lines
// (e.g. an invoice: one AR line, N revenue/tax lines) has no single natural
// "the other side" — the anchor-selection loop below picks the one line
// that's alone on its side (AR/AP/bank/retained-earnings, in every
// auto-posted entry in this codebase) as the implicit Gegenkonto for every
// other line, exactly the design decision documented in the original
// accounting plan. A manual
// entry with more than one line on *both* sides has no such anchor; those
// either use organizations.datevClearingAccountId as a synthetic anchor
// (splitting the entry into elementary bookings against it, which nets to
// zero across the derived rows) or, if that's unset, are collected as a
// blocking validation problem — never silently dropped from the file.
//
// Deliberately unset: "Generalumkehr" (column 118), which would flag a
// reversed entry's rows as a reversal for the importer's benefit. A
// reversed entry and its reversal both export as ordinary opposite-side
// bookings instead — arithmetically correct (the trial balance still nets
// to zero), just not specially marked. Revisit if a real DATEV import ever
// needs that distinction.
func (d *Database) GenerateDATEV(organizationID, fiscalYearID string) (content []byte, filename string, err error) {
	org, err := d.GetOrganization(organizationID)
	if err != nil {
		return nil, "", fmt.Errorf("generate_datev get_organization: %w", err)
	}
	fiscalYear, err := d.GetFiscalYear(fiscalYearID)
	if err != nil {
		return nil, "", fmt.Errorf("generate_datev get_fiscal_year: %w", err)
	}
	if fiscalYear.OrganizationID != organizationID {
		return nil, "", newValidationError("fiscal year does not belong to this organization")
	}

	rows := []datevLineRow{}
	if err := d.DB.Select(&rows, `
		SELECT
			je.id AS entryId, je.date AS date, je.reference AS reference, je.description AS entryDescription,
			jl.accountId AS accountId, a.code AS accountCode, a.name AS accountName, a.datevAccountNumber AS datevAccountNumber,
			jl.description AS lineDescription, jl.debit AS debit, jl.credit AS credit,
			tr.datev_bu_key AS datevBuKey
		FROM journal_lines jl
		JOIN journal_entries je ON je.id = jl.journalEntryId
		JOIN accounts a ON a.id = jl.accountId
		LEFT JOIN taxRates tr ON tr.id = jl.taxRateId
		WHERE je.organizationId = ? AND je.fiscalYearId = ?
		      AND je.status IN ('posted', 'reversed') AND je.entryNumber IS NOT NULL
		ORDER BY je.date ASC, je.entryNumber ASC, jl.position ASC`,
		organizationID, fiscalYearID,
	); err != nil {
		return nil, "", fmt.Errorf("generate_datev select: %w", err)
	}

	var entryIDOrder []string
	linesByEntry := map[string][]datevLineRow{}
	for _, r := range rows {
		if _, ok := linesByEntry[r.EntryID]; !ok {
			entryIDOrder = append(entryIDOrder, r.EntryID)
		}
		linesByEntry[r.EntryID] = append(linesByEntry[r.EntryID], r)
	}

	problems := newDATEVProblems()

	consultantNumber := validateDATEVNumericField(problems, "DATEV consultant number", org.DatevConsultantNumber, 1001, 9999999)
	clientNumber := validateDATEVNumericField(problems, "DATEV client number", org.DatevClientNumber, 1, 99999)

	accountLength, accountNumberOf := validateDATEVAccountNumbers(problems, rows)

	// The clearing account is resolved separately from accountNumberOf,
	// which only covers accounts that actually appear in an exported
	// journal_lines row — the clearing account itself normally doesn't
	// (it exists purely as a synthetic anchor for entries that need one).
	clearingAccountNumber := ""
	if org.DatevClearingAccountID != nil {
		clearingAccount, err := d.GetAccount(*org.DatevClearingAccountID)
		if err != nil {
			return nil, "", fmt.Errorf("generate_datev get_clearing_account: %w", err)
		}
		if clearingAccount.DATEVAccountNumber == nil || strings.TrimSpace(*clearingAccount.DATEVAccountNumber) == "" {
			problems.fieldErrors = append(problems.fieldErrors, fmt.Sprintf(
				"DATEV clearing account %s (%s) has no DATEV account number configured", clearingAccount.Code, clearingAccount.Name,
			))
		} else {
			number := strings.TrimSpace(*clearingAccount.DATEVAccountNumber)
			if accountLength > 0 && int64(len(number)) != accountLength {
				problems.mixedLengths = append(problems.mixedLengths, fmt.Sprintf(
					"clearing account %s has DATEV number %q (length %d), inconsistent with the other accounts' length %d",
					clearingAccount.Code, number, len(number), accountLength,
				))
			} else {
				clearingAccountNumber = number
			}
		}
	}

	type datevOutputRow struct {
		date                       int64
		reference                  *string
		buchungstext               string
		kontoNumber, gegenkontoNum string
		kennzeichen                string
		amount                     int64
		buSchluessel               string
	}
	var outRows []datevOutputRow

	for _, entryID := range entryIDOrder {
		lines := linesByEntry[entryID]
		var debitLines, creditLines []datevLineRow
		for _, l := range lines {
			if l.Debit > 0 {
				debitLines = append(debitLines, l)
			} else {
				creditLines = append(creditLines, l)
			}
		}

		var anchorNumber string
		var others []datevLineRow
		switch {
		case len(debitLines) == 1:
			anchorNumber, others = accountNumberOf[debitLines[0].AccountID], creditLines
		case len(creditLines) == 1:
			anchorNumber, others = accountNumberOf[creditLines[0].AccountID], debitLines
		default:
			if clearingAccountNumber == "" {
				problems.addClearing(lines[0].Date, lines[0].Reference, lines[0].EntryDescription)
				continue
			}
			anchorNumber, others = clearingAccountNumber, lines
		}
		if anchorNumber == "" {
			// A missing DATEV account number on the anchor/clearing account
			// itself was already collected by validateDATEVAccountNumbers
			// (for anchor lines) or the org-level check above (for the
			// clearing account) — skip emitting rows for this entry rather
			// than emitting one with a blank Konto/Gegenkonto.
			continue
		}

		for _, other := range others {
			otherNumber := accountNumberOf[other.AccountID]
			if otherNumber == "" {
				continue // already collected as a missing-account-number problem
			}
			kennzeichen, amount := "H", other.Credit
			if other.Debit > 0 {
				kennzeichen, amount = "S", other.Debit
			}
			buchungstext := derefString(other.LineDescription)
			if buchungstext == "" {
				buchungstext = other.EntryDescription
			}
			outRows = append(outRows, datevOutputRow{
				date: other.Date, reference: other.Reference, buchungstext: buchungstext,
				kontoNumber: otherNumber, gegenkontoNum: anchorNumber,
				kennzeichen: kennzeichen, amount: amount, buSchluessel: derefString(other.DatevBuKey),
			})
		}
	}

	if err := problems.asError(); err != nil {
		return nil, "", err
	}

	var b strings.Builder
	now := time.Now()
	header1 := []string{
		quoteDATEV("EXTF"), "700", "21", quoteDATEV("Buchungsstapel"), "13",
		now.Format("20060102150405") + fmt.Sprintf("%03d", now.Nanosecond()/1e6),
		"", quoteDATEV("RE"), quoteDATEV("FaturaCloud"), "",
		strconv.FormatInt(consultantNumber, 10), strconv.FormatInt(clientNumber, 10),
		formatDATEVDate(fiscalYear.StartDate), strconv.FormatInt(accountLength, 10),
		formatDATEVDate(fiscalYear.StartDate), formatDATEVDate(fiscalYear.EndDate),
		quoteDATEV(truncateDATEV("Buchungsstapel "+fiscalYear.Name, 30)),
		"", "1", "", "", quoteDATEV(orgCurrencyOrDefault(org)),
		"", "", "", "", "", "", "", "", "",
	}
	b.WriteString(strings.Join(header1, ";"))
	b.WriteString("\r\n")
	b.WriteString(strings.Join(datevColumns, ";"))
	b.WriteString("\r\n")

	for _, r := range outRows {
		fields := make([]string, len(datevColumns))
		fields[0] = formatDATEVAmount(r.amount)
		fields[1] = quoteDATEV(r.kennzeichen)
		fields[6] = r.kontoNumber
		fields[7] = r.gegenkontoNum
		fields[8] = quoteDATEV(r.buSchluessel)
		fields[9] = formatDATEVBelegdatum(r.date)
		fields[10] = quoteDATEV(sanitizeDATEVBelegfeld(derefString(r.reference)))
		fields[13] = quoteDATEV(truncateDATEV(r.buchungstext, 60))
		b.WriteString(strings.Join(fields, ";"))
		b.WriteString("\r\n")
	}

	// ReplaceUnsupported substitutes any rune outside cp1252 (e.g. an
	// out-of-range Unicode character in a free-text description) with '?'
	// rather than erroring — this app is multi-locale and free text reaches
	// this exporter unfiltered, so a bare .NewEncoder() would turn one
	// stray character anywhere in a fiscal year's worth of entries into a
	// 500 with no actionable message. Both reference implementations make
	// the same choice (the Ruby gem passes `invalid: :replace, undef: :replace`).
	encoded, err := encoding.ReplaceUnsupported(charmap.Windows1252.NewEncoder()).String(b.String())
	if err != nil {
		return nil, "", fmt.Errorf("generate_datev encode: %w", err)
	}

	filename = fmt.Sprintf("EXTF_Buchungsstapel_%s.csv", fiscalYear.Name)
	return []byte(encoded), filename, nil
}

// datevProblems collects every blocking export problem at once — same idiom
// as validateEInvoiceCompleteness — grouped by kind rather than one line per
// offending account/entry, since a real org can easily have dozens of
// accounts missing a DATEV number and a wall of individual error lines
// would bury the actionable summary.
type datevProblems struct {
	fieldErrors          []string
	missingAccounts      map[string]bool // accountId set, for de-duplication
	missingAccountLabels []string
	mixedLengths         []string
	clearingNeeded       []string
}

func newDATEVProblems() *datevProblems {
	return &datevProblems{missingAccounts: map[string]bool{}}
}

func (p *datevProblems) addMissingAccount(accountID, code, name string) {
	if p.missingAccounts[accountID] {
		return
	}
	p.missingAccounts[accountID] = true
	p.missingAccountLabels = append(p.missingAccountLabels, fmt.Sprintf("%s (%s)", code, name))
}

func (p *datevProblems) addClearing(date int64, reference *string, description string) {
	label := description
	if reference != nil && *reference != "" {
		label = fmt.Sprintf("%s (%s)", description, *reference)
	}
	p.clearingNeeded = append(p.clearingNeeded, fmt.Sprintf("%s on %s", label, formatDATEVDate(date)))
}

// capList joins up to 10 items, then "and N more" — long enough to be
// useful, short enough that the actionable summary doesn't scroll away.
func capList(items []string) string {
	const max = 10
	if len(items) <= max {
		return strings.Join(items, ", ")
	}
	return fmt.Sprintf("%s, and %d more", strings.Join(items[:max], ", "), len(items)-max)
}

func (p *datevProblems) asError() error {
	var messages []string
	messages = append(messages, p.fieldErrors...)
	if len(p.missingAccountLabels) > 0 {
		sort.Strings(p.missingAccountLabels)
		messages = append(messages, fmt.Sprintf(
			"%d account(s) have no DATEV account number configured: %s",
			len(p.missingAccountLabels), capList(p.missingAccountLabels),
		))
	}
	if len(p.mixedLengths) > 0 {
		messages = append(messages, fmt.Sprintf(
			"DATEV account numbers must all be the same length (Sachkontenlänge is one value per export): %s",
			capList(p.mixedLengths),
		))
	}
	if len(p.clearingNeeded) > 0 {
		messages = append(messages, fmt.Sprintf(
			"%d entr(y/ies) have more than one line on both sides with no DATEV clearing account configured to split them: %s",
			len(p.clearingNeeded), capList(p.clearingNeeded),
		))
	}
	if len(messages) == 0 {
		return nil
	}
	return newValidationError("cannot generate DATEV export: %s", strings.Join(messages, "; "))
}

// validateDATEVNumericField parses an organization's DATEV consultant/client
// number (stored as free-text so the column can be blank) and checks it
// falls in DATEV's documented valid range, collecting a problem instead of
// returning early — same "report everything at once" idiom as the rest of
// this file.
func validateDATEVNumericField(problems *datevProblems, label string, value *string, min, max int64) int64 {
	if value == nil || strings.TrimSpace(*value) == "" {
		problems.fieldErrors = append(problems.fieldErrors, fmt.Sprintf("%s is required", label))
		return 0
	}
	n, err := strconv.ParseInt(strings.TrimSpace(*value), 10, 64)
	if err != nil || n < min || n > max {
		problems.fieldErrors = append(problems.fieldErrors, fmt.Sprintf("%s must be a number between %d and %d", label, min, max))
		return 0
	}
	return n
}

// validateDATEVAccountNumbers checks every account referenced by an
// exported line has a datevAccountNumber, and that they're all the same
// length — this app has no per-customer/vendor subsidiary account
// (Personenkonto) concept, so unlike a real SKR chart there's no legitimate
// reason for the accounts this app exports to have mixed lengths, and
// DATEV's Sachkontenlänge header field is a single value for the whole
// batch. Returns the (single, validated) length and an accountID→DATEV
// number lookup for every account that has one.
func validateDATEVAccountNumbers(problems *datevProblems, rows []datevLineRow) (int64, map[string]string) {
	accountNumberOf := map[string]string{}
	lengths := map[int]bool{}
	seen := map[string]bool{}
	for _, r := range rows {
		if seen[r.AccountID] {
			continue
		}
		seen[r.AccountID] = true
		if r.DATEVAccountNumber == nil || strings.TrimSpace(*r.DATEVAccountNumber) == "" {
			problems.addMissingAccount(r.AccountID, r.AccountCode, r.AccountName)
			continue
		}
		number := strings.TrimSpace(*r.DATEVAccountNumber)
		accountNumberOf[r.AccountID] = number
		lengths[len(number)] = true
	}
	if len(lengths) > 1 {
		var distinct []int
		for l := range lengths {
			distinct = append(distinct, l)
		}
		sort.Ints(distinct)
		strs := make([]string, len(distinct))
		for i, l := range distinct {
			strs[i] = strconv.Itoa(l)
		}
		problems.mixedLengths = append(problems.mixedLengths, "lengths found: "+strings.Join(strs, ", "))
		return 0, accountNumberOf
	}
	for l := range lengths {
		if l < 4 || l > 8 {
			problems.mixedLengths = append(problems.mixedLengths, fmt.Sprintf(
				"DATEV account numbers must be 4-8 digits (Sachkontenlänge), found length %d", l,
			))
			return 0, accountNumberOf
		}
		return int64(l), accountNumberOf
	}
	return 4, accountNumberOf // no accounts referenced at all (empty fiscal year) — DATEV's own minimum
}

func formatDATEVDate(millis int64) string {
	return time.UnixMilli(millis).UTC().Format("20060102")
}

// formatDATEVBelegdatum renders Belegdatum (column 10) as DDMM — day and
// month only, no year, per both reference implementations
// (`format: '%d%m'`) and the real example file. This is a different format
// from every other date field in this exporter (header dates are
// YYYYMMDD), so it deliberately isn't formatDATEVDate.
func formatDATEVBelegdatum(millis int64) string {
	return time.UnixMilli(millis).UTC().Format("0201")
}

// formatDATEVAmount renders integer cents as a plain decimal with a comma
// separator, always positive — DATEV's Umsatz field carries the sign
// separately via Soll/Haben-Kennzeichen ("Muss immer ein positiver Wert
// sein" per DATEV's own field documentation).
func formatDATEVAmount(cents int64) string {
	if cents < 0 {
		cents = -cents
	}
	return fmt.Sprintf("%d,%02d", cents/100, cents%100)
}

// quoteDATEV wraps a non-empty string in double quotes, doubling any
// embedded quote — DATEV's reserved/unused fields must be completely empty
// (no bare `""`), which is why an empty value returns "" here rather than a
// quoted empty string.
func quoteDATEV(s string) string {
	if s == "" {
		return ""
	}
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

func truncateDATEV(s string, limit int) string {
	r := []rune(s)
	if len(r) > limit {
		return string(r[:limit])
	}
	return s
}

// sanitizeDATEVBelegfeld strips every character outside DATEV's documented
// allowlist for Belegfeld 1 (alphanumerics plus $&%*+-/) — unlike FEC's
// tab/newline-only sanitizer, this field's contract is a full allowlist
// regex, and DATEV's OPOS matching depends on this field, so silently
// passing through a disallowed character (a plain space, a '#', ...) risks
// either a rejected import or a field that no longer matches the real
// invoice number it's supposed to key open-item matching on.
func sanitizeDATEVBelegfeld(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || strings.ContainsRune("$&%*+-/", r) {
			b.WriteRune(r)
		}
	}
	return truncateDATEV(b.String(), 36)
}
