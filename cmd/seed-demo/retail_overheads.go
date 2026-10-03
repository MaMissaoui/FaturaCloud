package main

import (
	"fmt"
	"time"

	"github.com/MaMissaoui/fatura-cloud/db"
)

// The retail shop's running costs and its other bookkeeping. Before this the
// Profit & Loss had only cost of goods sold under expenses, the balance sheet
// had no equity, and the journal had no manual entry at all. A real shop pays
// rent, electricity, water, telephone and its staff every month, and its
// accountant books some of that by hand:
//
//   - utility and rent bills arrive as vendor bills with no purchase order,
//     each line on an expense product carrying its own expense account;
//   - salaries and bank charges are manual journal entries, posted;
//   - one salary entry is booked twice by mistake and the duplicate reversed;
//   - one entry is left in draft at the end, as an accountant's work in
//     progress.

// retailAccounts are the accounts the retail scenario adds to the generic
// chart, by code.
var retailAccounts = []struct {
	code, name, kind, parent string
}{
	{"3200", "Capital social", "equity", "3000"},
	{"2300", "Droit de timbre à payer", "liability", "2000"},
	{"2400", "Provision pour congés payés", "liability", "2000"},
	{"5300", "Loyer", "expense", "5000"},
	{"5310", "Électricité et eau", "expense", "5000"},
	{"5320", "Télécommunications", "expense", "5000"},
	{"5400", "Salaires et charges sociales", "expense", "5000"},
	{"5500", "Frais bancaires", "expense", "5000"},
}

// overheadVendor is a vendor billing the shop a running cost every
// everyMonths months, on dayOfMonth, for an amount in amountLo..amountHi
// (cents, before tax), in summer up to summerFactor times higher.
type overheadVendor struct {
	name, product, sku, account string
	everyMonths, dayOfMonth     int
	amountLo, amountHi          int64
	summerFactor                float64
	reducedTax                  bool
	dueDays                     int
}

var overheadVendors = []overheadVendor{
	{name: "Société Immobilière El Menzah", product: "Loyer du local commercial", sku: "FRAIS-LOYER", account: "5300",
		everyMonths: 1, dayOfMonth: 1, amountLo: 280000, amountHi: 280000, summerFactor: 1, dueDays: 10},
	{name: "STEG", product: "Consommation d'électricité", sku: "FRAIS-ELEC", account: "5310",
		everyMonths: 2, dayOfMonth: 12, amountLo: 60000, amountHi: 90000, summerFactor: 1.8, dueDays: 15},
	{name: "SONEDE", product: "Consommation d'eau", sku: "FRAIS-EAU", account: "5310",
		everyMonths: 3, dayOfMonth: 20, amountLo: 8000, amountHi: 15000, summerFactor: 1.3, reducedTax: true, dueDays: 15},
	{name: "Tunisie Telecom", product: "Abonnement téléphone et internet", sku: "FRAIS-TEL", account: "5320",
		everyMonths: 1, dayOfMonth: 5, amountLo: 11900, amountHi: 13900, summerFactor: 1, dueDays: 20},
}

type overheadRef struct {
	overheadVendor
	vendorID, productID, taxRateID string
	taxPercent                     float64
}

