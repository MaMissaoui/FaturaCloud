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
		var err error
		startDate, endDate, err = h.db.DashboardYearRange(orgID, year)
		if err != nil {
			writeInternalError(w, err)
			return
		}
	} else {
		months := parseIntParam(r, "months")
		if months <= 0 {
			months = 12
		}
		startDate, endDate = db.DashboardCutoff(months), 0
	}

	// The till is Cash Book data, which the section guard keeps from the
	// roles without the Cash Book (sales, purchasing, accounting), so it is
	// only filled in for a role that may use it. The rest of the payload is
	// shared and filtered on screen (audit F147).
	// The follow-up list reads the whole loan and payment history, so it is
	// only built for a role that sees what clients owe (Sales or Accounting,
	// dashboardWidgetsForRole's receivables).
	var opts db.DashboardOptions
	if claims := getClaims(r); claims != nil {
		role, _, err := h.db.GetOrganizationRole(orgID, claims.UserID)
		if err != nil {
			writeInternalError(w, err)
			return
		}
		opts.CashRegister = roleCanUseSection(role, sectionCashbook)
		opts.LoanFollowUp = roleCanUseSection(role, sectionSales) || roleCanUseSection(role, sectionAccounting)
	}
	data, err := h.db.GetDashboardData(orgID, startDate, endDate, opts)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, data)
}
