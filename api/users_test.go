package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
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
	req.Header.Set("Authorization", "Bearer "+token)
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

// TestUpdateUser_LastActiveAdminGuarded exercises the guard against a THIRD
// party demotion/deactivation, distinct from the self-lockout guard above.
// The acting token's claims.Role is trusted as-is by adminOnly (unchanged,
// pre-existing behavior — role changes take effect on next login), so the
// actor here is seeded as a plain "user" and made "admin" only in the JWT,
// isolating the last-admin count check to the target.
func TestUpdateUser_LastActiveAdminGuarded(t *testing.T) {
	mux, database, _, _ := newTestRouter(t)
	seedUser(t, database, "actor", "user", 1)
	seedUser(t, database, "target-admin", "admin", 1)
	token := mintTestJWT(t, "actor", "admin")

	rec := doJSON(t, mux, token, http.MethodPut, "/api/users/target-admin", map[string]any{"role": "user"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected demoting the last active admin to be rejected, got %d: %s", rec.Code, rec.Body.String())
	}

	isActive := 0
	rec = doJSON(t, mux, token, http.MethodPut, "/api/users/target-admin", map[string]any{"isActive": &isActive})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected deactivating the last active admin to be rejected, got %d: %s", rec.Code, rec.Body.String())
	}

	// A second active admin makes the same change legal.
	seedUser(t, database, "second-admin", "admin", 1)
	rec = doJSON(t, mux, token, http.MethodPut, "/api/users/target-admin", map[string]any{"role": "user"})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected demotion to succeed once another active admin exists, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestDeleteUser_LastActiveAdminRejected(t *testing.T) {
	mux, database, _, _ := newTestRouter(t)
	seedUser(t, database, "actor", "user", 1)
	seedUser(t, database, "target-admin", "admin", 1)
	token := mintTestJWT(t, "actor", "admin")

	req := httptest.NewRequest(http.MethodDelete, "/api/users/target-admin", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected deleting the last active admin to be rejected, got %d: %s", rec.Code, rec.Body.String())
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