// setupRetailBookkeeping adds the scenario's accounts, wires the fiscal stamp
// (timbre fiscal: 1 dinar on every business invoice, credited to its own
// liability account), and creates the overhead vendors and their expense
// products. The expense products are kept out of s.products: nothing sells
// them.
func (s *Seeder) setupRetailBookkeeping() error {
	var accounts []db.Account
	if err := s.c.Get("/api/organizations/"+s.orgID+"/accounts", &accounts); err != nil {
		return fmt.Errorf("list accounts: %w", err)
	}
	byCode := map[string]string{}
	for _, a := range accounts {
		byCode[a.Code] = a.ID
	}
	for _, a := range retailAccounts {
		if byCode[a.code] != "" {
			continue
		}
		parent := byCode[a.parent]
		req := db.CreateAccountRequest{OrganizationID: s.orgID, ParentID: nonEmptyStrPtr(parent), Code: a.code, Name: a.name, Type: a.kind}
		var created db.Account
		if err := s.c.Post("/api/accounts", req, &created); err != nil {
			return fmt.Errorf("account %s %s: %w", a.code, a.name, err)
		}
		byCode[a.code] = created.ID
	}
	s.accountByCode = byCode

	stampAccount := byCode["2300"]
	if err := s.c.Put("/api/organizations/"+s.orgID, db.UpdateOrganizationRequest{
		DefaultStampDutyAccountID: &stampAccount,
		DefaultFiscalStampAmount:  int64Ptr(fiscalStampCents),
	}, nil); err != nil {
		return fmt.Errorf("fiscal stamp settings: %w", err)
	}

	var journals []db.Journal
	if err := s.c.Get("/api/organizations/"+s.orgID+"/journals", &journals); err != nil {
		return fmt.Errorf("list journals: %w", err)
	}
	s.journalByType = map[string]string{}
	for _, j := range journals {
		s.journalByType[j.Type] = j.ID
	}

	for _, ov := range overheadVendors {
		var v db.Vendor
		city, plz := s.rng.City()
		if err := s.c.Post("/api/vendors", db.CreateVendorRequest{
			OrganizationID: s.orgID, Name: strPtr(ov.name), Vatin: strPtr(s.rng.VATIN()), Phone: strPtr(s.rng.Phone()),
			City: strPtr(city), PostalCode: strPtr(plz), CountryCode: nonEmptyStrPtr(s.orgProfile.countryCode),
		}, &v); err != nil {
			return fmt.Errorf("vendor %s: %w", ov.name, err)
		}
		s.stats.Vendors++
		tax := s.standardTax
		if ov.reducedTax {
			tax = s.reducedTax
		}
		var p db.Product
		if err := s.c.Post("/api/products", db.CreateProductRequest{
			OrganizationID: s.orgID, Name: ov.product, Type: "service", Price: 0,
			SKU:       strPtr(ov.sku),
			TaxRateID: strPtr(tax.id), ExpenseAccountID: strPtr(byCode[ov.account]),
		}, &p); err != nil {
			return fmt.Errorf("expense product %s: %w", ov.product, err)
		}
		s.overheads = append(s.overheads, overheadRef{
			overheadVendor: ov, vendorID: v.ID, productID: p.ID, taxRateID: tax.id, taxPercent: tax.percent,
		})
	}
	return nil
}

// fiscalStampCents is the timbre fiscal on a Tunisian invoice: 1 dinar.
const fiscalStampCents = 100

// postJournalEntry creates a journal entry in the journal of journalType and,
// unless draft, posts it.
func (s *Seeder) postJournalEntry(day time.Time, journalType, description, reference string, lines []db.CreateJournalLineRequest, draft bool) (db.JournalEntry, error) {
	req := db.CreateJournalEntryRequest{
		OrganizationID: s.orgID,
		JournalID:      s.journalByType[journalType],
		Date:           midnightUTC(day),
		Description:    description,
		Reference:      nonEmptyStrPtr(reference),
		Lines:          lines,
	}
	var entry db.JournalEntry
	if err := s.c.Post("/api/journal-entries", req, &entry); err != nil {
		return entry, fmt.Errorf("journal entry %q: %w", description, err)
	}
	s.stats.JournalEntries++
	if draft {
		return entry, nil
	}
	if err := s.c.Patch("/api/journal-entries/"+entry.ID+"/post", nil, nil); err != nil {
		return entry, fmt.Errorf("post journal entry %q: %w", description, err)
	}
	return entry, nil
}

// recordCapitalContribution books the owner's opening capital into the bank.
func (s *Seeder) recordCapitalContribution(day time.Time, amount int64) error {
	_, err := s.postJournalEntry(day, "bank", "Apport en capital du gérant", "CAP-OUV", []db.CreateJournalLineRequest{
		{AccountID: s.cashAccountID, Debit: amount},
		{AccountID: s.accountByCode["3200"], Credit: amount},
	}, false)
	return err
}

// maybeBookOverheads runs once a day: the overhead bills due on this day of
// the month, and on the last day of the month the payroll and bank charges.
func (s *Seeder) maybeBookOverheads(day time.Time) error {
	for _, ov := range s.overheads {
		if day.Day() != ov.dayOfMonth || monthsSince(s.start, day)%ov.everyMonths != 0 {
			continue
		}
		if err := s.createOverheadBill(day, ov); err != nil {
			return err
		}
	}
	if day.Day() == 28 {
		if err := s.fileVATReturn(day); err != nil {
			return err
		}
	}
	if day.AddDate(0, 0, 1).Day() == 1 {
		return s.bookMonthEnd(day)
	}
	return nil
}

