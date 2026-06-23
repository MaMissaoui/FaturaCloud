package api

import (
	"net/http"

	"github.com/MaMissaoui/fatura-cloud/db"
)

func (h *handler) listClients(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	clients, err := h.db.GetClients(orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, clients)
}

func (h *handler) getClient(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	client, err := h.db.GetClient(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if client == nil {
		writeError(w, http.StatusNotFound, "client not found")
		return
	}
	writeJSON(w, http.StatusOK, client)
}

func (h *handler) createClient(w http.ResponseWriter, r *http.Request) {
	var req db.CreateClientRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	client, err := h.db.CreateClient(req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, client)
}

func (h *handler) updateClient(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req db.UpdateClientRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	client, err := h.db.UpdateClient(id, req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, client)
}

func (h *handler) deleteClient(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ok, err := h.db.DeleteClient(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": ok})
}

func (h *handler) getClientInvoiceCount(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	count, err := h.db.GetClientInvoiceCount(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{"count": count})
}
