package api

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/MaMissaoui/fatura-cloud/db"
)

func (h *handler) listOrganizationMembers(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	members, err := h.db.GetOrganizationUsers(orgID)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, members)
}

// getMyOrganizationRole reports the caller's own role in an organization, if
// any. Unlike listOrganizationMembers, this is not org-admin gated — any
// authenticated user may ask "am I an admin here" about themselves, which is
// what the frontend needs to decide whether to show org-admin-only actions
// (delete/reset organization, close fiscal year, GL exports) for the
// currently selected organization. It reveals nothing about the org beyond
// the caller's own membership.
func (h *handler) getMyOrganizationRole(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	claims := getClaims(r)
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	role, isMember, err := h.db.GetOrganizationRole(orgID, claims.UserID)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"role": role, "isMember": isMember})
}

type addOrganizationUserRequest struct {
	// Email, not userId: an org admin isn't necessarily a platform admin and
	// has no route to list every user account to find an id by — email is
	// what they'd actually have on hand to invite someone. The account
	// itself must already exist (via platformAdminProtected /api/users);
	// this only grants an existing user access to this organization.
	Email string `json:"email"`
	Role  string `json:"role"`
}

func (h *handler) addOrganizationMember(w http.ResponseWriter, r *http.Request) {
	orgID := r.PathValue("orgId")
	var req addOrganizationUserRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	if req.Email == "" {
		writeError(w, http.StatusBadRequest, "email is required")
		return
	}

	var userID string
	err := h.db.DB.Get(&userID, `SELECT id FROM users WHERE email = ?`, req.Email)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "no user account exists with that email")
		return
	}
	if err != nil {
		writeInternalError(w, err)
		return
	}

	ou, err := h.db.AddOrganizationUser(orgID, userID, req.Role)
	if err != nil {
		if errors.Is(err, db.ErrLastOrgAdmin) {
			writeError(w, http.StatusConflict, "organization must keep at least one admin member")
			return
		}
		writeMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, ou)
}

type updateOrganizationUserRoleRequest struct {
	Role string `json:"role"`
}

func (h *handler) updateOrganizationMemberRole(w http.ResponseWriter, r *http.Request) {
	orgID, userID := r.PathValue("orgId"), r.PathValue("userId")
	var req updateOrganizationUserRoleRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	if err := h.db.UpdateOrganizationUserRole(orgID, userID, req.Role); err != nil {
		if errors.Is(err, db.ErrLastOrgAdmin) {
			writeError(w, http.StatusConflict, "organization must keep at least one admin member")
			return
		}
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "this user is not a member of the organization")
			return
		}
		writeMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"updated": true})
}

func (h *handler) removeOrganizationMember(w http.ResponseWriter, r *http.Request) {
	orgID, userID := r.PathValue("orgId"), r.PathValue("userId")
	if err := h.db.RemoveOrganizationUser(orgID, userID); err != nil {
		if errors.Is(err, db.ErrLastOrgAdmin) {
			writeError(w, http.StatusConflict, "organization must keep at least one admin member")
			return
		}
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "this user is not a member of the organization")
			return
		}
		writeInternalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
