package api

import (
	"errors"
	"net/http"

	"github.com/MaMissaoui/fatura-cloud/db"
)

func (h *handler) listIncomingInvoices(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	rows, err := h.db.GetIncomingInvoices(orgID)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (h *handler) getIncomingInvoice(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	row, err := h.db.GetIncomingInvoice(id)
	if err != nil {
		writeDBError(w, err, "incoming invoice not found")
		return
	}
	writeJSON(w, http.StatusOK, row)
}

func (h *handler) getIncomingInvoiceLineItems(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	items, err := h.db.GetIncomingInvoiceLineItems(id)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

// getIncomingInvoiceMatch returns the 3-way comparison per line. It is computed
// on demand rather than stored, so it always reflects the current state of the
// linked order and receipts.
func (h *handler) getIncomingInvoiceMatch(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	lines, err := h.db.GetIncomingInvoiceMatch(id)
	if err != nil {
		writeMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, lines)
}

func (h *handler) createIncomingInvoice(w http.ResponseWriter, r *http.Request) {
	var req db.CreateIncomingInvoiceRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	row, err := h.db.CreateIncomingInvoice(req)
	if err != nil {
		if errors.Is(err, db.ErrDuplicateVendorInvoiceNumber) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, row)
}

func (h *handler) updateIncomingInvoice(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req db.UpdateIncomingInvoiceRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	row, err := h.db.UpdateIncomingInvoice(id, req)
	if err != nil {
		if errors.Is(err, db.ErrDuplicateVendorInvoiceNumber) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, row)
}

func (h *handler) updateIncomingInvoiceState(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		State string `json:"state"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return
	}
	row, err := h.db.UpdateIncomingInvoiceState(id, body.State)
	if err != nil {
		writeMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, row)
}

func (h *handler) deleteIncomingInvoice(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ok, err := h.db.DeleteIncomingInvoice(id)
	if err != nil {
		writeMutationError(w, err)
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "incoming invoice not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}
