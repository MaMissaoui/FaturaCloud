package api

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/MaMissaoui/fatura-cloud/db"
)

func (h *handler) getVendorSummaries(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	summaries, err := h.db.GetVendorSummaries(orgID)
	if err != nil {
		if verr, ok := err.(*db.ValidationError); ok {
			writeError(w, http.StatusConflict, verr.Error())
			return
		}
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, summaries)
}

func (h *handler) getVendorSummary(w http.ResponseWriter, r *http.Request) {
	vendorID := r.PathValue("id")
	vendor, err := h.db.GetVendor(vendorID)
	if err != nil {
		writeDBError(w, err, "vendor not found")
		return
	}
	summary, err := h.db.GetVendorSummary(vendor.OrganizationID, vendorID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "vendor not found")
			return
		}
		if verr, ok := err.(*db.ValidationError); ok {
			writeError(w, http.StatusConflict, verr.Error())
			return
		}
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, summary)
}
