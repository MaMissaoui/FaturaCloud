package db

import (
	"strconv"
	"strings"
	"testing"
)

func fecFixtureFiscalYearID(t *testing.T, d *Database, orgID string) string {
	t.Helper()
	years, err := d.GetFiscalYears(orgID)
	if err != nil {
		t.Fatalf("GetFiscalYears: %v", err)
	}
	if len(years) != 1 {
		t.Fatalf("expected exactly one fiscal year, got %d", len(years))
	}
	return years[0].ID
}

func TestGenerateFECRejectsMissingSIREN(t *testing.T) {
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-fec-no-siren")
	fyID := fecFixtureFiscalYearID(t, d, fx.orgID)

	if _, _, err := d.GenerateFEC(fx.orgID, fyID); err == nil {
		t.Fatal("expected an error when the organization has no registration number")
	}
}

func TestGenerateFECRejectsInvalidSIREN(t *testing.T) {
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-fec-bad-siren")
	fyID := fecFixtureFiscalYearID(t, d, fx.orgID)

	if _, err := d.UpdateOrganization(fx.orgID, UpdateOrganizationRequest{RegistrationNumber: ptr("12345")}); err != nil {
		t.Fatalf("UpdateOrganization: %v", err)
	}
	if _, _, err := d.GenerateFEC(fx.orgID, fyID); err == nil {
		t.Fatal("expected an error for a non-9-digit registration number")
	}
}

func TestGenerateFECRejectsFiscalYearFromOtherOrg(t *testing.T) {
	d := newTestDB(t)
	fx1 := newGLPostingTestFixture(t, d, "org-fec-a")
	fx2 := newGLPostingTestFixture(t, d, "org-fec-b")
	fy2ID := fecFixtureFiscalYearID(t, d, fx2.orgID)

	if _, err := d.UpdateOrganization(fx1.orgID, UpdateOrganizationRequest{RegistrationNumber: ptr("552100554")}); err != nil {
		t.Fatalf("UpdateOrganization: %v", err)
	}
	if _, _, err := d.GenerateFEC(fx1.orgID, fy2ID); err == nil {
		t.Fatal("expected an error when the fiscal year belongs to a different organization")
	}
}

