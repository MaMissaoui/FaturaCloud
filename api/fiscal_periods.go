package api

import (
	"net/http"

	"github.com/MaMissaoui/fatura-cloud/db"
)

func (h *handler) listFiscalYears(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	years, err := h.db.GetFiscalYears(orgID)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, years)
}

func (h *handler) createFiscalYear(w http.ResponseWriter, r *http.Request) {
	var req db.CreateFiscalYearRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	year, err := h.db.CreateFiscalYear(req)
	if err != nil {
		writeMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, year)
}

func (h *handler) listFiscalPeriods(w http.ResponseWriter, r *http.Request) {
	fiscalYearID := r.PathValue("id")
	periods, err := h.db.GetFiscalPeriods(fiscalYearID)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, periods)
}

func (h *handler) createFiscalPeriod(w http.ResponseWriter, r *http.Request) {
	var req db.CreateFiscalPeriodRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	period, err := h.db.CreateFiscalPeriod(req)
	if err != nil {
		writeMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, period)
}

func (h *handler) updateFiscalPeriodStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		Status string `json:"status"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	period, err := h.db.UpdateFiscalPeriodStatus(id, req.Status)
	if err != nil {
		writeMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, period)
}

// closeFiscalYear is irreversible — there is no ReopenFiscalYear — so it's
// admin-only, the same class as backup/GL export/organization reset, unlike
// its protected-but-not-admin siblings above (creating a year, toggling a
// single period) which are easy to undo.
func (h *handler) closeFiscalYear(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	year, err := h.db.CloseFiscalYear(id)
	if err != nil {
		writeMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, year)
}
