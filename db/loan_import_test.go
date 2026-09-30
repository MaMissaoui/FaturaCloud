package db

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
)

// loanSheet builds a register file: the template's header row, then rows.
func loanSheet(t *testing.T, rows ...[]any) []byte {
	t.Helper()
	f := excelize.NewFile()
	defer f.Close() //nolint:errcheck
	all := append([][]any{{}}, rows...)
	for c, h := range loanImportHeaders {
		all[0] = append(all[0], h)
		_ = c
	}
	for r, row := range all {
		for c, v := range row {
			cell, _ := excelize.CoordinatesToCellName(c+1, r+1)
			if err := f.SetCellValue("Sheet1", cell, v); err != nil {
				t.Fatalf("SetCellValue: %v", err)
			}
		}
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatalf("write sheet: %v", err)
	}
	return buf.Bytes()
}

type loanImportFixture struct {
	glPostingTestFixture
	tunis *time.Location
	// cutoverDay is what the screen sends (calendarDayMs of 1 February
	// 2025); cutover is the resulting local midnight in Tunis.
	cutoverDay int64
	cutover    int64
	// "Existing Customer", phone 98 111 222 — matched by name + phone.
	existingID string
	// "Karim Trabelsi", CIN 07654321 — matched by CIN.
	cinClientID string
}

func newLoanImportFixture(t *testing.T, d *Database, orgID string) loanImportFixture {
	t.Helper()
	fx := newGLPostingTestFixture(t, d, orgID)
	setOrgTimezone(t, d, fx.orgID, "Africa/Tunis")
	tunis, err := time.LoadLocation("Africa/Tunis")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}
	existing, err := d.CreateClient(CreateClientRequest{OrganizationID: fx.orgID, Name: ptr("Existing Customer"), Phone: ptr("98 111 222")})
	if err != nil {
		t.Fatalf("CreateClient: %v", err)
	}
	withCIN, err := d.CreateClient(CreateClientRequest{OrganizationID: fx.orgID, Name: ptr("Karim Trabelsi"), IdentityNumber: ptr("07654321")})
	if err != nil {
		t.Fatalf("CreateClient: %v", err)
	}
	return loanImportFixture{
		glPostingTestFixture: fx,
		tunis:                tunis,
		cutoverDay:           time.Date(2025, 2, 1, 12, 0, 0, 0, time.UTC).UnixMilli(),
		cutover:              time.Date(2025, 2, 1, 0, 0, 0, 0, tunis).UnixMilli(),
		existingID:           existing.ID,
		cinClientID:          withCIN.ID,
	}
}

// Three loans: a two-item open loan for a new customer (one item by SKU,
// one free text, a decimal-comma amount), a settled loan for an existing
// customer matched by name + phone, and an open loan matched by CIN under a
// differently spelled name (a warning).
func (fx loanImportFixture) goodSheet(t *testing.T, d *Database) []byte {
	t.Helper()
	product, err := d.GetProduct(fx.productID)
	if err != nil {
		t.Fatalf("GetProduct: %v", err)
	}
	if _, err := d.UpdateProduct(fx.productID, UpdateProductRequest{Name: product.Name, SKU: ptr("WID-001"), Type: product.Type, Price: product.Price}); err != nil {
		t.Fatalf("UpdateProduct: %v", err)
	}
	return loanSheet(t,
		[]any{"C1-P001-1", "15/06/2024", "Nouveau Client", "01234567", "98 999 000", "", "Sousse", "Garant A", "WID-001", 1, "1 200,500", "450", "10/01/2025", "12 x 100"},
		[]any{"C1-P001-1", "15/06/2024", "Nouveau Client", "", "", "", "", "", "Micro-ondes", 2, 300, "", "", ""},
		[]any{"C1-P002-1", "01/03/2024", "existing customer", "", "98111222", "", "", "", "Frigo", 1, 800, 800, "", ""},
		[]any{"C1-P003-1", "20/12/2024", "K. Trabelsi", "07654321", "", "", "", "", "TV", 1, 900, 0, "", ""},
	)
}

func invoiceCount(t *testing.T, d *Database, orgID string) int {
	t.Helper()
	var n int
	if err := d.DB.Get(&n, `SELECT COUNT(*) FROM invoices WHERE organizationId = ?`, orgID); err != nil {
		t.Fatalf("count invoices: %v", err)
	}
	return n
}

