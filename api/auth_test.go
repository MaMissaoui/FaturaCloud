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

	"github.com/golang-jwt/jwt/v5"
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

// TestClientIP_WalksXForwardedForFromTheRight is F110's regression test: only
// the direct peer's trust was checked before, but the *leftmost* X-Forwarded-For
// entry was then read as the client. A trusted proxy appends to whatever the
// client sent rather than replacing it, so the leftmost value is
// attacker-controlled ("1.2.3.4" below) and the real client address is the
// rightmost non-proxy hop. clientIP must walk from the right and skip
// configured trusted proxies.
func TestClientIP_WalksXForwardedForFromTheRight(t *testing.T) {
	trusted := []netip.Prefix{
		netip.MustParsePrefix("10.0.0.0/8"),
		netip.MustParsePrefix("192.168.0.0/16"),
	}
	h := &handler{trustedProxies: trusted}

	tests := []struct {
		name       string
		remoteAddr string
		xff        string
		want       string
	}{
		{"untrusted peer ignores the header", "203.0.113.5:1234", "1.2.3.4", "203.0.113.5"},
		{"trusted peer, no header", "10.1.2.3:1234", "", "10.1.2.3"},
		{"trusted peer, single client", "10.1.2.3:1234", "203.0.113.7", "203.0.113.7"},
		// The spoof that the old leftmost read fell for: attacker-supplied
		// 1.2.3.4, then the real client appended by the trusted proxy.
		{"spoofed leftmost entry ignored", "10.1.2.3:1234", "1.2.3.4, 203.0.113.7", "203.0.113.7"},
		// A chain of trusted proxies is skipped hop by hop.
		{"trusted hops on the right are skipped", "10.1.2.3:1234", "1.2.3.4, 192.168.5.5, 203.0.113.9", "203.0.113.9"},
		// Nothing but trusted proxies: fall back to the direct peer.
		{"all hops trusted falls back to peer", "10.1.2.3:1234", "10.9.9.9, 192.168.5.5", "10.1.2.3"},
		// An unparseable entry can't be a trusted proxy, so it's returned
		// rather than skipped to an attacker-controlled entry further left.
		{"unparseable entry returned as-is", "10.1.2.3:1234", "203.0.113.7, nonsense", "nonsense"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tc.remoteAddr
			if tc.xff != "" {
				req.Header.Set("X-Forwarded-For", tc.xff)
			}
			if got := h.clientIP(req); got != tc.want {
				t.Errorf("clientIP() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestLogout_RevokesAllSessions is F109's regression test: logout must not
// merely expire the browser's own cookie (an httpOnly cookie can't be cleared
// by page JS, and the user may hold a copy elsewhere) — it bumps
// users.tokenVersion, so the already-issued JWT itself stops being accepted,
// even though its 24h expiry hasn't passed.
func TestLogout_RevokesAllSessions(t *testing.T) {
	mux, database, _, _ := newTestRouter(t)
	seedUser(t, database, "test-user", "user", 1)
	token := mintTestJWT(t, "test-user", "user")

	protected := func() int {
		req := httptest.NewRequest(http.MethodGet, "/api/organizations", nil)
		authRequest(req, token)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec.Code
	}

	if code := protected(); code != http.StatusOK {
		t.Fatalf("expected the token to be accepted before logout, got %d", code)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	authRequest(req, token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 from logout, got %d: %s", rec.Code, rec.Body.String())
	}

	var tokenVersion int
	if err := database.DB.Get(&tokenVersion, `SELECT tokenVersion FROM users WHERE id = ?`, "test-user"); err != nil {
		t.Fatalf("query tokenVersion: %v", err)
	}
	if tokenVersion != 1 {
		t.Fatalf("tokenVersion after logout = %d, want 1", tokenVersion)
	}

	if code := protected(); code != http.StatusUnauthorized {
		t.Fatalf("expected the pre-logout token to be rejected after logout, got %d", code)
	}
}

// TestIssueToken_EmbedsCurrentTokenVersion locks in the half of F109 that the
// revocation tests above can't see: the token is only revocable if the value
// issued into every token actually survives signing and re-parsing. Claims
// carries TokenVersion with a json tag (not json:"-") for exactly this
// reason — if it were dropped from the payload, a token minted *after* a
// logout/password-change bump would parse as version 0 and be rejected no
// matter what, locking the user out permanently rather than merely revoking
// old sessions.
func TestIssueToken_EmbedsCurrentTokenVersion(t *testing.T) {
	h := &handler{jwtSecret: testRestoreJWTSecret}
	token, err := h.issueTokenWithProvider(userRow{ID: "u1", Email: "u1@test.local", TokenVersion: 7}, "local")
	if err != nil {
		t.Fatalf("issueTokenWithProvider: %v", err)
	}

	claims := &Claims{}
	if _, err := jwt.ParseWithClaims(token, claims, func(*jwt.Token) (any, error) {
		return []byte(testRestoreJWTSecret), nil
	}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithIssuer(jwtIssuer), jwt.WithAudience(jwtAudience)); err != nil {
		t.Fatalf("parse issued token: %v", err)
	}
	if claims.TokenVersion != 7 {
		t.Fatalf("issued token carries TokenVersion=%d, want 7 — the claim did not survive serialization", claims.TokenVersion)
	}
}
