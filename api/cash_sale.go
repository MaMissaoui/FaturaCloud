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

// createCashSalePayment settles one line of a loan sale from the Cash Book
// (db.CreateCashSalePayment). Gated by orgRoleProtected on the invoice's own
// organization with the cashbook role, and by the section guard's cashbook
// section — the cashbook role's counterpart to POST /api/payments, which
// stays accounting-tier (audit F104).
func (h *handler) createCashSalePayment(w http.ResponseWriter, r *http.Request) {
	var req db.CreateCashSalePaymentRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	req.InvoiceID = r.PathValue("id")
	result, err := h.db.CreateCashSalePayment(req)
	if err != nil {
		writeMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}
