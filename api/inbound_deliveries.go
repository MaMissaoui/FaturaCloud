package api

import (
	"net/http"

	"github.com/MaMissaoui/fatura-cloud/db"
)

func (h *handler) listInboundDeliveries(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	rows, err := h.db.GetInboundDeliveries(orgID)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (h *handler) nextInboundDeliveryNumber(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	writeJSON(w, http.StatusOK, map[string]string{"number": h.db.NextInboundDeliveryNumber(orgID)})
}

func (h *handler) getInboundDelivery(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	row, err := h.db.GetInboundDelivery(id)
	if err != nil {
		writeDBError(w, err, "goods receipt not found")
		return
	}
	writeJSON(w, http.StatusOK, row)
}

func (h *handler) getInboundDeliveryLineItems(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	items, err := h.db.GetInboundDeliveryLineItems(id)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *handler) createInboundDelivery(w http.ResponseWriter, r *http.Request) {
	var req db.CreateInboundDeliveryRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	row, err := h.db.CreateInboundDelivery(req)
	if err != nil {
		writeMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, row)
}

func (h *handler) updateInboundDelivery(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req db.UpdateInboundDeliveryRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	row, err := h.db.UpdateInboundDelivery(id, req)
	if err != nil {
		writeMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, row)
}

func (h *handler) updateInboundDeliveryStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Status string `json:"status"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return
	}
	row, err := h.db.UpdateInboundDeliveryStatus(id, body.Status)
	if err != nil {
		writeMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, row)
}

func (h *handler) deleteInboundDelivery(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ok, err := h.db.DeleteInboundDelivery(id)
	if err != nil {
		writeMutationError(w, err)
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "goods receipt not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

func (h *handler) getPurchaseOrderReceivedQuantities(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	quantities, err := h.db.GetPurchaseOrderReceivedQuantities(id)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, quantities)
}
