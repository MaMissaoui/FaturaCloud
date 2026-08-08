package api

import (
	"errors"
	"net/http"

	"github.com/MaMissaoui/fatura-cloud/db"
)

func (h *handler) listJournals(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	journals, err := h.db.GetJournals(orgID)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, journals)
}

func (h *handler) createJournal(w http.ResponseWriter, r *http.Request) {
	var req db.CreateJournalRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	journal, err := h.db.CreateJournal(req)
	if err != nil {
		writeMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, journal)
}

func (h *handler) updateJournal(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req db.UpdateJournalRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	journal, err := h.db.UpdateJournal(id, req)
	if err != nil {
		writeMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, journal)
}

func (h *handler) deleteJournal(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ok, err := h.db.DeleteJournal(id)
	if err != nil {
		if errors.Is(err, db.ErrJournalInUse) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": ok})
}
