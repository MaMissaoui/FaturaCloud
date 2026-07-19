package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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

// TestLoginPerAccountRateLimit covers F24: attempts against one account are
// throttled even when each comes from a distinct source IP, so IP rotation
// can't grind a single email past the limit.
func TestLoginPerAccountRateLimit(t *testing.T) {
	mux, _, _, _ := newTestRouter(t)

	attempt := func(ip string) int {
		body, _ := json.Marshal(map[string]any{
			"email":    "throttle-target@example.com",
			"password": "whatever",
		})
		req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(csrfHeaderName, "1") // login route requires the CSRF header
		req.RemoteAddr = ip + ":1234"       // unique IP each call → IP bucket never trips
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec.Code
	}

	// loginMaxAttempts (10) attempts from distinct IPs: rejected as bad
	// credentials (401), never 429.
	for i := 0; i < loginMaxAttempts; i++ {
		if code := attempt(fmt.Sprintf("10.9.9.%d", i+1)); code == http.StatusTooManyRequests {
			t.Fatalf("attempt %d unexpectedly rate-limited (IP bucket should not trip): %d", i+1, code)
		}
	}

	// The next attempt on the same email, from yet another fresh IP, trips the
	// per-account limit.
	if code := attempt("10.9.9.250"); code != http.StatusTooManyRequests {
		t.Fatalf("expected per-account throttle to return 429, got %d", code)
	}
}
