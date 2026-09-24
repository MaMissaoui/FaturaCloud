package db

import (
	"bytes"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
)

// This file is the shared engine behind mass-maintenance Excel download/
// upload for the app's flat reference-data lists (clients, vendors,
// products, tax rates, payment terms, units of measure, chart of accounts)
// — the "download it, edit a lot of rows in Excel, upload it back" workflow.
// It knows nothing about any one table; massDataSpec (one small
// implementation per table, in mass_data_<table>.go) supplies that. Every
// row still goes through the table's own Create*/Update* function, so this
// never bypasses the validation and business rules those already enforce —
// it only saves the caller from doing it one record at a time through the
// UI. There is no bulk delete: removing a row from the spreadsheet does
// nothing server-side, since silently deleting on absence would be far more
// dangerous than a caller forgetting a row.
const massDataSheetName = "Data"

// massDataSpec is one table's mapping between spreadsheet columns and its
// own Create*/Update* requests. ctx is whatever NewImportContext returns
// (nil for a table with no foreign-key columns to resolve) — built once per
// import call rather than re-queried per row.
type massDataSpec interface {
	Headers() []string
	ExportRows(d *Database, organizationID string) ([][]string, error)
	NewImportContext(d *Database, organizationID string) (any, error)
	// ImportRow creates or updates one record from a row already padded to
	// len(Headers()). cells[0] is always the ID column: blank creates, a
	// non-blank value updates that record (after checking it belongs to
	// organizationID). Returns the record's own display name/code for the
	// report even on error, when it could be read before the failure.
	ImportRow(d *Database, organizationID string, ctx any, cells []string) (action, identifier string, err error)
}

// massDataColumnAware is implemented by a spec that has added columns since
// its first release (clients: migration 0087's counter fields). The importer
// tells it how many columns the uploaded sheet's header row actually has, so
// a workbook exported before a column existed — which padCells hands over as
// blank — leaves that field as stored instead of clearing it on every
// updated row. A blank cell in a column the sheet *does* have still clears.
type massDataColumnAware interface {
	SetPresentColumns(ctx any, n int) any
}

// MassDataRowResult reports what happened to one spreadsheet row.
type MassDataRowResult struct {
	// Row is the 1-based row number as it appears in Excel (row 1 is the
	// header, so the first data row is 2) — lets a caller jump straight to
	// the offending row instead of counting.
	Row        int    `json:"row"`
	Identifier string `json:"identifier"`
	Action     string `json:"action"` // "created" | "updated" | "error"
	Error      string `json:"error,omitempty"`
}

// MassDataImportResult is the whole report for one uploaded file.
type MassDataImportResult struct {
	Created int                 `json:"created"`
	Updated int                 `json:"updated"`
	Failed  int                 `json:"failed"`
	Rows    []MassDataRowResult `json:"rows"`
}

// exportMassDataXLSX builds one workbook: a bold header row from
// spec.Headers(), then one row per existing record in the org's own default
// list order — the same order the app's own list page would show.
func exportMassDataXLSX(spec massDataSpec, d *Database, organizationID string) ([]byte, error) {
	rows, err := spec.ExportRows(d, organizationID)
	if err != nil {
		return nil, err
	}

	f := excelize.NewFile()
	defer f.Close() //nolint:errcheck

	if err := f.SetSheetName(f.GetSheetName(0), massDataSheetName); err != nil {
		return nil, fmt.Errorf("mass_data export: rename sheet: %w", err)
	}

	headers := spec.Headers()
	for i, header := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		if err := f.SetCellValue(massDataSheetName, cell, header); err != nil {
			return nil, fmt.Errorf("mass_data export: write header: %w", err)
		}
	}
	if boldStyle, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}}); err == nil {
		lastCol, _ := excelize.CoordinatesToCellName(len(headers), 1)
		_ = f.SetCellStyle(massDataSheetName, "A1", lastCol, boldStyle)
	}

	for r, row := range rows {
		for c, value := range row {
			cell, _ := excelize.CoordinatesToCellName(c+1, r+2)
			if err := f.SetCellValue(massDataSheetName, cell, value); err != nil {
				return nil, fmt.Errorf("mass_data export: write row %d: %w", r+2, err)
			}
		}
	}

	for i := range headers {
		col, _ := excelize.ColumnNumberToName(i + 1)
		_ = f.SetColWidth(massDataSheetName, col, col, 22)
	}

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, fmt.Errorf("mass_data export: %w", err)
	}
	return buf.Bytes(), nil
}

