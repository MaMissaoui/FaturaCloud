package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/MaMissaoui/fatura-cloud/db"
)

// getDailyCashMovementsExport streams the Cash Book screen's daily register
// panel — opening/in/out/closing plus the per-transaction detail table for
// one UTC day — as .xlsx or, if converted, .pdf.
//
// Registered directly on mux, NOT through protected()'s auto-withDB — same
// reasoning as exportInvoiceDocument: a multi-second LibreOffice subprocess
// held inside withDB's request-long RLock would block a pending
// /api/restore write-lock acquisition and every request behind it.
func (h *handler) getDailyCashMovementsExport(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	format := r.URL.Query().Get("format")
	if format != "xlsx" && format != "pdf" {
		writeError(w, http.StatusBadRequest, "format must be xlsx or pdf")
		return
	}
	accountID := r.URL.Query().Get("accountId")
	if accountID == "" {
		writeError(w, http.StatusBadRequest, "accountId is required")
		return
	}
	dayMs, err := strconv.ParseInt(r.URL.Query().Get("date"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "date is required and must be a Unix millisecond timestamp")
		return
	}

	h.dbMu.RLock()
	xlsxBytes, filenameBase, err := h.db.GenerateDailyCashMovementsExport(orgID, accountID, dayMs)
	h.dbMu.RUnlock()
	if err != nil {
		if _, ok := errors.AsType[*db.ValidationError](err); ok {
			writeMutationError(w, err)
			return
		}
		writeDBError(w, err, "organization or account not found")
		return
	}

	streamReportExport(w, r, xlsxBytes, filenameBase, format)
}

// getLoanStatusExport streams the Cash Book screen's loan status table
// (GetLoanStatus) as .xlsx or, if converted, .pdf, honoring the same
// clientId filter the screen's own Select applies plus an openOnly filter
// with no screen-side query-param equivalent.
func (h *handler) getLoanStatusExport(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	format := r.URL.Query().Get("format")
	if format != "xlsx" && format != "pdf" {
		writeError(w, http.StatusBadRequest, "format must be xlsx or pdf")
		return
	}
	clientID := r.URL.Query().Get("clientId")
	openOnly := r.URL.Query().Get("openOnly") == "true"

	h.dbMu.RLock()
	xlsxBytes, filenameBase, err := h.db.GenerateLoanStatusExport(orgID, clientID, openOnly)
	h.dbMu.RUnlock()
	if err != nil {
		writeDBError(w, err, "organization not found")
		return
	}

	streamReportExport(w, r, xlsxBytes, filenameBase, format)
}

// getPaymentHistoryExport streams the Cash Book screen's Payment history card
// (inbound payments, newest first) as .xlsx or, if converted, .pdf, honoring
// the same clientId filter the screen applies to that card (the customer
// being served, or the loan report's own customer filter).
func (h *handler) getPaymentHistoryExport(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	format := r.URL.Query().Get("format")
	if format != "xlsx" && format != "pdf" {
		writeError(w, http.StatusBadRequest, "format must be xlsx or pdf")
		return
	}
	clientID := r.URL.Query().Get("clientId")

	h.dbMu.RLock()
	xlsxBytes, filenameBase, err := h.db.GeneratePaymentHistoryExport(orgID, clientID)
	h.dbMu.RUnlock()
	if err != nil {
		writeDBError(w, err, "organization not found")
		return
	}

	streamReportExport(w, r, xlsxBytes, filenameBase, format)
}

// streamReportExport is the shared xlsx/pdf response tail for the report
// exports above — same shape as exportInvoiceDocument's, factored out since
// none of them has per-type totals/unresolved-placeholder handling to set it
// apart.
func streamReportExport(w http.ResponseWriter, r *http.Request, xlsxBytes []byte, filenameBase, format string) {
	if format == "xlsx" {
		w.Header().Set("Content-Type", documentTemplateContentType)
		w.Header().Set("Content-Disposition", `attachment; filename="`+sanitizeContentDispositionFilename(filenameBase)+`.xlsx"`)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(xlsxBytes)
		return
	}

	pdfBytes, err := db.ConvertXLSXToPDF(r.Context(), xlsxBytes)
	if err != nil {
		if errors.Is(err, db.ErrPDFConversionUnavailable) {
			writeError(w, http.StatusServiceUnavailable, err.Error())
			return
		}
		writeInternalError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `attachment; filename="`+sanitizeContentDispositionFilename(filenameBase)+`.pdf"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(pdfBytes)
}
