package api

import (
	"errors"
	"net/http"

	"github.com/MaMissaoui/fatura-cloud/db"
)

func (h *handler) listTaxRates(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	rates, err := h.db.GetTaxRates(orgID)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rates)
}

func (h *handler) getTaxRate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	rate, err := h.db.GetTaxRate(id)
	if err != nil {
		writeDBError(w, err, "tax rate not found")
		return
	}
	writeJSON(w, http.StatusOK, rate)
}

func (h *handler) createTaxRate(w http.ResponseWriter, r *http.Request) {
	var req db.CreateTaxRateRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	rate, err := h.db.CreateTaxRate(req)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, rate)
}

func (h *handler) updateTaxRate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req db.UpdateTaxRateRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	rate, err := h.db.UpdateTaxRate(id, req)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rate)
}

func (h *handler) deleteTaxRate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ok, err := h.db.DeleteTaxRate(id)
	if err != nil {
		if errors.Is(err, db.ErrTaxRateInUse) {
			writeError(w, http.StatusConflict, "This tax rate is used by one or more invoices and cannot be deleted")
			return
		}
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": ok})
}

func (h *handler) getTaxRateUsageCount(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	count, err := h.db.GetTaxRateUsageCount(id)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{"count": count})
}
