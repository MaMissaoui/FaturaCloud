package db

import (
	"strconv"
	"strings"
	"testing"

	"golang.org/x/text/encoding/charmap"
)

// decodeDATEV converts the exported Windows-1252 bytes back to UTF-8 for
// test assertions.
func decodeDATEV(t *testing.T, content []byte) string {
	t.Helper()
	s, err := charmap.Windows1252.NewDecoder().String(string(content))
	if err != nil {
		t.Fatalf("decode windows-1252: %v", err)
	}
	return s
}

func setDatevAccountNumber(t *testing.T, d *Database, accountID, number string) {
	t.Helper()
	account, err := d.GetAccount(accountID)
	if err != nil {
		t.Fatalf("GetAccount: %v", err)
	}
	_, err = d.UpdateAccount(accountID, UpdateAccountRequest{
		ParentID: account.ParentID, Code: account.Code, Name: account.Name, Type: account.Type,
		IsGroup: account.IsGroup, IsActive: account.IsActive, Description: account.Description,
		DATEVAccountNumber: &number,
	})
	if err != nil {
		t.Fatalf("UpdateAccount: %v", err)
	}
}

func setDatevOrgFields(t *testing.T, d *Database, orgID string) {
	t.Helper()
	if _, err := d.UpdateOrganization(orgID, UpdateOrganizationRequest{
		DatevConsultantNumber: ptr("1001"), DatevClientNumber: ptr("456"),
	}); err != nil {
		t.Fatalf("UpdateOrganization: %v", err)
	}
}

func TestGenerateDATEVRejectsMissingConsultantAndClientNumber(t *testing.T) {
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-datev-no-numbers")
	fyID := fecFixtureFiscalYearID(t, d, fx.orgID)

	_, _, err := d.GenerateDATEV(fx.orgID, fyID)
	if err == nil {
		t.Fatal("expected an error when consultant/client numbers are unset")
	}
	if !strings.Contains(err.Error(), "consultant number") || !strings.Contains(err.Error(), "client number") {
		t.Fatalf("error = %q, want it to mention both missing numbers", err.Error())
	}
}

func TestGenerateDATEVRejectsInvalidConsultantNumber(t *testing.T) {
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-datev-bad-consultant")
	fyID := fecFixtureFiscalYearID(t, d, fx.orgID)

	if _, err := d.UpdateOrganization(fx.orgID, UpdateOrganizationRequest{
		DatevConsultantNumber: ptr("42"), DatevClientNumber: ptr("456"), // below the 1001 minimum
	}); err != nil {
		t.Fatalf("UpdateOrganization: %v", err)
	}

	_, _, err := d.GenerateDATEV(fx.orgID, fyID)
	if err == nil {
		t.Fatal("expected an error for a consultant number below 1001")
	}
}

func TestGenerateDATEVRejectsFiscalYearFromOtherOrg(t *testing.T) {
	d := newTestDB(t)
	fx1 := newGLPostingTestFixture(t, d, "org-datev-a")
	fx2 := newGLPostingTestFixture(t, d, "org-datev-b")
	setDatevOrgFields(t, d, fx1.orgID)
	fy2ID := fecFixtureFiscalYearID(t, d, fx2.orgID)

	if _, _, err := d.GenerateDATEV(fx1.orgID, fy2ID); err == nil {
		t.Fatal("expected an error when the fiscal year belongs to a different organization")
	}
}

func TestGenerateDATEVRejectsMissingAccountNumbers(t *testing.T) {
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-datev-no-acct-numbers")
	fyID := fecFixtureFiscalYearID(t, d, fx.orgID)
	setDatevOrgFields(t, d, fx.orgID)

	inv := fx.createInvoice(t, d, "datev-inv-noacct", 1, 1000)
	if _, err := d.UpdateInvoiceState(inv.ID, "sent"); err != nil {
		t.Fatalf("UpdateInvoiceState(sent): %v", err)
	}

	_, _, err := d.GenerateDATEV(fx.orgID, fyID)
	if err == nil {
		t.Fatal("expected an error when referenced accounts have no DATEV account number")
	}
	if !strings.Contains(err.Error(), "DATEV account number") {
		t.Fatalf("error = %q, want it to mention missing DATEV account numbers", err.Error())
	}
}

