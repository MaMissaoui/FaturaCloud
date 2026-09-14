package api

import (
	"net/http"

	"github.com/MaMissaoui/fatura-cloud/db"
)

func (h *handler) listUnitsOfMeasure(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	units, err := h.db.GetUnitsOfMeasure(orgID)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, units)
}

func (h *handler) createUnitOfMeasure(w http.ResponseWriter, r *http.Request) {
	var req db.CreateUnitOfMeasureRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	if !h.requireOrgMember(w, r, req.OrganizationID) {
		return
	}
	unit, err := h.db.CreateUnitOfMeasure(req)
	if err != nil {
		writeMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, unit)
}

func (h *handler) updateUnitOfMeasure(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req db.UpdateUnitOfMeasureRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	unit, err := h.db.UpdateUnitOfMeasure(id, req)
	if err != nil {
		writeMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, unit)
}

func (h *handler) deleteUnitOfMeasure(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ok, err := h.db.DeleteUnitOfMeasure(id)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": ok})
}
