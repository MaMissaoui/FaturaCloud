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
	// TokenVersion is the users.tokenVersion value at issue time (F109).
	// Unlike IsPlatformAdmin it MUST round-trip through the signed JWT (it
	// carries a json tag, not json:"-", for the same reason Provider does):
	// authMiddleware can only compare "the version this token was minted
	// with" against "the version stored now" if the minted value is actually
	// in the token. Logout and password change bump the stored column, which
	// makes every token carrying an older value fail this check — revoking
	// all of that user's sessions at once.
	TokenVersion int `json:"tokenVersion"`
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
		// 24h lifetime. The stored tokenVersion is re-read here too (F109):
		// logout and password change bump it, so every token minted before
		// that bump now fails the claims.TokenVersion != tokenVersion check
		// below. Acquired and released here (not deferred past
		// next.ServeHTTP) so this never nests under a route's own withDB
		// read lock (api/router.go).
		h.dbMu.RLock()
		var isActive, isPlatformAdmin, tokenVersion int
		err = h.db.DB.QueryRow(`SELECT isActive, isPlatformAdmin, tokenVersion FROM users WHERE id = ?`, claims.UserID).
			Scan(&isActive, &isPlatformAdmin, &tokenVersion)
		h.dbMu.RUnlock()
		if err != nil || isActive == 0 || claims.TokenVersion != tokenVersion {
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

// orgAuthMode selects orgAuthorized's failure-response shape — see its own
// doc comment for why the two differ.
type orgAuthMode int

const (
	// modeMemberCollapse collapses BOTH "resolve failed" and "caller isn't
	// a member" to a plain 404: used by resolvers keyed off an
	// attacker-controlled resource {id} (an invoice, a vendor, …), where a
	// distinct 403 would let any authenticated user learn "this id exists,
	// just in someone else's org" — a cross-tenant existence oracle.
	modeMemberCollapse orgAuthMode = iota
	// modeAdminStrict 404s only when resolve() itself fails (the row is
	// genuinely absent) and 403s otherwise — fine for routes the caller is
	// almost always already "inside" via pathOrgID, where existence isn't
	// secret.
	modeAdminStrict
)

// orgAuthorized is the shared implementation orgAdmin/orgMember/orgRole/
// orgRoleAdmin all build on. Runs before withDB (see api/router.go's route
// wiring), so — like authMiddleware's isActive check — it takes its own
// short-lived dbMu read lock around resolve()+the role check (resolve() may
// itself hit the DB, e.g. looking up which organization a fiscal year
// belongs to) rather than relying on a route's own withDB wrapper, released
// before next.ServeHTTP.
//
// allowed decides, given the resolved (role, isMember), whether the request
// may proceed — see mode's doc comment for how a "no" there turns into a
// 403 vs. a 404 depending on which resolver shape the caller is protecting.
func (h *handler) orgAuthorized(resolve orgIDResolver, mode orgAuthMode, allowed func(role string, isMember bool) bool) func(http.Handler) http.Handler {
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

			if err != nil {
				writeInternalError(w, err)
				return
			}

			switch mode {
			case modeAdminStrict:
				if resolveErr != nil || orgID == "" {
					writeError(w, http.StatusNotFound, "not found")
					return
				}
				if !allowed(role, isMember) {
					writeError(w, http.StatusForbidden, "forbidden")
					return
				}
			default: // modeMemberCollapse
				if resolveErr != nil || orgID == "" || !isMember {
					writeError(w, http.StatusNotFound, "not found")
					return
				}
				if !allowed(role, isMember) {
					writeError(w, http.StatusForbidden, "forbidden")
					return
				}
			}
			// Section guard (api/sections.go): a member of a restricted role
			// may only reach the sections its UI exposes. Independent of the
			// allowed predicate above, which decides membership/role tier.
			if isMember && !routeAllowedForRole(role, r.Pattern) {
				writeError(w, http.StatusForbidden, "forbidden")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// orgAdmin gates an org-scoped admin action: the caller must hold the
// "admin" role in the organization resolve identifies.
func (h *handler) orgAdmin(resolve orgIDResolver) func(http.Handler) http.Handler {
	return h.orgAuthorized(resolve, modeAdminStrict, func(role string, isMember bool) bool {
		return isMember && role == "admin"
	})
}

// orgMember gates an ordinary org-scoped action on plain membership (any
// role) — the Phase C counterpart to orgAdmin, for routes that don't
// require admin privileges, just that the caller belongs to the
// organization resolve identifies.
func (h *handler) orgMember(resolve orgIDResolver) func(http.Handler) http.Handler {
	return h.orgAuthorized(resolve, modeMemberCollapse, func(role string, isMember bool) bool {
		return true
	})
}

// orgRole gates an org-scoped mutation to "admin", "power_user" or "general"
// (the org-wide non-admin roles — full read/write everywhere non-admin-gated),
// or one of the listed domain roles (org role redesign, migration 0081/0086).
// Uses modeMemberCollapse — the same
// resource-id resolvers (clientOrgID, invoiceOrgID, …) already protect
// these routes' own GET counterparts via orgMember, so a member-but-wrong-
// domain caller learns nothing a non-member wouldn't already be told by the
// read route; only genuinely new information (this org has this resource,
// and I'm a member) is gated as 403 instead of 404.
func (h *handler) orgRole(resolve orgIDResolver, roles ...string) func(http.Handler) http.Handler {
	allowed := map[string]bool{"admin": true, "power_user": true, "general": true}
	for _, role := range roles {
		allowed[role] = true
	}
	return h.orgAuthorized(resolve, modeMemberCollapse, func(role string, isMember bool) bool {
		return allowed[role]
	})
}

// orgRoleAdmin gates an org-scoped admin-tier action (same modeAdminStrict
// failure shape as orgAdmin — these routes are reached via pathOrgID/
// fiscalYearOrgID, contexts the caller is already "inside", so a 403 for a
// non-member reveals nothing new) to "admin" plus the listed roles. Used
// for the two actions the org role redesign folded "accounting" into
// alongside admin: closing a fiscal year, exporting the GL.
func (h *handler) orgRoleAdmin(resolve orgIDResolver, roles ...string) func(http.Handler) http.Handler {
	allowed := map[string]bool{"admin": true}
	for _, role := range roles {
		allowed[role] = true
	}
	return h.orgAuthorized(resolve, modeAdminStrict, func(role string, isMember bool) bool {
		return isMember && allowed[role]
	})
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
	role, isMember, err := h.db.GetOrganizationRole(orgID, claims.UserID)
	if err != nil {
		writeInternalError(w, err)
		return false
	}
	if !isMember {
		writeError(w, http.StatusForbidden, "forbidden")
		return false
	}
	if !routeAllowedForRole(role, r.Pattern) {
		writeError(w, http.StatusForbidden, "forbidden")
		return false
	}
	return true
}

// requireOrgRole is requireOrgMember's domain-role-checking sibling — same
// "runs from inside an already-withDB'd Create* handler, after decodeJSON,
// 403 not 404" reasoning, but also requires the caller's role to be
// "admin", "power_user", "general", or one of roles (org role redesign,
// migration 0081/0086).
func (h *handler) requireOrgRole(w http.ResponseWriter, r *http.Request, orgID string, roles ...string) bool {
	claims := getClaims(r)
	if claims == nil || orgID == "" {
		writeError(w, http.StatusForbidden, "forbidden")
		return false
	}
	role, isMember, err := h.db.GetOrganizationRole(orgID, claims.UserID)
	if err != nil {
		writeInternalError(w, err)
		return false
	}
	if !isMember {
		writeError(w, http.StatusForbidden, "forbidden")
		return false
	}
	allowed := role == "admin" || role == "power_user" || role == "general"
	for _, want := range roles {
		if role == want {
			allowed = true
		}
	}
	if !allowed {
		writeError(w, http.StatusForbidden, "forbidden")
		return false
	}
	if !routeAllowedForRole(role, r.Pattern) {
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
