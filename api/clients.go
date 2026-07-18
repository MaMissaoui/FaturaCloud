package api

import (
	"net/http"

	"github.com/MaMissaoui/fatura-cloud/db"
)

func (h *handler) listClients(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	clients, err := h.db.GetClients(orgID)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, clients)
}

func (h *handler) getClient(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	client, err := h.db.GetClient(id)
	if err != nil {
		writeDBError(w, err, "client not found")
		return
	}
	writeJSON(w, http.StatusOK, client)
}

func (h *handler) createClient(w http.ResponseWriter, r *http.Request) {
	var req db.CreateClientRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	client, err := h.db.CreateClient(req)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, client)
}

func (h *handler) updateClient(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req db.UpdateClientRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	client, err := h.db.UpdateClient(id, req)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, client)
}

func (h *handler) deleteClient(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ok, err := h.db.DeleteClient(id)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": ok})
}

func (h *handler) getClientInvoiceCount(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	count, err := h.db.GetClientInvoiceCount(id)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{"count": count})
}
