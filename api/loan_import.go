package api

import (
	"net/http"
	"strconv"
)

// The paper loan register import (db/loan_import.go). Org admin only: it
// creates customers and receivables in bulk, and undo deletes them.

func (h *handler) getLoanImportTemplate(w http.ResponseWriter, r *http.Request) {
	content, err := h.db.LoanImportTemplateXLSX()
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeMassDataXLSX(w, "loan-register", content)
}

func (h *handler) listLoanImports(w http.ResponseWriter, r *http.Request) {
	batches, err := h.db.GetLoanImportBatches(r.PathValue("orgId"))
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, batches)
}

// importLoanRegister takes a multipart upload: file, cutoverDate (the picked
// day as calendarDayMs) and dryRun ("false" imports; anything else, or
// absent, only reports). It always answers 200 with the report — a file with
// errors is a report to act on, not a failed request.
func (h *handler) importLoanRegister(w http.ResponseWriter, r *http.Request) {
	data, ok := readMassDataUpload(w, r)
	if !ok {
		return
	}
	cutoverDay, err := strconv.ParseInt(r.FormValue("cutoverDate"), 10, 64)
	if err != nil || cutoverDay <= 0 {
		writeError(w, http.StatusBadRequest, "cutoverDate is required")
		return
	}
	orgID := r.PathValue("orgId")
	if r.FormValue("dryRun") != "false" {
		report, err := h.db.DryRunLoanImport(orgID, cutoverDay, data)
		if err != nil {
			writeMutationError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, report)
		return
	}
	fileName := ""
	if files := r.MultipartForm.File["file"]; len(files) > 0 {
		fileName = files[0].Filename
	}
	report, err := h.db.ImportLoanRegister(orgID, getClaims(r).UserID, fileName, cutoverDay, data)
	if err != nil {
		writeMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (h *handler) undoLoanImport(w http.ResponseWriter, r *http.Request) {
	result, err := h.db.UndoLoanImportBatch(r.PathValue("id"))
	if err != nil {
		writeMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
