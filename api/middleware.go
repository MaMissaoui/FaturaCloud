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
	// Provider records how this token was issued ("local" or "oidc"). It must
	// round-trip through the signed JWT itself (not just live in memory at
	// issue time) since /api/auth/me reads it back from a re-parsed token on
	// a later request — a json:"-" tag here would silently make every token
	// look like "local" after the first parse.
	Provider string `json:"authProvider"`
	// IsPlatformAdmin is never part of the signed JWT payload (no json tag
	// needed; jwt.ParseWithClaims never touches unexported... it IS
	// exported, but simply isn't set by token parsing) — it's looked up
	// fresh from the DB on every request by authMiddleware, alongside the
	// existing isActive check, so a revoked platform-admin flag takes effect
	// on the very next request rather than staying baked into a token for
	// up to its full 24h lifetime. Org membership/role works the same way
	// (see GetOrganizationRole) — nothing authorization-relevant survives in
	// the token itself beyond identity.
	IsPlatformAdmin bool `json:"-"`
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
		// alone — otherwise deactivating or deleting a user (or revoking
		// platform-admin) leaves their token effective for up to its full
		// 24h lifetime. Acquired and released here (not deferred past
		// next.ServeHTTP) so this never nests under a route's own withDB
		// read lock (api/router.go).
		h.dbMu.RLock()
		var isActive, isPlatformAdmin int
		err = h.db.DB.QueryRow(`SELECT isActive, isPlatformAdmin FROM users WHERE id = ?`, claims.UserID).
			Scan(&isActive, &isPlatformAdmin)
		h.dbMu.RUnlock()
		if err != nil || isActive == 0 {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		claims.IsPlatformAdmin = isPlatformAdmin != 0

		ctx := context.WithValue(r.Context(), claimsKey, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// platformAdmin gates the handful of genuinely global routes that have no
// natural per-org owner (user account management, backups, DB restore,
// countries) — org-scoped admin actions use orgAdmin instead.
func (h *handler) platformAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := getClaims(r)
		if claims == nil || !claims.IsPlatformAdmin {
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// orgIDResolver extracts the organization id a request targets. Most
// org-scoped admin routes have it directly as a path value; a few (like
// closing a fiscal year, addressed by the fiscal year's own id) need a DB
// lookup first.
type orgIDResolver func(r *http.Request) (string, error)

// pathOrgID returns an orgIDResolver that reads the organization id straight
// from a named path value — the common case.
func pathOrgID(param string) orgIDResolver {
	return func(r *http.Request) (string, error) {
		return r.PathValue(param), nil
	}
}

// orgAdmin gates an org-scoped admin action: the caller must hold the
// "admin" role in the organization resolve identifies. Runs before withDB
// (see api/router.go's route wiring), so — like authMiddleware's isActive
// check — it takes its own short-lived dbMu read lock rather than relying on
// a route's own withDB wrapper.
func (h *handler) orgAdmin(resolve orgIDResolver) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := getClaims(r)
			if claims == nil {
				writeError(w, http.StatusForbidden, "forbidden")
				return
			}
			// resolve() may itself hit the DB (e.g. looking up which
			// organization a fiscal year belongs to) — held under the same
			// lock as the role check below, not just the role check alone,
			// since it runs before withDB's own read lock ever engages.
			h.dbMu.RLock()
			orgID, resolveErr := resolve(r)
			var role string
			var isMember bool
			var err error
			if resolveErr == nil && orgID != "" {
				role, isMember, err = h.db.GetOrganizationRole(orgID, claims.UserID)
			}
			h.dbMu.RUnlock()
			if resolveErr != nil || orgID == "" {
				writeError(w, http.StatusNotFound, "not found")
				return
			}
			if err != nil {
				writeInternalError(w, err)
				return
			}
			if !isMember || role != "admin" {
				writeError(w, http.StatusForbidden, "forbidden")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
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
