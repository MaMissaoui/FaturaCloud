package api

import (
	"encoding/json"
	"net/http"

	"github.com/MaMissaoui/fatura-cloud/db"
)

func (h *handler) listInvoices(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	invoices, err := h.db.GetInvoices(orgID)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, invoices)
}

func (h *handler) getInvoice(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	invoice, err := h.db.GetInvoice(id)
	if err != nil {
		writeDBError(w, err, "invoice not found")
		return
	}
	writeJSON(w, http.StatusOK, invoice)
}

func (h *handler) getInvoiceLineItems(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	items, err := h.db.GetInvoiceLineItems(id)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *handler) createInvoice(w http.ResponseWriter, r *http.Request) {
	var req db.CreateInvoiceRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	invoice, err := h.db.CreateInvoice(req)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, invoice)
}

func (h *handler) updateInvoice(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req db.UpdateInvoiceRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	invoice, err := h.db.UpdateInvoice(id, req)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, invoice)
}

func (h *handler) updateInvoiceState(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		State string `json:"state"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	invoice, err := h.db.UpdateInvoiceState(id, body.State)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, invoice)
}

func (h *handler) deleteInvoice(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ok, err := h.db.DeleteInvoice(id)
	if err != nil {
		writeMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": ok})
}