func TestLoanImportDryRunWritesNothingAndReportsTotals(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newLoanImportFixture(t, d, "org-loanimport-dry")

	report, err := d.DryRunLoanImport(fx.orgID, fx.cutoverDay, fx.goodSheet(t, d))
	if err != nil {
		t.Fatalf("DryRunLoanImport: %v", err)
	}
	if report.Errors != 0 {
		t.Fatalf("errors = %+v", report.Problems)
	}
	if report.CutoverDate != fx.cutover {
		t.Fatalf("cutover = %d, want 1 February 2025 local midnight (%d)", report.CutoverDate, fx.cutover)
	}
	// 1200.50 + 300 = 1500.50 (paid 450), 800 (paid 800), 900 (paid 0).
	if report.Loans != 3 || report.OpenLoans != 2 || report.SettledLoans != 1 || report.Lines != 4 ||
		report.Total != 320050 || report.Paid != 125000 || report.Outstanding != 195050 ||
		report.CustomersCreated != 1 || report.CustomersMatched != 2 {
		t.Fatalf("report = %+v", report)
	}
	warnings := 0
	for _, p := range report.Problems {
		if p.Severity == "warning" {
			warnings++
		}
	}
	// Free text: "Micro-ondes", "Frigo", "TV"; CIN under another name.
	if warnings != 4 {
		t.Fatalf("warnings = %+v, want 4", report.Problems)
	}
	if n := invoiceCount(t, d, fx.orgID); n != 0 {
		t.Fatalf("a dry run wrote %d invoices", n)
	}
}

func TestLoanImportWritesOneBatch(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newLoanImportFixture(t, d, "org-loanimport-import")
	sheet := fx.goodSheet(t, d)

	report, err := d.ImportLoanRegister(fx.orgID, "user-1", "registre.xlsx", fx.cutoverDay, sheet)
	if err != nil {
		t.Fatalf("ImportLoanRegister: %v", err)
	}
	if !report.Imported || report.BatchID == "" {
		t.Fatalf("report = %+v, want imported", report)
	}
	invoices, err := d.GetInvoices(fx.orgID)
	if err != nil {
		t.Fatalf("GetInvoices: %v", err)
	}
	byRef := map[string]Invoice{}
	for _, inv := range invoices {
		byRef[inv.Number] = inv
	}
	first := byRef["C1-P001-1"]
	if !isOpeningLoan(&first) || first.ImportBatchID == nil || *first.ImportBatchID != report.BatchID ||
		first.Total != 150050 || first.State != "sent" ||
		first.Date != time.Date(2024, 6, 15, 0, 0, 0, 0, fx.tunis).UnixMilli() {
		t.Fatalf("C1-P001-1 = %+v, want a 1500.50 open loan dated 15/06/2024 local midnight in Tunis", first)
	}
	if byRef["C1-P002-1"].ClientID != fx.existingID || byRef["C1-P002-1"].State != "paid" {
		t.Fatalf("C1-P002-1 = %+v, want the existing customer's settled loan", byRef["C1-P002-1"])
	}
	if byRef["C1-P003-1"].ClientID != fx.cinClientID {
		t.Fatalf("C1-P003-1 went to %s, want the customer with that CIN", byRef["C1-P003-1"].ClientID)
	}
	created, err := d.GetClient(first.ClientID)
	if err != nil || created.ImportBatchID == nil || *created.IdentityNumber != "01234567" ||
		*created.Guarantor != "Garant A" || created.Code == nil || *created.Code != "NO" {
		t.Fatalf("created customer = %+v, %v", created, err)
	}

	// The same file again: everything already imported, nothing written.
	again, err := d.ImportLoanRegister(fx.orgID, "user-1", "registre.xlsx", fx.cutoverDay, sheet)
	if err != nil {
		t.Fatalf("second ImportLoanRegister: %v", err)
	}
	if again.Imported || again.SkippedLoans != 3 {
		t.Fatalf("second import = %+v, want 3 skipped and nothing imported", again)
	}
	if n := invoiceCount(t, d, fx.orgID); n != 3 {
		t.Fatalf("invoices after re-upload = %d, want 3", n)
	}
}

