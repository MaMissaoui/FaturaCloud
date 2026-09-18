package api

import (
	"net/http"

	"github.com/MaMissaoui/fatura-cloud/db"
)

func (h *handler) createCashMovement(w http.ResponseWriter, r *http.Request) {
	var req db.CreateCashMovementRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	if !h.requireOrgRole(w, r, req.OrganizationID, "cashbook") {
		return
	}
	result, err := h.db.CreateCashMovement(req)
	if err != nil {
		writeMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}
