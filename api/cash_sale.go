package api

import (
	"net/http"

	"github.com/MaMissaoui/fatura-cloud/db"
)

func (h *handler) createCashSale(w http.ResponseWriter, r *http.Request) {
	var req db.CreateCashSaleRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	if !h.requireOrgRole(w, r, req.OrganizationID, "cashbook") {
		return
	}
	result, err := h.db.CreateCashSale(req)
	if err != nil {
		writeMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (h *handler) getClientOpenInvoices(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	invoices, err := h.db.GetClientOpenInvoices(id)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, invoices)
}
