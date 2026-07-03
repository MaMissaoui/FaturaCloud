package api

import (
	"net/http"

	"github.com/MaMissaoui/fatura-cloud/db"
)

func (h *handler) listDeliveries(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	rows, err := h.db.GetDeliveries(orgID)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (h *handler) getDelivery(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	row, err := h.db.GetDelivery(id)
	if err != nil {
		writeDBError(w, err, "delivery not found")
		return
	}
	writeJSON(w, http.StatusOK, row)
}

func (h *handler) getDeliveryLineItems(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	items, err := h.db.GetDeliveryLineItems(id)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *handler) createDelivery(w http.ResponseWriter, r *http.Request) {
	var req db.CreateDeliveryRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	row, err := h.db.CreateDelivery(req)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, row)
}

func (h *handler) updateDelivery(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req db.UpdateDeliveryRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	row, err := h.db.UpdateDelivery(id, req)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, row)
}

func (h *handler) updateDeliveryStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Status string `json:"status"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	row, err := h.db.UpdateDeliveryStatus(id, body.Status)
	if err != nil {
		writeMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, row)
}

func (h *handler) deleteDelivery(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ok, err := h.db.DeleteDelivery(id)
	if err != nil {
		writeMutationError(w, err)
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "delivery not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func (h *handler) nextDeliveryNumber(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	writeJSON(w, http.StatusOK, map[string]string{"number": h.db.NextDeliveryNumber(orgID)})
}