// fileVATReturn settles, on the 28th (the monthly declaration's deadline),
// the previous month's VAT and fiscal stamps: the output tax collected less
// the input tax paid goes to the tax office from the bank, with the stamps.
// A month where more VAT was paid than collected carries the credit forward
// instead (only the collected VAT is offset). Balances are read from the
// balance sheet as of the previous month's last day.
func (s *Seeder) fileVATReturn(day time.Time) error {
	monthEnd := time.Date(day.Year(), day.Month(), 1, 0, 0, 0, 0, time.UTC).Add(-time.Millisecond)
	if monthEnd.Before(s.start) {
		return nil
	}
	var bs db.BalanceSheet
	if err := s.c.Get(fmt.Sprintf("/api/organizations/%s/reports/balance-sheet?asOfDate=%d", s.orgID, monthEnd.UnixMilli()), &bs); err != nil {
		return fmt.Errorf("balance sheet for the VAT return: %w", err)
	}
	balance := func(lines []db.BalanceSheetLine, code string) (string, int64) {
		for _, l := range lines {
			if l.Code == code {
				return l.AccountID, l.Amount
			}
		}
		return "", 0
	}
	outputID, output := balance(bs.Liabilities, s.orgProfile.outputTaxCode)
	inputID, input := balance(bs.Assets, s.orgProfile.inputTaxCode)
	stampID, stamps := balance(bs.Liabilities, "2300")
	if output <= 0 {
		return nil
	}
	offset := min(input, output)
	pay := output - offset + max(stamps, 0)
	lines := []db.CreateJournalLineRequest{{AccountID: outputID, Debit: output}}
	if offset > 0 {
		lines = append(lines, db.CreateJournalLineRequest{AccountID: inputID, Credit: offset})
	}
	if stamps > 0 {
		lines = append(lines, db.CreateJournalLineRequest{AccountID: stampID, Debit: stamps})
	}
	if pay > 0 {
		lines = append(lines, db.CreateJournalLineRequest{AccountID: s.cashAccountID, Credit: pay})
	}
	prev := monthEnd
	_, err := s.postJournalEntry(day, "bank", fmt.Sprintf("Déclaration mensuelle TVA et timbres — %s %d", frenchMonths[prev.Month()], prev.Year()),
		"TVA-"+prev.Format("200601"), lines, false)
	return err
}

func monthsSince(from, to time.Time) int {
	return (to.Year()-from.Year())*12 + int(to.Month()) - int(from.Month())
}

func (s *Seeder) createOverheadBill(day time.Time, ov overheadRef) error {
	amount := s.rng.Int64Range(ov.amountLo, ov.amountHi)
	if m := day.Month(); m >= time.June && m <= time.September {
		amount = int64(float64(amount) * ov.summerFactor)
	}
	items := []lineItem{{quantity: 1, unitPriceCents: amount, taxPercent: ov.taxPercent}}
	subTotal, taxTotal, total := computeTotals(items)
	dueDate := midnightUTC(day.AddDate(0, 0, ov.dueDays))
	description := fmt.Sprintf("%s — %s %d", ov.product, frenchMonths[day.Month()], day.Year())
	req := db.CreateIncomingInvoiceRequest{
		OrganizationID:      s.orgID,
		VendorID:            ov.vendorID,
		VendorInvoiceNumber: fmt.Sprintf("%s-%s-%d", vendorInitials(ov.name), day.Format("200601"), s.rng.IntRange(1000, 9999)),
		State:               "draft",
		Date:                midnightUTC(day),
		DueDate:             &dueDate,
		Currency:            s.cfg.Currency,
		Total:               total,
		TaxTotal:            taxTotal,
		SubTotal:            subTotal,
		LineItems: []db.CreateInvoiceLineItemRequest{{
			Description: strPtr(description), Quantity: 1, UnitPrice: float64(amount),
			TaxRate: strPtr(ov.taxRateID), ProductID: strPtr(ov.productID),
		}},
	}
	var bill db.IncomingInvoice
	if err := s.c.Post("/api/incoming-invoices", req, &bill); err != nil {
		return fmt.Errorf("bill from %s: %w", ov.name, err)
	}
	s.stats.IncomingInvoices++
	if err := s.c.Patch("/api/incoming-invoices/"+bill.ID+"/state", map[string]string{"state": "approved"}, nil); err != nil {
		return fmt.Errorf("approve bill from %s: %w", ov.name, err)
	}
	// Running costs are paid on time, from the bank, a few days before they
	// fall due; one still to pay when the run ends is simply not due yet.
	payDay := businessDaysLater(day, s.rng.IntRange(2, max(3, ov.dueDays-2)))
	s.schedulePayment(payDay, bill.ID, total, ov.vendorID, "incoming_invoice", s.cfg.Currency, nil, s.cashAccountID)
	return nil
}

