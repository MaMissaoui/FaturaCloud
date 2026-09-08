package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/MaMissaoui/fatura-cloud/db"
)

// doJSON issues req through mux with an admin bearer token and a JSON body,
// returning the recorded response.
func doJSON(t *testing.T, mux http.Handler, token, method, path string, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	authRequest(req, token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

// TestUpdateUser_RoleOnlyChangePersists covers F6: a role-only PUT used to be
// silently dropped because the SQL only ran when displayName was also set.
func TestUpdateUser_RoleOnlyChangePersists(t *testing.T) {
	mux, database, _, _ := newTestRouter(t)
	seedUser(t, database, "actor", "admin", 1)
	seedUser(t, database, "target", "user", 1)
	token := mintTestJWT(t, "actor", "admin")

	rec := doJSON(t, mux, token, http.MethodPut, "/api/users/target", map[string]any{"role": "admin"})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["role"] != "admin" {
		t.Fatalf("expected role-only update to persist, got %v", resp["role"])
	}

	var role string
	if err := database.DB.Get(&role, `SELECT role FROM users WHERE id = ?`, "target"); err != nil {
		t.Fatalf("query role: %v", err)
	}
	if role != "admin" {
		t.Fatalf("role not persisted in db, got %q", role)
	}
}

func TestUpdateUser_RejectsInvalidRole(t *testing.T) {
	mux, database, _, _ := newTestRouter(t)
	seedUser(t, database, "actor", "admin", 1)
	seedUser(t, database, "target", "user", 1)
	token := mintTestJWT(t, "actor", "admin")

	rec := doJSON(t, mux, token, http.MethodPut, "/api/users/target", map[string]any{"role": "superadmin"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid role, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUpdateUser_NotFound(t *testing.T) {
	mux, database, _, _ := newTestRouter(t)
	seedUser(t, database, "actor", "admin", 1)
	token := mintTestJWT(t, "actor", "admin")

	rec := doJSON(t, mux, token, http.MethodPut, "/api/users/does-not-exist", map[string]any{"displayName": "X"})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for nonexistent user, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUpdateUser_SelfLockoutRejected(t *testing.T) {
	mux, database, _, _ := newTestRouter(t)
	seedUser(t, database, "actor", "admin", 1)
	seedUser(t, database, "other-admin", "admin", 1) // so this isn't also the last-admin case
	token := mintTestJWT(t, "actor", "admin")

	if rec := doJSON(t, mux, token, http.MethodPut, "/api/users/actor", map[string]any{"role": "user"}); rec.Code != http.StatusBadRequest {
		t.Fatalf("expected self-demotion to be rejected, got %d: %s", rec.Code, rec.Body.String())
	}
	isActive := 0
	if rec := doJSON(t, mux, token, http.MethodPut, "/api/users/actor", map[string]any{"isActive": &isActive}); rec.Code != http.StatusBadRequest {
		t.Fatalf("expected self-deactivation to be rejected, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestUpdateUser_ThirdPartyCanDemoteDownToOneRemainingAdmin covers the
// third-party counterpart to the self-lockout guard above, updated for
// isPlatformAdmin being re-derived from the DB on every request (see
// api/middleware.go's authMiddleware) rather than trusted from the JWT.
//
// That change makes the *previous* version of this test — a third party
// demotes/deactivates the sole remaining admin, expecting 400 — structurally
// unreachable now: reaching platformAdminProtected at all requires the actor
// to themselves be a real, DB-backed platform admin, so they're always
// counted by countActiveAdmins alongside the target. A third party can
// therefore never observe "target is the last admin" while still holding
// enough privilege to act — the only way to hit that guard is the
// self-lockout path, which is already covered above. What's left worth
// asserting here is the positive case: a third party CAN demote/deactivate
// one of two admins, leaving exactly one — the guard shouldn't over-block.
func TestUpdateUser_ThirdPartyCanDemoteDownToOneRemainingAdmin(t *testing.T) {
	mux, database, _, _ := newTestRouter(t)
	seedUser(t, database, "actor", "admin", 1)
	seedUser(t, database, "target-admin", "admin", 1)
	token := mintTestJWT(t, "actor", "admin")

	rec := doJSON(t, mux, token, http.MethodPut, "/api/users/target-admin", map[string]any{"role": "user"})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected demoting one of two admins to succeed, got %d: %s", rec.Code, rec.Body.String())
	}

	var isPlatformAdmin int
	if err := database.DB.Get(&isPlatformAdmin, `SELECT isPlatformAdmin FROM users WHERE id = ?`, "target-admin"); err != nil {
		t.Fatalf("query target: %v", err)
	}
	if isPlatformAdmin != 0 {
		t.Fatalf("expected target-admin to no longer be a platform admin, got isPlatformAdmin=%d", isPlatformAdmin)
	}
}

// TestDeleteUser_ThirdPartyCanDeleteDownToOneRemainingAdmin is DeleteUser's
// counterpart to the Update test above — see that test's comment for why the
// old "reject deleting the sole remaining admin" scenario is now
// structurally unreachable for a third-party actor.
func TestDeleteUser_ThirdPartyCanDeleteDownToOneRemainingAdmin(t *testing.T) {
	mux, database, _, _ := newTestRouter(t)
	seedUser(t, database, "actor", "admin", 1)
	seedUser(t, database, "target-admin", "admin", 1)
	token := mintTestJWT(t, "actor", "admin")

	req := httptest.NewRequest(http.MethodDelete, "/api/users/target-admin", nil)
	authRequest(req, token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected deleting one of two admins to succeed, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestDeleteUser_SoleOrgAdminRejected covers F-per-org-roles' Phase A guard:
// organization_users cascades on user delete, so deleting the sole admin of
// an organization wouldn't just lock it out, it would silently erase the
// membership row too — with no platform-admin bypass to recover it
// afterward (org-admin access is per-organization by design).
func TestDeleteUser_SoleOrgAdminRejected(t *testing.T) {
	mux, database, _, _ := newTestRouter(t)
	seedUser(t, database, "actor", "admin", 1)
	seedUser(t, database, "target", "user", 1)
	token := mintTestJWT(t, "actor", "admin")

	if _, err := database.CreateOrganization(db.CreateOrganizationRequest{ID: "org-1", Name: strPtr("ACME")}); err != nil {
		t.Fatalf("seed CreateOrganization: %v", err)
	}
	if _, err := database.AddOrganizationUser("org-1", "target", "admin"); err != nil {
		t.Fatalf("seed org membership: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/users/target", nil)
	authRequest(req, token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 deleting the sole admin of an organization, got %d: %s", rec.Code, rec.Body.String())
	}

	// The user (and its org membership) must survive the rejected delete.
	var count int
	if err := database.DB.Get(&count, `SELECT COUNT(*) FROM users WHERE id = ?`, "target"); err != nil {
		t.Fatalf("query user: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected target user to survive a rejected delete, got count=%d", count)
	}
}

// TestUpdateUser_DeactivateSoleOrgAdminRejected is the deactivation
// counterpart — a deactivated user can't authenticate (authMiddleware
// rejects them), so an org left with only a deactivated "admin" is
// functionally the same orphaned state as a deleted one.
func TestUpdateUser_DeactivateSoleOrgAdminRejected(t *testing.T) {
	mux, database, _, _ := newTestRouter(t)
	seedUser(t, database, "actor", "admin", 1)
	seedUser(t, database, "target", "user", 1)
	token := mintTestJWT(t, "actor", "admin")

	if _, err := database.CreateOrganization(db.CreateOrganizationRequest{ID: "org-1", Name: strPtr("ACME")}); err != nil {
		t.Fatalf("seed CreateOrganization: %v", err)
	}
	if _, err := database.AddOrganizationUser("org-1", "target", "admin"); err != nil {
		t.Fatalf("seed org membership: %v", err)
	}

	isActive := 0
	rec := doJSON(t, mux, token, http.MethodPut, "/api/users/target", map[string]any{"isActive": &isActive})
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 deactivating the sole admin of an organization, got %d: %s", rec.Code, rec.Body.String())
	}

	var active int
	if err := database.DB.Get(&active, `SELECT isActive FROM users WHERE id = ?`, "target"); err != nil {
		t.Fatalf("query user: %v", err)
	}
	if active != 1 {
		t.Fatalf("expected target to remain active after a rejected deactivation, got isActive=%d", active)
	}
}

func TestCreateUser_ValidatesInput(t *testing.T) {
	mux, database, _, _ := newTestRouter(t)
	seedUser(t, database, "actor", "admin", 1)
	token := mintTestJWT(t, "actor", "admin")

	cases := []struct {
		name string
		body map[string]any
	}{
		{"bad email", map[string]any{"email": "not-an-email", "password": "longenoughpw"}},
		{"short password", map[string]any{"email": "new@test.local", "password": "short"}},
		{"invalid role", map[string]any{"email": "new@test.local", "password": "longenoughpw", "role": "root"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := doJSON(t, mux, token, http.MethodPost, "/api/users", tc.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
			}
		})
	}
}
