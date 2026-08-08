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

// getDATEVExport streams a fiscal year's DATEV Buchungsstapel EXTF file.
// Missing consultant/client numbers, missing per-account DATEV numbers, a
// mixed Sachkontenlänge, an unresolvable multi-line manual entry with no
// clearing account configured, or a fiscal year belonging to a different
// organization all surface as db.ValidationError → 409, same as the FEC
// endpoint above.
func (h *handler) getDATEVExport(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	fiscalYearID := r.URL.Query().Get("fiscalYearId")
	if fiscalYearID == "" {
		writeError(w, http.StatusBadRequest, "fiscalYearId is required")
		return
	}

	content, filename, err := h.db.GenerateDATEV(orgID, fiscalYearID)
	if err != nil {
		if _, ok := errors.AsType[*db.ValidationError](err); ok {
			writeMutationError(w, err)
			return
		}
		writeDBError(w, err, "organization or fiscal year not found")
		return
	}

	// text/csv, not the app/octet-stream a semicolon-separated cp1252 file
	// might suggest — DATEV import tools and Excel both key off this to
	// treat it as delimited text rather than an opaque download.
	w.Header().Set("Content-Type", "text/csv; charset=windows-1252")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(content)
}
