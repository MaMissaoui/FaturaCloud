package api

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MaMissaoui/fatura-cloud/db"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-jose/go-jose/v4"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/oauth2"
)

const testClientID = "fatura-cloud"
const testRedirectURL = "http://app.local/api/auth/oidc/callback"

// fakeIdP is a minimal OIDC provider — discovery document, JWKS, and a token
// endpoint whose ID token response is controlled per-test via idTokenFor —
// enough to exercise FaturaCloud's OIDC client against a real HTTP round
// trip without depending on a live Authelia instance.
type fakeIdP struct {
	server     *httptest.Server
	key        *rsa.PrivateKey
	kid        string
	idTokenFor func(code string) (idToken string, oauthErr string)
}

func newFakeIdP(t *testing.T) *fakeIdP {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	f := &fakeIdP{key: key, kid: "test-key-1"}

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"issuer":                                f.server.URL,
			"authorization_endpoint":                f.server.URL + "/auth",
			"token_endpoint":                        f.server.URL + "/token",
			"jwks_uri":                              f.server.URL + "/jwks",
			"response_types_supported":              []string{"code"},
			"subject_types_supported":               []string{"public"},
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		jwks := jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{
			Key:       &f.key.PublicKey,
			KeyID:     f.kid,
			Algorithm: "RS256",
			Use:       "sig",
		}}}
		writeJSON(w, http.StatusOK, jwks)
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if f.idTokenFor == nil {
			writeError(w, http.StatusInternalServerError, "no fake response configured")
			return
		}
		idToken, oauthErr := f.idTokenFor(r.FormValue("code"))
		if oauthErr != "" {
			writeError(w, http.StatusBadRequest, oauthErr)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"access_token": "test-access-token",
			"token_type":   "Bearer",
			"id_token":     idToken,
			"expires_in":   3600,
		})
	})

	f.server = httptest.NewServer(mux)
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeIdP) sign(t *testing.T, claims map[string]any) string {
	t.Helper()
	signer, err := jose.NewSigner(
		jose.SigningKey{Algorithm: jose.RS256, Key: f.key},
		(&jose.SignerOptions{}).WithHeader("kid", f.kid).WithType("JWT"),
	)
	if err != nil {
		t.Fatalf("new signer: %v", err)
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	jws, err := signer.Sign(payload)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	compact, err := jws.CompactSerialize()
	if err != nil {
		t.Fatalf("compact serialize: %v", err)
	}
	return compact
}

// baseClaims returns a valid, fresh ID token claim set that individual tests
// mutate to exercise a specific failure mode.
func (f *fakeIdP) baseClaims(nonce string) map[string]any {
	now := time.Now()
	return map[string]any{
		"iss":    f.server.URL,
		"aud":    testClientID,
		"sub":    "user-1",
		"email":  "alice@example.com",
		"name":   "Alice",
		"groups": []string{"fatura-users", "admins"},
		"nonce":  nonce,
		"iat":    now.Unix(),
		"exp":    now.Add(5 * time.Minute).Unix(),
	}
}

func newTestHandler(t *testing.T, oidcCfg OIDCConfig) *handler {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.NewDatabase(dbPath)
	if err != nil {
		t.Fatalf("new database: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return &handler{
		db:        database,
		jwtSecret: "test-jwt-secret-at-least-32-characters-long",
		oidcCfg:   oidcCfg,
	}
}

// callbackRequest builds a GET /api/auth/oidc/callback request carrying a
// validly-signed state cookie for (state, nonce, verifier), plus the given
// query string overrides (state/code by default match the cookie).
func callbackRequest(h *handler, state, nonce, verifier, queryState, code string) *http.Request {
	payload, _ := json.Marshal(oidcStateCookiePayload{State: state, Nonce: nonce, CodeVerifier: verifier})
	cookieVal := signOIDCCookie(h.jwtSecret, payload)

	q := "state=" + queryState + "&code=" + code
	req := httptest.NewRequest(http.MethodGet, "/api/auth/oidc/callback?"+q, nil)
	req.AddCookie(&http.Cookie{Name: oidcStateCookieName, Value: cookieVal})
	return req
}

// parseIssuedJWT verifies the callback delivered the session the new way — an
// httpOnly fc_token cookie plus a redirect to "/" with no token in the URL —
// and returns the decoded claims.
func parseIssuedJWT(t *testing.T, h *handler, rec *httptest.ResponseRecorder) *Claims {
	t.Helper()
	if loc := rec.Header().Get("Location"); loc != "/" {
		t.Fatalf("expected redirect to /, got %q", loc)
	}
	if strings.Contains(rec.Header().Get("Location"), "#token=") {
		t.Fatalf("token must not appear in the redirect URL")
	}
	var tokenStr string
	for _, c := range rec.Result().Cookies() {
		if c.Name == authCookieName {
			tokenStr = c.Value
			if !c.HttpOnly {
				t.Fatalf("%s cookie must be HttpOnly", authCookieName)
			}
		}
	}
	if tokenStr == "" {
		t.Fatalf("callback set no %s cookie", authCookieName)
	}
	claims := &Claims{}
	_, err := jwt.ParseWithClaims(tokenStr, claims, func(*jwt.Token) (any, error) {
		return []byte(h.jwtSecret), nil
	}, jwt.WithValidMethods([]string{"HS256"}))
	if err != nil {
		t.Fatalf("parse issued jwt: %v", err)
	}
	return claims
}

func TestOIDCLoginStart_RedirectsAndSetsCookie(t *testing.T) {
	idp := newFakeIdP(t)
	h := newTestHandler(t, OIDCConfig{
		IssuerURL: idp.server.URL, ClientID: testClientID, ClientSecret: "secret",
		RedirectURL: testRedirectURL, Scopes: []string{"openid", "email", "groups"},
		EmailClaim: "email", NameClaim: "name", GroupsClaim: "groups", AdminGroup: "admins",
	})

	req := httptest.NewRequest(http.MethodGet, "/api/auth/oidc/login", nil)
	rec := httptest.NewRecorder()
	h.oidcLoginStart(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.HasPrefix(loc, idp.server.URL+"/auth") {
		t.Errorf("expected redirect to idp authorization endpoint, got %s", loc)
	}
	if !strings.Contains(loc, "code_challenge_method=S256") {
		t.Errorf("expected PKCE S256 challenge in auth URL, got %s", loc)
	}
	if rec.Result().Cookies() == nil {
		t.Fatalf("expected a state cookie to be set")
	}
}

func TestOIDCLoginStart_DisabledWhenUnconfigured(t *testing.T) {
	h := newTestHandler(t, OIDCConfig{})
	req := httptest.NewRequest(http.MethodGet, "/api/auth/oidc/login", nil)
	rec := httptest.NewRecorder()
	h.oidcLoginStart(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when oidc unconfigured, got %d", rec.Code)
	}
}

func TestOIDCCallback_SuccessGrantsAdminForGroupMember(t *testing.T) {
	idp := newFakeIdP(t)
	h := newTestHandler(t, OIDCConfig{
		IssuerURL: idp.server.URL, ClientID: testClientID, ClientSecret: "secret",
		RedirectURL: testRedirectURL, Scopes: []string{"openid", "email", "groups"},
		EmailClaim: "email", NameClaim: "name", GroupsClaim: "groups", AdminGroup: "admins",
	})

	idp.idTokenFor = func(code string) (string, string) {
		if code != "good-code" {
			return "", "invalid_grant"
		}
		return idp.sign(t, idp.baseClaims("n1")), ""
	}

	req := callbackRequest(h, "st1", "n1", "verifier1", "st1", "good-code")
	rec := httptest.NewRecorder()
	h.oidcCallback(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302 on success, got %d body=%s", rec.Code, rec.Body.String())
	}
	claims := parseIssuedJWT(t, h, rec)
	if claims.Email != "alice@example.com" {
		t.Errorf("expected email alice@example.com, got %s", claims.Email)
	}
	if claims.Provider != "oidc" {
		t.Errorf("expected provider oidc, got %q", claims.Provider)
	}
	// Role/platform-admin status is never carried in the token itself (see
	// api/middleware.go's Claims doc comment) — assert it against the DB row
	// the callback provisioned instead.
	var isPlatformAdmin int
	if err := h.db.DB.Get(&isPlatformAdmin, `SELECT isPlatformAdmin FROM users WHERE email = ?`, "alice@example.com"); err != nil {
		t.Fatalf("query provisioned user: %v", err)
	}
	if isPlatformAdmin != 1 {
		t.Errorf("expected isPlatformAdmin=1 (member of admins group), got %d", isPlatformAdmin)
	}
}

func TestOIDCCallback_NonAdminGroupGetsUserRole(t *testing.T) {
	idp := newFakeIdP(t)
	h := newTestHandler(t, OIDCConfig{
		IssuerURL: idp.server.URL, ClientID: testClientID, ClientSecret: "secret",
		RedirectURL: testRedirectURL, Scopes: []string{"openid", "email", "groups"},
		EmailClaim: "email", NameClaim: "name", GroupsClaim: "groups", AdminGroup: "admins",
	})

	idp.idTokenFor = func(code string) (string, string) {
		claims := idp.baseClaims("n1")
		claims["groups"] = []string{"fatura-users"}
		claims["email"] = "bob@example.com"
		return idp.sign(t, claims), ""
	}

	req := callbackRequest(h, "st1", "n1", "verifier1", "st1", "good-code")
	rec := httptest.NewRecorder()
	h.oidcCallback(rec, req)

	parseIssuedJWT(t, h, rec)
	var isPlatformAdmin int
	if err := h.db.DB.Get(&isPlatformAdmin, `SELECT isPlatformAdmin FROM users WHERE email = ?`, "bob@example.com"); err != nil {
		t.Fatalf("query provisioned user: %v", err)
	}
	if isPlatformAdmin != 0 {
		t.Errorf("expected isPlatformAdmin=0 (not in admins group), got %d", isPlatformAdmin)
	}
}

// TestOIDCCallback_UnverifiedEmailRejected is F65's regression test: a
// present-and-false email_verified claim must block the login rather than
// JIT-provisioning a user against an address the IdP itself says isn't
// confirmed.
func TestOIDCCallback_UnverifiedEmailRejected(t *testing.T) {
	idp := newFakeIdP(t)
	h := newTestHandler(t, OIDCConfig{
		IssuerURL: idp.server.URL, ClientID: testClientID, ClientSecret: "secret",
		RedirectURL: testRedirectURL, Scopes: []string{"openid", "email", "groups"},
		EmailClaim: "email", NameClaim: "name", GroupsClaim: "groups", AdminGroup: "admins",
	})

	idp.idTokenFor = func(code string) (string, string) {
		claims := idp.baseClaims("n1")
		claims["email_verified"] = false
		return idp.sign(t, claims), ""
	}

	req := callbackRequest(h, "st1", "n1", "verifier1", "st1", "good-code")
	rec := httptest.NewRecorder()
	h.oidcCallback(rec, req)

	if rec.Code != http.StatusFound || !strings.Contains(rec.Header().Get("Location"), "error=sso_failed") {
		t.Fatalf("expected a redirect to the login error page, got %d location=%q", rec.Code, rec.Header().Get("Location"))
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == authCookieName {
			t.Fatal("expected no auth cookie to be set for an unverified email")
		}
	}
}

// TestOIDCCallback_VerifiedEmailAccepted confirms a present-and-true
// email_verified claim doesn't regress the normal success path.
func TestOIDCCallback_VerifiedEmailAccepted(t *testing.T) {
	idp := newFakeIdP(t)
	h := newTestHandler(t, OIDCConfig{
		IssuerURL: idp.server.URL, ClientID: testClientID, ClientSecret: "secret",
		RedirectURL: testRedirectURL, Scopes: []string{"openid", "email", "groups"},
		EmailClaim: "email", NameClaim: "name", GroupsClaim: "groups", AdminGroup: "admins",
	})

	idp.idTokenFor = func(code string) (string, string) {
		claims := idp.baseClaims("n1")
		claims["email_verified"] = true
		return idp.sign(t, claims), ""
	}

	req := callbackRequest(h, "st1", "n1", "verifier1", "st1", "good-code")
	rec := httptest.NewRecorder()
	h.oidcCallback(rec, req)

	claims := parseIssuedJWT(t, h, rec)
	if claims.Email != "alice@example.com" {
		t.Errorf("expected email alice@example.com, got %s", claims.Email)
	}
}

func TestOIDCCallback_StateMismatchRejected(t *testing.T) {
	idp := newFakeIdP(t)
	h := newTestHandler(t, OIDCConfig{
		IssuerURL: idp.server.URL, ClientID: testClientID, ClientSecret: "secret",
		RedirectURL: testRedirectURL, Scopes: []string{"openid", "email", "groups"},
		EmailClaim: "email", NameClaim: "name", GroupsClaim: "groups", AdminGroup: "admins",
	})
	idp.idTokenFor = func(code string) (string, string) {
		return idp.sign(t, idp.baseClaims("n1")), ""
	}

	// Cookie says "st1" but the query string (as if tampered/replayed) says "different".
	req := callbackRequest(h, "st1", "n1", "verifier1", "different", "good-code")
	rec := httptest.NewRecorder()
	h.oidcCallback(rec, req)

	assertRejectedToLogin(t, rec)
}

func TestOIDCCallback_NonceMismatchRejected(t *testing.T) {
	idp := newFakeIdP(t)
	h := newTestHandler(t, OIDCConfig{
		IssuerURL: idp.server.URL, ClientID: testClientID, ClientSecret: "secret",
		RedirectURL: testRedirectURL, Scopes: []string{"openid", "email", "groups"},
		EmailClaim: "email", NameClaim: "name", GroupsClaim: "groups", AdminGroup: "admins",
	})
	// ID token is signed with a different nonce than what the cookie recorded —
	// simulates a replayed/stolen authorization code from a different session.
	idp.idTokenFor = func(code string) (string, string) {
		return idp.sign(t, idp.baseClaims("wrong-nonce")), ""
	}

	req := callbackRequest(h, "st1", "n1", "verifier1", "st1", "good-code")
	rec := httptest.NewRecorder()
	h.oidcCallback(rec, req)

	assertRejectedToLogin(t, rec)
}

func TestOIDCCallback_WrongAudienceRejected(t *testing.T) {
	idp := newFakeIdP(t)
	h := newTestHandler(t, OIDCConfig{
		IssuerURL: idp.server.URL, ClientID: testClientID, ClientSecret: "secret",
		RedirectURL: testRedirectURL, Scopes: []string{"openid", "email", "groups"},
		EmailClaim: "email", NameClaim: "name", GroupsClaim: "groups", AdminGroup: "admins",
	})
	idp.idTokenFor = func(code string) (string, string) {
		claims := idp.baseClaims("n1")
		claims["aud"] = "some-other-client"
		return idp.sign(t, claims), ""
	}

	req := callbackRequest(h, "st1", "n1", "verifier1", "st1", "good-code")
	rec := httptest.NewRecorder()
	h.oidcCallback(rec, req)

	assertRejectedToLogin(t, rec)
}

func TestOIDCCallback_ExpiredTokenRejected(t *testing.T) {
	idp := newFakeIdP(t)
	h := newTestHandler(t, OIDCConfig{
		IssuerURL: idp.server.URL, ClientID: testClientID, ClientSecret: "secret",
		RedirectURL: testRedirectURL, Scopes: []string{"openid", "email", "groups"},
		EmailClaim: "email", NameClaim: "name", GroupsClaim: "groups", AdminGroup: "admins",
	})
	idp.idTokenFor = func(code string) (string, string) {
		claims := idp.baseClaims("n1")
		claims["iat"] = time.Now().Add(-time.Hour).Unix()
		claims["exp"] = time.Now().Add(-time.Minute).Unix()
		return idp.sign(t, claims), ""
	}

	req := callbackRequest(h, "st1", "n1", "verifier1", "st1", "good-code")
	rec := httptest.NewRecorder()
	h.oidcCallback(rec, req)

	assertRejectedToLogin(t, rec)
}

func TestOIDCCallback_DeactivatedUserRejected(t *testing.T) {
	idp := newFakeIdP(t)
	h := newTestHandler(t, OIDCConfig{
		IssuerURL: idp.server.URL, ClientID: testClientID, ClientSecret: "secret",
		RedirectURL: testRedirectURL, Scopes: []string{"openid", "email", "groups"},
		EmailClaim: "email", NameClaim: "name", GroupsClaim: "groups", AdminGroup: "admins",
	})
	idp.idTokenFor = func(code string) (string, string) {
		return idp.sign(t, idp.baseClaims("n1")), ""
	}

	// First login JIT-provisions alice@example.com.
	req := callbackRequest(h, "st1", "n1", "verifier1", "st1", "good-code")
	rec := httptest.NewRecorder()
	h.oidcCallback(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("expected first login to succeed, got %d %s", rec.Code, rec.Header().Get("Location"))
	}
	parseIssuedJWT(t, h, rec) // asserts fc_token cookie set + redirect to /

	h.db.DB.Exec(`UPDATE users SET isActive = 0 WHERE email = ?`, "alice@example.com")

	req2 := callbackRequest(h, "st2", "n2", "verifier2", "st2", "good-code-2")
	idp.idTokenFor = func(code string) (string, string) {
		return idp.sign(t, idp.baseClaims("n2")), ""
	}
	rec2 := httptest.NewRecorder()
	h.oidcCallback(rec2, req2)

	assertRejectedToLogin(t, rec2)
}

func TestOIDCCallback_RoleResyncsWithoutTouchingDisplayName(t *testing.T) {
	idp := newFakeIdP(t)
	h := newTestHandler(t, OIDCConfig{
		IssuerURL: idp.server.URL, ClientID: testClientID, ClientSecret: "secret",
		RedirectURL: testRedirectURL, Scopes: []string{"openid", "email", "groups"},
		EmailClaim: "email", NameClaim: "name", GroupsClaim: "groups", AdminGroup: "admins",
	})

	idp.idTokenFor = func(code string) (string, string) {
		claims := idp.baseClaims("n1")
		claims["groups"] = []string{"fatura-users", "admins"}
		return idp.sign(t, claims), ""
	}
	req := callbackRequest(h, "st1", "n1", "verifier1", "st1", "good-code")
	rec := httptest.NewRecorder()
	h.oidcCallback(rec, req)
	parseIssuedJWT(t, h, rec)
	assertPlatformAdmin(t, h, "alice@example.com", 1, "expected first login")

	// Admin curates the display name locally between logins.
	h.db.DB.Exec(`UPDATE users SET displayName = ? WHERE email = ?`, "Alice (Finance)", "alice@example.com")

	// Seed a second admin so Alice is no longer the *last* active admin —
	// otherwise the last-admin demotion guard (provisionOrSyncUser) correctly
	// refuses to demote her via SSO role sync, and the resync below can't run.
	seedUser(t, h.db, "other-admin", "admin", 1)

	// Second login: removed from admins group at the IdP.
	idp.idTokenFor = func(code string) (string, string) {
		claims := idp.baseClaims("n2")
		claims["groups"] = []string{"fatura-users"}
		return idp.sign(t, claims), ""
	}
	req2 := callbackRequest(h, "st2", "n2", "verifier2", "st2", "good-code-2")
	rec2 := httptest.NewRecorder()
	h.oidcCallback(rec2, req2)
	parseIssuedJWT(t, h, rec2)
	assertPlatformAdmin(t, h, "alice@example.com", 0, "expected resync after group removal")

	var displayName string
	h.db.DB.Get(&displayName, `SELECT displayName FROM users WHERE email = ?`, "alice@example.com")
	if displayName != "Alice (Finance)" {
		t.Errorf("expected displayName to remain locally-curated value, got %q", displayName)
	}
}

// assertPlatformAdmin checks a provisioned user's isPlatformAdmin column
// directly — role/platform-admin status is never carried in the JWT itself
// (see api/middleware.go's Claims doc comment), so OIDC role-sync tests
// assert against the DB row provisionOrSyncUser wrote, not the issued token.
func assertPlatformAdmin(t *testing.T, h *handler, email string, want int, context string) {
	t.Helper()
	var got int
	if err := h.db.DB.Get(&got, `SELECT isPlatformAdmin FROM users WHERE email = ?`, email); err != nil {
		t.Fatalf("query provisioned user %s: %v", email, err)
	}
	if got != want {
		t.Errorf("%s: expected isPlatformAdmin=%d, got %d", context, want, got)
	}
}

func assertRejectedToLogin(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if rec.Code != http.StatusFound {
		t.Fatalf("expected a redirect on rejection, got %d", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.HasPrefix(loc, "/login?error=sso_failed") {
		t.Errorf("expected redirect to /login?error=sso_failed, got %s", loc)
	}
}

// TestOIDCEmailVerified_TypeHandling is F108's unit-level regression: the
// claim must be honored whether the IdP spells it as a JSON boolean or as the
// string "true"/"false" (case-insensitive), absent must stay "nothing to
// check", and a present-but-unparseable value must fail closed rather than
// silently skip the guard as the old bool-only assertion did.
func TestOIDCEmailVerified_TypeHandling(t *testing.T) {
	cases := []struct {
		name   string
		claims map[string]any
		want   bool
	}{
		{"absent", map[string]any{}, true},
		{"bool true", map[string]any{"email_verified": true}, true},
		{"bool false", map[string]any{"email_verified": false}, false},
		{"string true", map[string]any{"email_verified": "true"}, true},
		{"string false", map[string]any{"email_verified": "false"}, false},
		{"string TRUE uppercase", map[string]any{"email_verified": "TRUE"}, true},
		{"string False mixed case", map[string]any{"email_verified": "False"}, false},
		{"string padded true", map[string]any{"email_verified": " true "}, true},
		{"unparseable string", map[string]any{"email_verified": "yes"}, false},
		{"unexpected type", map[string]any{"email_verified": 1}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := oidcEmailVerified(tc.claims); got != tc.want {
				t.Errorf("oidcEmailVerified(%v) = %v, want %v", tc.claims["email_verified"], got, tc.want)
			}
		})
	}
}

// TestOIDCCallback_StringEmailVerifiedRejected is F108's integration half: a
// string "false" that the old bool assertion dropped must now block the
// callback, and a string "true" must not.
func TestOIDCCallback_StringEmailVerifiedRejected(t *testing.T) {
	idp := newFakeIdP(t)
	h := newTestHandler(t, OIDCConfig{
		IssuerURL: idp.server.URL, ClientID: testClientID, ClientSecret: "secret",
		RedirectURL: testRedirectURL, Scopes: []string{"openid", "email", "groups"},
		EmailClaim: "email", NameClaim: "name", GroupsClaim: "groups", AdminGroup: "admins",
	})

	idp.idTokenFor = func(code string) (string, string) {
		claims := idp.baseClaims("n1")
		claims["email_verified"] = "false"
		return idp.sign(t, claims), ""
	}

	req := callbackRequest(h, "st1", "n1", "verifier1", "st1", "good-code")
	rec := httptest.NewRecorder()
	h.oidcCallback(rec, req)

	assertRejectedToLogin(t, rec)
	for _, c := range rec.Result().Cookies() {
		if c.Name == authCookieName {
			t.Fatal("expected no auth cookie to be set for a string-false unverified email")
		}
	}
}

func TestOIDCCallback_StringEmailVerifiedAccepted(t *testing.T) {
	idp := newFakeIdP(t)
	h := newTestHandler(t, OIDCConfig{
		IssuerURL: idp.server.URL, ClientID: testClientID, ClientSecret: "secret",
		RedirectURL: testRedirectURL, Scopes: []string{"openid", "email", "groups"},
		EmailClaim: "email", NameClaim: "name", GroupsClaim: "groups", AdminGroup: "admins",
	})

	idp.idTokenFor = func(code string) (string, string) {
		claims := idp.baseClaims("n1")
		claims["email_verified"] = "true"
		return idp.sign(t, claims), ""
	}

	req := callbackRequest(h, "st1", "n1", "verifier1", "st1", "good-code")
	rec := httptest.NewRecorder()
	h.oidcCallback(rec, req)

	parseIssuedJWT(t, h, rec)
}

// TestEnsureOIDC_DiscoveryErrorThenSuccess pins F107's central guarantee: a
// discovery failure (here, a black-holing issuer that trips the bounded
// timeout) returns an error instead of wedging the goroutine, and does not
// permanently poison the cache — a later call against a healthy issuer
// succeeds. The timeout is shortened for the test so it doesn't wait the
// production 10s.
func TestEnsureOIDC_DiscoveryErrorThenSuccess(t *testing.T) {
	orig := oidcDiscoveryTimeout
	oidcDiscoveryTimeout = 200 * time.Millisecond
	t.Cleanup(func() { oidcDiscoveryTimeout = orig })

	// blockForever never responds to the discovery request; the bounded
	// context is the only thing that can end this call.
	blockForever := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	t.Cleanup(blockForever.Close)

	h := newTestHandler(t, OIDCConfig{
		IssuerURL: blockForever.URL, ClientID: testClientID, ClientSecret: "secret",
		RedirectURL: testRedirectURL, Scopes: []string{"openid", "email"},
		EmailClaim: "email", NameClaim: "name", GroupsClaim: "groups", AdminGroup: "admins",
	})

	done := make(chan error, 1)
	go func() {
		_, _, err := h.ensureOIDC()
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected discovery against a hung issuer to fail, got nil")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ensureOIDC did not return within the bounded timeout — goroutine is wedged")
	}

	// The failed attempt must not have cached anything: a subsequent call
	// against a healthy issuer succeeds. (h.oidcCfg is swapped directly here
	// as the test handler is single-goroutine at this point.)
	idp := newFakeIdP(t)
	h.oidcCfg.IssuerURL = idp.server.URL
	verifier, cfg, err := h.ensureOIDC()
	if err != nil {
		t.Fatalf("expected discovery against a healthy issuer to succeed after a prior failure, got %v", err)
	}
	if verifier == nil || cfg == nil {
		t.Fatal("expected non-nil verifier and oauth2 config after successful discovery")
	}
}

// TestEnsureOIDC_DoesNotHoldLockDuringDiscovery proves the discovery call no
// longer runs under oidcMu: while one goroutine is stuck in discovery against
// a hung issuer, another call that can be served from cache must not block.
func TestEnsureOIDC_DoesNotHoldLockDuringDiscovery(t *testing.T) {
	orig := oidcDiscoveryTimeout
	oidcDiscoveryTimeout = 2 * time.Second
	t.Cleanup(func() { oidcDiscoveryTimeout = orig })

	blockForever := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	t.Cleanup(blockForever.Close)

	h := newTestHandler(t, OIDCConfig{
		IssuerURL: blockForever.URL, ClientID: testClientID, ClientSecret: "secret",
		RedirectURL: testRedirectURL, Scopes: []string{"openid", "email"},
		EmailClaim: "email", NameClaim: "name", GroupsClaim: "groups", AdminGroup: "admins",
	})
	// Prime the cache directly so the second call takes the fast path.
	h.oidcVerifier = &oidc.IDTokenVerifier{}
	h.oidcOAuth2 = &oauth2.Config{}

	started := make(chan struct{})
	released := make(chan struct{})
	go func() {
		close(started)
		_, _, _ = h.ensureOIDC() // triggers the slow discovery
		close(released)
	}()
	<-started

	// Give the discovery goroutine a moment to actually enter the HTTP call.
	time.Sleep(50 * time.Millisecond)

	cached := make(chan error, 1)
	go func() {
		_, _, err := h.ensureOIDC() // fast path — must not wait on discovery
		cached <- err
	}()
	select {
	case err := <-cached:
		if err != nil {
			t.Fatalf("expected cached fast path to succeed, got %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("cached ensureOIDC blocked while discovery was in flight — lock still held across the network call")
	}
	<-released
}