func TestGenerateFECProducesExpectedRows(t *testing.T) {
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-fec-happy")
	fyID := fecFixtureFiscalYearID(t, d, fx.orgID)

	if _, err := d.UpdateOrganization(fx.orgID, UpdateOrganizationRequest{RegistrationNumber: ptr("552100554")}); err != nil {
		t.Fatalf("UpdateOrganization: %v", err)
	}

	inv := fx.createInvoice(t, d, "fec-inv-1", 2, 1000) // 2000 revenue, 400 tax, total 2400
	if _, err := d.UpdateInvoiceState(inv.ID, "sent"); err != nil {
		t.Fatalf("UpdateInvoiceState(sent): %v", err)
	}
	bill := fx.createIncomingInvoice(t, d, "fec-bill-1", 1, 1200) // 1200 expense, 240 tax, total 1440
	if _, err := d.UpdateIncomingInvoiceState(bill.ID, "approved"); err != nil {
		t.Fatalf("UpdateIncomingInvoiceState(approved): %v", err)
	}

	// A draft manual entry must never appear in the export.
	if _, err := d.CreateJournalEntry(CreateJournalEntryRequest{
		OrganizationID: fx.orgID, JournalID: mustJournalID(t, d, fx.orgID, "OD"),
		Date: fx.date, Description: "draft entry, should be excluded",
		Lines: []CreateJournalLineRequest{
			{AccountID: fx.arAccountID, Debit: 100},
			{AccountID: fx.revenueAccountID, Credit: 100},
		},
	}); err != nil {
		t.Fatalf("CreateJournalEntry (draft): %v", err)
	}

	content, filename, err := d.GenerateFEC(fx.orgID, fyID)
	if err != nil {
		t.Fatalf("GenerateFEC: %v", err)
	}
	if filename != "552100554FEC20251231.txt" {
		t.Fatalf("filename = %q, want 552100554FEC20251231.txt", filename)
	}

	lines := strings.Split(strings.TrimRight(string(content), "\n"), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected a header row plus data rows, got %d lines", len(lines))
	}
	header := strings.Split(lines[0], "\t")
	if len(header) != 18 || header[0] != "JournalCode" || header[len(header)-1] != "Idevise" {
		t.Fatalf("unexpected header: %v", header)
	}

	var totalDebit, totalCredit float64
	dataRows := lines[1:]
	for _, line := range dataRows {
		fields := strings.Split(line, "\t")
		if len(fields) != 18 {
			t.Fatalf("row has %d fields, want 18: %v", len(fields), fields)
		}
		if fields[3] != "20250201" { // EcritureDate
			t.Fatalf("EcritureDate = %q, want 20250201", fields[3])
		}
		if fields[9] != "20250201" { // PieceDate mirrors the entry date
			t.Fatalf("PieceDate = %q, want 20250201", fields[9])
		}
		if fields[15] == "" { // ValidDate — each posted entry's own postedAt
			t.Fatalf("ValidDate is blank for a posted entry: %v", fields)
		}
		if _, err := strconv.ParseInt(fields[2], 10, 64); err != nil { // EcritureNum
			t.Fatalf("EcritureNum %q is not an integer: %v", fields[2], err)
		}
		debit, err := strconv.ParseFloat(fields[11], 64)
		if err != nil {
			t.Fatalf("Debit %q not parseable: %v", fields[11], err)
		}
		credit, err := strconv.ParseFloat(fields[12], 64)
		if err != nil {
			t.Fatalf("Credit %q not parseable: %v", fields[12], err)
		}
		if debit != 0 && credit != 0 {
			t.Fatalf("row has both debit and credit set: %v", fields)
		}
		totalDebit += debit
		totalCredit += credit
	}
	if totalDebit != totalCredit {
		t.Fatalf("FEC file does not balance: debit %v, credit %v", totalDebit, totalCredit)
	}
	// The draft entry contributed nothing — only the two posted entries'
	// lines (invoice: AR/revenue/tax = 3, bill: AP/expense/tax = 3) appear.
	if len(dataRows) != 6 {
		t.Fatalf("expected 6 data rows (draft entry excluded), got %d: %v", len(dataRows), dataRows)
	}
}

