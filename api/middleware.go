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

// orgAuthorized is the shared implementation orgAdmin and orgMember both
// call. Runs before withDB (see api/router.go's route wiring), so — like
// authMiddleware's isActive check — it takes its own short-lived dbMu read
// lock around resolve()+the role check (resolve() may itself hit the DB,
// e.g. looking up which organization a fiscal year belongs to) rather than
// relying on a route's own withDB wrapper, released before next.ServeHTTP.
//
// orgAdmin and orgMember deliberately diverge on the failure response:
// orgAdmin 404s only when resolve() itself fails (the row is genuinely
// absent) and 403s when the caller isn't an admin of an org they already
// know exists — fine for admin-tier routes, which the caller is almost
// always already inside via pathOrgID. orgMember instead collapses BOTH
// "resolve failed" and "caller isn't a member" to a plain 404: most of its
// resolvers key off an attacker-controlled resource {id} unrelated to any
// path the caller is otherwise authorized into, so a distinct 403 there
// would let any authenticated user learn "this invoice id exists (in
// someone else's org)" — a cross-tenant existence oracle. This is a
// deliberate choice, not an inherited accident.
func (h *handler) orgAuthorized(resolve orgIDResolver, requireAdmin bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := getClaims(r)
			if claims == nil {
				writeError(w, http.StatusForbidden, "forbidden")
				return
			}
			h.dbMu.RLock()
			orgID, resolveErr := resolve(r)
			var role string
			var isMember bool
			var err error
			if resolveErr == nil && orgID != "" {
				role, isMember, err = h.db.GetOrganizationRole(orgID, claims.UserID)
			}
			h.dbMu.RUnlock()

			if requireAdmin {
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
			} else {
				if err != nil {
					writeInternalError(w, err)
					return
				}
				if resolveErr != nil || orgID == "" || !isMember {
					writeError(w, http.StatusNotFound, "not found")
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// orgAdmin gates an org-scoped admin action: the caller must hold the
// "admin" role in the organization resolve identifies.
func (h *handler) orgAdmin(resolve orgIDResolver) func(http.Handler) http.Handler {
	return h.orgAuthorized(resolve, true)
}

// orgMember gates an ordinary org-scoped action on plain membership (any
// role) — the Phase C counterpart to orgAdmin, for routes that don't
// require admin privileges, just that the caller belongs to the
// organization resolve identifies.
func (h *handler) orgMember(resolve orgIDResolver) func(http.Handler) http.Handler {
	return h.orgAuthorized(resolve, false)
}

// requireOrgMember checks org membership from INSIDE a Create* handler,
// after decodeJSON — orgIDResolver-based middleware can't run before the
// body is parsed, since organizationId lives in the JSON, not the path.
//
// Unlike orgMember (which runs before withDB and takes its own
// dbMu.RLock()), every Create* handler this is called from is already
// running inside protected()'s withDB wrapper, which holds dbMu.RLock() for
// the request's full duration (api/router.go). This must NOT call RLock()
// again: Go's sync.RWMutex blocks new readers once a writer (e.g.
// /api/restore's write-locked swapDatabase) is queued, so a second RLock()
// from the same goroutine after that point deadlocks. Just use h.db
// directly — the enclosing withDB already makes it safe.
//
// 403 here, not 404 (unlike orgMember): the caller supplied organizationId
// themselves in the body, so there's no existence-probing angle — a plain
// validation-style rejection is the right shape for a bad create payload.
func (h *handler) requireOrgMember(w http.ResponseWriter, r *http.Request, orgID string) bool {
	claims := getClaims(r)
	if claims == nil || orgID == "" {
		writeError(w, http.StatusForbidden, "forbidden")
		return false
	}
	_, isMember, err := h.db.GetOrganizationRole(orgID, claims.UserID)
	if err != nil {
		writeInternalError(w, err)
		return false
	}
	if !isMember {
		writeError(w, http.StatusForbidden, "forbidden")
		return false
	}
	return true
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
