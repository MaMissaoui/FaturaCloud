package db

import (
	"bytes"
	"strings"

	"github.com/xuri/excelize/v2"
)

// Every workbook the app reads — an uploaded import or template, or a stored
// template an export fills — is opened through openXLSX (audit F173).
// excelize's defaults extract up to 16 GB, so a 1 MB upload that unzips to
// 248 MB held 570 MB of extra memory, and a stored template re-inflated on
// every export. The 8 MB upload cap limits only the compressed size.
//
// The limits come from measured sizes: the largest real workbook — the
// products export of the quarter-scale retail demo — unzips to 114 KB, the
// embedded document templates and the loan register template to 25–35 KB.
// 64 MB is several times anything a real organization sends (a 20,000-row
// products sheet is about 12 MB), and a sheet over 16 MB is read from a
// temporary file rather than memory; f.Close removes it.
const (
	maxXLSXUnzipSize   = 64 << 20
	maxXLSXSheetMemory = 16 << 20
)

// maxImportRows caps the data rows one upload may carry; a bigger file is
// split into several.
const maxImportRows = 20000

// openXLSX opens content with the size limits above. The caller must Close
// the file. A workbook over the limit, or not a workbook at all, is a
// validation error (409).
func openXLSX(content []byte) (*excelize.File, error) {
	f, err := excelize.OpenReader(bytes.NewReader(content), excelize.Options{
		UnzipSizeLimit:    maxXLSXUnzipSize,
		UnzipXMLSizeLimit: maxXLSXSheetMemory,
	})
	if err != nil {
		if strings.Contains(err.Error(), "unzip size exceeds") {
			return nil, newValidationError("the workbook is too large: it unzips to more than %d MB", maxXLSXUnzipSize>>20)
		}
		return nil, newValidationError("file is not a valid Excel (.xlsx) workbook")
	}
	return f, nil
}

// checkImportRowCount refuses a sheet with more than maxImportRows rows
// below its header.
func checkImportRowCount(rows [][]string) error {
	if len(rows)-1 > maxImportRows {
		return newValidationError("the file has %d rows; split it into files of at most %d rows", len(rows)-1, maxImportRows)
	}
	return nil
}
