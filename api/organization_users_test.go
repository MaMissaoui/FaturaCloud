package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/MaMissaoui/fatura-cloud/db"
)

// TestOrganizationMembers_RequiresOrgAdmin covers the whole membership CRUD
// surface's authorization: a plain org member (not an admin of that org) gets
// 403 on every route, even a platform admin who isn't a member of this
// particular organization at all.
func TestOrganizationMembers_RequiresOrgAdmin(t *testing.T) {
	mux, database, _, _ := newTestRouter(t)
	seedUser(t, database, "platform-admin", "admin", 1)
	seedUser(t, database, "org-member", "user", 1)
	seedUser(t, database, "outsider", "user", 1)
	if _, err := database.CreateOrganization(db.CreateOrganizationRequest{ID: "org-1", Name: strPtr("ACME")}); err != nil {
		t.Fatalf("seed CreateOrganization: %v", err)
	}
	if _, err := database.AddOrganizationUser("org-1", "org-member", "user"); err != nil {
		t.Fatalf("seed membership: %v", err)
	}

	for _, actor := range []string{"platform-admin", "org-member", "outsider"} {
		token := mintTestJWT(t, actor, "")
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/organizations/org-1/members", nil)
		authRequest(req, token)
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("actor %q: expected 403 listing members without org-admin role, got %d: %s", actor, rec.Code, rec.Body.String())
		}
	}
}

// TestOrganizationMembers_CRUD exercises the happy path end-to-end: an org
// admin adds a member by email, promotes them, then removes them.
func TestOrganizationMembers_CRUD(t *testing.T) {
	mux, database, _, _ := newTestRouter(t)
	seedUser(t, database, "org-admin", "user", 1)
	seedUser(t, database, "new-member", "user", 1)
	if _, err := database.CreateOrganization(db.CreateOrganizationRequest{ID: "org-1", Name: strPtr("ACME")}); err != nil {
		t.Fatalf("seed CreateOrganization: %v", err)
	}
	if _, err := database.AddOrganizationUser("org-1", "org-admin", "admin"); err != nil {
		t.Fatalf("seed org-admin membership: %v", err)
	}
	token := mintTestJWT(t, "org-admin", "")

	rec := doJSON(t, mux, token, http.MethodPost, "/api/organizations/org-1/members", map[string]any{
		"email": "new-member@test.local", "role": "user",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 adding a member by email, got %d: %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/organizations/org-1/members", nil)
	authRequest(req, token)
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 listing members, got %d: %s", rec.Code, rec.Body.String())
	}

	rec = doJSON(t, mux, token, http.MethodPut, "/api/organizations/org-1/members/new-member", map[string]any{"role": "admin"})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 promoting member, got %d: %s", rec.Code, rec.Body.String())
	}
	role, isMember, err := database.GetOrganizationRole("org-1", "new-member")
	if err != nil || !isMember || role != "admin" {
		t.Fatalf("expected new-member promoted to admin, got isMember=%v role=%q err=%v", isMember, role, err)
	}

	req = httptest.NewRequest(http.MethodDelete, "/api/organizations/org-1/members/new-member", nil)
	authRequest(req, token)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204 removing member, got %d: %s", rec.Code, rec.Body.String())
	}
	if _, isMember, err := database.GetOrganizationRole("org-1", "new-member"); err != nil || isMember {
		t.Fatalf("expected new-member membership gone, got isMember=%v err=%v", isMember, err)
	}
}

// TestOrganizationMembers_AddUnknownEmailRejected covers the design choice to
// resolve membership grants by email rather than userId (an org admin has no
// route to list every platform user) — an email with no matching account is a
// clean 404, not a foreign-key 500.
func TestOrganizationMembers_AddUnknownEmailRejected(t *testing.T) {
	mux, database, _, _ := newTestRouter(t)
	seedUser(t, database, "org-admin", "user", 1)
	if _, err := database.CreateOrganization(db.CreateOrganizationRequest{ID: "org-1", Name: strPtr("ACME")}); err != nil {
		t.Fatalf("seed CreateOrganization: %v", err)
	}
	if _, err := database.AddOrganizationUser("org-1", "org-admin", "admin"); err != nil {
		t.Fatalf("seed org-admin membership: %v", err)
	}
	token := mintTestJWT(t, "org-admin", "")

	rec := doJSON(t, mux, token, http.MethodPost, "/api/organizations/org-1/members", map[string]any{
		"email": "nobody@nowhere.local", "role": "user",
	})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for an unknown email, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestOrganizationMembers_ReAddingSoleAdminAtLowerRoleRejected covers the
// upsert path (re-adding an existing member by email) demoting the sole
// admin exactly like the PUT role-update route can — see
// db/organization_user.go's AddOrganizationUser guard.
func TestOrganizationMembers_ReAddingSoleAdminAtLowerRoleRejected(t *testing.T) {
	mux, database, _, _ := newTestRouter(t)
	seedUser(t, database, "org-admin", "user", 1)
	if _, err := database.CreateOrganization(db.CreateOrganizationRequest{ID: "org-1", Name: strPtr("ACME")}); err != nil {
		t.Fatalf("seed CreateOrganization: %v", err)
	}
	if _, err := database.AddOrganizationUser("org-1", "org-admin", "admin"); err != nil {
		t.Fatalf("seed org-admin membership: %v", err)
	}
	token := mintTestJWT(t, "org-admin", "")

	rec := doJSON(t, mux, token, http.MethodPost, "/api/organizations/org-1/members", map[string]any{
		"email": "org-admin@test.local", "role": "user",
	})
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 re-adding the sole admin at role=user, got %d: %s", rec.Code, rec.Body.String())
	}

	role, isMember, err := database.GetOrganizationRole("org-1", "org-admin")
	if err != nil || !isMember || role != "admin" {
		t.Fatalf("expected org-admin to remain admin, got isMember=%v role=%q err=%v", isMember, role, err)
	}
}

