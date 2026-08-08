package api

import (
	"net/http"

	"github.com/MaMissaoui/fatura-cloud/db"
)

func (h *handler) listJournalEntries(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	journalID := r.URL.Query().Get("journalId")
	status := r.URL.Query().Get("status")
	entries, err := h.db.GetJournalEntries(orgID, journalID, status)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

func (h *handler) getJournalEntry(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	entry, err := h.db.GetJournalEntry(id)
	if err != nil {
		writeDBError(w, err, "journal entry not found")
		return
	}
	writeJSON(w, http.StatusOK, entry)
}

func (h *handler) getJournalEntryLines(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	lines, err := h.db.GetJournalEntryLines(id)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, lines)
}

func (h *handler) createJournalEntry(w http.ResponseWriter, r *http.Request) {
	var req db.CreateJournalEntryRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	entry, err := h.db.CreateJournalEntry(req)
	if err != nil {
		writeMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, entry)
}

func (h *handler) postJournalEntry(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	entry, err := h.db.PostJournalEntry(id)
	if err != nil {
		writeMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, entry)
}

func (h *handler) reverseJournalEntry(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		Reason string `json:"reason"`
		Date   int64  `json:"date"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	reversal, err := h.db.ReverseJournalEntry(id, req.Reason, req.Date)
	if err != nil {
		writeMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, reversal)
}

func (h *handler) deleteJournalEntry(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ok, err := h.db.DeleteJournalEntry(id)
	if err != nil {
		writeMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": ok})
}
