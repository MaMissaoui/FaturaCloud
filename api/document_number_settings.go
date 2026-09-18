package api

import (
	"net/http"

	"github.com/MaMissaoui/fatura-cloud/db"
)

// getDocumentNumberSetting returns the org's configured numbering
// format/counter for documentType, or the type's default with a zero
// counter if the organization never saved one — see
// db.GetDocumentNumberSetting.
func (h *handler) getDocumentNumberSetting(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	documentType := r.PathValue("documentType")
	if !db.IsKnownDocumentNumberType(documentType) {
		writeError(w, http.StatusBadRequest, "unknown document type")
		return
	}

	setting, err := h.db.GetDocumentNumberSetting(orgID, documentType)
	if err != nil {
		writeMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, setting)
}

// updateDocumentNumberSetting validates and upserts the org's numbering
// format/counter for documentType.
func (h *handler) updateDocumentNumberSetting(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	documentType := r.PathValue("documentType")
	if !db.IsKnownDocumentNumberType(documentType) {
		writeError(w, http.StatusBadRequest, "unknown document type")
		return
	}

	var req db.UpdateDocumentNumberSettingRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}

	setting, err := h.db.UpdateDocumentNumberSetting(orgID, documentType, req)
	if err != nil {
		writeMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, setting)
}
