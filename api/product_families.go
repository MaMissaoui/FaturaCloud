package api

import (
	"net/http"

	"github.com/MaMissaoui/fatura-cloud/db"
)

func (h *handler) listProductFamilies(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	families, err := h.db.GetProductFamilies(orgID)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, families)
}

func (h *handler) createProductFamily(w http.ResponseWriter, r *http.Request) {
	var req db.CreateProductFamilyRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	if !h.requireOrgMember(w, r, req.OrganizationID) {
		return
	}
	family, err := h.db.CreateProductFamily(req)
	if err != nil {
		writeMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, family)
}

func (h *handler) updateProductFamily(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req db.UpdateProductFamilyRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	family, err := h.db.UpdateProductFamily(id, req)
	if err != nil {
		writeMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, family)
}

func (h *handler) deleteProductFamily(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ok, err := h.db.DeleteProductFamily(id)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": ok})
}
