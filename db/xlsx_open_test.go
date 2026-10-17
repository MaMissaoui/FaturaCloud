package db

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// minimalXLSX builds a one-sheet workbook by hand, its sheet holding rows
// rows of one inline-string cell each, padded with pad bytes of whitespace
// so a small zip can unzip to a large sheet — how a decompression bomb is
// made.
func minimalXLSX(t *testing.T, rows int, pad int) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	write := func(name, body string) {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	write("[Content_Types].xml", `<?xml version="1.0" encoding="UTF-8"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/><Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/></Types>`)
	write("_rels/.rels", `<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>`)
	write("xl/workbook.xml", `<?xml version="1.0" encoding="UTF-8"?><workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="Sheet1" sheetId="1" r:id="rId1"/></sheets></workbook>`)
	write("xl/_rels/workbook.xml.rels", `<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/></Relationships>`)

	w, err := zw.Create("xl/worksheets/sheet1.xml")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`))
	for i := 1; i <= rows; i++ {
		_, _ = fmt.Fprintf(w, `<row r="%d"><c r="A%d" t="inlineStr"><is><t>x%d</t></is></c></row>`, i, i, i)
	}
	chunk := []byte(strings.Repeat(" ", 1<<20))
	for pad > 0 {
		n := min(pad, len(chunk))
		_, _ = w.Write(chunk[:n])
		pad -= n
	}
	_, _ = w.Write([]byte(`</sheetData></worksheet>`))
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// TestOpenXLSXRefusesADecompressionBomb pins audit F173: a small upload that
// unzips past the limit is refused before it is extracted, on every upload
// route.
func TestOpenXLSXRefusesADecompressionBomb(t *testing.T) {
	t.Parallel()
	bomb := minimalXLSX(t, 1, maxXLSXUnzipSize+(1<<20))
	if len(bomb) > 1<<20 {
		t.Fatalf("bomb is %d bytes compressed; the test wants a small file", len(bomb))
	}
	if _, err := openXLSX(bomb); err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("openXLSX(bomb) = %v; want a too-large refusal", err)
	}

	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-bomb")
	if err := d.UploadDocumentTemplate(fx.orgID, "invoice", "bomb.xlsx", bomb); !isValidationErr(err) {
		t.Fatalf("UploadDocumentTemplate(bomb) = %v; want a validation error", err)
	}
	if _, err := d.ImportProductsXLSX(fx.orgID, bomb); !isValidationErr(err) {
		t.Fatalf("ImportProductsXLSX(bomb) = %v; want a validation error", err)
	}
	if _, err := d.DryRunLoanImport(fx.orgID, fx.date, bomb); !isValidationErr(err) {
		t.Fatalf("DryRunLoanImport(bomb) = %v; want a validation error", err)
	}

	// A workbook just under the limit still opens.
	if f, err := openXLSX(minimalXLSX(t, 1, 1<<20)); err != nil {
		t.Fatalf("openXLSX(normal) = %v", err)
	} else {
		f.Close() //nolint:errcheck
	}
}

func isValidationErr(err error) bool {
	var ve *ValidationError
	return errors.As(err, &ve)
}

// TestUploadDocumentTemplateLeavesNoTempFile: a sheet over the in-memory
// limit is read from a temporary file, and validating an upload must
// remove it (each upload used to leave one behind, audit F173).
func TestUploadDocumentTemplateLeavesNoTempFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-tmp")

	big := minimalXLSX(t, 1, maxXLSXSheetMemory+(1<<20))
	if err := d.UploadDocumentTemplate(fx.orgID, "invoice", "big.xlsx", big); err != nil {
		t.Fatalf("UploadDocumentTemplate: %v", err)
	}
	left, _ := filepath.Glob(filepath.Join(tmp, "excelize-*"))
	if len(left) > 0 {
		t.Fatalf("temporary files left behind: %v", left)
	}
	if _, err := os.Stat(tmp); err != nil {
		t.Fatal(err)
	}
}

// TestImportRefusesTooManyRows: an import over maxImportRows is refused as
// a whole rather than run row by row.
func TestImportRefusesTooManyRows(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-rows")
	content := minimalXLSX(t, maxImportRows+2, 0)
	_, err := d.ImportProductsXLSX(fx.orgID, content)
	if err == nil || !strings.Contains(err.Error(), "split it into files") {
		t.Fatalf("ImportProductsXLSX(%d rows) = %v; want a row-count refusal", maxImportRows+1, err)
	}
}
