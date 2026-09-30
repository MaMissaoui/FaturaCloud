package db

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	gonanoid "github.com/matoous/go-nanoid/v2"
	"github.com/xuri/excelize/v2"
)

// The paper loan register import (docs/loan-register-migration-plan.md,
// phase 2). An admin downloads an Excel template, transcribes the register
// into it (one row per loan item), and uploads it with a cutover date:
//
//   - a dry run (the default) writes nothing and reports what the import
//     would do: loans, customers matched vs created, totals per customer to
//     tick against the paper book, and every problem row by row;
//   - the real import re-plans the same file and, only if it has no errors,
//     writes everything in ONE transaction — all or nothing — as a batch
//     (loan_import_batches, migration 0098) that can later be undone while
//     none of its loans has been collected in the app;
//   - a loan whose register reference was already imported is skipped, so a
//     corrected file can be uploaded again safely.
//
// Each loan becomes an opening loan (db/opening_loan.go). Sheet dates are
// calendar days, stored as local midnight in the organization's zone (the
// calendar-day invariant), like a date picked in the app.

const (
	loanColRef = iota
	loanColSaleDate
	loanColCustomer
	loanColCIN
	loanColPhone
	loanColPhone2
	loanColAddress
	loanColGuarantor
	loanColProduct
	loanColQuantity
	loanColAmount
	loanColPaid
	loanColLastPayment
	loanColNote
	loanColumnCount
)

var loanImportHeaders = []string{
	"Réf. prêt (Loan ref) *",
	"Date de vente (Sale date) *",
	"Client (Customer) *",
	"CIN",
	"Téléphone (Phone)",
	"Téléphone 2 (Phone 2)",
	"Adresse (Address)",
	"Garant (Guarantor)",
	"Article (Product) *",
	"Quantité (Quantity)",
	"Montant (Amount) *",
	"Déjà payé (Paid to date)",
	"Dernier paiement (Last payment)",
	"Note",
}