var frenchMonths = [13]string{"", "janvier", "février", "mars", "avril", "mai", "juin", "juillet", "août", "septembre", "octobre", "novembre", "décembre"}

func vendorInitials(name string) string {
	out := ""
	for _, w := range splitWords(name) {
		out += string([]rune(w)[0])
	}
	if len(out) > 4 {
		out = out[:4]
	}
	return out
}

func splitWords(s string) []string {
	var words []string
	cur := ""
	for _, r := range s {
		if r == ' ' {
			if cur != "" {
				words = append(words, cur)
			}
			cur = ""
			continue
		}
		cur += string(r)
	}
	if cur != "" {
		words = append(words, cur)
	}
	return words
}

// bookMonthEnd posts the month's payroll and bank charges. The payroll grows
// with the business (a sixth employee is hired a year in). In the run's
// eighth month the payroll is booked twice by mistake and the duplicate is
// reversed two days later; in the last month, the leave provision is left in
// draft for the accountant to finish.
func (s *Seeder) bookMonthEnd(day time.Time) error {
	staff := 5
	if day.Sub(s.start) > 365*24*time.Hour {
		staff = 6
	}
	payroll := int64(staff) * s.rng.Int64Range(150000, 175000)
	label := fmt.Sprintf("Salaires %s %d", frenchMonths[day.Month()], day.Year())
	lines := []db.CreateJournalLineRequest{
		{AccountID: s.accountByCode["5400"], Debit: payroll},
		{AccountID: s.cashAccountID, Credit: payroll},
	}
	if _, err := s.postJournalEntry(day, "bank", label, "PAIE-"+day.Format("200601"), lines, false); err != nil {
		return err
	}
	if monthsSince(s.start, day) == 7 {
		duplicate, err := s.postJournalEntry(day, "bank", label, "PAIE-"+day.Format("200601"), lines, false)
		if err != nil {
			return err
		}
		reverseDay := businessDaysLater(day, 2)
		if !reverseDay.After(s.cfg.EndDate) {
			s.sched.Schedule(reverseDay, func() error {
				body := map[string]any{"reason": "Écriture de paie saisie en double", "date": midnightUTC(reverseDay)}
				if err := s.c.Post("/api/journal-entries/"+duplicate.ID+"/reverse", body, nil); err != nil {
					return fmt.Errorf("reverse duplicate payroll: %w", err)
				}
				s.stats.JournalEntries++
				return nil
			})
		}
	}

	fees := s.rng.Int64Range(2500, 6000)
	if _, err := s.postJournalEntry(day, "bank", "Frais de tenue de compte et commissions", "", []db.CreateJournalLineRequest{
		{AccountID: s.accountByCode["5500"], Debit: fees},
		{AccountID: s.cashAccountID, Credit: fees},
	}, false); err != nil {
		return err
	}
	return nil
}

// leaveDraftEntry leaves one entry in draft on the run's last day: the
// accountant's month-end provision, not yet checked.
func (s *Seeder) leaveDraftEntry(day time.Time) error {
	amount := s.rng.Int64Range(250000, 400000)
	_, err := s.postJournalEntry(day, "miscellaneous", "Provision pour congés payés (à valider)", "", []db.CreateJournalLineRequest{
		{AccountID: s.accountByCode["5400"], Debit: amount},
		{AccountID: s.accountByCode["2400"], Credit: amount},
	}, true)
	return err
}
