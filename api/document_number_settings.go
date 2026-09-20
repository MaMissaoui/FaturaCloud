package api

import (
	"errors"
	"net/http"

	"github.com/MaMissaoui/fatura-cloud/db"
)

// writeDocumentNumberError surfaces a numbering lookup/update failure with
// the right status: a *db.ValidationError (unknown type, bad format,
// negative counter) is a user-safe 409 via writeMutationError, while any
// other error — a real query failure that db.GetDocumentNumberSetting/
// UpdateDocumentNumberSetting now let propagate instead of masking as a
// default — is a genuine internal failure and must be a 500, not a
// misleading "invalid request" 409.
func writeDocumentNumberError(w http.ResponseWriter, err error) {
	if _, ok := errors.AsType[*db.ValidationError](err); ok {
		writeMutationError(w, err)
		return
	}
	writeInternalError(w, err)
}

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
		writeDocumentNumberError(w, err)
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
		writeDocumentNumberError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, setting)
}
