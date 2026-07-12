package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestAuthMiddleware_RejectsDeactivatedUser covers F5: a JWT is only a claim
// about who logged in, not whether they're still allowed in — deactivating
// (or deleting) a user must invalidate their already-issued, unexpired token
// immediately, not after it naturally expires up to 24h later.
func TestAuthMiddleware_RejectsDeactivatedUser(t *testing.T) {
	mux, database, _, _ := newTestRouter(t)
	seedUser(t, database, "test-user", "user", 1)
	token := mintTestJWT(t, "test-user", "user")

	get := func() int {
		req := httptest.NewRequest(http.MethodGet, "/api/organizations", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec.Code
	}

	if code := get(); code != http.StatusOK {
		t.Fatalf("expected an active user's token to be accepted, got %d", code)
	}

	if _, err := database.DB.Exec(`UPDATE users SET isActive = 0 WHERE id = ?`, "test-user"); err != nil {
		t.Fatalf("deactivate user: %v", err)
	}

	if code := get(); code != http.StatusUnauthorized {
		t.Fatalf("expected a deactivated user's still-unexpired token to be rejected, got %d", code)
	}
}

// TestAuthMiddleware_RejectsDeletedUser covers the companion case: a token
// whose user row no longer exists at all (deleted, not merely deactivated).
func TestAuthMiddleware_RejectsDeletedUser(t *testing.T) {
	mux, database, _, _ := newTestRouter(t)
	seedUser(t, database, "test-user", "user", 1)
	token := mintTestJWT(t, "test-user", "user")

	if _, err := database.DB.Exec(`DELETE FROM users WHERE id = ?`, "test-user"); err != nil {
		t.Fatalf("delete user: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/organizations", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected a deleted user's token to be rejected, got %d", rec.Code)
	}
}
