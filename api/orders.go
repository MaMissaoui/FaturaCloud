package api

import (
	"encoding/json"
	"net/http"

	"github.com/MaMissaoui/fatura-cloud/db"
)

func (h *handler) listOrders(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	orders, err := h.db.GetOrders(orgID)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, orders)
}

func (h *handler) getOrder(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	order, err := h.db.GetOrder(id)
	if err != nil {
		writeDBError(w, err, "order not found")
		return
	}
	writeJSON(w, http.StatusOK, order)
}

func (h *handler) getOrderLineItems(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	items, err := h.db.GetOrderLineItems(id)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *handler) getOrderDeliveredQuantities(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	quantities, err := h.db.GetOrderDeliveredQuantities(id)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, quantities)
}

func (h *handler) createOrder(w http.ResponseWriter, r *http.Request) {
	var req db.CreateOrderRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	order, err := h.db.CreateOrder(req)
	if err != nil {
		writeMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, order)
}

func (h *handler) updateOrder(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req db.UpdateOrderRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	order, err := h.db.UpdateOrder(id, req)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, order)
}

func (h *handler) updateOrderStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	order, err := h.db.UpdateOrderStatus(id, body.Status)
	if err != nil {
		writeMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, order)
}

func (h *handler) deleteOrder(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ok, err := h.db.DeleteOrder(id)
	if err != nil {
		writeMutationError(w, err)
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "order not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}
