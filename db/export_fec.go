package db

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// fecColumns are the 18 mandatory FEC columns, in the mandated order
// (arrêté du 29 juillet 2013 / BOI-CF-CPF-10-40-30). This is a legally
// specified, stable format — unlike the DATEV exporter (deliberately not
// implemented, see the GL Export page), there's no ambiguity here to hedge
// against.
var fecColumns = []string{
	"JournalCode", "JournalLib", "EcritureNum", "EcritureDate",
	"CompteNum", "CompteLib", "CompAuxNum", "CompAuxLib",
	"PieceRef", "PieceDate", "EcritureLib", "Debit", "Credit",
	"EcritureLet", "DateLet", "ValidDate", "Montantdevise", "Idevise",
}

// fecRow is what the export query returns per journal_lines row — FEC is
// one row per line, not per entry, unlike the trial balance / P&L / balance
// sheet reports which aggregate by account.
type fecRow struct {
	JournalCode      string  `db:"journalCode"`
	JournalName      string  `db:"journalName"`
	EntryNumber      int64   `db:"entryNumber"`
	Date             int64   `db:"date"`
	AccountCode      string  `db:"accountCode"`
	AccountName      string  `db:"accountName"`
	ClientCode       *string `db:"clientCode"`
	ClientName       *string `db:"clientName"`
	VendorCode       *string `db:"vendorCode"`
	VendorName       *string `db:"vendorName"`
	Reference        *string `db:"reference"`
	LineDescription  *string `db:"lineDescription"`
	EntryDescription string  `db:"entryDescription"`
	Debit            int64   `db:"debit"`
	Credit           int64   `db:"credit"`
	Currency         *string `db:"currency"`
	ForeignAmount    *int64  `db:"foreignAmount"`
	PostedAt         *int64  `db:"postedAt"`
}

