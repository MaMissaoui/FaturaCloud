package api

import (
	"log"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

type loginBucket struct {
	count     int
	windowEnd time.Time
}

const (
	loginMaxAttempts = 10
	loginWindow      = time.Minute

	// maxLoginBucketEntries bounds each rate-limit map (IP and email) so a
	// distributed attacker spraying requests across many thousands of
	// unique IPs/emails can't grow it without limit in the window between
	// sweepLoginBuckets runs. Defense-in-depth on top of the sweep, not a
	// replacement for it.
	maxLoginBucketEntries = 10000
)

var (
	loginMu sync.Mutex
	// loginBuckets is keyed on source IP; loginEmailBuckets on the lowercased
	// email. Both share the same window/limit. Keying only on IP lets a botnet
	// rotating source addresses grind one account unthrottled; keying only on
	// email lets one noisy IP lock every account out — so both apply.
	loginBuckets      = map[string]*loginBucket{}
	loginEmailBuckets = map[string]*loginBucket{}
)

// checkRate enforces loginMaxAttempts per loginWindow for one key in the given
// bucket map. It takes loginMu itself.
func checkRate(buckets map[string]*loginBucket, key string) bool {
	loginMu.Lock()
	defer loginMu.Unlock()
	b, ok := buckets[key]
	if !ok || time.Now().After(b.windowEnd) {
		if !ok && len(buckets) >= maxLoginBucketEntries {
			// The map is already at its bound and this is a brand new key —
			// from this vantage point that's indistinguishable from a
			// distributed attack, so fail closed (rate-limited) rather than
			// grow the map further.
			return false
		}
		buckets[key] = &loginBucket{count: 1, windowEnd: time.Now().Add(loginWindow)}
		return true
	}
	b.count++
	return b.count <= loginMaxAttempts
}

// sweepLoginBuckets periodically evicts expired rate-limit entries so neither
// bucket map grows unbounded as distinct IPs/emails attempt to log in.
func sweepLoginBuckets() {
	for {
		time.Sleep(loginWindow)
		loginMu.Lock()
		now := time.Now()
		for _, buckets := range []map[string]*loginBucket{loginBuckets, loginEmailBuckets} {
			for key, b := range buckets {
				if now.After(b.windowEnd) {
					delete(buckets, key)
				}
			}
		}
		loginMu.Unlock()
	}
}

// isTrustedProxyAddr reports whether addr falls inside any configured trusted
// proxy prefix. Shared by IsTrustedProxyPeer (which parses the direct peer
// out of RemoteAddr first) and clientIP (which tests each X-Forwarded-For hop
// against it) so the two can't drift on what "trusted" means.
func isTrustedProxyAddr(addr netip.Addr, trustedProxies []netip.Prefix) bool {
	for _, p := range trustedProxies {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

// IsTrustedProxyPeer reports whether the request's direct TCP peer matches
// one of the configured trusted-proxy prefixes. Shared by clientIP (decides
// whether to honor X-Forwarded-For) and IsHTTPS below (decides whether to
// honor X-Forwarded-Proto) — both headers are only as trustworthy as the
// peer that's allowed to set them, and an empty trustedProxies list means
// nobody is trusted, matching the "only the direct peer" default.
func IsTrustedProxyPeer(r *http.Request, trustedProxies []netip.Prefix) bool {
	if len(trustedProxies) == 0 {
		return false
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	peer, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	return isTrustedProxyAddr(peer, trustedProxies)
}

// IsHTTPS reports whether the original client request was HTTPS, accounting
// for TLS termination upstream. F64 (2026-08-13 audit): X-Forwarded-Proto is
// only honored from a trusted proxy peer — the same trust boundary
// TRUSTED_PROXIES already draws for X-Forwarded-For (clientIP above).
// Before this fix, any peer able to reach the app directly (misconfigured
// proxy, or the app exposed without one) could set Secure cookies/HSTS on a
// plain-HTTP deployment just by sending the header, locking clients out of
// their session.
func IsHTTPS(r *http.Request, trustedProxies []netip.Prefix) bool {
	if r.TLS != nil {
		return true
	}
	if !IsTrustedProxyPeer(r, trustedProxies) {
		return false
	}
	return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

// clientIP returns the address to key login rate-limiting on. By default
// (h.trustedProxies empty) it's always the direct TCP peer — safe with no
// reverse proxy in front, but every client sharing that proxy then shares one
// bucket. When the peer matches a configured trusted proxy, the real client
// address is read from X-Forwarded-For; an untrusted peer can send any value
// it likes, so this only takes effect once the peer itself is verified.
//
// F110 (2026-08-13 audit): the X-Forwarded-For list is walked from the RIGHT,
// skipping any entry that is itself a trusted proxy, and the first remaining
// (non-trusted) address is returned. The old leftmost-entry read was
// attacker-controlled: nginx (and most proxies) *append* to an incoming
// X-Forwarded-For rather than replacing it, so a client sending
// "X-Forwarded-For: 1.2.3.4" produced "1.2.3.4, <real client>" and the
// spoofed 1.2.3.4 became the rate-limit key — letting one attacker rotate
// fake keys to dodge the limiter while a legitimate client's real address was
// never seen. Which of the trailing hops are proxies is unavoidably a guess
// (X-Forwarded-For can't distinguish a client from a proxy), so the trusted
// list bounds the guess: every trusted hop on the way in is skipped, and the
// first address after them is as close to the real client as this header
// permits. Falls back to the direct peer when the header is absent, empty, or
// contains nothing but trusted proxies.
func (h *handler) clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if !IsTrustedProxyPeer(r, h.trustedProxies) {
		return host
	}
	xff := r.Header.Get("X-Forwarded-For")
	if xff == "" {
		return host
	}
	parts := strings.Split(xff, ",")
	for i := len(parts) - 1; i >= 0; i-- {
		candidate := strings.TrimSpace(parts[i])
		if candidate == "" {
			continue
		}
		// An unparseable entry can't be a trusted proxy, so it's returned as
		// the client — the same "first non-trusted value wins" rule.
		if addr, parseErr := netip.ParseAddr(candidate); parseErr == nil && isTrustedProxyAddr(addr, h.trustedProxies) {
			continue
		}
		return candidate
	}
	return host
}

// dummyPasswordHash is a fixed bcrypt hash that no real password will ever
// match. login compares against it when the email lookup misses, so a
// bcrypt.CompareHashAndPassword call runs on both the found and not-found
// paths — otherwise response time leaks whether an email is registered.
var dummyPasswordHash = mustBcryptHash("not-a-real-password-timing-decoy-only")

func mustBcryptHash(password string) string {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		panic(err)
	}
	return string(hash)
}

func (h *handler) login(w http.ResponseWriter, r *http.Request) {
	ip := h.clientIP(r)
	if !checkRate(loginBuckets, ip) {
		writeError(w, http.StatusTooManyRequests, "too many login attempts — try again in a minute")
		return
	}

	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		return
	}

	// Also throttle per account so IP rotation can't grind a single email.
	// Same 429 message as the IP limit — no signal about which limit tripped.
	if body.Email != "" && !checkRate(loginEmailBuckets, strings.ToLower(body.Email)) {
		writeError(w, http.StatusTooManyRequests, "too many login attempts — try again in a minute")
		return
	}

	h.dbMu.RLock()
	var user userRow
	err := h.db.DB.Get(&user, `SELECT * FROM users WHERE email = ? AND isActive = 1`, body.Email)
	h.dbMu.RUnlock()
	if err != nil {
		// Compare against a decoy hash so this path costs the same as a real
		// mismatch (see dummyPasswordHash) instead of returning instantly.
		bcrypt.CompareHashAndPassword([]byte(dummyPasswordHash), []byte(body.Password))
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(body.Password)); err != nil {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	token, err := h.issueToken(user)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not issue token")
		return
	}

	h.dbMu.RLock()
	_, lastLoginErr := h.db.DB.Exec(`UPDATE users SET lastLoginAt = ? WHERE id = ?`, time.Now().UnixMilli(), user.ID)
	h.dbMu.RUnlock()
	if lastLoginErr != nil {
		// Non-fatal: the login itself succeeded, only the bookkeeping failed.
		log.Printf("login: failed to update lastLoginAt for user %s: %v", user.ID, lastLoginErr)
	}

	// The token rides in an httpOnly cookie, never the response body — page
	// JavaScript never sees it, so XSS can't steal the session.
	h.setAuthCookie(w, r, token)
	writeJSON(w, http.StatusOK, map[string]any{
		"user": map[string]any{
			"id":           user.ID,
			"email":        user.Email,
			"displayName":  user.DisplayName,
			"role":         user.Role,
			"isActive":     user.IsActive,
			"authProvider": "local",
		},
	})
}

func (h *handler) logout(w http.ResponseWriter, r *http.Request) {
	// F109: logout revokes every session for the user, not just this cookie.
	// The cookie is httpOnly so the browser (and page JS) can't clear another
	// copy of it, and a JWT is otherwise valid until its 24h exp — bumping
	// users.tokenVersion makes every previously-issued token fail
	// authMiddleware's claims.TokenVersion != stored check on its next use.
	// logout is intentionally NOT behind authMiddleware (the route has to
	// work for an expired/invalid cookie too), so parse the token here,
	// best-effort: an absent or invalid token simply has nothing to revoke.
	if cookie, err := r.Cookie(authCookieName); err == nil && cookie.Value != "" {
		claims := &Claims{}
		if _, err := jwt.ParseWithClaims(cookie.Value, claims, func(t *jwt.Token) (any, error) {
			return []byte(h.jwtSecret), nil
		}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithIssuer(jwtIssuer), jwt.WithAudience(jwtAudience)); err == nil && claims.UserID != "" {
			h.dbMu.RLock()
			_, _ = h.db.DB.Exec(`UPDATE users SET tokenVersion = tokenVersion + 1 WHERE id = ?`, claims.UserID)
			h.dbMu.RUnlock()
		}
	}
	// The cookie is httpOnly, so the client can't clear it itself — the server
	// must expire it here.
	h.clearAuthCookie(w, r)
	writeJSON(w, http.StatusOK, map[string]string{"message": "logged out"})
}

const authCookieMaxAge = int(24 * 60 * 60) // seconds; matches the JWT's 24h exp

// setAuthCookie writes the session JWT as an httpOnly, SameSite=Lax cookie.
// Secure is set only when the original request arrived over HTTPS (IsHTTPS),
// so it still works for a plain-HTTP LAN deployment. SameSite=Lax lets the
// cookie ride top-level GET navigations (deep links, the OIDC return) while
// withholding it from cross-site subrequests and non-GET requests — the CSRF
// primary defense.
func (h *handler) setAuthCookie(w http.ResponseWriter, r *http.Request, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     authCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   IsHTTPS(r, h.trustedProxies),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   authCookieMaxAge,
	})
}

// clearAuthCookie expires the session cookie (same attributes, MaxAge<0).
func (h *handler) clearAuthCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     authCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   IsHTTPS(r, h.trustedProxies),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

func (h *handler) me(w http.ResponseWriter, r *http.Request) {
	claims := getClaims(r)
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var user userRow
	err := h.db.DB.Get(&user, `SELECT * FROM users WHERE id = ?`, claims.UserID)
	if err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	resp := userToJSON(user)
	provider := claims.Provider
	if provider == "" {
		provider = "local"
	}
	resp["authProvider"] = provider
	writeJSON(w, http.StatusOK, resp)
}

func (h *handler) issueToken(user userRow) (string, error) {
	return h.issueTokenWithProvider(user, "local")
}

func (h *handler) issueTokenWithProvider(user userRow, provider string) (string, error) {
	claims := Claims{
		UserID:       user.ID,
		Email:        user.Email,
		Provider:     provider,
		TokenVersion: user.TokenVersion,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    jwtIssuer,
			Audience:  jwt.ClaimStrings{jwtAudience},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(h.jwtSecret))
}
