package api

import "net/http"

func (h *handler) getTrialBalance(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	fiscalPeriodID := r.URL.Query().Get("fiscalPeriodId")
	rows, err := h.db.GetTrialBalance(orgID, fiscalPeriodID)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}