func TestGenerateDATEVRejectsMixedAccountNumberLengths(t *testing.T) {
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-datev-mixed-lengths")
	fyID := fecFixtureFiscalYearID(t, d, fx.orgID)
	setDatevOrgFields(t, d, fx.orgID)

	setDatevAccountNumber(t, d, fx.arAccountID, "1100")
	setDatevAccountNumber(t, d, fx.revenueAccountID, "48000") // 5 digits, inconsistent with 1100's 4
	setDatevAccountNumber(t, d, fx.outputTaxAccountID, "2200")

	// Use a 0% tax rate to keep the entry to exactly AR + revenue (2 lines,
	// a natural anchor) so this test isolates the length-mismatch problem.
	inv, err := d.CreateInvoice(CreateInvoiceRequest{
		OrganizationID: fx.orgID, Number: "datev-inv-mixed", ClientID: fx.clientID,
		Date: fx.date, Currency: "EUR", SubTotal: 1000, TaxTotal: 0, Total: 1000,
		LineItems: []CreateInvoiceLineItemRequest{{Quantity: 1, UnitPrice: 1000, ProductID: &fx.productID}},
	})
	if err != nil {
		t.Fatalf("CreateInvoice: %v", err)
	}
	if _, err := d.UpdateInvoiceState(inv.ID, "sent"); err != nil {
		t.Fatalf("UpdateInvoiceState(sent): %v", err)
	}

	_, _, err = d.GenerateDATEV(fx.orgID, fyID)
	if err == nil {
		t.Fatal("expected an error when referenced DATEV account numbers have inconsistent lengths")
	}
	if !strings.Contains(err.Error(), "same length") {
		t.Fatalf("error = %q, want it to mention the length mismatch", err.Error())
	}
}

