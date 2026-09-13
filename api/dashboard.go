package api

import (
	"net/http"

	"github.com/MaMissaoui/fatura-cloud/db"
)

func (h *handler) getDashboard(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")

	var startDate, endDate int64
	if year := parseIntParam(r, "year"); year > 0 {
		// A calendar year (?year=2026) — "Current year"/"Last year"/an
		// explicit past year in the Dashboard's period picker — takes
		// priority over ?months when both are somehow present, since a
		// year selection is the more specific/explicit request.
		startDate, endDate = db.DashboardYearRange(year)
	} else {
		months := parseIntParam(r, "months")
		if months <= 0 {
			months = 12
		}
		startDate, endDate = db.DashboardCutoff(months), 0
	}

	data, err := h.db.GetDashboardData(orgID, startDate, endDate)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, data)
}