// importMassDataXLSX reads every non-blank data row of the uploaded
// workbook's first sheet (whatever it's named — a resave in Excel doesn't
// have to keep massDataSheetName) and runs each through spec.ImportRow,
// continuing past a failed row rather than aborting the whole file: a typo
// on row 40 of 500 shouldn't cost the other 499 a valid import.
func importMassDataXLSX(spec massDataSpec, d *Database, organizationID string, content []byte) (*MassDataImportResult, error) {
	f, err := excelize.OpenReader(bytes.NewReader(content))
	if err != nil {
		return nil, newValidationError("file is not a valid Excel (.xlsx) workbook")
	}
	defer f.Close() //nolint:errcheck

	sheet := f.GetSheetName(0)
	allRows, err := f.GetRows(sheet)
	if err != nil {
		return nil, fmt.Errorf("mass_data import: read rows: %w", err)
	}

	ctx, err := spec.NewImportContext(d, organizationID)
	if err != nil {
		return nil, err
	}

	numCols := len(spec.Headers())
	if aware, ok := spec.(massDataColumnAware); ok {
		present := 0
		if len(allRows) > 0 {
			present = len(allRows[0])
		}
		ctx = aware.SetPresentColumns(ctx, present)
	}
	result := &MassDataImportResult{}
	for i, raw := range allRows {
		rowNum := i + 1
		if rowNum == 1 {
			continue // header
		}
		cells := padCells(raw, numCols)
		if rowIsBlank(cells) {
			continue
		}

		action, identifier, err := spec.ImportRow(d, organizationID, ctx, cells)
		rr := MassDataRowResult{Row: rowNum, Identifier: identifier}
		if err != nil {
			rr.Action = "error"
			rr.Error = err.Error()
			result.Failed++
		} else {
			rr.Action = action
			if action == "created" {
				result.Created++
			} else {
				result.Updated++
			}
		}
		result.Rows = append(result.Rows, rr)
	}
	return result, nil
}

// padCells returns row's first n cells, zero-padded — excelize's GetRows
// trims trailing empty cells off a row entirely, so a row with content only
// in its first few columns would otherwise come back shorter than
// len(Headers()) and panic on a direct index.
func padCells(row []string, n int) []string {
	out := make([]string, n)
	copy(out, row)
	return out
}

func rowIsBlank(cells []string) bool {
	for _, c := range cells {
		if strings.TrimSpace(c) != "" {
			return false
		}
	}
	return true
}

// cellStr trims a cell and returns nil for blank — the same "absent means
// don't set/unset" convention every nullable Create*/Update* field in this
// app already uses, so a blank cell round-trips as "no value" rather than a
// literal empty string.
func cellStr(v string) *string {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	return &v
}

// cellBool accepts the handful of spellings a business user would type for
// a checkbox-shaped column, defaulting anything else (including blank) to
// false — Yes/No is what boolCell writes back on export, so a re-uploaded,
// untouched file round-trips exactly.
func cellBool(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "yes", "y", "true", "x":
		return true
	default:
		return false
	}
}

func boolCell(b bool) string {
	if b {
		return "Yes"
	}
	return "No"
}

func intCell(b bool) int {
	if b {
		return 1
	}
	return 0
}

// int64Cell renders a *int64 the same "blank means unset" way cellStr does
// for strings, used for isDefault-shaped 0/1 pointer columns that are
// nullable rather than a plain bool.
func int64BoolCell(v *int64) string {
	return boolCell(v != nil && *v == 1)
}

func cellToInt64BoolPtr(v string) *int64 {
	var n int64
	if cellBool(v) {
		n = 1
	}
	return &n
}

// cellFloat parses a decimal number cell (percentage, quantity, ...),
// treating blank as 0 rather than an error — most numeric columns in this
// app's own forms already default an empty field to 0.
func cellFloat(v string) (float64, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, nil
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, fmt.Errorf("%q is not a number", v)
	}
	return f, nil
}

// centsCell renders integer cents as a plain decimal string in the
// organization's display units (e.g. 1999 -> "19.99") — a spreadsheet cell
// a business user edits directly, not a formatted-with-currency-symbol
// string, since this is a data-maintenance sheet, not a printed document.
func centsCell(cents int64) string {
	return strconv.FormatFloat(float64(cents)/100, 'f', 2, 64)
}

func centsCellPtr(cents *int64) string {
	if cents == nil {
		return ""
	}
	return centsCell(*cents)
}

// cellToCents is centsCell's inverse: parses a decimal display-units amount
// back into rounded integer cents, the same math/rounding shape every
// money-input form field in this app already applies at the input boundary
// (see CLAUDE.md's "Money" invariant).
func cellToCents(v string) (int64, error) {
	f, err := cellFloat(v)
	if err != nil {
		return 0, err
	}
	return int64(math.Round(f * 100)), nil
}

func cellToCentsPtr(v string) (*int64, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil, nil
	}
	cents, err := cellToCents(v)
	if err != nil {
		return nil, err
	}
	return &cents, nil
}

func strOrEmpty(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
