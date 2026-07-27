package api

import "net/http"

func (h *handler) getDashboard(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	months := parseIntParam(r, "months")
	if months <= 0 {
		months = 12
	}
	data, err := h.db.GetDashboardData(orgID, months)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, data)
}
