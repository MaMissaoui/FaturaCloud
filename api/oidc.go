package api

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// OIDCConfig holds the settings needed to enable OIDC SSO login against
// Authelia or any other standards-compliant OIDC provider. An empty
// IssuerURL disables the feature entirely — no route becomes reachable and
// no behavior changes for local login.
type OIDCConfig struct {
	IssuerURL    string
	ClientID     string
	ClientSecret string
	RedirectURL  string
	Scopes       []string
	EmailClaim   string
	NameClaim    string
	GroupsClaim  string
	AdminGroup   string
}

var errOIDCNotConfigured = errors.New("oidc is not configured")

const (
	oidcStateCookieName = "oidc_state"
	oidcStateCookieTTL  = 5 * 60 // seconds
)

// oidcStateCookiePayload is round-tripped through a signed, HttpOnly cookie
// between the login redirect and the callback — it never touches the
// database or any server-side store.
type oidcStateCookiePayload struct {
	State        string `json:"state"`
	Nonce        string `json:"nonce"`
	CodeVerifier string `json:"codeVerifier"`
}

// ensureOIDC lazily builds the provider/verifier/oauth2 config from the
// issuer's discovery document. Lazy + retried (rather than only attempted
// once at startup) so a temporarily-unreachable IdP doesn't permanently
// disable SSO until the process restarts — local login is never affected
// either way.
func (h *handler) ensureOIDC() (*oidc.IDTokenVerifier, *oauth2.Config, error) {
	if h.oidcCfg.IssuerURL == "" {
		return nil, nil, errOIDCNotConfigured
	}

	h.oidcMu.Lock()
	defer h.oidcMu.Unlock()

	if h.oidcVerifier != nil && h.oidcOAuth2 != nil {
		return h.oidcVerifier, h.oidcOAuth2, nil
	}

	provider, err := oidc.NewProvider(context.Background(), h.oidcCfg.IssuerURL)
	if err != nil {
		return nil, nil, err
	}

	h.oidcVerifier = provider.Verifier(&oidc.Config{ClientID: h.oidcCfg.ClientID})
	h.oidcOAuth2 = &oauth2.Config{
		ClientID:     h.oidcCfg.ClientID,
		ClientSecret: h.oidcCfg.ClientSecret,
		RedirectURL:  h.oidcCfg.RedirectURL,
		Endpoint:     provider.Endpoint(),
		Scopes:       h.oidcCfg.Scopes,
	}
	return h.oidcVerifier, h.oidcOAuth2, nil
}

func (h *handler) oidcEnabled(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"enabled": h.oidcCfg.IssuerURL != ""})
}

// oidcLoginStart begins the Authorization Code + PKCE flow: generate
// state/nonce/verifier, stash them in a signed cookie, redirect to the IdP.
func (h *handler) oidcLoginStart(w http.ResponseWriter, r *http.Request) {
	_, oauth2Cfg, err := h.ensureOIDC()
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "sso is not available")
		return
	}

	state, err := randomOIDCString()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not start sso login")
		return
	}
	nonce, err := randomOIDCString()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not start sso login")
		return
	}
	verifier := oauth2.GenerateVerifier()

	h.setOIDCStateCookie(w, r, oidcStateCookiePayload{State: state, Nonce: nonce, CodeVerifier: verifier})

	authURL := oauth2Cfg.AuthCodeURL(state,
		oidc.Nonce(nonce),
		oauth2.S256ChallengeOption(verifier),
	)
	http.Redirect(w, r, authURL, http.StatusFound)
}

