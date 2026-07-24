package api

import (
	"errors"
	"net/http"

	"github.com/MaMissaoui/fatura-cloud/db"
)

func (h *handler) listVendors(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	vendors, err := h.db.GetVendors(orgID)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, vendors)
}

func (h *handler) getVendor(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	vendor, err := h.db.GetVendor(id)
	if err != nil {
		writeDBError(w, err, "vendor not found")
		return
	}
	writeJSON(w, http.StatusOK, vendor)
}

func (h *handler) createVendor(w http.ResponseWriter, r *http.Request) {
	var req db.CreateVendorRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	vendor, err := h.db.CreateVendor(req)
	if err != nil {
		writeMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, vendor)
}

func (h *handler) updateVendor(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req db.UpdateVendorRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	vendor, err := h.db.UpdateVendor(id, req)
	if err != nil {
		writeMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, vendor)
}

func (h *handler) deleteVendor(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ok, err := h.db.DeleteVendor(id)
	if err != nil {
		if errors.Is(err, db.ErrVendorInUse) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": ok})
}

func (h *handler) getVendorDocumentCount(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	count, err := h.db.GetVendorDocumentCount(id)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{"count": count})
}
