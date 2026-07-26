package api

import (
	"net/http"
	"regexp"
)

var countryCodePattern = regexp.MustCompile(`^[A-Z]{2}$`)

func (h *handler) listActiveCountries(w http.ResponseWriter, r *http.Request) {
	codes, err := h.db.GetActiveCountryCodes()
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, codes)
}

func (h *handler) setCountryActive(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	if !countryCodePattern.MatchString(code) {
		writeError(w, http.StatusBadRequest, "invalid country code")
		return
	}
	var req struct {
		Active bool `json:"active"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	if err := h.db.SetCountryActive(code, req.Active); err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": code, "active": req.Active})
}