func TestLoanImportRefusesAFileWithErrors(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newLoanImportFixture(t, d, "org-loanimport-errors")
	if _, err := d.CreateClient(CreateClientRequest{OrganizationID: fx.orgID, Name: ptr("Twin Name")}); err != nil {
		t.Fatalf("CreateClient: %v", err)
	}
	if _, err := d.CreateClient(CreateClientRequest{OrganizationID: fx.orgID, Name: ptr("Twin Name")}); err != nil {
		t.Fatalf("CreateClient: %v", err)
	}

	sheet := loanSheet(t,
		[]any{"OK-1", "01/01/2024", "Fine Customer", "", "", "", "", "", "TV", 1, 500, "", "", ""},
		[]any{"BAD-DATE", "31/02/2024", "Someone", "", "", "", "", "", "TV", 1, 500, "", "", ""},
		[]any{"NO-AMOUNT", "01/01/2024", "Someone Else", "", "", "", "", "", "TV", 1, "", "", "", ""},
		[]any{"TWINS", "01/01/2024", "Twin Name", "", "", "", "", "", "TV", 1, 500, "", "", ""},
		[]any{"CIN-A", "01/01/2024", "Person A", "11112222", "", "", "", "", "TV", 1, 500, "", "", ""},
		[]any{"CIN-B", "01/01/2024", "Person B", "11112222", "", "", "", "", "TV", 1, 500, "", "", ""},
		[]any{"PHONE", "01/01/2024", "Not Existing", "", "98111222", "", "", "", "TV", 1, 500, "", "", ""},
		[]any{"OVERPAID", "01/01/2024", "Payer", "", "", "", "", "", "TV", 1, 500, 600, "", ""},
		[]any{"FUTURE", "01/03/2025", "Future", "", "", "", "", "", "TV", 1, 500, "", "", ""},
		[]any{"MIXED", "01/01/2024", "Mixed", "", "", "", "", "", "TV", 1, 500, "", "", ""},
		[]any{"MIXED", "02/01/2024", "Mixed", "", "", "", "", "", "Radio", 1, 100, "", "", ""},
	)
	report, err := d.ImportLoanRegister(fx.orgID, "user-1", "bad.xlsx", fx.cutoverDay, sheet)
	if err != nil {
		t.Fatalf("ImportLoanRegister: %v", err)
	}
	refs := map[string]bool{}
	for _, p := range report.Problems {
		if p.Severity == "error" {
			refs[p.Ref] = true
		}
	}
	for _, want := range []string{"BAD-DATE", "NO-AMOUNT", "TWINS", "CIN-B", "PHONE", "OVERPAID", "FUTURE", "MIXED"} {
		if !refs[want] {
			t.Errorf("no error reported for %s; problems = %+v", want, report.Problems)
		}
	}
	if refs["OK-1"] || refs["CIN-A"] {
		t.Errorf("unexpected error on a valid loan; problems = %+v", report.Problems)
	}
	if report.Imported {
		t.Fatalf("a file with errors was imported")
	}
	if n := invoiceCount(t, d, fx.orgID); n != 0 {
		t.Fatalf("a refused import wrote %d invoices", n)
	}
}

func TestLoanImportUndo(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newLoanImportFixture(t, d, "org-loanimport-undo")

	report, err := d.ImportLoanRegister(fx.orgID, "user-1", "registre.xlsx", fx.cutoverDay, fx.goodSheet(t, d))
	if err != nil || !report.Imported {
		t.Fatalf("ImportLoanRegister: %+v, %v", report, err)
	}
	result, err := d.UndoLoanImportBatch(report.BatchID)
	if err != nil {
		t.Fatalf("UndoLoanImportBatch: %v", err)
	}
	// The new customer's open loan posted an AR line that its reversal keeps,
	// so that customer stays (and a re-import matches it by CIN).
	if result.LoansRemoved != 3 || result.CustomersRemoved != 0 || result.CustomersKept != 1 {
		t.Fatalf("undo = %+v, want 3 loans removed, the 1 created customer kept", result)
	}
	if n := invoiceCount(t, d, fx.orgID); n != 0 {
		t.Fatalf("invoices after undo = %d, want 0", n)
	}
	var posted int
	if err := d.DB.Get(&posted, `
		SELECT COUNT(*) FROM journal_entries WHERE organizationId = ? AND sourceDocumentType = ? AND status = 'posted' AND reversalOfEntryId IS NULL`,
		fx.orgID, openingLoanSourceType); err != nil || posted != 0 {
		t.Fatalf("live opening entries after undo = %d, %v; want 0 (all reversed)", posted, err)
	}
	if _, err := d.UndoLoanImportBatch(report.BatchID); err == nil {
		t.Fatalf("undoing twice succeeded")
	}

	// Re-import, collect one loan at the counter, and undo is refused.
	report, err = d.ImportLoanRegister(fx.orgID, "user-1", "registre.xlsx", fx.cutoverDay, fx.goodSheet(t, d))
	if err != nil || !report.Imported || report.CustomersCreated != 0 {
		t.Fatalf("re-import: %+v, %v; want imported, the kept customer matched", report, err)
	}
	register := accountByCode(t, d, fx.orgID, "1010").ID
	if _, err := d.UpdateOrganization(fx.orgID, UpdateOrganizationRequest{DefaultCashRegisterAccountID: &register}); err != nil {
		t.Fatalf("set register: %v", err)
	}
	rows, err := d.GetLoanStatus(fx.orgID, "")
	if err != nil {
		t.Fatalf("GetLoanStatus: %v", err)
	}
	for _, r := range rows {
		if r.Outstanding > 0 {
			if _, err := d.CreateCashSalePayment(CreateCashSalePaymentRequest{
				InvoiceID: r.InvoiceID, InvoiceLineItemID: r.LineID, Amount: 1000, Date: fx.cutover,
			}); err != nil {
				t.Fatalf("CreateCashSalePayment: %v", err)
			}
			break
		}
	}
	_, err = d.UndoLoanImportBatch(report.BatchID)
	requireFrozen(t, "undo after a collection", err)
}

