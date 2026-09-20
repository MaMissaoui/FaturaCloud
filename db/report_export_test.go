package db

import (
	"bytes"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

// TestReportExportsFitToPageWidthWithSmallMargins locks in the page setup
// both Cash Book report exports must carry so their PDF conversion fits a
// portrait page: FitToPage + FitToWidth=1 (scale to one page wide) and small
// print margins. Without it LibreOffice converted the Loan status report's
// six columns at the sheet's full width, overflowing/clipping the printable
// area.
func TestReportExportsFitToPageWidthWithSmallMargins(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-report-export-page-setup")
	register := accountByCode(t, d, fx.orgID, "1010")

	cases := []struct {
		name string
		gen  func() ([]byte, error)
	}{
		{"daily cash movements", func() ([]byte, error) {
			raw, _, err := d.GenerateDailyCashMovementsExport(fx.orgID, register.ID, fx.date)
			return raw, err
		}},
		{"loan status", func() ([]byte, error) {
			raw, _, err := d.GenerateLoanStatusExport(fx.orgID, "", false)
			return raw, err
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := tc.gen()
			if err != nil {
				t.Fatalf("generate: %v", err)
			}
			f, err := excelize.OpenReader(bytes.NewReader(raw))
			if err != nil {
				t.Fatalf("open workbook: %v", err)
			}
			defer f.Close()
			sheet := f.GetSheetName(0)

			layout, err := f.GetPageLayout(sheet)
			if err != nil {
				t.Fatalf("GetPageLayout: %v", err)
			}
			if layout.FitToWidth == nil || *layout.FitToWidth != 1 {
				t.Errorf("FitToWidth = %v, want 1", layout.FitToWidth)
			}
			if layout.FitToHeight == nil || *layout.FitToHeight != 0 {
				t.Errorf("FitToHeight = %v, want 0 (height unconstrained)", layout.FitToHeight)
			}
			// A4 (Excel code 9) — without an explicit size, writing the
			// pageSetup makes LibreOffice default to Letter.
			if layout.Size == nil || *layout.Size != 9 {
				t.Errorf("paper size = %v, want 9 (A4)", layout.Size)
			}

			props, err := f.GetSheetProps(sheet)
			if err != nil {
				t.Fatalf("GetSheetProps: %v", err)
			}
			if props.FitToPage == nil || !*props.FitToPage {
				t.Errorf("FitToPage = %v, want true", props.FitToPage)
			}

			margins, err := f.GetPageMargins(sheet)
			if err != nil {
				t.Fatalf("GetPageMargins: %v", err)
			}
			checkMargin := func(name string, got *float64, max float64) {
				if got == nil {
					t.Errorf("%s margin is nil, want <= %v", name, max)
					return
				}
				if *got > max {
					t.Errorf("%s margin = %v, want <= %v", name, *got, max)
				}
			}
			checkMargin("left", margins.Left, 0.25)
			checkMargin("right", margins.Right, 0.25)
			checkMargin("top", margins.Top, 0.25)
			checkMargin("bottom", margins.Bottom, 0.25)
			checkMargin("header", margins.Header, 0.15)
			checkMargin("footer", margins.Footer, 0.15)
		})
	}
}

// TestLoanStatusExportNamesFilterRepeatsHeaderAndTotals locks in the three
// additions that make the exported loan status report usable as a printed
// document: the active filter named in the first-page header block, the
// column-label row set as print titles so it repeats on every page, and a
// totals row summing the money columns at the end.
func TestLoanStatusExportNamesFilterRepeatsHeaderAndTotals(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-loan-export")

	// Two loan sales (no payment yet, state sent): 1000 + 20% = 1200 and
	// 500 + 20% = 600, for a 1800-cent original total.
	for _, inv := range []struct {
		id   string
		unit float64
	}{{"INV-LOAN-1", 1000}, {"INV-LOAN-2", 500}} {
		created := fx.createInvoice(t, d, inv.id, 1, inv.unit)
		if _, err := d.UpdateInvoiceState(created.ID, "sent"); err != nil {
			t.Fatalf("UpdateInvoiceState(%s): %v", inv.id, err)
		}
	}

	org, err := d.GetOrganization(fx.orgID)
	if err != nil {
		t.Fatalf("GetOrganization: %v", err)
	}

	raw, _, err := d.GenerateLoanStatusExport(fx.orgID, "", false)
	if err != nil {
		t.Fatalf("GenerateLoanStatusExport: %v", err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("open workbook: %v", err)
	}
	defer f.Close()
	sheet := f.GetSheetName(0)

	subtitle, _ := f.GetCellValue(sheet, "A2")
	if !strings.Contains(subtitle, "All customers") {
		t.Errorf("A2 = %q, want the unfiltered export to name its filter", subtitle)
	}

	// reportWorkbook starts the table at row 4; that label row is what must
	// repeat on every printed page.
	if label, _ := f.GetCellValue(sheet, "A4"); label != "Customer" {
		t.Fatalf("A4 = %q, want the column-label row", label)
	}
	var printTitles string
	for _, dn := range f.GetDefinedName() {
		if dn.Name == "_xlnm.Print_Titles" {
			printTitles = dn.RefersTo
		}
	}
	if !strings.Contains(printTitles, "$4:$4") {
		t.Errorf("Print_Titles = %q, want it to repeat row 4", printTitles)
	}

	// Two data rows (5, 6) then the totals row (7). Amount is column F.
	if total, _ := f.GetCellValue(sheet, "A7"); total != "Total" {
		t.Fatalf("A7 = %q, want the totals row", total)
	}
	currency := ""
	if org.Currency != nil {
		currency = *org.Currency
	}
	wantAmount := formatMoneyCents(1800, currency, org.MinimumFractionDigits, org.CountryCode)
	if got, _ := f.GetCellValue(sheet, "F7"); got != wantAmount {
		t.Errorf("total amount = %q, want %q", got, wantAmount)
	}

	// A customer-scoped export names that customer in the first-page header.
	scoped, _, err := d.GenerateLoanStatusExport(fx.orgID, fx.clientID, false)
	if err != nil {
		t.Fatalf("GenerateLoanStatusExport (scoped): %v", err)
	}
	sf, err := excelize.OpenReader(bytes.NewReader(scoped))
	if err != nil {
		t.Fatalf("open scoped workbook: %v", err)
	}
	defer sf.Close()
	if got, _ := sf.GetCellValue(sf.GetSheetName(0), "A2"); !strings.Contains(got, "Test Client") {
		t.Errorf("scoped A2 = %q, want it to name the customer", got)
	}
}
