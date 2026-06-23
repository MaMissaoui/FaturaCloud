package api

import (
	"net/http"

	"github.com/MaMissaoui/fatura-cloud/db"
)

func (h *handler) listStockMovements(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	movements, err := h.db.GetStockMovements(orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, movements)
}

func (h *handler) listProductStockMovements(w http.ResponseWriter, r *http.Request) {
	productID := r.PathValue("id")
	movements, err := h.db.GetProductStockMovements(productID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, movements)
}

func (h *handler) createStockMovement(w http.ResponseWriter, r *http.Request) {
	var req db.CreateStockMovementRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	movement, err := h.db.CreateStockMovement(req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, movement)
}

func (h *handler) deleteStockMovement(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ok, err := h.db.DeleteStockMovement(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": ok})
}
