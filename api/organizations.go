package api

import (
	"io"
	"net/http"
	"strings"

	"github.com/MaMissaoui/fatura-cloud/db"
)

func (h *handler) listOrganizations(w http.ResponseWriter, r *http.Request) {
	orgs, err := h.db.GetOrganizations()
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, orgs)
}

func (h *handler) getOrganization(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	org, err := h.db.GetOrganization(id)
	if err != nil {
		writeDBError(w, err, "organization not found")
		return
	}
	writeJSON(w, http.StatusOK, org)
}

func (h *handler) createOrganization(w http.ResponseWriter, r *http.Request) {
	var req db.CreateOrganizationRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	org, err := h.db.CreateOrganization(req)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, org)
}

func (h *handler) updateOrganization(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req db.UpdateOrganizationRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	org, err := h.db.UpdateOrganization(id, req)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, org)
}

func (h *handler) deleteOrganization(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ok, err := h.db.DeleteOrganization(id)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": ok})
}

// getOrganizationLogo serves the raw logo bytes with a sniffed Content-Type —
// the logo isn't part of the JSON organization payload (excluded via
// json:"-" on db.Organization), so this is the only way to read it. A
// missing organization and an organization with no logo both resolve to a
// plain 404, matching how <img>/react-pdf failure handling expects it.
func (h *handler) getOrganizationLogo(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	logo, err := h.db.GetOrganizationLogo(id)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	if len(logo) == 0 {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", http.DetectContentType(logo))
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.Write(logo)
}

// uploadOrganizationLogo replaces an organization's logo from a multipart
// upload. ParseMultipartForm's argument is only the in-memory threshold, not
// a request size cap — the body is bounded separately so an oversized upload
// is rejected up front rather than buffered to disk.
func (h *handler) uploadOrganizationLogo(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	r.Body = http.MaxBytesReader(w, r.Body, 4<<20)
	if err := r.ParseMultipartForm(2 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "file too large (max 2 MB)")
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file field is required")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	if !strings.HasPrefix(http.DetectContentType(data), "image/") {
		writeError(w, http.StatusBadRequest, "file must be an image (PNG, JPEG, GIF, WebP)")
		return
	}
	ok, err := h.db.SetOrganizationLogo(id, data)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "organization not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) deleteOrganizationLogo(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ok, err := h.db.SetOrganizationLogo(id, nil)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "organization not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) getOrganizationUsageCount(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	counts, err := h.db.GetOrganizationUsageCount(id)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, counts)
}
