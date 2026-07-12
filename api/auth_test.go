package api

import (
	"net/http"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

// TestLogin seeds a user with a real bcrypt hash (seedUser's placeholder
// "unused-hash" isn't a valid bcrypt hash and would make every login fail,
// not just wrong-password attempts) and exercises login end-to-end.
func TestLogin(t *testing.T) {
	mux, database, _, _ := newTestRouter(t)

	hash, err := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	if _, err := database.DB.Exec(
		`INSERT INTO users (id, email, passwordHash, displayName, role, isActive) VALUES (?, ?, ?, ?, ?, ?)`,
		"user-1", "real@test.local", string(hash), "Real User", "user", 1,
	); err != nil {
		t.Fatalf("seed local user: %v", err)
	}

	t.Run("correct credentials succeed", func(t *testing.T) {
		rec := doJSON(t, mux, "", http.MethodPost, "/api/auth/login", map[string]any{
			"email": "real@test.local", "password": "correct-password",
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("wrong password rejected", func(t *testing.T) {
		rec := doJSON(t, mux, "", http.MethodPost, "/api/auth/login", map[string]any{
			"email": "real@test.local", "password": "wrong-password",
		})
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("unknown email rejected", func(t *testing.T) {
		rec := doJSON(t, mux, "", http.MethodPost, "/api/auth/login", map[string]any{
			"email": "nobody@test.local", "password": "anything",
		})
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("deactivated user rejected", func(t *testing.T) {
		if _, err := database.DB.Exec(`UPDATE users SET isActive = 0 WHERE id = ?`, "user-1"); err != nil {
			t.Fatalf("deactivate: %v", err)
		}
		rec := doJSON(t, mux, "", http.MethodPost, "/api/auth/login", map[string]any{
			"email": "real@test.local", "password": "correct-password",
		})
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}
