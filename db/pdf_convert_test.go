package db

import (
	"context"
	"errors"
	"os/exec"
	"testing"
	"time"
)

// TestConvertXLSXToPDFSmoke is this repo's first skip-if-tool-missing test:
// libreoffice-calc isn't installed in every dev/CI environment (it's a new
// runtime dependency introduced for issue #115's PDF export path), so this
// skips rather than fails when soffice isn't on PATH, and actually exercises
// the conversion when it is.
func TestConvertXLSXToPDFSmoke(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("soffice"); err != nil {
		t.Skip("soffice not on PATH — skipping LibreOffice conversion smoke test")
	}

	pdfBytes, err := ConvertXLSXToPDF(context.Background(), invoiceDefaultTemplate)
	if err != nil {
		t.Fatalf("ConvertXLSXToPDF: %v", err)
	}
	if len(pdfBytes) < 5 || string(pdfBytes[:5]) != "%PDF-" {
		t.Fatalf("output does not look like a PDF (first bytes: %q)", pdfBytes[:min(5, len(pdfBytes))])
	}
}

func TestConvertXLSXToPDFUnavailableWhenSofficeMissing(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("soffice"); err == nil {
		t.Skip("soffice is on PATH in this environment — can't exercise the missing-binary path")
	}

	_, err := ConvertXLSXToPDF(context.Background(), invoiceDefaultTemplate)
	if err != ErrPDFConversionUnavailable {
		t.Fatalf("got %v, want ErrPDFConversionUnavailable", err)
	}
}

// Conversions share a fixed number of slots (audit F192). With every slot
// taken, a request waits pdfSlotWait and then gets ErrPDFConversionBusy, or
// returns as soon as its caller goes away. Not parallel: it fills the
// package-level slots.
func TestConvertXLSXToPDFBusyWhenSlotsAreTaken(t *testing.T) {
	saved := pdfSlotWait
	pdfSlotWait = 50 * time.Millisecond
	for range cap(pdfConvertSlots) {
		pdfConvertSlots <- struct{}{}
	}
	t.Cleanup(func() {
		for range cap(pdfConvertSlots) {
			<-pdfConvertSlots
		}
		pdfSlotWait = saved
	})

	if _, err := ConvertXLSXToPDF(context.Background(), invoiceDefaultTemplate); !errors.Is(err, ErrPDFConversionBusy) {
		t.Fatalf("got %v, want ErrPDFConversionBusy", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ConvertXLSXToPDF(ctx, invoiceDefaultTemplate); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want context.Canceled for a caller that went away", err)
	}
}
