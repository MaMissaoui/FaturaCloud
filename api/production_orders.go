package api

import (
	"net/http"

	"github.com/MaMissaoui/fatura-cloud/db"
)

func (h *handler) listProductionOrders(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	rows, err := h.db.GetProductionOrders(orgID)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (h *handler) nextProductionOrderNumber(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	writeJSON(w, http.StatusOK, map[string]string{"number": h.db.NextProductionOrderNumber(orgID)})
}

func (h *handler) getProductionOrder(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	row, err := h.db.GetProductionOrder(id)
	if err != nil {
		writeDBError(w, err, "production order not found")
		return
	}
	writeJSON(w, http.StatusOK, row)
}

func (h *handler) getProductionOrderComponentLines(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	lines, err := h.db.GetProductionOrderComponentLines(id)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, lines)
}

func (h *handler) createProductionOrder(w http.ResponseWriter, r *http.Request) {
	var req db.CreateProductionOrderRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	if !h.requireOrgMember(w, r, req.OrganizationID) {
		return
	}
	row, err := h.db.CreateProductionOrder(req)
	if err != nil {
		writeMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, row)
}

func (h *handler) updateProductionOrderStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Status string `json:"status"`
		// SerialNumbers is required (matching quantity) when the finished
		// product is serialized and this transitions draft->completed;
		// ignored otherwise.
		SerialNumbers []string `json:"serialNumbers"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return
	}
	row, err := h.db.UpdateProductionOrderStatus(id, body.Status, body.SerialNumbers)
	if err != nil {
		writeMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, row)
}

func (h *handler) deleteProductionOrder(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ok, err := h.db.DeleteProductionOrder(id)
	if err != nil {
		writeMutationError(w, err)
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "production order not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}
