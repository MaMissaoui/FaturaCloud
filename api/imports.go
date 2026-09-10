package api

import (
	"errors"
	"net/http"

	"github.com/MaMissaoui/fatura-cloud/db"
)

func (h *handler) listImports(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	imports, err := h.db.GetImports(orgID)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, imports)
}

func (h *handler) listImportSummaries(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	summaries, err := h.db.GetImportSummaries(orgID)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, summaries)
}

func (h *handler) nextImportNumber(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	writeJSON(w, http.StatusOK, map[string]string{"number": h.db.NextImportNumber(orgID)})
}

func (h *handler) getImport(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	imp, err := h.db.GetImport(id)
	if err != nil {
		writeDBError(w, err, "import not found")
		return
	}
	writeJSON(w, http.StatusOK, imp)
}

func (h *handler) getImportSummary(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	summary, err := h.db.GetImportSummary(id)
	if err != nil {
		writeDBError(w, err, "import not found")
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

func (h *handler) createImport(w http.ResponseWriter, r *http.Request) {
	var req db.CreateImportRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	if !h.requireOrgMember(w, r, req.OrganizationID) {
		return
	}
	imp, err := h.db.CreateImport(req)
	if err != nil {
		writeMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, imp)
}

func (h *handler) updateImport(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req db.UpdateImportRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	imp, err := h.db.UpdateImport(id, req)
	if err != nil {
		writeMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, imp)
}

func (h *handler) deleteImport(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ok, err := h.db.DeleteImport(id)
	if err != nil {
		if errors.Is(err, db.ErrImportInUse) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": ok})
}
