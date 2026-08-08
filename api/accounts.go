package api

import (
	"errors"
	"net/http"

	"github.com/MaMissaoui/fatura-cloud/db"
)

func (h *handler) listAccounts(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	accounts, err := h.db.GetAccounts(orgID)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, accounts)
}

func (h *handler) getAccount(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	account, err := h.db.GetAccount(id)
	if err != nil {
		writeDBError(w, err, "account not found")
		return
	}
	writeJSON(w, http.StatusOK, account)
}

func (h *handler) createAccount(w http.ResponseWriter, r *http.Request) {
	var req db.CreateAccountRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	account, err := h.db.CreateAccount(req)
	if err != nil {
		writeMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, account)
}

func (h *handler) updateAccount(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req db.UpdateAccountRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	account, err := h.db.UpdateAccount(id, req)
	if err != nil {
		writeMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, account)
}

func (h *handler) deleteAccount(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ok, err := h.db.DeleteAccount(id)
	if err != nil {
		if errors.Is(err, db.ErrAccountHasChildren) || errors.Is(err, db.ErrAccountInUse) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": ok})
}
