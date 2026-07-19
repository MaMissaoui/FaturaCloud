package api

import (
	"context"
	"net/http"

	"github.com/golang-jwt/jwt/v5"
)

type contextKey string

const claimsKey contextKey = "claims"

// jwtIssuer and jwtAudience bind every issued token to this application, so a
// token minted for (or replayed from) some other service that happens to share
// the JWT secret is rejected. They are set on issue (api/auth.go) and enforced
// on parse in authMiddleware.
const (
	jwtIssuer   = "faturacloud"
	jwtAudience = "faturacloud"
)

// authCookieName is the httpOnly cookie the JWT rides in. httpOnly means
// page JavaScript can't read it, so an XSS payload can't exfiltrate the
// session token the way it could from localStorage.
const authCookieName = "fc_token"

// csrfHeaderName is a custom request header the frontend sends on every
// state-changing request. Because the auth token now travels in a cookie
// (sent automatically by the browser), a cross-site page could otherwise
// forge authenticated requests; requiring a custom header defeats that, since
// cross-site JavaScript can't set custom headers without a CORS preflight this
// server never grants. SameSite=Lax on the cookie is the primary defense; this
// is defense-in-depth. Its value is irrelevant — only its presence matters.
const csrfHeaderName = "X-CSRF-Protection"

type Claims struct {
	UserID string `json:"userId"`
	Email  string `json:"email"`
	Role   string `json:"role"`
	// Provider records how this token was issued ("local" or "oidc"). It must
	// round-trip through the signed JWT itself (not just live in memory at
	// issue time) since /api/auth/me reads it back from a re-parsed token on
	// a later request — a json:"-" tag here would silently make every token
	// look like "local" after the first parse.
	Provider string `json:"authProvider"`
	jwt.RegisteredClaims
}

func (h *handler) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(authCookieName)
		if err != nil || cookie.Value == "" {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		tokenStr := cookie.Value
		claims := &Claims{}
		_, err = jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
			return []byte(h.jwtSecret), nil
		}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithIssuer(jwtIssuer), jwt.WithAudience(jwtAudience))
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		// Re-check the user on every request rather than trusting the JWT
		// alone — otherwise deactivating or deleting a user leaves their
		// token usable for up to its full 24h lifetime. Acquired and
		// released here (not deferred past next.ServeHTTP) so this never
		// nests under a route's own withDB read lock (api/router.go).
		h.dbMu.RLock()
		var isActive int
		err = h.db.DB.Get(&isActive, `SELECT isActive FROM users WHERE id = ?`, claims.UserID)
		h.dbMu.RUnlock()
		if err != nil || isActive == 0 {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		ctx := context.WithValue(r.Context(), claimsKey, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (h *handler) adminOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := getClaims(r)
		if claims == nil || claims.Role != "admin" {
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// csrfRequired wraps state-changing routes and rejects any request that omits
// the CSRF header (see csrfHeaderName). Safe methods (GET/HEAD/OPTIONS) pass
// through untouched — every such route in this app is a read, and browser
// navigations can't set custom headers anyway.
func (h *handler) csrfRequired(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
		default:
			if r.Header.Get(csrfHeaderName) == "" {
				writeError(w, http.StatusForbidden, "missing CSRF header")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func getClaims(r *http.Request) *Claims {
	c, _ := r.Context().Value(claimsKey).(*Claims)
	return c
}
