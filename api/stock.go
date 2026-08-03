package api

import (
	"net/http"

	"github.com/MaMissaoui/fatura-cloud/db"
)

func (h *handler) listStockMovements(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	opts := db.StockMovementListOptions{
		ProductID: r.URL.Query().Get("productId"),
		Limit:     parseIntParam(r, "limit"),
		Offset:    parseIntParam(r, "offset"),
		SortField: r.URL.Query().Get("sort"),
		SortDesc:  r.URL.Query().Get("order") == "desc",
	}
	movements, total, err := h.db.GetStockMovements(orgID, opts)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": movements, "total": total})
}

func (h *handler) listProductStockMovements(w http.ResponseWriter, r *http.Request) {
	productID := r.PathValue("id")
	movements, err := h.db.GetProductStockMovements(productID)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, movements)
}

func (h *handler) createStockMovement(w http.ResponseWriter, r *http.Request) {
	var req db.CreateStockMovementRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	result, err := h.db.CreateStockMovement(req)
	if err != nil {
		writeMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (h *handler) listProductSerialNumbers(w http.ResponseWriter, r *http.Request) {
	productID := r.PathValue("id")
	rows, err := h.db.GetProductSerialNumbers(productID)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (h *handler) deleteStockMovement(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ok, err := h.db.DeleteStockMovement(id)
	if err != nil {
		writeMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": ok})
}
