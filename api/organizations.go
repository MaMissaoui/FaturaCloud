package api

import (
	"net/http"

	"github.com/MaMissaoui/fatura-cloud/db"
)

func (h *handler) listOrganizations(w http.ResponseWriter, r *http.Request) {
	orgs, err := h.db.GetOrganizations()
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, orgs)
}

func (h *handler) getOrganization(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	org, err := h.db.GetOrganization(id)
	if err != nil {
		writeDBError(w, err, "organization not found")
		return
	}
	writeJSON(w, http.StatusOK, org)
}

func (h *handler) createOrganization(w http.ResponseWriter, r *http.Request) {
	var req db.CreateOrganizationRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	org, err := h.db.CreateOrganization(req)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, org)
}

func (h *handler) updateOrganization(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req db.UpdateOrganizationRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	org, err := h.db.UpdateOrganization(id, req)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, org)
}

func (h *handler) deleteOrganization(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ok, err := h.db.DeleteOrganization(id)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": ok})
}

func (h *handler) getOrganizationUsageCount(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	counts, err := h.db.GetOrganizationUsageCount(id)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, counts)
}
