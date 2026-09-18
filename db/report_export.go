package db

import (
	"bytes"
	"fmt"
	"time"

	"github.com/xuri/excelize/v2"
)

// movementKindLabel mirrors src/routes/cash-book.tsx's movementKindLabel —
// English only, like every other server-built export in this file's family
// (FEC/DATEV, invoice template placeholders): there's no per-org template
// for these two reports to carry a translated label instead.
func movementKindLabel(kind string) string {
	switch kind {
	case "sale":
		return "Sale"
	case "loan":
		return "Loan (deposit)"
	case "repayment":
		return "Loan repayment"
	case "withdrawal":
		return "Withdrawal"
	default:
		return kind
	}
}

// reportWorkbook is the shared skeleton every report export below builds
// on: a title, a generated-on line, then a table starting at row 4 (one
// blank row of breathing room under the subtitle). Callers get back the
// active sheet name and the row index to start writing table headers at.
func reportWorkbook(title, subtitle string) (f *excelize.File, sheet string, headerRow int) {
	f = excelize.NewFile()
	sheet = f.GetSheetName(0)

	titleStyle, _ := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true, Size: 14}})
	subtitleStyle, _ := f.NewStyle(&excelize.Style{Font: &excelize.Font{Color: "666666"}})

	_ = f.SetCellValue(sheet, "A1", title)
	_ = f.SetCellStyle(sheet, "A1", "A1", titleStyle)
	_ = f.SetCellValue(sheet, "A2", subtitle)
	_ = f.SetCellStyle(sheet, "A2", "A2", subtitleStyle)

	return f, sheet, 4
}

// writeHeaderRow writes bold column headers at the given row and returns
// the style id, reused for nothing else — callers just need the row filled.
func writeHeaderRow(f *excelize.File, sheet string, row int, headers []string) error {
	headerStyle, err := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true},
		Fill: excelize.Fill{Type: "pattern", Color: []string{"F0F0F0"}, Pattern: 1},
	})
	if err != nil {
		return err
	}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, row)
		if err := f.SetCellValue(sheet, cell, h); err != nil {
			return err
		}
	}
	endCell, _ := excelize.CoordinatesToCellName(len(headers), row)
	startCell, _ := excelize.CoordinatesToCellName(1, row)
	return f.SetCellStyle(sheet, startCell, endCell, headerStyle)
}

func setRow(f *excelize.File, sheet string, row int, values []any) error {
	for i, v := range values {
		cell, _ := excelize.CoordinatesToCellName(i+1, row)
		if err := f.SetCellValue(sheet, cell, v); err != nil {
			return err
		}
	}
	return nil
}

func setColWidths(f *excelize.File, sheet string, widths []float64) {
	for i, w := range widths {
		col, _ := excelize.ColumnNumberToName(i + 1)
		_ = f.SetColWidth(sheet, col, col, w)
	}
}

