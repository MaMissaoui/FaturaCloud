package api

import (
	"net/http"

	"github.com/MaMissaoui/fatura-cloud/db"
)

func (h *handler) listPaymentTerms(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	terms, err := h.db.GetPaymentTerms(orgID)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, terms)
}

func (h *handler) createPaymentTerm(w http.ResponseWriter, r *http.Request) {
	var req db.CreatePaymentTermRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	term, err := h.db.CreatePaymentTerm(req)
	if err != nil {
		writeMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, term)
}

func (h *handler) updatePaymentTerm(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req db.UpdatePaymentTermRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	term, err := h.db.UpdatePaymentTerm(id, req)
	if err != nil {
		writeMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, term)
}

func (h *handler) deletePaymentTerm(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ok, err := h.db.DeletePaymentTerm(id)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": ok})
}
