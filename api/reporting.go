package api

import (
	"net/http"
	"time"
)

// Reporting handlers back the sales/purchasing analytics tier — document-
// derived, distinct from api/reports.go's GL-derived statutory reports (see
// db/sales_reports.go's top-of-file comment for the full rationale).
//
// startDate/endDate come from parseInt64Param, which returns 0 if absent —
// the db layer treats 0 as "unbounded on that side" (see dateRangeFilter).
// endDate is the one exception: an absent endDate here defaults to "now"
// rather than staying unbounded, since a Reporting page with no range
// picked yet should show data up to today, not into the future. This
// default lives at the API boundary, not the db layer, so
// GetDashboardData's existing endDate=0 contract stays untouched.
func reportingEndDate(r *http.Request) int64 {
	endDate := parseInt64Param(r, "endDate")
	if endDate == 0 {
		endDate = time.Now().UnixMilli()
	}
	return endDate
}

func (h *handler) getRevenueTrend(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	startDate := parseInt64Param(r, "startDate")
	rows, err := h.db.GetRevenueByMonth(orgID, startDate, reportingEndDate(r))
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (h *handler) getSalesByClient(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	startDate := parseInt64Param(r, "startDate")
	rows, err := h.db.GetSalesByClient(orgID, startDate, reportingEndDate(r), 0)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (h *handler) getSalesByProduct(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	startDate := parseInt64Param(r, "startDate")
	rows, err := h.db.GetSalesByProduct(orgID, startDate, reportingEndDate(r), 0)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (h *handler) getPurchasesByVendor(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	startDate := parseInt64Param(r, "startDate")
	rows, err := h.db.GetPurchasesByVendor(orgID, startDate, reportingEndDate(r), 0)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (h *handler) getTaxSummary(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	startDate := parseInt64Param(r, "startDate")
	report, err := h.db.GetTaxSummary(orgID, startDate, reportingEndDate(r))
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}
