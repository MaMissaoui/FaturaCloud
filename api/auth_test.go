package api

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

// TestIsHTTPS_OnlyTrustsForwardedProtoFromTrustedProxy is F64's regression
// test: X-Forwarded-Proto must only be honored when the request's direct
// peer is a configured trusted proxy — the same trust boundary
// TRUSTED_PROXIES already draws for X-Forwarded-For. Before this fix, any
// peer could set the header directly and force a Secure cookie/HSTS on a
// plain-HTTP deployment.
func TestIsHTTPS_OnlyTrustsForwardedProtoFromTrustedProxy(t *testing.T) {
	trusted := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}

	tests := []struct {
		name           string
		remoteAddr     string
		trustedProxies []netip.Prefix
		forwardedProto string
		want           bool
	}{
		{"no proxies configured, header ignored", "203.0.113.5:12345", nil, "https", false},
		{"untrusted peer sets header, ignored", "203.0.113.5:12345", trusted, "https", false},
		{"trusted peer sets header, honored", "10.1.2.3:54321", trusted, "https", true},
		{"trusted peer, no header", "10.1.2.3:54321", trusted, "", false},
		{"trusted peer, header says http", "10.1.2.3:54321", trusted, "http", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tc.remoteAddr
			if tc.forwardedProto != "" {
				req.Header.Set("X-Forwarded-Proto", tc.forwardedProto)
			}
			if got := IsHTTPS(req, tc.trustedProxies); got != tc.want {
				t.Errorf("IsHTTPS() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestIsHTTPS_DirectTLSAlwaysTrusted confirms r.TLS != nil (a direct HTTPS
// connection to the Go process itself, no proxy involved) is trusted
// regardless of trustedProxies or headers.
func TestIsHTTPS_DirectTLSAlwaysTrusted(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.5:12345"
	req.TLS = &tls.ConnectionState{}
	if !IsHTTPS(req, nil) {
		t.Error("expected a direct TLS connection to report HTTPS regardless of trustedProxies")
	}
}

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