// TestGenerateFECSanitizesTabsAndNewlines is the regression test for a
// tab or newline in a free-text source field (here, a journal entry's own
// description) corrupting the tab-separated file — it would still split
// into 18 fields, just with the wrong values shifted into them, so a naive
// field-count assertion alone can't catch this.
func TestGenerateFECSanitizesTabsAndNewlines(t *testing.T) {
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-fec-sanitize")
	fyID := fecFixtureFiscalYearID(t, d, fx.orgID)
	if _, err := d.UpdateOrganization(fx.orgID, UpdateOrganizationRequest{RegistrationNumber: ptr("552100554")}); err != nil {
		t.Fatalf("UpdateOrganization: %v", err)
	}

	entry, err := d.CreateJournalEntry(CreateJournalEntryRequest{
		OrganizationID: fx.orgID, JournalID: mustJournalID(t, d, fx.orgID, "OD"),
		Date: fx.date, Description: "line with a\ttab and a\nnewline",
		Lines: []CreateJournalLineRequest{
			{AccountID: fx.arAccountID, Debit: 100},
			{AccountID: fx.revenueAccountID, Credit: 100},
		},
	})
	if err != nil {
		t.Fatalf("CreateJournalEntry: %v", err)
	}
	if _, err := d.PostJournalEntry(entry.ID); err != nil {
		t.Fatalf("PostJournalEntry: %v", err)
	}

	content, _, err := d.GenerateFEC(fx.orgID, fyID)
	if err != nil {
		t.Fatalf("GenerateFEC: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(content), "\n"), "\n")
	if len(lines) != 3 { // header + 2 lines of the one posted entry
		t.Fatalf("expected 3 lines, got %d: %v", len(lines), lines)
	}
	for _, line := range lines[1:] {
		fields := strings.Split(line, "\t")
		if len(fields) != 18 {
			t.Fatalf("row has %d fields, want 18: %v", len(fields), fields)
		}
		ecritureLib := fields[10]
		if strings.ContainsAny(ecritureLib, "\t\n\r") {
			t.Fatalf("EcritureLib still contains a raw tab/newline: %q", ecritureLib)
		}
		if ecritureLib != "line with a tab and a newline" {
			t.Fatalf("EcritureLib = %q, want tabs/newlines replaced with spaces", ecritureLib)
		}
	}
}

// TestGenerateFECIncludesForeignCurrencyFields exercises the
// Montantdevise/Idevise branch, otherwise unreached by every other test
// here: a foreign-currency payment (see TestCreatePaymentForeignCurrencyRealizesGain
// in payment_test.go) posts a bank line whose currency differs from the
// org's functional currency (EUR), which is exactly what
// glLine populates the foreign-currency shadow columns for.
func TestGenerateFECIncludesForeignCurrencyFields(t *testing.T) {
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-fec-fx")
	fyID := fecFixtureFiscalYearID(t, d, fx.orgID)
	if _, err := d.UpdateOrganization(fx.orgID, UpdateOrganizationRequest{RegistrationNumber: ptr("552100554")}); err != nil {
		t.Fatalf("UpdateOrganization: %v", err)
	}

	inv, err := d.CreateInvoice(CreateInvoiceRequest{
		OrganizationID: fx.orgID, Number: "fec-inv-usd-1", ClientID: fx.clientID,
		Date: fx.date, Currency: "USD", ExchangeRate: ptr(0.90),
		SubTotal: 10000, TaxTotal: 0, Total: 10000,
		LineItems: []CreateInvoiceLineItemRequest{{Quantity: 1, UnitPrice: 10000, ProductID: &fx.productID}},
	})
	if err != nil {
		t.Fatalf("CreateInvoice: %v", err)
	}
	if _, err := d.UpdateInvoiceState(inv.ID, "sent"); err != nil {
		t.Fatalf("UpdateInvoiceState(sent): %v", err)
	}
	bank := accountByCode(t, d, fx.orgID, "1020")
	if _, err := d.CreatePayment(CreatePaymentRequest{
		OrganizationID: fx.orgID, Direction: "inbound", ClientID: &fx.clientID,
		BankAccountID: bank.ID, Amount: 10000, Currency: "USD", ExchangeRate: ptr(1.00),
		Date: fx.date, Method: "bank_transfer",
		Applications: []CreatePaymentApplicationRequest{{DocumentType: "invoice", DocumentID: inv.ID, Amount: 10000}},
	}); err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}

	content, _, err := d.GenerateFEC(fx.orgID, fyID)
	if err != nil {
		t.Fatalf("GenerateFEC: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(content), "\n"), "\n")

	var sawForeignBankLine bool
	for _, line := range lines[1:] {
		fields := strings.Split(line, "\t")
		if len(fields) != 18 {
			t.Fatalf("row has %d fields, want 18: %v", len(fields), fields)
		}
		compteNum, montantDevise, iDevise := fields[4], fields[16], fields[17]
		if compteNum != "1020" {
			continue
		}
		sawForeignBankLine = true
		if iDevise != "USD" {
			t.Fatalf("Idevise = %q, want USD", iDevise)
		}
		if montantDevise != "100.00" {
			t.Fatalf("Montantdevise = %q, want 100.00", montantDevise)
		}
	}
	if !sawForeignBankLine {
		t.Fatal("expected a bank account (1020) line in the export")
	}
}

func mustJournalID(t *testing.T, d *Database, orgID, code string) string {
	t.Helper()
	journals, err := d.GetJournals(orgID)
	if err != nil {
		t.Fatalf("GetJournals: %v", err)
	}
	for _, j := range journals {
		if j.Code == code {
			return j.ID
		}
	}
	t.Fatalf("no journal with code %q", code)
	return ""
}
