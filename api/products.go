package api

import (
	"errors"
	"net/http"

	"github.com/MaMissaoui/fatura-cloud/db"
)

func (h *handler) listProducts(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	products, err := h.db.GetProducts(orgID)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, products)
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
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
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
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, product)
}

func (h *handler) updateProduct(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req db.UpdateProductRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
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
