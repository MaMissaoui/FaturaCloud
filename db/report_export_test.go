package db

import (
	"bytes"
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