func TestGenerateDATEVProducesExpectedRows(t *testing.T) {
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-datev-happy")
	fyID := fecFixtureFiscalYearID(t, d, fx.orgID)
	setDatevOrgFields(t, d, fx.orgID)

	setDatevAccountNumber(t, d, fx.arAccountID, "1100")
	setDatevAccountNumber(t, d, fx.apAccountID, "2100")
	setDatevAccountNumber(t, d, fx.revenueAccountID, "4100")
	setDatevAccountNumber(t, d, fx.expenseAccountID, "5100")
	setDatevAccountNumber(t, d, fx.outputTaxAccountID, "2200")
	setDatevAccountNumber(t, d, fx.inputTaxAccountID, "1200")
	bank := accountByCode(t, d, fx.orgID, "1020")
	setDatevAccountNumber(t, d, bank.ID, "1020")

	inv := fx.createInvoice(t, d, "datev-inv-1", 2, 1000) // 2000 revenue, 400 tax, total 2400
	if _, err := d.UpdateInvoiceState(inv.ID, "sent"); err != nil {
		t.Fatalf("UpdateInvoiceState(sent): %v", err)
	}
	bill := fx.createIncomingInvoice(t, d, "datev-bill-1", 1, 1200) // 1200 expense, 240 tax, total 1440
	if _, err := d.UpdateIncomingInvoiceState(bill.ID, "approved"); err != nil {
		t.Fatalf("UpdateIncomingInvoiceState(approved): %v", err)
	}
	if _, err := d.CreatePayment(CreatePaymentRequest{
		OrganizationID: fx.orgID, Direction: "inbound", ClientID: &fx.clientID,
		BankAccountID: bank.ID, Amount: 2400, Currency: "EUR", Date: fx.date, Method: "bank_transfer",
		Applications: []CreatePaymentApplicationRequest{{DocumentType: "invoice", DocumentID: inv.ID, Amount: 2400}},
	}); err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}

	content, filename, err := d.GenerateDATEV(fx.orgID, fyID)
	if err != nil {
		t.Fatalf("GenerateDATEV: %v", err)
	}
	if filename != "EXTF_Buchungsstapel_2025.csv" {
		t.Fatalf("filename = %q, want EXTF_Buchungsstapel_2025.csv", filename)
	}

	text := decodeDATEV(t, content)
	lines := strings.Split(strings.TrimRight(text, "\r\n"), "\r\n")
	if len(lines) != 7 { // header1 + header2 + 2 invoice rows + 2 bill rows + 1 payment row
		t.Fatalf("expected 7 lines, got %d: %v", len(lines), lines)
	}

	header1 := strings.Split(lines[0], ";")
	if len(header1) != 31 {
		t.Fatalf("header1 has %d fields, want 31", len(header1))
	}
	if header1[0] != `"EXTF"` || header1[1] != "700" || header1[2] != "21" || header1[4] != "13" {
		t.Fatalf("unexpected header1 identity fields: %v", header1[:5])
	}
	if header1[10] != "1001" || header1[11] != "456" {
		t.Fatalf("header1 consultant/client numbers = %q/%q, want 1001/456", header1[10], header1[11])
	}
	if header1[13] != "4" {
		t.Fatalf("Sachkontenlänge = %q, want 4", header1[13])
	}

	header2 := strings.Split(lines[1], ";")
	if len(header2) != 125 {
		t.Fatalf("header2 has %d fields, want 125", len(header2))
	}
	if header2[0] != "Umsatz (ohne Soll/Haben-Kz)" || header2[124] != "Abw. Skontokonto" {
		t.Fatalf("unexpected header2 boundary columns: first=%q last=%q", header2[0], header2[124])
	}

	// DATEV's Konto/Gegenkonto format doesn't preserve "total debit fields
	// == total credit fields" across the file the way FEC's per-line format
	// does — each row is already a self-balanced elementary posting (Konto
	// gets the stated side, Gegenkonto implicitly gets the opposite side).
	// The real invariant to check is that expanding every row into its two
	// implicit postings reconstructs each account's actual net balance —
	// reconcile against GetTrialBalance instead of a naive column sum.
	datevNumberToAccountID := map[string]string{
		"1100": fx.arAccountID, "2100": fx.apAccountID,
		"4100": fx.revenueAccountID, "5100": fx.expenseAccountID,
		"2200": fx.outputTaxAccountID, "1200": fx.inputTaxAccountID,
		"1020": bank.ID,
	}
	reconstructed := map[string]int64{} // accountID -> debit-credit
	for _, line := range lines[2:] {
		fields := strings.Split(line, ";")
		if len(fields) != 125 {
			t.Fatalf("data row has %d fields, want 125: %v", len(fields), fields)
		}
		amountStr := strings.Replace(fields[0], ",", "", 1) // "20,00" -> "2000" cents
		amount, err := strconv.ParseInt(amountStr, 10, 64)
		if err != nil {
			t.Fatalf("Umsatz %q not parseable as cents: %v", fields[0], err)
		}
		if strings.HasPrefix(fields[0], "-") {
			t.Fatalf("Umsatz must never be negative, got %q", fields[0])
		}
		if strings.Contains(fields[6], `"`) || strings.Contains(fields[7], `"`) {
			t.Fatalf("Konto/Gegenkonto must be bare (unquoted) numbers: %v / %v", fields[6], fields[7])
		}
		// Belegdatum (column 10) is DDMM only, not YYYYMMDD — a distinct
		// format from every other date field in this exporter. fx.date is
		// 2025-02-01, so day=01, month=02.
		if fields[9] != "0102" {
			t.Fatalf("Belegdatum = %q, want \"0102\" (DDMM, not YYYYMMDD)", fields[9])
		}
		konto, gegenkonto := datevNumberToAccountID[fields[6]], datevNumberToAccountID[fields[7]]
		switch fields[1] {
		case `"S"`:
			reconstructed[konto] += amount
			reconstructed[gegenkonto] -= amount
		case `"H"`:
			reconstructed[konto] -= amount
			reconstructed[gegenkonto] += amount
		default:
			t.Fatalf("Soll/Haben-Kennzeichen = %q, want \"S\" or \"H\"", fields[1])
		}
	}

	tb, err := d.GetTrialBalance(fx.orgID, "")
	if err != nil {
		t.Fatalf("GetTrialBalance: %v", err)
	}
	for _, row := range tb {
		want := row.Debit - row.Credit
		got, ok := reconstructed[row.AccountID]
		if !ok && want != 0 {
			t.Fatalf("account %s (%s) has trial balance %d but no DATEV rows reference it", row.AccountCode, row.AccountName, want)
		}
		if ok && got != want {
			t.Fatalf("account %s (%s): DATEV rows reconstruct to %d, trial balance says %d", row.AccountCode, row.AccountName, got, want)
		}
	}
}

