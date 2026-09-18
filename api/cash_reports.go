package api

import (
	"net/http"
)

func (h *handler) getAccountBalance(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	accountID := r.URL.Query().Get("accountId")
	asOfDate := parseInt64Param(r, "asOfDate")
	balance, err := h.db.GetAccountBalance(orgID, accountID, asOfDate)
	if err != nil {
		writeMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{"balance": balance})
}

func (h *handler) getDailyCashMovements(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	accountID := r.URL.Query().Get("accountId")
	startDate := parseInt64Param(r, "startDate")
	endDate := parseInt64Param(r, "endDate")
	rows, err := h.db.GetDailyCashMovements(orgID, accountID, startDate, endDate)
	if err != nil {
		writeMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (h *handler) getCashMovementDetails(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	accountID := r.URL.Query().Get("accountId")
	startDate := parseInt64Param(r, "startDate")
	endDate := parseInt64Param(r, "endDate")
	rows, err := h.db.GetCashMovementDetails(orgID, accountID, startDate, endDate)
	if err != nil {
		writeMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (h *handler) getLoanStatus(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	clientID := r.URL.Query().Get("clientId")
	rows, err := h.db.GetLoanStatus(orgID, clientID)
	if err != nil {
		writeMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}
