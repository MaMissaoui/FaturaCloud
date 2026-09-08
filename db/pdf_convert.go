package db

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// ErrPDFConversionUnavailable means the soffice binary isn't on PATH — the
// expected state in a bare `go run` dev environment or any CI without
// libreoffice-calc installed, and — as of this writing — also the expected
// state in the *shipped* Docker image: libreoffice-calc crashes on startup
// on Alpine (`terminate called after throwing an instance of
// 'com::sun::star::uno::RuntimeException'`, reproduced with every standard
// workaround — SAL_USE_VCLPLUGIN=svp, SAL_DISABLE_SKIA=1, a JRE installed,
// gcompat installed — none fixed it; this matches long-standing, unresolved
// Alpine/musl LibreOffice packaging issues, not something specific to this
// app), so the Dockerfile does NOT install libreoffice-calc. This function
// is complete and tested (db/pdf_convert_test.go) and works wherever soffice
// actually runs (e.g. local dev with LibreOffice installed) — getting it
// running in the shipped image (most likely a Debian-based runtime stage,
// since LibreOffice runs reliably there) is deliberately deferred rather
// than rushed. The API layer maps this error to 503, not a bare 500: XLSX
// export works regardless.
var ErrPDFConversionUnavailable = errors.New("PDF conversion is not available on this server (LibreOffice not installed)")

// pdfConvertTimeout bounds a single soffice invocation — a malformed or
// pathological input must not hang the request indefinitely.
const pdfConvertTimeout = 60 * time.Second

// ConvertXLSXToPDF shells out to `soffice --headless --convert-to pdf`.
// Each call gets its own temp work directory and LibreOffice user-profile
// directory (`-env:UserInstallation`) — required both because the runtime
// container's non-root user has no writable ~/.config/libreoffice, and
// because concurrent soffice invocations sharing one profile directory can
// collide/hang, which would matter under concurrent exports from different
// organizations.
func ConvertXLSXToPDF(ctx context.Context, xlsxBytes []byte) ([]byte, error) {
	sofficePath, err := exec.LookPath("soffice")
	if err != nil {
		return nil, ErrPDFConversionUnavailable
	}

	workDir, err := os.MkdirTemp("", "fatura-xlsx-convert-*")
	if err != nil {
		return nil, fmt.Errorf("convert_xlsx_to_pdf: mkdir temp: %w", err)
	}
	defer os.RemoveAll(workDir)

	inputPath := filepath.Join(workDir, "input.xlsx")
	if err := os.WriteFile(inputPath, xlsxBytes, 0600); err != nil {
		return nil, fmt.Errorf("convert_xlsx_to_pdf: write input: %w", err)
	}

	profileDir := filepath.Join(workDir, "lo-profile")

	ctx, cancel := context.WithTimeout(ctx, pdfConvertTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, sofficePath,
		"--headless",
		"--convert-to", "pdf",
		"--outdir", workDir,
		"-env:UserInstallation=file://"+profileDir,
		inputPath,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("convert_xlsx_to_pdf: soffice timed out after %s", pdfConvertTimeout)
		}
		return nil, fmt.Errorf("convert_xlsx_to_pdf: soffice failed: %w (output: %s)", err, output)
	}

	pdfBytes, err := os.ReadFile(filepath.Join(workDir, "input.pdf"))
	if err != nil {
		return nil, fmt.Errorf("convert_xlsx_to_pdf: read output: %w (soffice output: %s)", err, output)
	}
	return pdfBytes, nil
}
