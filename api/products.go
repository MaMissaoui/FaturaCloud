package api

import (
	"net/http"

	"github.com/MaMissaoui/fatura-cloud/db"
)

func (h *handler) listProducts(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	products, err := h.db.GetProducts(orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, products)
}

func (h *handler) getProduct(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	product, err := h.db.GetProduct(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if product == nil {
		writeError(w, http.StatusNotFound, "product not found")
		return
	}
	writeJSON(w, http.StatusOK, product)
}

func (h *handler) createProduct(w http.ResponseWriter, r *http.Request) {
	var req db.CreateProductRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	product, err := h.db.CreateProduct(req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, product)
}

func (h *handler) updateProduct(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req db.UpdateProductRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	product, err := h.db.UpdateProduct(id, req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, product)
}

func (h *handler) deleteProduct(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ok, err := h.db.DeleteProduct(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": ok})
}
