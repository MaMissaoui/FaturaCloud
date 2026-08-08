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

func (h *handler) getProfitAndLoss(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	startDate := parseInt64Param(r, "startDate")
	endDate := parseInt64Param(r, "endDate")
	report, err := h.db.GetProfitAndLoss(orgID, startDate, endDate)
	if err != nil {
		writeMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (h *handler) getBalanceSheet(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	asOfDate := parseInt64Param(r, "asOfDate")
	report, err := h.db.GetBalanceSheet(orgID, asOfDate)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (h *handler) getReceivableAging(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	summary, err := h.db.GetReceivableAging(orgID)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

func (h *handler) getPayableAging(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	summary, err := h.db.GetPayableAging(orgID)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, summary)
}