func TestGenerateDATEVSplitsMultiLineBothSidesAgainstClearingAccount(t *testing.T) {
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-datev-clearing")
	fyID := fecFixtureFiscalYearID(t, d, fx.orgID)
	setDatevOrgFields(t, d, fx.orgID)

	setDatevAccountNumber(t, d, fx.revenueAccountID, "4100")
	setDatevAccountNumber(t, d, fx.expenseAccountID, "5100")
	setDatevAccountNumber(t, d, fx.arAccountID, "1100")
	setDatevAccountNumber(t, d, fx.apAccountID, "2100")

	entry, err := d.CreateJournalEntry(CreateJournalEntryRequest{
		OrganizationID: fx.orgID, JournalID: mustJournalID(t, d, fx.orgID, "OD"),
		Date: fx.date, Description: "multi-line both sides",
		Lines: []CreateJournalLineRequest{
			{AccountID: fx.revenueAccountID, Debit: 100},
			{AccountID: fx.expenseAccountID, Debit: 200},
			{AccountID: fx.arAccountID, Credit: 150},
			{AccountID: fx.apAccountID, Credit: 150},
		},
	})
	if err != nil {
		t.Fatalf("CreateJournalEntry: %v", err)
	}
	if _, err := d.PostJournalEntry(entry.ID); err != nil {
		t.Fatalf("PostJournalEntry: %v", err)
	}

	// Without a clearing account configured, this entry blocks the export.
	if _, _, err := d.GenerateDATEV(fx.orgID, fyID); err == nil {
		t.Fatal("expected an error for a multi-line-both-sides entry with no clearing account")
	}

	clearing, err := d.CreateAccount(CreateAccountRequest{
		OrganizationID: fx.orgID, Code: "9000", Name: "DATEV Clearing", Type: "asset",
	})
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	setDatevAccountNumber(t, d, clearing.ID, "9000")
	if _, err := d.UpdateOrganization(fx.orgID, UpdateOrganizationRequest{DatevClearingAccountID: &clearing.ID}); err != nil {
		t.Fatalf("UpdateOrganization: %v", err)
	}

	content, _, err := d.GenerateDATEV(fx.orgID, fyID)
	if err != nil {
		t.Fatalf("GenerateDATEV: %v", err)
	}
	text := decodeDATEV(t, content)
	lines := strings.Split(strings.TrimRight(text, "\r\n"), "\r\n")
	dataRows := lines[2:]
	if len(dataRows) != 4 {
		t.Fatalf("expected 4 rows (one per original line), got %d: %v", len(dataRows), dataRows)
	}
	for _, line := range dataRows {
		fields := strings.Split(line, ";")
		if fields[7] != "9000" {
			t.Fatalf("Gegenkonto = %q, want the clearing account 9000: %v", fields[7], fields)
		}
	}
}

// TestGenerateDATEVReplacesUnsupportedCharactersInsteadOfFailing is the
// regression test for a bug where charmap.Windows1252.NewEncoder().String
// (with no ReplaceUnsupported wrapper) returns an error on any rune outside
// cp1252 — this app is multi-locale, so a free-text description containing
// e.g. a CJK character anywhere in a fiscal year would otherwise turn the
// whole export into a 500 with no actionable message.
func TestGenerateDATEVReplacesUnsupportedCharactersInsteadOfFailing(t *testing.T) {
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-datev-encoding")
	fyID := fecFixtureFiscalYearID(t, d, fx.orgID)
	setDatevOrgFields(t, d, fx.orgID)

	setDatevAccountNumber(t, d, fx.revenueAccountID, "4100")
	setDatevAccountNumber(t, d, fx.arAccountID, "1100")

	entry, err := d.CreateJournalEntry(CreateJournalEntryRequest{
		OrganizationID: fx.orgID, JournalID: mustJournalID(t, d, fx.orgID, "OD"),
		Date: fx.date, Description: "invoice 請求書 for客户", // contains CJK, outside cp1252
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

	content, _, err := d.GenerateDATEV(fx.orgID, fyID)
	if err != nil {
		t.Fatalf("GenerateDATEV must not fail on a non-cp1252 character, got: %v", err)
	}
	// ReplaceUnsupported substitutes each unsupported rune with the ASCII
	// SUB control character (0x1A), not '?' — decodeDATEV would choke on
	// raw 0x1A as invalid UTF-8 if concatenated blindly, so check the raw
	// bytes directly instead of round-tripping through the cp1252 decoder.
	if strings.Contains(string(content), "\xE8") || strings.Contains(string(content), "\xAC") {
		t.Fatalf("expected no raw multi-byte UTF-8 remnants in the cp1252-encoded output")
	}
	if !strings.Contains(string(content), "invoice \x1a\x1a\x1a for\x1a\x1a") {
		t.Fatalf("expected unsupported CJK runes replaced 1:1 with SUB (0x1A) in buchungstext, got: %q", content)
	}
}
