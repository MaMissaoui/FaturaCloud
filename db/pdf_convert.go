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
// expected state in a bare `go run` dev environment or any CI job that
// hasn't installed libreoffice-calc (this repo's CI doesn't, deliberately —
// see db/pdf_convert_test.go). The *shipped* Docker image does carry
// libreoffice-calc as of the Debian-based runtime stage (see the
// Dockerfile's stage-3 comment for why Alpine couldn't run it and Debian
// can) — this error path exists for those non-image environments, and as a
// clean 503 rather than a 500 if a future change ever ships without it
// again. XLSX export works regardless either way.
var ErrPDFConversionUnavailable = errors.New("PDF conversion is not available on this server (LibreOffice not installed)")

// ErrPDFConversionBusy means every conversion slot stayed taken for
// pdfSlotWait. The caller answers 503 so the user can retry.
var ErrPDFConversionBusy = errors.New("the server is busy converting other PDFs — try again in a moment")

// pdfConvertTimeout bounds a single soffice invocation — a malformed or
// pathological input must not hang the request indefinitely.
const pdfConvertTimeout = 60 * time.Second

// Each soffice process takes hundreds of MB, so conversions share a fixed
// number of slots: without them, a few dozen simultaneous exports ran the
// Raspberry Pi out of memory for every organization (audit F192). A request
// waits up to pdfSlotWait for a slot, then gets ErrPDFConversionBusy.
var (
	pdfConvertSlots = make(chan struct{}, 2)
	pdfSlotWait     = 20 * time.Second
)

// ConvertXLSXToPDF shells out to `soffice --headless --convert-to pdf`.
// Each call gets its own temp work directory and LibreOffice user-profile
// directory (`-env:UserInstallation`) — required both because the runtime
// container's non-root user has no writable ~/.config/libreoffice, and
// because concurrent soffice invocations sharing one profile directory can
// collide/hang, which would matter under concurrent exports from different
// organizations.
func ConvertXLSXToPDF(ctx context.Context, xlsxBytes []byte) ([]byte, error) {
	wait := time.NewTimer(pdfSlotWait)
	defer wait.Stop()
	select {
	case pdfConvertSlots <- struct{}{}:
		defer func() { <-pdfConvertSlots }()
	case <-wait.C:
		return nil, ErrPDFConversionBusy
	case <-ctx.Done():
		return nil, ctx.Err()
	}

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
