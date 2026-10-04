package api

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/MaMissaoui/fatura-cloud/db"
)

func (h *handler) getClientSummaries(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	summaries, err := h.db.GetClientSummaries(orgID)
	if err != nil {
		if verr, ok := err.(*db.ValidationError); ok {
			writeError(w, http.StatusConflict, verr.Error())
			return
		}
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, summaries)
}

func (h *handler) getClientSummary(w http.ResponseWriter, r *http.Request) {
	clientID := r.PathValue("id")
	client, err := h.db.GetClient(clientID)
	if err != nil {
		writeDBError(w, err, "client not found")
		return
	}
	summary, err := h.db.GetClientSummary(client.OrganizationID, clientID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "client not found")
			return
		}
		if verr, ok := err.(*db.ValidationError); ok {
			writeError(w, http.StatusConflict, verr.Error())
			return
		}
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, summary)
}
