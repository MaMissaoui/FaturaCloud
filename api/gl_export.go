package api

import (
	"errors"
	"net/http"

	"github.com/MaMissaoui/fatura-cloud/db"
)

// getFECExport streams a fiscal year's France FEC flat file. Missing SIREN,
// an invalid one, or a fiscal year that belongs to a different organization
// all surface as db.ValidationError → 409 via writeMutationError, same as
// the e-invoice endpoint's missing-field handling.
func (h *handler) getFECExport(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	fiscalYearID := r.URL.Query().Get("fiscalYearId")
	if fiscalYearID == "" {
		writeError(w, http.StatusBadRequest, "fiscalYearId is required")
		return
	}

	content, filename, err := h.db.GenerateFEC(orgID, fiscalYearID)
	if err != nil {
		if _, ok := errors.AsType[*db.ValidationError](err); ok {
			writeMutationError(w, err)
			return
		}
		writeDBError(w, err, "organization or fiscal year not found")
		return
	}

	w.Header().Set("Content-Type", "text/tab-separated-values; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(content)
}
