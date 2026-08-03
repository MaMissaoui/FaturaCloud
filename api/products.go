package api

import (
	"errors"
	"net/http"

	"github.com/MaMissaoui/fatura-cloud/db"
)

func (h *handler) listProducts(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	opts := db.ProductListOptions{
		Search:    r.URL.Query().Get("search"),
		Limit:     parseIntParam(r, "limit"),
		Offset:    parseIntParam(r, "offset"),
		SortField: r.URL.Query().Get("sort"),
		SortDesc:  r.URL.Query().Get("order") == "desc",
	}
	products, total, err := h.db.GetProducts(orgID, opts)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": products, "total": total})
}

func (h *handler) getProduct(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	product, err := h.db.GetProduct(id)
	if err != nil {
		writeDBError(w, err, "product not found")
		return
	}
	writeJSON(w, http.StatusOK, product)
}

func (h *handler) createProduct(w http.ResponseWriter, r *http.Request) {
	var req db.CreateProductRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	if req.SKU == nil || *req.SKU == "" {
		writeError(w, http.StatusBadRequest, "product code is required")
		return
	}
	product, err := h.db.CreateProduct(req)
	if err != nil {
		if errors.Is(err, db.ErrDuplicateSKU) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, product)
}

func (h *handler) updateProduct(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req db.UpdateProductRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	if req.SKU == nil || *req.SKU == "" {
		writeError(w, http.StatusBadRequest, "product code is required")
		return
	}
	product, err := h.db.UpdateProduct(id, req)
	if err != nil {
		if errors.Is(err, db.ErrDuplicateSKU) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, product)
}

func (h *handler) deleteProduct(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ok, err := h.db.DeleteProduct(id)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": ok})
}
