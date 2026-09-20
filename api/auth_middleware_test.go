package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
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
		authRequest(req, token)
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
	authRequest(req, token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected a deleted user's token to be rejected, got %d", rec.Code)
	}
}

// TestAuthMiddleware_RejectsTokenWithoutIssuerAudience covers F23: tokens are
// bound to this app via iss/aud, so a signature-valid token that lacks those
// claims (e.g. one minted for another service sharing the JWT secret, or an
// older token issued before this binding) is rejected.
func TestAuthMiddleware_RejectsTokenWithoutIssuerAudience(t *testing.T) {
	mux, database, _, _ := newTestRouter(t)
	seedUser(t, database, "test-user", "user", 1)

	// Mint a correctly-signed token that omits Issuer/Audience.
	claims := Claims{
		UserID:   "test-user",
		Provider: "local",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(testRestoreJWTSecret))
	if err != nil {
		t.Fatalf("mint jwt: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/organizations", nil)
	authRequest(req, token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected a token without iss/aud to be rejected, got %d", rec.Code)
	}
}

// TestAuthMiddleware_RejectsTokenAfterPasswordChange is F109's other
// revocation path: a password change (here an admin resetting another user's
// password) bumps that user's tokenVersion, so their already-issued token
// stops working immediately. Without it, a reset after a suspected compromise
// would leave the old session valid until its natural expiry.
func TestAuthMiddleware_RejectsTokenAfterPasswordChange(t *testing.T) {
	mux, database, _, _ := newTestRouter(t)
	seedUser(t, database, "actor", "admin", 1)
	seedUser(t, database, "target", "user", 1)
	actorToken := mintTestJWT(t, "actor", "admin")
	targetToken := mintTestJWT(t, "target", "user")

	protected := func(token string) int {
		req := httptest.NewRequest(http.MethodGet, "/api/organizations", nil)
		authRequest(req, token)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec.Code
	}

	if code := protected(targetToken); code != http.StatusOK {
		t.Fatalf("expected target's token to be accepted before the change, got %d", code)
	}

	rec := doJSON(t, mux, actorToken, http.MethodPut, "/api/users/target", map[string]any{"password": "brand-new-password"})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected password change to succeed, got %d: %s", rec.Code, rec.Body.String())
	}

	if code := protected(targetToken); code != http.StatusUnauthorized {
		t.Fatalf("expected target's pre-change token to be rejected, got %d", code)
	}
}