func TestParseLoanSheetValues(t *testing.T) {
	tunis, _ := time.LoadLocation("Africa/Tunis")
	want := time.Date(2024, 6, 15, 0, 0, 0, 0, tunis).UnixMilli()
	for _, v := range []string{"15/06/2024", "15-06-2024", "15.06.2024", "2024-06-15", "15/6/2024", "45458"} {
		got, err := parseLoanSheetDate(v, tunis)
		if err != nil || got != want {
			t.Errorf("parseLoanSheetDate(%q) = %d, %v; want %d", v, got, err, want)
		}
	}
	for _, v := range []string{"31/02/2024", "tomorrow", "15/06/1850"} {
		if _, err := parseLoanSheetDate(v, tunis); err == nil {
			t.Errorf("parseLoanSheetDate(%q) accepted", v)
		}
	}
	for in, cents := range map[string]int64{"1200": 120000, "1 200,5": 120050, "1200.50": 120050, "1,200.50": 120050, "0,75": 75} {
		got, err := parseLoanSheetCents(in)
		if err != nil || got != cents {
			t.Errorf("parseLoanSheetCents(%q) = %d, %v; want %d", in, got, err, cents)
		}
	}
	if code := generateClientCodeGo("Ben Ali Mohamed"); code != "BA" {
		t.Errorf("generateClientCodeGo = %q, want BA", code)
	}
	if !strings.HasPrefix(loanImportHeaders[0], "Réf.") {
		t.Errorf("headers changed: %v", loanImportHeaders)
	}
}

// One customer written several times in the register — the CIN on one page
// only, name only (or name + phone) on others — is created once, whichever
// comes first, with every loan on it and the details merged.
func TestLoanImportCreatesARepeatedCustomerOnce(t *testing.T) {
	t.Parallel()
	for name, rows := range map[string][][]any{
		"CIN first": {
			{"A-1", "01/01/2024", "Salma Jaziri", "01112223", "", "", "", "", "TV", 1, 500, 500, "", ""},
			{"A-2", "01/02/2024", "Salma Jaziri", "", "22 333 444", "", "", "", "TV", 1, 500, 500, "", ""},
			{"A-3", "01/03/2024", "salma  jaziri", "", "", "", "Monastir", "", "TV", 1, 500, 100, "", ""},
		},
		"name first": {
			{"A-1", "01/01/2024", "Salma Jaziri", "", "", "", "", "", "TV", 1, 500, 500, "", ""},
			{"A-2", "01/02/2024", "Salma Jaziri", "01112223", "22 333 444", "", "Monastir", "", "TV", 1, 500, 500, "", ""},
			{"A-3", "01/03/2024", "Salma Jaziri", "", "22333444", "", "", "", "TV", 1, 500, 100, "", ""},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			d := newTestDB(t)
			fx := newLoanImportFixture(t, d, "org-loanimport-dedupe-"+strings.ReplaceAll(name, " ", "-"))
			sheet := make([][]any, len(rows))
			copy(sheet, rows)
			report, err := d.ImportLoanRegister(fx.orgID, "u", "f.xlsx", fx.cutoverDay, loanSheet(t, sheet...))
			if err != nil || !report.Imported {
				t.Fatalf("import: %+v, %v", report, err)
			}
			if report.CustomersCreated != 1 || len(report.Customers) != 1 || report.Customers[0].Loans != 3 {
				t.Fatalf("report = %+v, want one new customer with 3 loans", report)
			}
			var created []Client
			if err := d.DB.Select(&created, `SELECT * FROM clients WHERE importBatchId = ?`, report.BatchID); err != nil || len(created) != 1 {
				t.Fatalf("created customers = %d, %v; want 1", len(created), err)
			}
			c := created[0]
			if c.IdentityNumber == nil || *c.IdentityNumber != "01112223" || c.Phone == nil || normalizePhone(*c.Phone) != "22333444" ||
				c.Address == nil || *c.Address != "Monastir" {
				t.Fatalf("created customer = %+v, want CIN, phone and address merged from every loan", c)
			}
		})
	}
}

