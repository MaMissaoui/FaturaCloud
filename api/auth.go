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

// clientIP returns the address to key login rate-limiting on. By default
// (h.trustedProxies empty) it's always the direct TCP peer — safe with no
// reverse proxy in front, but every client sharing that proxy then shares one
// bucket. When the peer matches a configured trusted proxy, the real client
// address is instead read from the leftmost X-Forwarded-For entry, which
// only that proxy is trusted to have set truthfully; an untrusted peer can
// send any X-Forwarded-For value it likes, so this only takes effect once
// the peer itself is verified.
func (h *handler) clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if len(h.trustedProxies) == 0 {
		return host
	}
	peer, err := netip.ParseAddr(host)
	if err != nil {
		return host
	}
	trusted := false
	for _, p := range h.trustedProxies {
		if p.Contains(peer) {
			trusted = true
			break
		}
	}
	if !trusted {
		return host
	}
	xff := r.Header.Get("X-Forwarded-For")
	if xff == "" {
		return host
	}
	real := strings.TrimSpace(strings.SplitN(xff, ",", 2)[0])
	if real == "" {
		return host
	}
	return real
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

	writeJSON(w, http.StatusOK, map[string]any{
		"token": token,
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

func (h *handler) logout(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"message": "logged out"})
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
		UserID:   user.ID,
		Email:    user.Email,
		Role:     user.Role,
		Provider: provider,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    jwtIssuer,
			Audience:  jwt.ClaimStrings{jwtAudience},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(h.jwtSecret))
}
