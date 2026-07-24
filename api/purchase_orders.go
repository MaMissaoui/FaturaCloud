package api

import (
	"net/http"

	"github.com/MaMissaoui/fatura-cloud/db"
)

func (h *handler) listPurchaseOrders(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	orders, err := h.db.GetPurchaseOrders(orgID)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, orders)
}

func (h *handler) nextPurchaseOrderNumber(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	writeJSON(w, http.StatusOK, map[string]string{"number": h.db.NextPurchaseOrderNumber(orgID)})
}

func (h *handler) getPurchaseOrder(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	order, err := h.db.GetPurchaseOrder(id)
	if err != nil {
		writeDBError(w, err, "purchase order not found")
		return
	}
	writeJSON(w, http.StatusOK, order)
}

func (h *handler) getPurchaseOrderLineItems(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	items, err := h.db.GetPurchaseOrderLineItems(id)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *handler) createPurchaseOrder(w http.ResponseWriter, r *http.Request) {
	var req db.CreatePurchaseOrderRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	order, err := h.db.CreatePurchaseOrder(req)
	if err != nil {
		writeMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, order)
}

func (h *handler) updatePurchaseOrder(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req db.UpdatePurchaseOrderRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	order, err := h.db.UpdatePurchaseOrder(id, req)
	if err != nil {
		writeMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, order)
}

func (h *handler) updatePurchaseOrderStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Status string `json:"status"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return
	}
	order, err := h.db.UpdatePurchaseOrderStatus(id, body.Status)
	if err != nil {
		writeMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, order)
}

func (h *handler) deletePurchaseOrder(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ok, err := h.db.DeletePurchaseOrder(id)
	if err != nil {
		writeMutationError(w, err)
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "purchase order not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}
