package api

import (
	"net/http"
	"time"

	"github.com/MaMissaoui/fatura-cloud/db"
)

func (h *handler) listPayments(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	payments, err := h.db.GetPayments(orgID)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payments)
}

func (h *handler) getPayment(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	payment, err := h.db.GetPayment(id)
	if err != nil {
		writeDBError(w, err, "payment not found")
		return
	}
	writeJSON(w, http.StatusOK, payment)
}

func (h *handler) getPaymentApplications(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	applications, err := h.db.GetPaymentApplications(id)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, applications)
}

func (h *handler) createPayment(w http.ResponseWriter, r *http.Request) {
	var req db.CreatePaymentRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	payment, err := h.db.CreatePayment(req)
	if err != nil {
		writeMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, payment)
}

func (h *handler) voidPayment(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	payment, err := h.db.VoidPayment(id, time.Now().UnixMilli())
	if err != nil {
		writeMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, payment)
}

func (h *handler) getInvoicePayments(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	applications, err := h.db.GetDocumentPaymentApplications("invoice", id)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, applications)
}

func (h *handler) getIncomingInvoicePayments(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	applications, err := h.db.GetDocumentPaymentApplications("incoming_invoice", id)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, applications)
}