// oidcCallback completes the flow: validate state/nonce, exchange the code,
// verify the ID token's signature/issuer/audience, extract claims, JIT
// provision or role-sync the local user, and mint the same JWT local login
// issues. Any failure at any step redirects to the login page with a generic
// error — details go to the server log only, never to the client.
func (h *handler) oidcCallback(w http.ResponseWriter, r *http.Request) {
	fail := func(reason string) {
		log.Printf("oidc callback rejected: %s", reason)
		http.Redirect(w, r, "/login?error=sso_failed", http.StatusFound)
	}

	verifier, oauth2Cfg, err := h.ensureOIDC()
	if err != nil {
		fail("oidc not configured: " + err.Error())
		return
	}

	stored, ok := h.readAndClearOIDCStateCookie(w, r)
	if !ok {
		fail("missing or invalid state cookie")
		return
	}

	query := r.URL.Query()
	if idpErr := query.Get("error"); idpErr != "" {
		fail("idp returned error: " + idpErr)
		return
	}
	if query.Get("state") != stored.State {
		fail("state mismatch")
		return
	}
	code := query.Get("code")
	if code == "" {
		fail("missing code")
		return
	}

	ctx := r.Context()
	token, err := oauth2Cfg.Exchange(ctx, code, oauth2.VerifierOption(stored.CodeVerifier))
	if err != nil {
		fail("code exchange failed: " + err.Error())
		return
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		fail("token response had no id_token")
		return
	}
	idToken, err := verifier.Verify(ctx, rawIDToken)
	if err != nil {
		fail("id token verification failed: " + err.Error())
		return
	}
	if idToken.Nonce != stored.Nonce {
		fail("nonce mismatch")
		return
	}

	var claims map[string]any
	if err := idToken.Claims(&claims); err != nil {
		fail("could not parse id token claims")
		return
	}

	email, _ := claims[h.oidcCfg.EmailClaim].(string)
	email = strings.TrimSpace(email)
	if email == "" {
		fail("id token had no usable email claim")
		return
	}
	name, _ := claims[h.oidcCfg.NameClaim].(string)
	isAdmin := oidcClaimHasGroup(claims, h.oidcCfg.GroupsClaim, h.oidcCfg.AdminGroup)

	user, err := h.provisionOrSyncUser(email, name, isAdmin)
	if err != nil {
		fail("provisioning failed: " + err.Error())
		return
	}

	jwtToken, err := h.issueTokenWithProvider(user, "oidc")
	if err != nil {
		fail("could not issue token")
		return
	}

	// Deliver the session the same way local login does — an httpOnly cookie —
	// then redirect straight to the app. The token never appears in the URL
	// (no fragment, no query), so it can't leak via history, and page JS never
	// sees it. Set-Cookie on this cross-site redirect response works fine; it's
	// the browser sending an existing cookie cross-site that SameSite governs.
	setAuthCookie(w, r, jwtToken)
	http.Redirect(w, r, "/", http.StatusFound)
}

// oidcClaimHasGroup checks a claims map's group-list claim (a flat array of
// strings, e.g. Authelia's "groups") for an exact match against wantGroup.
// Nested claim shapes (e.g. Keycloak's realm_access.roles) aren't resolved by
// this simple lookup — those providers need an IdP-side mapper that flattens
// the claim before FaturaCloud can read it.
func oidcClaimHasGroup(claims map[string]any, groupsClaim, wantGroup string) bool {
	raw, ok := claims[groupsClaim].([]any)
	if !ok {
		return false
	}
	for _, g := range raw {
		if gs, ok := g.(string); ok && strings.TrimSpace(gs) == wantGroup {
			return true
		}
	}
	return false
}

func randomOIDCString() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func (h *handler) setOIDCStateCookie(w http.ResponseWriter, r *http.Request, payload oidcStateCookiePayload) {
	raw, _ := json.Marshal(payload)
	http.SetCookie(w, &http.Cookie{
		Name:     oidcStateCookieName,
		Value:    signOIDCCookie(h.jwtSecret, raw),
		Path:     "/api/auth/oidc",
		MaxAge:   oidcStateCookieTTL,
		HttpOnly: true,
		Secure:   isHTTPS(r),
		SameSite: http.SameSiteLaxMode,
	})
}

// readAndClearOIDCStateCookie consumes the state cookie — it is single-use,
// so it's cleared here regardless of whether validation below succeeds.
func (h *handler) readAndClearOIDCStateCookie(w http.ResponseWriter, r *http.Request) (oidcStateCookiePayload, bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     oidcStateCookieName,
		Value:    "",
		Path:     "/api/auth/oidc",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   isHTTPS(r),
		SameSite: http.SameSiteLaxMode,
	})

	cookie, err := r.Cookie(oidcStateCookieName)
	if err != nil {
		return oidcStateCookiePayload{}, false
	}
	payload, err := verifyOIDCCookie(h.jwtSecret, cookie.Value)
	if err != nil {
		return oidcStateCookiePayload{}, false
	}
	return payload, true
}

// signOIDCCookie/verifyOIDCCookie use the app's existing JWT secret to HMAC
// the state/nonce/PKCE-verifier payload, so a client can't forge or tamper
// with it between the login redirect and the callback.
func signOIDCCookie(secret string, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	sig := mac.Sum(nil)
	return base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func verifyOIDCCookie(secret, cookieVal string) (oidcStateCookiePayload, error) {
	parts := strings.SplitN(cookieVal, ".", 2)
	if len(parts) != 2 {
		return oidcStateCookiePayload{}, errors.New("malformed oidc state cookie")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return oidcStateCookiePayload{}, err
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return oidcStateCookiePayload{}, err
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return oidcStateCookiePayload{}, errors.New("oidc state cookie signature mismatch")
	}
	var v oidcStateCookiePayload
	if err := json.Unmarshal(payload, &v); err != nil {
		return oidcStateCookiePayload{}, err
	}
	return v, nil
}

// isHTTPS reports whether the original client request was HTTPS, accounting
// for TLS termination happening upstream (Traefik/NPM) — the Go server
// itself is plain HTTP behind that boundary in every real deployment.
func isHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}