// A CIN typed into a number cell loses its leading zero; it still matches.
func TestLoanImportMatchesACINThatLostItsLeadingZero(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newLoanImportFixture(t, d, "org-loanimport-cin-zero")
	report, err := d.ImportLoanRegister(fx.orgID, "u", "f.xlsx", fx.cutoverDay, loanSheet(t,
		[]any{"Z-1", "01/01/2024", "Karim Trabelsi", 7654321, "", "", "", "", "TV", 1, 500, 0, "", ""},
	))
	if err != nil || !report.Imported || report.CustomersCreated != 0 || report.CustomersMatched != 1 {
		t.Fatalf("import = %+v, %v; want the existing customer with CIN 07654321 matched", report, err)
	}
}

// The downloaded template, filled in, imports as-is — even with its help
// sheet moved first — so the template and the reader can't drift apart.
func TestLoanImportTemplateRoundTrip(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newLoanImportFixture(t, d, "org-loanimport-template")
	template, err := d.LoanImportTemplateXLSX()
	if err != nil {
		t.Fatalf("LoanImportTemplateXLSX: %v", err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(template))
	if err != nil {
		t.Fatalf("open template: %v", err)
	}
	defer f.Close() //nolint:errcheck
	if sheets := f.GetSheetList(); len(sheets) != 2 || sheets[0] != loanImportSheet {
		t.Fatalf("sheets = %v, want [%s, Mode d'emploi]", sheets, loanImportSheet)
	}
	if rows, _ := f.GetRows(loanImportSheet); len(rows) != 1 {
		t.Fatalf("data sheet has %d rows, want the header only (no example to import by mistake)", len(rows))
	}
	for c, v := range []any{"T-1", "15/06/2024", "Ahmed Gharbi", "00123456", "98 000 111", "", "", "", "TV", 1, "750,5", "", "", "0 payé"} {
		cell, _ := excelize.CoordinatesToCellName(c+1, 2)
		_ = f.SetCellValue(loanImportSheet, cell, v)
	}
	for c, v := range []any{"T-1", "", "", "", "", "", "", "", "Radio", 2, 100, 0, "", ""} {
		cell, _ := excelize.CoordinatesToCellName(c+1, 3)
		_ = f.SetCellValue(loanImportSheet, cell, v)
	}
	if err := f.MoveSheet("Mode d'emploi", loanImportSheet); err != nil {
		t.Fatalf("MoveSheet: %v", err)
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatalf("write: %v", err)
	}
	report, err := d.DryRunLoanImport(fx.orgID, fx.cutoverDay, buf.Bytes())
	if err != nil {
		t.Fatalf("DryRunLoanImport: %v", err)
	}
	if report.Errors != 0 || report.Loans != 1 || report.Lines != 2 || report.Total != 85050 || report.Paid != 0 {
		t.Fatalf("report = %+v, want one clean 850.50 loan (the 0 on row 3 is not a conflict)", report)
	}
}

// Undo removes a customer the import created when nothing references it —
// here one whose only loan was settled, so no GL line names it.
func TestLoanImportUndoRemovesAnUnreferencedCustomer(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newLoanImportFixture(t, d, "org-loanimport-undo-client")
	report, err := d.ImportLoanRegister(fx.orgID, "u", "f.xlsx", fx.cutoverDay, loanSheet(t,
		[]any{"S-1", "01/01/2024", "Settled Only", "", "", "", "", "", "TV", 1, 500, 500, "", ""},
	))
	if err != nil || !report.Imported {
		t.Fatalf("import: %+v, %v", report, err)
	}
	result, err := d.UndoLoanImportBatch(report.BatchID)
	if err != nil || result.LoansRemoved != 1 || result.CustomersRemoved != 1 || result.CustomersKept != 0 {
		t.Fatalf("undo = %+v, %v; want the loan and its customer removed", result, err)
	}
}