// GenerateFEC renders every posted (or reversed — same reasoning as
// GetTrialBalance: a reversed entry's lines are real history) journal line
// in a fiscal year as a France FEC flat file: tab-separated, one header row
// of exact column names, one data row per journal_lines row, in
// EcritureDate/EcritureNum/position order.
//
// Left deliberately unimplemented:
//   - EcritureLet/DateLet (lettrage) are always blank — CreatePayment
//     (db/payment.go) doesn't tag settled AR/AP lines with a
//     reconciliation_groups code yet ("deliberately not implemented yet…
//     deferred until the exporter's exact needs are known"). Blank is a
//     legal value for an unlettered line, not a missing-field error.
//   - PieceDate is always the entry's own date — there's no separate
//     "source document date" column on journal_entries to read instead.
//
// ValidDate is each entry's own postedAt (the moment
// allocateAndFinalizeEntryTx stamped it), not fiscal_years.lockDate —
// lockDate is a year-level lock, not a per-entry validation date, and is
// always NULL until Phase 6 exists to set it.
//
// Returns the rendered file content and the mandated SirenFECAAAAMMJJ.txt
// filename together, so callers (the API handler) don't need a second
// lookup of the organization/fiscal year just to name the download.
func (d *Database) GenerateFEC(organizationID, fiscalYearID string) (content []byte, filename string, err error) {
	org, err := d.GetOrganization(organizationID)
	if err != nil {
		return nil, "", fmt.Errorf("generate_fec get_organization: %w", err)
	}
	fiscalYear, err := d.GetFiscalYear(fiscalYearID)
	if err != nil {
		return nil, "", fmt.Errorf("generate_fec get_fiscal_year: %w", err)
	}
	if fiscalYear.OrganizationID != organizationID {
		return nil, "", newValidationError("fiscal year does not belong to this organization")
	}

	siren, err := validateSIREN(org.RegistrationNumber)
	if err != nil {
		return nil, "", err
	}
	filename = fmt.Sprintf("%sFEC%s.txt", siren, formatFECDate(fiscalYear.EndDate))

	rows := []fecRow{}
	err = d.DB.Select(&rows, `
		SELECT
			j.code AS journalCode, j.name AS journalName,
			je.entryNumber AS entryNumber, je.date AS date,
			a.code AS accountCode, a.name AS accountName,
			c.code AS clientCode, c.name AS clientName,
			v.code AS vendorCode, v.name AS vendorName,
			je.reference AS reference,
			jl.description AS lineDescription, je.description AS entryDescription,
			jl.debit AS debit, jl.credit AS credit,
			jl.currency AS currency, jl.foreignAmount AS foreignAmount, je.postedAt AS postedAt
		FROM journal_lines jl
		JOIN journal_entries je ON je.id = jl.journalEntryId
		JOIN journals j ON j.id = je.journalId
		JOIN accounts a ON a.id = jl.accountId
		LEFT JOIN clients c ON c.id = jl.clientId
		LEFT JOIN vendors v ON v.id = jl.vendorId
		WHERE je.organizationId = ? AND je.fiscalYearId = ?
		      AND je.status IN ('posted', 'reversed') AND je.entryNumber IS NOT NULL
		ORDER BY je.date ASC, je.entryNumber ASC, jl.position ASC`,
		organizationID, fiscalYearID,
	)
	if err != nil {
		return nil, "", fmt.Errorf("generate_fec select: %w", err)
	}

	var b strings.Builder
	b.WriteString(strings.Join(fecColumns, "\t"))
	b.WriteString("\n")
	for _, r := range rows {
		date := formatFECDate(r.Date)
		compAuxNum, compAuxLib := "", ""
		if r.ClientCode != nil || r.ClientName != nil {
			compAuxNum, compAuxLib = strPtr(r.ClientCode), strPtr(r.ClientName)
		} else if r.VendorCode != nil || r.VendorName != nil {
			compAuxNum, compAuxLib = strPtr(r.VendorCode), strPtr(r.VendorName)
		}
		ecritureLib := strPtr(r.LineDescription)
		if ecritureLib == "" {
			ecritureLib = r.EntryDescription
		}
		montantDevise, iDevise := "", ""
		if r.Currency != nil && r.ForeignAmount != nil {
			montantDevise = formatFECAmount(*r.ForeignAmount)
			iDevise = *r.Currency
		}
		var validDate string
		if r.PostedAt != nil {
			validDate = formatFECDate(*r.PostedAt)
		}

		// Every free-text field is sanitized: a tab or newline typed into an
		// account name, a client/vendor name, a reference, or a description
		// would otherwise shift every subsequent column on the row and
		// silently corrupt the file (still 18 fields per split, just the
		// wrong values in them) rather than raise an error anywhere.
		fields := []string{
			fecField(r.JournalCode), fecField(r.JournalName), strconv.FormatInt(r.EntryNumber, 10), date,
			fecField(r.AccountCode), fecField(r.AccountName), fecField(compAuxNum), fecField(compAuxLib),
			fecField(strPtr(r.Reference)), date, fecField(ecritureLib),
			formatFECAmount(r.Debit), formatFECAmount(r.Credit),
			"", "", validDate, montantDevise, iDevise,
		}
		b.WriteString(strings.Join(fields, "\t"))
		b.WriteString("\n")
	}

	return []byte(b.String()), filename, nil
}

// validateSIREN strips spaces/dashes and requires exactly 9 digits — FEC's
// filename convention names the organization's SIREN, not just "some
// registration number".
func validateSIREN(registrationNumber *string) (string, error) {
	if registrationNumber == nil {
		return "", newValidationError("organization registration number (SIREN) is required for FEC export")
	}
	siren := strings.NewReplacer(" ", "", "-", "").Replace(*registrationNumber)
	if len(siren) != 9 {
		return "", newValidationError("organization registration number must be a 9-digit SIREN for FEC export")
	}
	for _, c := range siren {
		if c < '0' || c > '9' {
			return "", newValidationError("organization registration number must be a 9-digit SIREN for FEC export")
		}
	}
	return siren, nil
}

func formatFECDate(millis int64) string {
	return time.UnixMilli(millis).UTC().Format("20060102")
}

// formatFECAmount renders integer cents as a plain decimal with 2 places
// and a "." separator — accepted by the FEC spec since its 2019 update
// (previously comma-only); never negative, never blank (a zero side is
// "0.00", not "").
func formatFECAmount(cents int64) string {
	if cents < 0 {
		cents = -cents
	}
	return fmt.Sprintf("%d.%02d", cents/100, cents%100)
}

// fecField neutralizes the tab-separated format's own delimiters — a tab or
// newline typed into a free-text source column (account/client/vendor name,
// reference, description) would otherwise shift every later field on the
// row. Applied to every free-text field, never to the generated numeric/
// date/code fields that can't contain one.
var fecFieldReplacer = strings.NewReplacer("\t", " ", "\n", " ", "\r", " ")

func fecField(s string) string {
	return fecFieldReplacer.Replace(s)
}

func strPtr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
