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
// Neither bound is left at that default here, unlike GetDashboardData's own
// endDate=0 calls: a Reporting page always has a range picked before it
// fetches, but nothing stops a direct API caller from omitting one or both,
// and GetTaxSummary/GetRevenueByMonth's per-document subqueries have no
// LIMIT — an omitted startDate would otherwise aggregate the organization's
// entire history on every call (audit finding F42). Both defaults live at
// the API boundary, not the db layer, so GetDashboardData's own unbounded
// calls stay untouched.
func reportingEndDate(r *http.Request) int64 {
	endDate := parseInt64Param(r, "endDate")
	if endDate == 0 {
		endDate = time.Now().UnixMilli()
	}
	return endDate
}

// reportingDefaultLookback bounds an omitted startDate to 5 years before
// endDate — generous enough that no real report is ever truncated by it
// (every reporting page's own RangePicker defaults to 12 months and lets the
// user pick further back), while still capping the worst case to a fixed
// window instead of a full table scan.
const reportingDefaultLookback = 5

func reportingStartDate(r *http.Request, endDate int64) int64 {
	startDate := parseInt64Param(r, "startDate")
	if startDate == 0 {
		startDate = time.UnixMilli(endDate).AddDate(-reportingDefaultLookback, 0, 0).UnixMilli()
	}
	return startDate
}

func (h *handler) getRevenueTrend(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	endDate := reportingEndDate(r)
	rows, err := h.db.GetRevenueByMonth(orgID, reportingStartDate(r, endDate), endDate)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (h *handler) getSalesByClient(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	endDate := reportingEndDate(r)
	rows, err := h.db.GetSalesByClient(orgID, reportingStartDate(r, endDate), endDate, 0)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (h *handler) getSalesByProduct(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	endDate := reportingEndDate(r)
	rows, err := h.db.GetSalesByProduct(orgID, reportingStartDate(r, endDate), endDate, 0)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (h *handler) getPurchasesByVendor(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	endDate := reportingEndDate(r)
	rows, err := h.db.GetPurchasesByVendor(orgID, reportingStartDate(r, endDate), endDate, 0)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (h *handler) getTaxSummary(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	endDate := reportingEndDate(r)
	report, err := h.db.GetTaxSummary(orgID, reportingStartDate(r, endDate), endDate)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}
