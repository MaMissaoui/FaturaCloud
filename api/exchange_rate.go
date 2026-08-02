package api

import "net/http"

// getLastExchangeRate is a prefill convenience for document forms picking a
// non-organization currency: it returns whatever rate was last saved for
// that currency in this organization, or {"rate": null} if none exists yet.
// It never invents a rate and is never consulted server-side when saving —
// the user always confirms it (see db/exchange_rate.go).
func (h *handler) getLastExchangeRate(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	currency := r.URL.Query().Get("currency")
	if currency == "" {
		writeError(w, http.StatusBadRequest, "currency query parameter is required")
		return
	}
	rate, date, err := h.db.GetLastExchangeRate(orgID, currency)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Rate *float64 `json:"rate"`
		Date *int64   `json:"date"`
	}{Rate: rate, Date: date})
}