// GenerateDailyCashMovementsExport builds the Cash Book screen's daily
// register panel (opening/in/out/closing plus the per-transaction detail
// table) as an .xlsx workbook for one UTC day, the same accountID/day
// range the screen itself requests via GetDailyCashMovements/
// GetCashMovementDetails. Returns nil, "", err with the same
// cross-org/validation errors GetDailyCashMovements/GetCashMovementDetails
// already produce (resolveCashReportAccount) — no new validation here.
func (d *Database) GenerateDailyCashMovementsExport(organizationID, accountID string, dayMs int64) ([]byte, string, error) {
	org, err := d.GetOrganization(organizationID)
	if err != nil {
		return nil, "", err
	}

	summaryRows, err := d.GetDailyCashMovements(organizationID, accountID, dayMs, dayMs)
	if err != nil {
		return nil, "", err
	}
	details, err := d.GetCashMovementDetails(organizationID, accountID, dayMs, dayMs)
	if err != nil {
		return nil, "", err
	}

	orgName := ""
	if org.Name != nil {
		orgName = *org.Name
	}
	currency := ""
	if org.Currency != nil {
		currency = *org.Currency
	}
	money := func(cents int64) string {
		return formatMoneyCents(cents, currency, org.MinimumFractionDigits, org.CountryCode)
	}
	dateLabel := formatOrgDate(dayMs, org.DateFormat)

	f, sheet, row := reportWorkbook(
		"Daily Cash Movements",
		fmt.Sprintf("%s — %s — generated %s", orgName, dateLabel, formatOrgDate(time.Now().UnixMilli(), org.DateFormat)),
	)
	defer f.Close()

	if err := writeHeaderRow(f, sheet, row, []string{"Opening", "In", "Out", "Closing"}); err != nil {
		return nil, "", err
	}
	row++
	if len(summaryRows) > 0 {
		s := summaryRows[0]
		if err := setRow(f, sheet, row, []any{money(s.Opening), money(s.In), money(s.Out), money(s.Closing)}); err != nil {
			return nil, "", err
		}
	}
	row += 2

	if err := writeHeaderRow(f, sheet, row, []string{"Time", "Type", "Customer", "Amount"}); err != nil {
		return nil, "", err
	}
	row++
	for _, det := range details {
		customer := ""
		if det.ClientName != nil {
			customer = *det.ClientName
		} else if det.Note != nil {
			customer = *det.Note
		}
		sign := "+"
		if det.Direction != "in" {
			sign = "−"
		}
		if err := setRow(f, sheet, row, []any{
			time.UnixMilli(det.Date).UTC().Format("15:04"), movementKindLabel(det.Kind), customer, sign + money(det.Amount),
		}); err != nil {
			return nil, "", err
		}
		row++
	}

	setColWidths(f, sheet, []float64{14, 20, 28, 16})

	var buf bytes.Buffer
	raw, err := f.WriteToBuffer()
	if err != nil {
		return nil, "", err
	}
	buf.Write(raw.Bytes())

	// A filesystem-safe ISO date, deliberately not dateLabel (org.DateFormat
	// can contain literal "/", e.g. "DD/MM/YYYY" — fine in an on-sheet
	// subtitle, not fine in a Content-Disposition filename).
	filename := "daily-cash-movements-" + time.UnixMilli(dayMs).UTC().Format("2006-01-02")
	return buf.Bytes(), filename, nil
}

// GenerateLoanStatusExport builds the Cash Book screen's loan status table
// (GetLoanStatus) as an .xlsx workbook, honoring the same customer filter
// the screen's own Select applies (empty clientID means every customer)
// plus an openOnly filter with no screen-side equivalent in GetLoanStatus
// itself — filtered here in Go rather than added to that query, since the
// on-screen table filters identically client-side and the row counts this
// report deals with are small.
func (d *Database) GenerateLoanStatusExport(organizationID, clientID string, openOnly bool) ([]byte, string, error) {
	org, err := d.GetOrganization(organizationID)
	if err != nil {
		return nil, "", err
	}

	rows, err := d.GetLoanStatus(organizationID, clientID)
	if err != nil {
		return nil, "", err
	}
	if openOnly {
		filtered := rows[:0]
		for _, r := range rows {
			if r.Outstanding != 0 {
				filtered = append(filtered, r)
			}
		}
		rows = filtered
	}

	orgName := ""
	if org.Name != nil {
		orgName = *org.Name
	}
	currency := ""
	if org.Currency != nil {
		currency = *org.Currency
	}
	money := func(cents int64) string {
		return formatMoneyCents(cents, currency, org.MinimumFractionDigits, org.CountryCode)
	}

	subtitle := fmt.Sprintf("%s — generated %s", orgName, formatOrgDate(time.Now().UnixMilli(), org.DateFormat))
	if openOnly {
		subtitle += " — open loans only"
	}

	f, sheet, row := reportWorkbook("Loan Status", subtitle)
	defer f.Close()

	if err := writeHeaderRow(f, sheet, row, []string{
		"Customer", "Invoice", "Date", "Original", "Paid", "Outstanding",
	}); err != nil {
		return nil, "", err
	}
	row++
	for _, r := range rows {
		if err := setRow(f, sheet, row, []any{
			r.ClientName, r.Number, formatOrgDate(r.Date, org.DateFormat),
			money(r.Original), money(r.Paid), money(r.Outstanding),
		}); err != nil {
			return nil, "", err
		}
		row++
	}

	setColWidths(f, sheet, []float64{24, 16, 14, 16, 16, 16})

	var buf bytes.Buffer
	raw, err := f.WriteToBuffer()
	if err != nil {
		return nil, "", err
	}
	buf.Write(raw.Bytes())

	filename := "loan-status"
	if openOnly {
		filename += "-open"
	}
	return buf.Bytes(), filename, nil
}