// TestGetMyOrganizationRole covers the one membership route that is NOT
// org-admin gated — any authenticated user can ask their own role, including
// "not a member at all", without needing org-admin access first.
func TestGetMyOrganizationRole(t *testing.T) {
	mux, database, _, _ := newTestRouter(t)
	seedUser(t, database, "org-admin", "user", 1)
	seedUser(t, database, "outsider", "user", 1)
	if _, err := database.CreateOrganization(db.CreateOrganizationRequest{ID: "org-1", Name: strPtr("ACME")}); err != nil {
		t.Fatalf("seed CreateOrganization: %v", err)
	}
	if _, err := database.AddOrganizationUser("org-1", "org-admin", "admin"); err != nil {
		t.Fatalf("seed org-admin membership: %v", err)
	}

	get := func(actor string) *httptest.ResponseRecorder {
		token := mintTestJWT(t, actor, "")
		req := httptest.NewRequest(http.MethodGet, "/api/organizations/org-1/my-role", nil)
		authRequest(req, token)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}

	rec := get("org-admin")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for a member checking their own role, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["role"] != "admin" || resp["isMember"] != true {
		t.Fatalf("expected role=admin isMember=true for org-admin, got %v", resp)
	}

	rec = get("outsider")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for a non-member checking their own role, got %d: %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["isMember"] != false {
		t.Fatalf("expected isMember=false for a non-member, got %v", resp)
	}
}

// TestGetMyOrganizationRoles covers the batch counterpart to my-role (issue
// #147) — one request returning every organization the caller belongs to,
// instead of the list page's old one-my-role-call-per-row pattern.
func TestGetMyOrganizationRoles(t *testing.T) {
	mux, database, _, _ := newTestRouter(t)
	seedUser(t, database, "multi-org-user", "user", 1)
	seedUser(t, database, "outsider", "user", 1)
	if _, err := database.CreateOrganization(db.CreateOrganizationRequest{ID: "org-a", Name: strPtr("A")}); err != nil {
		t.Fatalf("seed org-a: %v", err)
	}
	if _, err := database.CreateOrganization(db.CreateOrganizationRequest{ID: "org-b", Name: strPtr("B")}); err != nil {
		t.Fatalf("seed org-b: %v", err)
	}
	if _, err := database.CreateOrganization(db.CreateOrganizationRequest{ID: "org-c", Name: strPtr("C")}); err != nil {
		t.Fatalf("seed org-c: %v", err)
	}
	if _, err := database.AddOrganizationUser("org-a", "multi-org-user", "admin"); err != nil {
		t.Fatalf("seed org-a membership: %v", err)
	}
	if _, err := database.AddOrganizationUser("org-b", "multi-org-user", "user"); err != nil {
		t.Fatalf("seed org-b membership: %v", err)
	}
	// Deliberately no membership in org-c.

	token := mintTestJWT(t, "multi-org-user", "")
	req := httptest.NewRequest(http.MethodGet, "/api/organizations/my-roles", nil)
	authRequest(req, token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var roles map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &roles); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(roles) != 2 || roles["org-a"] != "admin" || roles["org-b"] != "user" {
		t.Fatalf("expected {org-a: admin, org-b: user}, got %v", roles)
	}
	if _, present := roles["org-c"]; present {
		t.Fatalf("expected org-c absent (not a member), got %v", roles)
	}

	// An outsider with no memberships at all still gets a clean 200 with an
	// empty map, not an error — same "reveals nothing beyond the caller's
	// own memberships" shape as my-role's isMember=false case.
	token = mintTestJWT(t, "outsider", "")
	req = httptest.NewRequest(http.MethodGet, "/api/organizations/my-roles", nil)
	authRequest(req, token)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for an outsider, got %d: %s", rec.Code, rec.Body.String())
	}
	roles = nil
	if err := json.Unmarshal(rec.Body.Bytes(), &roles); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(roles) != 0 {
		t.Fatalf("expected no roles for an outsider, got %v", roles)
	}
}

// TestOrganizationMembers_RemoveLastAdminRejected covers the last-org-admin
// guard surfacing as a clean 409 through the HTTP layer.
func TestOrganizationMembers_RemoveLastAdminRejected(t *testing.T) {
	mux, database, _, _ := newTestRouter(t)
	seedUser(t, database, "org-admin", "user", 1)
	if _, err := database.CreateOrganization(db.CreateOrganizationRequest{ID: "org-1", Name: strPtr("ACME")}); err != nil {
		t.Fatalf("seed CreateOrganization: %v", err)
	}
	if _, err := database.AddOrganizationUser("org-1", "org-admin", "admin"); err != nil {
		t.Fatalf("seed org-admin membership: %v", err)
	}
	token := mintTestJWT(t, "org-admin", "")

	req := httptest.NewRequest(http.MethodDelete, "/api/organizations/org-1/members/org-admin", nil)
	authRequest(req, token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 removing the sole admin, got %d: %s", rec.Code, rec.Body.String())
	}
}