// LoanImportProblem is one finding of a dry run or import. Severity is
// "error" (blocks the import), "warning" (imported, but worth a look) or
// "skipped" (a loan already imported).
type LoanImportProblem struct {
	Row      int    `json:"row"`
	Ref      string `json:"ref"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

// LoanImportCustomer is one customer's totals, for ticking against the book.
type LoanImportCustomer struct {
	Name        string `json:"name"`
	CIN         string `json:"cin"`
	Phone       string `json:"phone"`
	New         bool   `json:"new"`
	Loans       int    `json:"loans"`
	Total       int64  `json:"total"`
	Paid        int64  `json:"paid"`
	Outstanding int64  `json:"outstanding"`
}

// LoanImportReport is what a dry run or an import returns. Imported is true
// only when the import actually wrote its batch.
type LoanImportReport struct {
	DryRun           bool                 `json:"dryRun"`
	Imported         bool                 `json:"imported"`
	BatchID          string               `json:"batchId,omitempty"`
	CutoverDate      int64                `json:"cutoverDate"`
	Loans            int                  `json:"loans"`
	OpenLoans        int                  `json:"openLoans"`
	SettledLoans     int                  `json:"settledLoans"`
	Lines            int                  `json:"lines"`
	SkippedLoans     int                  `json:"skippedLoans"`
	CustomersMatched int                  `json:"customersMatched"`
	CustomersCreated int                  `json:"customersCreated"`
	Total            int64                `json:"total"`
	Paid             int64                `json:"paid"`
	Outstanding      int64                `json:"outstanding"`
	Errors           int                  `json:"errors"`
	Warnings         int                  `json:"warnings"`
	Customers        []LoanImportCustomer `json:"customers"`
	Problems         []LoanImportProblem  `json:"problems"`
}

func (r *LoanImportReport) add(row int, ref, severity, format string, args ...any) {
	r.Problems = append(r.Problems, LoanImportProblem{Row: row, Ref: ref, Severity: severity, Message: fmt.Sprintf(format, args...)})
	switch severity {
	case "error":
		r.Errors++
	case "warning":
		r.Warnings++
	}
}

// LoanImportBatch is one import, as listed for undo.
type LoanImportBatch struct {
	ID               string  `db:"id"               json:"id"`
	OrganizationID   string  `db:"organizationId"   json:"organizationId"`
	FileName         *string `db:"fileName"         json:"fileName"`
	CutoverDate      int64   `db:"cutoverDate"      json:"cutoverDate"`
	LoanCount        int     `db:"loanCount"        json:"loanCount"`
	CustomersCreated int     `db:"customersCreated" json:"customersCreated"`
	Total            int64   `db:"total"            json:"total"`
	Outstanding      int64   `db:"outstanding"      json:"outstanding"`
	CreatedBy        *string `db:"createdBy"        json:"createdBy"`
	CreatedAt        int64   `db:"createdAt"        json:"createdAt"`
	UndoneAt         *int64  `db:"undoneAt"         json:"undoneAt"`
}

// ---- Template ----

// loanImportSheet is the data sheet's name. The reader looks for it by name
// (falling back to the first sheet, for a file typed from scratch), so
// reordering the sheets never makes it read the help text.
const loanImportSheet = "Prêts"

// LoanImportTemplateXLSX is the empty register template: the loan sheet
// (headers only — the example lives on the help sheet, so it can never be
// imported by mistake) and a "Mode d'emploi" sheet explaining each column.
func (d *Database) LoanImportTemplateXLSX() ([]byte, error) {
	f := excelize.NewFile()
	defer f.Close() //nolint:errcheck
	sheet := loanImportSheet
	if err := f.SetSheetName("Sheet1", sheet); err != nil {
		return nil, fmt.Errorf("loan_import template: %w", err)
	}
	bold, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
	if err != nil {
		return nil, fmt.Errorf("loan_import template style: %w", err)
	}
	for i, h := range loanImportHeaders {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		if err := f.SetCellValue(sheet, cell, h); err != nil {
			return nil, fmt.Errorf("loan_import template header: %w", err)
		}
		col, _ := excelize.ColumnNumberToName(i + 1)
		_ = f.SetColWidth(sheet, col, col, 18)
	}
	_ = f.SetRowStyle(sheet, 1, 1, bold)
	// Text columns: Excel would drop a CIN's or phone's leading zero from a
	// number cell (01234567 -> 1234567), and a reference like 0012 likewise.
	text, err := f.NewStyle(&excelize.Style{NumFmt: 49})
	if err != nil {
		return nil, fmt.Errorf("loan_import template text style: %w", err)
	}
	for _, col := range []int{loanColRef, loanColCIN, loanColPhone, loanColPhone2} {
		name, _ := excelize.ColumnNumberToName(col + 1)
		_ = f.SetColStyle(sheet, name, text)
	}

	help := "Mode d'emploi"
	if _, err := f.NewSheet(help); err != nil {
		return nil, fmt.Errorf("loan_import template help: %w", err)
	}
	lines := [][]string{
		{"Colonne", "Explication"},
		{"Réf. prêt *", "La référence du prêt dans le registre papier (ex. carnet 1, page 12, ligne 3 : C1-P012-3). Un prêt de plusieurs articles occupe une ligne par article, avec la même référence. Une référence déjà importée est ignorée : le fichier corrigé peut être importé à nouveau."},
		{"Date de vente *", "La date de la vente, jj/mm/aaaa. Elle doit être antérieure ou égale à la date de bascule."},
		{"Client *", "Nom du client. Le client existant est retrouvé par son CIN, sinon par nom + téléphone, sinon par nom seul s'il est unique ; sinon il est créé."},
		{"CIN, Téléphone, Téléphone 2, Adresse, Garant", "Facultatifs, mais le CIN et le téléphone évitent les confusions entre homonymes. Remplis sur la première ligne du prêt."},
		{"Article *", "Le code (SKU) ou le nom exact du produit ; sinon le texte est gardé tel quel comme description."},
		{"Quantité", "1 si vide."},
		{"Montant *", "Le prix de la ligne tel qu'écrit dans le registre (TTC), en dinars : 1200 ou 1200,500."},
		{"Déjà payé", "Le total déjà payé sur ce prêt (avance + versements), sur la première ligne du prêt. Égal au montant total : prêt soldé (historique). 0 ou vide : rien payé."},
		{"Dernier paiement", "Facultatif : la date du dernier versement."},
		{"Note", "Facultatif : échéancier, remarques…"},
		{"", ""},
		{"Avant l'import", "L'import se fait d'abord à blanc : il n'enregistre rien et affiche les totaux par client, à comparer avec le registre, et chaque problème ligne par ligne."},
		{"", ""},
		{"Exemple", "Un prêt de deux articles (la deuxième ligne répète la référence). À saisir dans la feuille « Prêts », pas ici :"},
	}
	for r, row := range lines {
		for c, v := range row {
			cell, _ := excelize.CoordinatesToCellName(c+1, r+1)
			_ = f.SetCellValue(help, cell, v)
		}
	}
	example := [][]any{
		loanImportHeadersAny(),
		{"C1-P012-3", "15/06/2024", "Ben Ali Mohamed", "01234567", "98 123 456", "", "Rue de Tunis, Sousse", "Ben Ali Salah", "Réfrigérateur Condor 400L", 1, 1200, 450, "10/01/2025", "12 x 100"},
		{"C1-P012-3", "15/06/2024", "Ben Ali Mohamed", "", "", "", "", "", "Micro-ondes", 1, 300, "", "", ""},
	}
	for r, row := range example {
		for c, v := range row {
			cell, _ := excelize.CoordinatesToCellName(c+1, len(lines)+r+2)
			_ = f.SetCellValue(help, cell, v)
		}
	}
	_ = f.SetColWidth(help, "A", "A", 28)
	_ = f.SetColWidth(help, "B", "B", 110)
	_ = f.SetRowStyle(help, 1, 1, bold)
	f.SetActiveSheet(0)

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, fmt.Errorf("loan_import template write: %w", err)
	}
	return buf.Bytes(), nil
}

func loanImportHeadersAny() []any {
	out := make([]any, len(loanImportHeaders))
	for i, h := range loanImportHeaders {
		out[i] = h
	}
	return out
}

// ---- Parsing ----

type loanSheetRow struct {
	row   int
	cells []string
}

type loanSheetLoan struct {
	ref       string
	firstRow  int
	rows      []loanSheetRow
	saleDate  int64
	customer  loanSheetCustomer
	paid      int64
	lastPaid  *int64
	notes     []string
	hasErrors bool
}

type loanSheetCustomer struct {
	name, cin, phone, phone2, address, guarantor string
}

var loanDateLayouts = []string{"02/01/2006", "2/1/2006", "02-01-2006", "2-1-2006", "02.01.2006", "2.1.2006", "2006-01-02", "02/01/06", "2/1/06"}

// parseLoanSheetDate reads a calendar day from a cell — an Excel date (raw
// serial number) or text in day-first form — as local midnight in loc.
func parseLoanSheetDate(v string, loc *time.Location) (int64, error) {
	v = strings.TrimSpace(v)
	if serial, err := strconv.ParseFloat(v, 64); err == nil {
		t, err := excelize.ExcelDateToTime(serial, false)
		if err != nil {
			return 0, fmt.Errorf("%q is not a date", v)
		}
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc).UnixMilli(), nil
	}
	for _, layout := range loanDateLayouts {
		if t, err := time.Parse(layout, v); err == nil {
			if t.Year() < 1970 || t.Year() > 2100 {
				break
			}
			return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc).UnixMilli(), nil
		}
	}
	return 0, fmt.Errorf("%q is not a date (use dd/mm/yyyy)", v)
}

var loanAmountSpaces = strings.NewReplacer(" ", "", " ", "", " ", "")

// parseLoanSheetNumber reads a number written the way the register is: a
// decimal comma or point, spaces as thousands separators.
func parseLoanSheetNumber(v string) (float64, error) {
	s := loanAmountSpaces.Replace(strings.TrimSpace(v))
	if strings.Contains(s, ",") {
		if strings.Contains(s, ".") {
			s = strings.ReplaceAll(s, ",", "") // "1,234.5"
		} else {
			s = strings.Replace(s, ",", ".", 1) // "1234,5"
		}
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, fmt.Errorf("%q is not a number", v)
	}
	return f, nil
}

func parseLoanSheetCents(v string) (int64, error) {
	f, err := parseLoanSheetNumber(v)
	if err != nil {
		return 0, err
	}
	return int64(math.Round(f * 100)), nil
}

func normalizePhone(v string) string {
	var b strings.Builder
	for _, r := range v {
		if unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func foldName(v string) string {
	return strings.ToLower(strings.Join(strings.Fields(v), " "))
}

// generateClientCodeGo is src/utils/client.ts's generateClientCode, so a
// customer the import creates gets the same kind of code as one created at
// the counter.
var clientCodeStrip = regexp.MustCompile(`[^a-zA-Z0-9\s]`)

func generateClientCodeGo(name string) string {
	words := strings.Fields(clientCodeStrip.ReplaceAllString(strings.TrimSpace(name), ""))
	switch {
	case len(words) == 0:
		return ""
	case len(words) <= 2:
		w := words[0]
		if len(w) >= 2 {
			return strings.ToUpper(w[:2])
		}
		return strings.ToUpper(w)
	default:
		return strings.ToUpper(words[0][:1] + words[1][:1])
	}
}

// readLoanSheet groups the sheet's rows into loans by reference, in order of
// first appearance, recording row-level problems.
func readLoanSheet(content []byte, loc *time.Location, cutover int64, report *LoanImportReport) ([]*loanSheetLoan, error) {
	f, err := excelize.OpenReader(bytes.NewReader(content))
	if err != nil {
		return nil, newValidationError("file is not a valid Excel (.xlsx) workbook")
	}
	defer f.Close() //nolint:errcheck
	sheet := f.GetSheetName(0)
	for _, name := range f.GetSheetList() {
		if name == loanImportSheet {
			sheet = name
		}
	}
	rows, err := f.GetRows(sheet, excelize.Options{RawCellValue: true})
	if err != nil {
		return nil, fmt.Errorf("loan_import read rows: %w", err)
	}

	byRef := map[string]*loanSheetLoan{}
	var loans []*loanSheetLoan
	for i, raw := range rows {
		rowNum := i + 1
		if rowNum == 1 {
			continue // header
		}
		cells := padCells(raw, loanColumnCount)
		if rowIsBlank(cells) {
			continue
		}
		for j := range cells {
			cells[j] = strings.TrimSpace(cells[j])
		}
		ref := cells[loanColRef]
		if ref == "" {
			report.add(rowNum, "", "error", "the loan reference is missing")
			continue
		}
		loan, ok := byRef[ref]
		if !ok {
			loan = &loanSheetLoan{ref: ref, firstRow: rowNum}
			byRef[ref] = loan
			loans = append(loans, loan)
		}
		loan.rows = append(loan.rows, loanSheetRow{row: rowNum, cells: cells})
	}

	for _, loan := range loans {
		readLoanHeader(loan, loc, cutover, report)
	}
	return loans, nil
}

// readLoanHeader reads the loan-level fields (sale date, customer, paid to
// date) from the loan's rows: the first non-empty value wins, and a
// different non-empty value on another row is an error.
func readLoanHeader(loan *loanSheetLoan, loc *time.Location, cutover int64, report *LoanImportReport) {
	fail := func(row int, format string, args ...any) {
		report.add(row, loan.ref, "error", format, args...)
		loan.hasErrors = true
	}
	pick := func(col int, what string) (string, int) {
		value, at := "", 0
		for _, r := range loan.rows {
			v := r.cells[col]
			if v == "" {
				continue
			}
			if value == "" {
				value, at = v, r.row
			} else if !strings.EqualFold(v, value) {
				fail(r.row, "%s %q differs from row %d's %q for the same loan", what, v, at, value)
			}
		}
		return value, at
	}

	// A 0 typed in "paid to date" on a loan's later rows means nothing was
	// written there, not a conflicting amount.
	for _, r := range loan.rows[1:] {
		if v := r.cells[loanColPaid]; v != "" {
			if f, err := parseLoanSheetNumber(v); err == nil && f == 0 {
				r.cells[loanColPaid] = ""
			}
		}
	}
	saleDate, _ := pick(loanColSaleDate, "sale date")
	name, _ := pick(loanColCustomer, "customer")
	cin, _ := pick(loanColCIN, "CIN")
	phone, _ := pick(loanColPhone, "phone")
	phone2, _ := pick(loanColPhone2, "phone 2")
	address, _ := pick(loanColAddress, "address")
	guarantor, _ := pick(loanColGuarantor, "guarantor")
	paid, paidRow := pick(loanColPaid, "paid to date")
	lastPaid, lastPaidRow := pick(loanColLastPayment, "last payment date")

	if saleDate == "" {
		fail(loan.firstRow, "the sale date is missing")
	} else if ms, err := parseLoanSheetDate(saleDate, loc); err != nil {
		fail(loan.firstRow, "sale date: %v", err)
	} else if ms > cutover {
		fail(loan.firstRow, "the sale date is after the cutover date")
	} else {
		loan.saleDate = ms
	}
	if name == "" {
		fail(loan.firstRow, "the customer name is missing")
	}
	loan.customer = loanSheetCustomer{
		name: strings.Join(strings.Fields(name), " "), cin: strings.ToUpper(cin), phone: phone,
		phone2: phone2, address: address, guarantor: guarantor,
	}
	if paid != "" {
		cents, err := parseLoanSheetCents(paid)
		switch {
		case err != nil:
			fail(paidRow, "paid to date: %v", err)
		case cents < 0:
			fail(paidRow, "paid to date can't be negative")
		default:
			loan.paid = cents
		}
	}
	if lastPaid != "" {
		ms, err := parseLoanSheetDate(lastPaid, loc)
		if err != nil {
			fail(lastPaidRow, "last payment date: %v", err)
		} else {
			loan.lastPaid = &ms
		}
	}
	for _, r := range loan.rows {
		if note := r.cells[loanColNote]; note != "" {
			loan.notes = append(loan.notes, note)
		}
	}
}

// ---- Planning ----

type loanImportIndex struct {
	byCIN       map[string][]*Client
	byName      map[string][]*Client
	byPhone     map[string][]*Client
	productSKU  map[string]string
	productName map[string][]Product
	imported    map[string]bool
}

// normalizeCIN folds a CIN for matching: case and spaces don't count, and an
// all-digit one ignores leading zeros — Excel drops them from a CIN typed
// into a number cell (01234567 becomes 1234567).
func normalizeCIN(v string) string {
	s := strings.ToUpper(strings.Join(strings.Fields(v), ""))
	if s != "" && strings.Trim(s, "0123456789") == "" {
		if t := strings.TrimLeft(s, "0"); t != "" {
			return t
		}
		return "0"
	}
	return s
}

// addClient indexes a client — one from the database, or one this import
// creates, so a later loan of the same customer finds it by the same rules.
func (idx *loanImportIndex) addClient(c *Client) {
	if c.IdentityNumber != nil {
		if k := normalizeCIN(*c.IdentityNumber); k != "" {
			idx.byCIN[k] = append(idx.byCIN[k], c)
		}
	}
	if c.Name != nil {
		k := foldName(*c.Name)
		idx.byName[k] = append(idx.byName[k], c)
	}
	for _, p := range []*string{c.Phone, c.Phone2, c.Phone3} {
		if p != nil {
			if k := normalizePhone(*p); k != "" {
				idx.byPhone[k] = append(idx.byPhone[k], c)
			}
		}
	}
}

// enrichNewClient copies onto a customer this import creates what a later
// loan of theirs adds — the CIN written on one page only, a second phone,
// the address — so the created record is as complete as the register.
func (idx *loanImportIndex) enrichNewClient(c *Client, s loanSheetCustomer) {
	if c.IdentityNumber == nil && s.cin != "" {
		c.IdentityNumber = &s.cin
		idx.byCIN[normalizeCIN(s.cin)] = append(idx.byCIN[normalizeCIN(s.cin)], c)
	}
	for _, phone := range []string{s.phone, s.phone2} {
		n := normalizePhone(phone)
		if n == "" || clientHasPhone(c, n) {
			continue
		}
		p := phone
		switch {
		case c.Phone == nil:
			c.Phone = &p
		case c.Phone2 == nil:
			c.Phone2 = &p
		default:
			continue
		}
		idx.byPhone[n] = append(idx.byPhone[n], c)
	}
	if c.Address == nil && s.address != "" {
		c.Address = &s.address
	}
	if c.Guarantor == nil && s.guarantor != "" {
		c.Guarantor = &s.guarantor
	}
}

func (d *Database) loanImportIndex(organizationID string) (*loanImportIndex, error) {
	clients, err := d.GetClients(organizationID)
	if err != nil {
		return nil, err
	}
	products, _, err := d.GetProducts(organizationID, ProductListOptions{})
	if err != nil {
		return nil, err
	}
	var imported []string
	if err := d.DB.Select(&imported,
		`SELECT number FROM invoices WHERE organizationId = ? AND origin = ?`, organizationID, OpeningOrigin,
	); err != nil {
		return nil, fmt.Errorf("loan_import imported refs: %w", err)
	}
	idx := &loanImportIndex{
		byCIN: map[string][]*Client{}, byName: map[string][]*Client{}, byPhone: map[string][]*Client{},
		productSKU: map[string]string{}, productName: map[string][]Product{}, imported: map[string]bool{},
	}
	for i := range clients {
		idx.addClient(&clients[i])
	}
	for _, p := range products {
		if p.SKU != nil && *p.SKU != "" {
			idx.productSKU[*p.SKU] = p.ID
		}
		idx.productName[foldName(p.Name)] = append(idx.productName[foldName(p.Name)], p)
	}
	for _, n := range imported {
		idx.imported[n] = true
	}
	return idx, nil
}

func clientHasPhone(c *Client, phone string) bool {
	for _, p := range []*string{c.Phone, c.Phone2, c.Phone3} {
		if p != nil && normalizePhone(*p) == phone {
			return true
		}
	}
	return false
}

func clientName(c *Client) string {
	if c.Name == nil {
		return ""
	}
	return *c.Name
}

// matchCustomer finds the client a sheet customer is — in the database or
// created earlier in this file — or returns nil for a new one. how says what
// matched ("cin", "name+phone" or "name"). A phone alone never matches —
// families share one — and anything ambiguous is an error, not a guess.
func (idx *loanImportIndex) matchCustomer(c loanSheetCustomer) (client *Client, how, warning string, err error) {
	if cin := normalizeCIN(c.cin); cin != "" {
		switch matches := idx.byCIN[cin]; len(matches) {
		case 0:
			// fall through to the name
		case 1:
			if foldName(clientName(matches[0])) != foldName(c.name) {
				warning = fmt.Sprintf("CIN %s belongs to customer %q; the loan goes to that customer", c.cin, clientName(matches[0]))
			}
			return matches[0], "cin", warning, nil
		default:
			return nil, "", "", fmt.Errorf("%d customers have CIN %s — merge them first", len(matches), c.cin)
		}
	}
	phone := normalizePhone(c.phone)
	named := idx.byName[foldName(c.name)]
	if phone != "" {
		var both []*Client
		for _, cl := range named {
			if clientHasPhone(cl, phone) {
				both = append(both, cl)
			}
		}
		if len(both) == 1 {
			return both[0], "name+phone", "", nil
		}
		if len(both) > 1 {
			return nil, "", "", fmt.Errorf("%d customers are named %q with phone %s — add the CIN", len(both), c.name, c.phone)
		}
		for _, owner := range idx.byPhone[phone] {
			if foldName(clientName(owner)) != foldName(c.name) {
				return nil, "", "", fmt.Errorf("phone %s belongs to customer %q, not %q — add the CIN or check the name", c.phone, clientName(owner), c.name)
			}
		}
	}
	switch len(named) {
	case 0:
		return nil, "", "", nil
	case 1:
		if phone != "" && (named[0].Phone != nil || named[0].Phone2 != nil || named[0].Phone3 != nil) && !clientHasPhone(named[0], phone) {
			return nil, "", "", fmt.Errorf("customer %q has a different phone — add the CIN if it's the same person", c.name)
		}
		return named[0], "name", "", nil
	default:
		return nil, "", "", fmt.Errorf("%d customers are named %q — add the CIN or phone", len(named), c.name)
	}
}

func (idx *loanImportIndex) matchProduct(v string) (*string, string) {
	if id, ok := idx.productSKU[v]; ok {
		return &id, ""
	}
	switch matches := idx.productName[foldName(v)]; len(matches) {
	case 1:
		return &matches[0].ID, ""
	case 0:
		return nil, fmt.Sprintf("no product %q — kept as a description", v)
	default:
		return nil, fmt.Sprintf("%d products are named %q — kept as a description; use the SKU to link one", len(matches), v)
	}
}

type loanImportPlan struct {
	plans      []*openingLoanPlan
	newClients []*Client
}

// planLoanImport reads and resolves the whole file without writing anything.
// cutoverDay is the picked cutover date as calendarDayMs (UTC noon of that
// day, like every day-based query); the cutover itself is that day's local
// midnight in the organization's zone.
func (d *Database) planLoanImport(organizationID string, cutoverDay int64, content []byte) (*LoanImportReport, *loanImportPlan, error) {
	if cutoverDay <= 0 {
		return nil, nil, newValidationError("the cutover date is required")
	}
	org, err := d.GetOrganization(organizationID)
	if err != nil {
		return nil, nil, fmt.Errorf("loan_import organization: %w", err)
	}
	loc := orgLocation(org.Timezone)
	cutoverDate := calendarDayStart(cutoverDay, loc).UnixMilli()
	report := &LoanImportReport{DryRun: true, CutoverDate: cutoverDate, Customers: []LoanImportCustomer{}, Problems: []LoanImportProblem{}}
	loans, err := readLoanSheet(content, loc, cutoverDate, report)
	if err != nil {
		return nil, nil, err
	}
	idx, err := d.loanImportIndex(organizationID)
	if err != nil {
		return nil, nil, err
	}

	plan := &loanImportPlan{}
	newFromLoan := map[string]string{} // created customer id -> the loan that created it
	cinName := map[string]string{}
	clients := map[string]*Client{}
	totals := map[string]*LoanImportCustomer{}
	var order []string
	needsFiscalYear := false

	for _, loan := range loans {
		if idx.imported[loan.ref] {
			report.SkippedLoans++
			report.add(loan.firstRow, loan.ref, "skipped", "already imported — skipped")
			continue
		}
		c := loan.customer
		if cin := normalizeCIN(c.cin); cin != "" && c.name != "" {
			if prev, ok := cinName[cin]; ok && foldName(prev) != foldName(c.name) {
				report.add(loan.firstRow, loan.ref, "error", "CIN %s is on both %q and %q in this file", c.cin, prev, c.name)
				loan.hasErrors = true
			} else {
				cinName[cin] = c.name
			}
		}

		var client *Client
		if c.name != "" && !loan.hasErrors {
			matched, how, warning, err := idx.matchCustomer(c)
			switch {
			case err != nil:
				report.add(loan.firstRow, loan.ref, "error", "%v", err)
				loan.hasErrors = true
			case matched != nil:
				client = matched
				if from, isNew := newFromLoan[matched.ID]; isNew {
					idx.enrichNewClient(matched, c)
					if how == "name" {
						report.add(loan.firstRow, loan.ref, "warning",
							"matched to the new customer %q from loan %s by name only — check it's the same person", clientName(matched), from)
					}
				} else if warning != "" {
					report.add(loan.firstRow, loan.ref, "warning", "%s", warning)
				}
			default:
				id, _ := gonanoid.New()
				name := c.name
				client = &Client{
					ID: id, OrganizationID: organizationID, Name: &name,
					Code:           nilIfEmpty(generateClientCodeGo(name)),
					IdentityNumber: nilIfEmpty(c.cin), Phone: nilIfEmpty(c.phone), Phone2: nilIfEmpty(c.phone2),
					Address: nilIfEmpty(c.address), Guarantor: nilIfEmpty(c.guarantor),
				}
				idx.addClient(client)
				newFromLoan[id] = loan.ref
				plan.newClients = append(plan.newClients, client)
			}
		}

		var lines []OpeningLoanLine
		for _, r := range loan.rows {
			product := r.cells[loanColProduct]
			if product == "" {
				report.add(r.row, loan.ref, "error", "the item is missing")
				loan.hasErrors = true
				continue
			}
			qty := 1.0
			if v := r.cells[loanColQuantity]; v != "" {
				q, err := parseLoanSheetNumber(v)
				if err != nil || q <= 0 {
					report.add(r.row, loan.ref, "error", "quantity %q must be a number above zero", v)
					loan.hasErrors = true
					continue
				}
				qty = q
			}
			amount, err := parseLoanSheetCents(r.cells[loanColAmount])
			if r.cells[loanColAmount] == "" || err != nil || amount <= 0 {
				report.add(r.row, loan.ref, "error", "the amount must be a number above zero")
				loan.hasErrors = true
				continue
			}
			productID, warning := idx.matchProduct(product)
			if warning != "" {
				report.add(r.row, loan.ref, "warning", "%s", warning)
			}
			lines = append(lines, OpeningLoanLine{ProductID: productID, Description: product, Quantity: qty, Amount: amount})
		}
		if loan.hasErrors || client == nil {
			continue
		}

		var notes *string
		if len(loan.notes) > 0 {
			n := strings.Join(loan.notes, " — ")
			notes = &n
		}
		planned, err := d.planOpeningLoan(org, OpeningLoanRequest{
			OrganizationID: organizationID, ClientID: client.ID, Number: loan.ref,
			Date: loan.saleDate, CutoverDate: cutoverDate, Lines: lines,
			PaidToDate: loan.paid, LastPaymentDate: loan.lastPaid, Notes: notes,
		})
		if err != nil {
			var verr *ValidationError
			if !errors.As(err, &verr) {
				return nil, nil, err
			}
			report.add(loan.firstRow, loan.ref, "error", "%v", err)
			continue
		}
		plan.plans = append(plan.plans, planned)

		outstanding := planned.total - loan.paid
		report.Loans++
		report.Lines += len(lines)
		report.Total += planned.total
		report.Paid += loan.paid
		report.Outstanding += outstanding
		if outstanding == 0 {
			report.SettledLoans++
		} else {
			report.OpenLoans++
			needsFiscalYear = true
		}
		t, ok := totals[client.ID]
		if !ok {
			_, isNew := newFromLoan[client.ID]
			t = &LoanImportCustomer{New: isNew}
			totals[client.ID] = t
			clients[client.ID] = client
			order = append(order, client.ID)
		}
		t.Loans++
		t.Total += planned.total
		t.Paid += loan.paid
		t.Outstanding += outstanding
	}

	if needsFiscalYear {
		if _, _, err := resolveFiscalPeriodForDate(d.DB, organizationID, cutoverDate); err != nil {
			report.add(0, "", "error", "cutover date: %v", err)
		}
	}

	// Only customers that end up with a planned loan are created.
	var kept []*Client
	for _, nc := range plan.newClients {
		if _, ok := totals[nc.ID]; ok {
			kept = append(kept, nc)
		}
	}
	plan.newClients = kept
	for _, id := range order {
		t, c := totals[id], clients[id]
		t.Name = clientName(c)
		if c.IdentityNumber != nil {
			t.CIN = *c.IdentityNumber
		}
		if c.Phone != nil {
			t.Phone = *c.Phone
		}
		if t.New {
			report.CustomersCreated++
		} else {
			report.CustomersMatched++
		}
		report.Customers = append(report.Customers, *t)
	}
	sort.SliceStable(report.Customers, func(a, b int) bool {
		return foldName(report.Customers[a].Name) < foldName(report.Customers[b].Name)
	})
	sort.SliceStable(report.Problems, func(a, b int) bool { return report.Problems[a].Row < report.Problems[b].Row })
	return report, plan, nil
}

// DryRunLoanImport reports what importing the file would do, writing nothing.
// cutoverDay is a calendarDayMs (see planLoanImport).
func (d *Database) DryRunLoanImport(organizationID string, cutoverDay int64, content []byte) (*LoanImportReport, error) {
	report, _, err := d.planLoanImport(organizationID, cutoverDay, content)
	return report, err
}

// ImportLoanRegister imports the file as one batch, all or nothing. A file
// with any error imports nothing and returns its report (Imported false).
func (d *Database) ImportLoanRegister(organizationID, userID, fileName string, cutoverDay int64, content []byte) (*LoanImportReport, error) {
	report, plan, err := d.planLoanImport(organizationID, cutoverDay, content)
	if err != nil {
		return nil, err
	}
	cutoverDate := report.CutoverDate
	report.DryRun = false
	if report.Errors > 0 || len(plan.plans) == 0 {
		if len(plan.plans) == 0 && report.Errors == 0 {
			report.add(0, "", "error", "there is nothing to import")
		}
		return report, nil
	}

	batchID, err := gonanoid.New()
	if err != nil {
		return nil, fmt.Errorf("loan_import new_id: %w", err)
	}
	tx, err := d.DB.Beginx()
	if err != nil {
		return nil, fmt.Errorf("loan_import begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.Exec(`
		INSERT INTO loan_import_batches (id, organizationId, fileName, cutoverDate, loanCount, customersCreated, total, outstanding, createdBy)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		batchID, organizationID, nilIfEmpty(fileName), cutoverDate, report.Loans, report.CustomersCreated,
		report.Total, report.Outstanding, nilIfEmpty(userID),
	); err != nil {
		return nil, fmt.Errorf("loan_import insert_batch: %w", err)
	}
	for _, c := range plan.newClients {
		if _, err := tx.Exec(`
			INSERT INTO clients (id, organizationId, name, code, identity_number, phone, phone2, address, guarantor, importBatchId)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			c.ID, organizationID, c.Name, c.Code, c.IdentityNumber, c.Phone, c.Phone2, c.Address, c.Guarantor, batchID,
		); err != nil {
			return nil, fmt.Errorf("loan_import insert_client: %w", err)
		}
	}
	for _, p := range plan.plans {
		p.req.ImportBatchID = &batchID
		if _, err := applyOpeningLoanTx(tx, p); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("loan_import commit: %w", err)
	}
	report.Imported = true
	report.BatchID = batchID
	return report, nil
}

func nilIfEmpty(s string) *string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return &s
}

// ---- Batches and undo ----

func (d *Database) GetLoanImportBatches(organizationID string) ([]LoanImportBatch, error) {
	batches := []LoanImportBatch{}
	if err := d.DB.Select(&batches,
		`SELECT * FROM loan_import_batches WHERE organizationId = ? ORDER BY createdAt DESC`, organizationID,
	); err != nil {
		return nil, fmt.Errorf("get_loan_import_batches: %w", err)
	}
	return batches, nil
}

func (d *Database) GetLoanImportBatch(id string) (*LoanImportBatch, error) {
	var b LoanImportBatch
	if err := d.DB.Get(&b, `SELECT * FROM loan_import_batches WHERE id = ?`, id); err != nil {
		return nil, err
	}
	return &b, nil
}

// LoanImportUndoResult is what undoing a batch removed.
type LoanImportUndoResult struct {
	LoansRemoved     int `json:"loansRemoved"`
	CustomersRemoved int `json:"customersRemoved"`
	CustomersKept    int `json:"customersKept"`
}

// loanImportCollectedQuery counts payments other than the imported
// paid-to-date — voided ones too — applied to a batch's loans: once the app
// has recorded anything against them, undo is no longer a clean removal.
const loanImportCollectedQuery = `
	SELECT COUNT(*) FROM payment_applications pa
	JOIN payments p ON p.id = pa.paymentId
	WHERE pa.documentType = 'invoice'
	  AND pa.documentId IN (SELECT id FROM invoices WHERE importBatchId = ?)
	  AND (p.origin IS NULL OR p.origin <> 'opening')`

// UndoLoanImportBatch removes an import while none of its loans has been
// collected in the app: their opening entries are reversed (posted entries
// are never deleted), and the loans, their paid-to-date records and the
// customers the import created are deleted — a customer only if nothing
// else references it by then (a later sale, or the reversed entries' AR
// lines, which keep it); the others stay, and a re-import matches them.
func (d *Database) UndoLoanImportBatch(batchID string) (*LoanImportUndoResult, error) {
	batch, err := d.GetLoanImportBatch(batchID)
	if err != nil {
		return nil, err
	}
	if batch.UndoneAt != nil {
		return nil, newValidationError("this import has already been undone")
	}
	var collected int
	if err := d.DB.Get(&collected, loanImportCollectedQuery, batchID); err != nil {
		return nil, fmt.Errorf("undo_loan_import collected: %w", err)
	}
	if collected > 0 {
		return nil, newValidationError("loans from this import have been collected in the app since — it can no longer be undone")
	}

	tx, err := d.DB.Beginx()
	if err != nil {
		return nil, fmt.Errorf("undo_loan_import begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if err := tx.Get(&collected, loanImportCollectedQuery, batchID); err != nil {
		return nil, fmt.Errorf("undo_loan_import collected recheck: %w", err)
	}
	if collected > 0 {
		return nil, newValidationError("loans from this import have been collected in the app since — it can no longer be undone")
	}
	var invoiceIDs []string
	if err := tx.Select(&invoiceIDs, `SELECT id FROM invoices WHERE importBatchId = ?`, batchID); err != nil {
		return nil, fmt.Errorf("undo_loan_import invoices: %w", err)
	}
	for _, id := range invoiceIDs {
		entry, err := findPostedEntryForSourceDocumentTx(tx, openingLoanSourceType, id)
		if err != nil {
			return nil, err
		}
		if entry != nil {
			if _, err := reverseEntryTx(tx, entry, "loan register import undone", batch.CutoverDate); err != nil {
				return nil, err
			}
		}
	}
	if _, err := tx.Exec(`
		DELETE FROM payments WHERE origin = ? AND id IN (
			SELECT paymentId FROM payment_applications
			WHERE documentType = 'invoice' AND documentId IN (SELECT id FROM invoices WHERE importBatchId = ?))`,
		OpeningOrigin, batchID,
	); err != nil {
		return nil, fmt.Errorf("undo_loan_import payments: %w", err)
	}
	res, err := tx.Exec(`DELETE FROM invoices WHERE importBatchId = ? AND origin = ?`, batchID, OpeningOrigin)
	if err != nil {
		return nil, fmt.Errorf("undo_loan_import invoices: %w", err)
	}
	loansRemoved, _ := res.RowsAffected()

	// A created customer goes only if nothing references it any more —
	// deleting a client cascades to its invoices, so any later document
	// keeps it, and journal lines (the reversed entries' AR lines) can't
	// lose theirs.
	res, err = tx.Exec(`
		DELETE FROM clients WHERE importBatchId = ?
		  AND NOT EXISTS (SELECT 1 FROM invoices i WHERE i.clientId = clients.id)
		  AND NOT EXISTS (SELECT 1 FROM orders o WHERE o.clientId = clients.id)
		  AND NOT EXISTS (SELECT 1 FROM outbound_deliveries od WHERE od.clientId = clients.id)
		  AND NOT EXISTS (SELECT 1 FROM payments p WHERE p.clientId = clients.id)
		  AND NOT EXISTS (SELECT 1 FROM journal_lines jl WHERE jl.clientId = clients.id)`,
		batchID,
	)
	if err != nil {
		return nil, fmt.Errorf("undo_loan_import clients: %w", err)
	}
	customersRemoved, _ := res.RowsAffected()
	if _, err := tx.Exec(`UPDATE loan_import_batches SET undoneAt = ? WHERE id = ?`, time.Now().UnixMilli(), batchID); err != nil {
		return nil, fmt.Errorf("undo_loan_import mark: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("undo_loan_import commit: %w", err)
	}
	return &LoanImportUndoResult{
		LoansRemoved:     int(loansRemoved),
		CustomersRemoved: int(customersRemoved),
		CustomersKept:    batch.CustomersCreated - int(customersRemoved),
	}, nil
}
