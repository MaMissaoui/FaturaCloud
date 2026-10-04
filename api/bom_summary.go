package api

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/MaMissaoui/fatura-cloud/db"
)

// getBOMOverview is the Bill of Materials screen's list: every finished
// product's parts cost and buildable units. 409 when the organization has
// master data summaries switched off.
func (h *handler) getBOMOverview(w http.ResponseWriter, r *http.Request) {
	overview, err := h.db.GetBOMOverview(r.PathValue("orgId"))
	if err != nil {
		if verr, ok := err.(*db.ValidationError); ok {
			writeError(w, http.StatusConflict, verr.Error())
			return
		}
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, overview)
}

// getBOMRecipeDetail is the recipe panel: each component's stock, cost and
// the units it allows.
func (h *handler) getBOMRecipeDetail(w http.ResponseWriter, r *http.Request) {
	productID := r.PathValue("id")
	product, err := h.db.GetProduct(productID)
	if err != nil {
		writeDBError(w, err, "product not found")
		return
	}
	detail, err := h.db.GetBOMRecipeDetail(product.OrganizationID, productID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "product not found")
			return
		}
		if verr, ok := err.(*db.ValidationError); ok {
			writeError(w, http.StatusConflict, verr.Error())
			return
		}
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}
