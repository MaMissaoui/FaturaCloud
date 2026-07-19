# OIDC SSO Support (Authelia / homelab-auth, provider-agnostic)

## Context

FaturaCloud supports local email/password login (JWT bearer tokens) as its
baseline. This document describes adding OpenID Connect (OIDC) single
sign-on on top of it, backed by a homelab SSO stack
(`homelab-auth`: Nginx Proxy Manager + Authelia + lldap + Redis — NPM is the
only reverse proxy; there is no internal router/Traefik in front of it),
with local login kept as a permanent fallback.

An earlier design considered a Traefik-based `forwardAuth` middleware
(header injection after proxy-side verification) instead of OIDC — and
separately, homelab-auth itself originally ran Traefik as an internal router
behind NPM. Both were dropped: `forwardAuth` in favor of OIDC for FaturaCloud
specifically (reasons below), and Traefik entirely in favor of NPM routing
directly to each service's own published port, once it became clear
Traefik's only remaining job (for FaturaCloud) was routing, which NPM
already does natively. `forwardAuth` still exists as a *pattern* for other
apps (OpenLedger, Grafana, lldap's own UI) — it's now implemented via nginx
`auth_request` in NPM's per-host "Advanced" config, not a Traefik
middleware label. See `homelab-auth`'s own `README.md`/`DEPLOY.md` for that
side.

OIDC was chosen over forwardAuth for FaturaCloud for three reasons:

1. **Deployment portability**: FaturaCloud ships as a Docker image today but
   may later run natively on a bare VM with no reverse proxy in front.
   `forwardAuth` only exists because a proxy intercepts the request — it has
   no equivalent on a VM without one. OIDC is a direct app-to-IdP protocol
   and works identically regardless of what (if anything) sits in front of
   the app.
2. **Internet-facing security**: `homelab-auth` is intended for
   internet-exposed apps, not LAN-only tools. `forwardAuth`'s entire trust
   model is "the app's port is never reachable except through the proxy" — a
   single misconfiguration (an exposed port, a firewall slip) is a full,
   unauthenticated bypass with no second layer. OIDC replaces that with a
   cryptographically signed ID token verified against the IdP's published
   keys — network isolation becomes defense-in-depth, not the sole
   safeguard.
3. **Reuse across apps, provider-agnostic**: this same design is meant to be
   reapplied later to other Go + web-frontend apps in this homelab
   (STBVirement, OpenLedger), and to work against Authelia or any other
   standards-compliant OIDC provider (Keycloak, Auth0, Okta, …), not just
   Authelia specifically.

Standard OIDC discovery (`/.well-known/openid-configuration`) plus
Authorization Code + PKCE satisfies all three: identical code regardless of
proxy topology, no dependency on network isolation for its security
guarantee, and every compliant IdP exposes it the same way. Authelia's OIDC
provider supports a `groups` claim (via `claims_policies`) and per-client
group restriction (via `identity_providers.oidc.authorization_policies`), so
the intended access model (restrict who can log in; a separate group maps to
admin) carries over cleanly.

**Decisions locked in:**
- OIDC, not `forwardAuth`.
- Documented as a reusable pattern for other apps, but **implemented in
  FaturaCloud only** for now — no shared library across the separate repos
  (premature abstraction for three independently-versioned apps).
- `homelab-auth` was not yet deployed when this was designed, so its
  Authelia config is written for OIDC from the start rather than retrofitted.

## Generic OIDC pattern (provider- and app-agnostic)

This section is written to be reapplied to other apps; the FaturaCloud
section after it is the concrete instance.

- **Discovery-driven**: fetch `{issuer}/.well-known/openid-configuration` at
  startup (via `github.com/coreos/go-oidc/v3/oidc`) to obtain authorization/
  token/JWKS endpoints — nothing is hardcoded, so swapping Authelia for
  Keycloak/Auth0/Okta is a config change, not a code change.
- **Authorization Code + PKCE (S256)**, via `golang.org/x/oauth2`. PKCE is
  mandatory even for a confidential (server-side) client — current best
  practice, protects against code interception.
- **CSRF/replay protection**: a cryptographically random `state` and `nonce`
  are generated per login attempt and held in a short-lived, signed,
  `HttpOnly`, `Secure`, **`SameSite=Lax`** cookie — not `Strict`. The
  redirect back from the IdP is a top-level cross-site navigation whenever
  the IdP lives on a different registrable domain (e.g. Auth0/Okta);
  `Strict` would silently drop the cookie there even though it happens to
  work with Authelia on a same-site subdomain. This is exactly the kind of
  detail that breaks silently on the second app if not called out
  explicitly.
- **ID token validation**: verified via `go-oidc`'s `oidc.Config{ClientID:
  ...}` verifier, which enforces signature (against the discovered JWKS,
  with rotation/caching handled by the library), `iss`, `aud` (must equal
  the configured client ID — do not skip this), and expiry/clock-skew.
  `nonce` is compared explicitly against the value stored in the login
  cookie (the verifier surfaces it as a claim but does not compare it for
  you).
- **Configurable claim mapping**, not hardcoded to Authelia's field names —
  env vars: `OIDC_ISSUER_URL`, `OIDC_CLIENT_ID`, `OIDC_CLIENT_SECRET`,
  `OIDC_REDIRECT_URL`, `OIDC_SCOPES` (default `openid profile email groups`),
  `OIDC_EMAIL_CLAIM` (default `email`), `OIDC_NAME_CLAIM` (default `name`),
  `OIDC_GROUPS_CLAIM` (default `groups`), `OIDC_ADMIN_GROUP` (default
  `admins`).
  **Limitation**: this only resolves flat, top-level array claims
  (Authelia's `groups` is one). Some providers nest roles (e.g. Keycloak's
  `realm_access.roles`) — those need an IdP-side mapper/claim-flattening
  step before they'll work with a simple claim-name lookup.
- **No per-request re-validation** — unlike `forwardAuth`, the app mints its
  own session token once at login and does not re-check the IdP on every
  request. This is a deliberate, visible trade-off (see Security below).
- **Feature is off unless configured**: no `OIDC_ISSUER_URL` set → SSO login
  is entirely absent (button hidden, login endpoint 503s) and local login is
  byte-for-byte unchanged. Discovery is fetched lazily with retries
  rather than only once at startup, so a temporarily-unreachable IdP doesn't
  permanently disable SSO until the app is restarted.

## FaturaCloud implementation

### New dependencies (`go.mod`)
`github.com/coreos/go-oidc/v3` and `golang.org/x/oauth2`.

### New file `api/oidc.go`
- Holds `oidcProvider *oidc.Provider`, `oidcVerifier *oidc.IDTokenVerifier`,
  `oauth2Config *oauth2.Config`, initialized lazily (first request or a
  retrying background init) from `OIDC_ISSUER_URL`, cached on `handler`.
- `oidcLoginStart(w, r)` — `GET /api/auth/oidc/login`, public route.
  Generates `state`/`nonce`/PKCE verifier, sets the signed cookie described
  above, redirects (302) to `oauth2Config.AuthCodeURL(state, oidc.Nonce(nonce),
  oauth2.S256ChallengeOption(verifier))`.
- `oidcCallback(w, r)` — `GET /api/auth/oidc/callback`, public route.
  Validates the state cookie matches the query param (single-use — clear the
  cookie either way), exchanges the code for tokens (`oauth2Config.Exchange`
  with the PKCE verifier), verifies the ID token via `oidcVerifier.Verify`,
  checks the `nonce` claim against the stored value, extracts email/name/
  groups per the configured claim names, computes `role` from
  `OIDC_ADMIN_GROUP` membership, calls `provisionOrSyncUser` (below), mints a
  FaturaCloud JWT via the existing `issueToken` (no new token format), sets it
  in the httpOnly `fc_token` cookie exactly like local login (`setAuthCookie`),
  and redirects the browser to `/`. The token never appears in the URL (no
  fragment, no query), so it can't leak via history, access logs, or a
  `Referer` header, and page JS never sees it.
- Any validation failure (bad/missing state, nonce mismatch, bad signature,
  wrong issuer/audience, expired token, IdP unreachable) → redirect to
  `/login?error=sso_failed` with details logged server-side only; never leak
  validation specifics to the client.

### `api/users.go` — JIT provisioning + role-sync (login-time, not per-request)
```go
func (h *handler) provisionOrSyncUser(email, name string, isAdmin bool) (userRow, error)
```
- Look up by `email` (`UNIQUE NOT NULL` — the identity anchor).
- Not found → JIT-provision: random bcrypt hash (satisfies the existing
  `passwordHash NOT NULL` constraint from migration `0023_add_users.up.sql`
  without being a usable credential), `displayName` from the name claim
  (fallback to email). Use `INSERT ... ON CONFLICT(email) DO NOTHING` then
  re-`SELECT`, to handle two near-simultaneous first logins for the same new
  SSO email without a duplicate-key error on the loser.
- Found and `isActive = 0` → return an error (an admin-deactivated local user
  must not be silently re-authorized via SSO), mirroring `login()`'s
  existing `WHERE isActive = 1` guard.
- Role: only `UPDATE` when the computed role differs from what's stored
  (read-compare-write).
- `displayName` is JIT-set once and never re-synced (cosmetic, may be locally
  curated via the Users admin page); `role` is authorization-critical and
  re-synced every login.
- Takes `h.dbMu.Lock()` (write lock) for the insert/update.

### `api/auth.go` — `authProvider` on `me`/`login`
- `me` response includes `"authProvider": claims.Provider` (`"local"` when
  the claim is empty, `"oidc"` when minted by `oidcCallback`). Add
  `Provider string` (`json:"authProvider"`) to `Claims` — it must round-trip
  through the signed JWT itself (not just live in memory at issue time),
  since `me` reads it back from a re-parsed token on a later request; a
  `json:"-"` tag would silently make every token look like `"local"` after
  the first parse.
- `login`'s inline response map adds `"authProvider": "local"`.

### `handler` struct / `NewRouter` (`api/router.go`) + `main.go`
New fields: `oidcIssuerURL`, `oidcClientID`, `oidcClientSecret`,
`oidcRedirectURL`, `oidcScopes`, `oidcEmailClaim`, `oidcNameClaim`,
`oidcGroupsClaim`, `oidcAdminGroup` — all read from env in `main.go` with the
defaults listed above; empty/unset `OIDC_ISSUER_URL` means the feature is
fully disabled. Two new **public** routes (not behind `authMiddleware` — they
*are* the auth entry point): `GET /api/auth/oidc/login`,
`GET /api/auth/oidc/callback`. `login` returns `503` immediately if OIDC
isn't configured; `callback` redirects to `/login?error=sso_failed` for
every failure mode, including "not configured" — it only ever fires after a
real IdP round-trip, so a friendly redirect is preferable to a bare 503 for
whatever went wrong in between.

### Frontend

- `src/routes/login.tsx`: add a "Sign in with SSO" button/link, shown only
  when a new public `GET /api/auth/oidc/enabled` (or an added boolean field
  on `/api/version`) reports the feature is on. Clicking it is a full
  browser navigation to `/api/auth/oidc/login` (not a `fetch` — this must be
  a real top-level redirect for the OAuth dance to work).
- No frontend callback route is needed: the server-side callback sets the
  httpOnly `fc_token` cookie and redirects straight to `/`. (An earlier
  iteration delivered the token via a `#token=` fragment to a `/auth/callback`
  route; that was removed when auth moved to the cookie.)
- `src/api/index.ts`: extend `CurrentUser.authProvider` to
  `"local" | "oidc"`.
- **No changes needed to `src/app.tsx`, `isAuthenticatedAtom`, or the session
  bootstrap.** Since OIDC mints the same local JWT and delivers it via the same
  cookie as local login, the existing model (`GetMe()` on mount → `currentUser`)
  is untouched — SSO is just a second way to obtain the session cookie.
- Logout: POSTs `/api/auth/logout` so the server expires the httpOnly cookie
  (JS can't clear it), then navigates to `/login` — fully logs the user out of
  FaturaCloud regardless of how they authenticated. Redirecting to the IdP's
  `end_session_endpoint` for a "global" logout is a reasonable follow-up, not
  required for v1.

### Security summary
- PKCE (S256) mandatory; `state`/`nonce` are cryptographically random,
  single-use, and cookie-scoped `HttpOnly` + `Secure` + `SameSite=Lax`.
- ID token signature verified against the IdP's live JWKS; `iss` and `aud`
  are checked, not just the signature.
- Redirect URI must exactly match what's registered with the IdP — no
  wildcarding; `OIDC_REDIRECT_URL` is the externally-visible HTTPS URL even
  when TLS is terminated upstream, matching how `/api/auth/login` already
  works behind the same termination today.
- Client secret lives server-side only (`OIDC_CLIENT_SECRET` env var), never
  reaches the frontend; token endpoint auth via `client_secret_basic`.
- **Revocation trade-off, stated plainly**: because FaturaCloud mints its
  own JWT at login instead of re-checking the IdP per request, a user
  removed or demoted at the IdP keeps FaturaCloud access until their
  existing token expires — same 24h window local login already has.
  Default for v1: accept this (consistent with today's local-login
  behavior). If tighter revocation is wanted later, the hardening path is a
  short-lived access token plus silent refresh against the IdP — a future
  option, not built now.
- Per Authelia's own documentation on client `authorization_policies`: these
  restrict who can even complete a login for a given client, but are
  explicitly not a substitute for the app's own access control —
  FaturaCloud independently checks the `groups` claim itself (defense in
  depth) rather than relying solely on the IdP-side policy.
- Fail-closed on every validation step: bad state, nonce mismatch, bad
  signature, wrong issuer/audience, or an expired token all reject the
  callback the same way (generic redirect + server-side-only logging).

## Docker / deployment (`docker-compose.oidc.yml`)

The overlay merges with the base file:
```bash
docker compose -f docker-compose.yml -f docker-compose.oidc.yml up -d
```
It sets the `OIDC_*` environment variables (`OIDC_ISSUER_URL`,
`OIDC_CLIENT_ID`, `OIDC_CLIENT_SECRET`, `OIDC_REDIRECT_URL`,
`OIDC_ADMIN_GROUP`) using `${DOMAIN}` and `${OIDC_CLIENT_SECRET}` from
FaturaCloud's own `.env`. No Traefik labels anywhere in this topology. The
overlay also joins the shared `proxy` Docker network (`docker network create
proxy`, done once by homelab-auth) and sets `container_name: fatura-cloud`
— homelab-auth's NPM reaches this container by that name over `proxy`, the
same way it reaches Authelia (`authelia`) and lldap (`lldap`); none of them
publish a host port. The explicit container name matters here specifically
because the underlying Compose service is generically named `app` — without
overriding it, a different project also joined to `proxy` with an "app"
service could collide. The overlay also removes the base file's host port
publish (`ports: !override []`) — once NPM is routing by container name,
nothing needs to reach this container directly from the LAN.

**The one non-obvious piece: `extra_hosts`.** `OIDC_ISSUER_URL` deliberately
stays the **public** hostname (`https://auth.${DOMAIN}`), not an internal
address like `http://authelia:9091` — the ID token's issuer identity has to
match what the browser authenticated against, or `go-oidc`'s `NewProvider`
rejects the discovery document (issuer mismatch), which would otherwise
force a code-level workaround (`oidc.InsecureIssuerURLContext`) that's best
avoided entirely. The consequence: FaturaCloud's own server-to-server call
(the token exchange in `oidcCallback`) also goes out to `auth.${DOMAIN}` —
and since Authelia has no published host port either, the *only* way this
resolves at all is through NPM. Even though FaturaCloud and Authelia share
the `proxy` network, this call can't just use `http://authelia:9091`
directly (that's the issuer-mismatch problem above), so it still needs
`auth.${DOMAIN}` to resolve to something reachable — which means NPM,
specifically, since NPM holds the TLS certificate for that hostname and
Authelia doesn't have a directly reachable address of its own for this
purpose. The overlay fixes this at the DNS layer with:
```yaml
extra_hosts:
  - "auth.${DOMAIN}:${NPM_LAN_IP}"
```
This points `auth.${DOMAIN}` directly at **NPM's** LAN IP (not Authelia's —
NPM is what holds the TLS certificate for that hostname) from inside
FaturaCloud's container only. TLS still validates normally since the
hostname/SNI/certificate check are unaffected — only where the TCP
connection physically goes changes. `NPM_LAN_IP` is a new required variable
in FaturaCloud's `.env` for this deployment path. This is untestable without
a live homelab-auth instance — verify the token-exchange hop specifically
once deployed (see Verification).

## `homelab-auth` changes (separate repo)

In `deploy/authelia/configuration.yml`:
1. Add `identity_providers.oidc` with a generated signing key (via
   `authelia crypto` — document the exact command used) and an
   `hmac_secret`.
2. Add an `authorization_policies` entry restricting login to subject groups
   `fatura-users` or `admins` (group-gated, not "any lldap user").
3. Add a `claims_policies` entry exposing the `groups` claim, and a
   `clients` entry for `fatura-cloud`: `client_id`, hashed `client_secret`
   (via `authelia crypto hash generate`), `redirect_uris:
   ['https://fatura.<DOMAIN>/api/auth/oidc/callback']`,
   `scopes: [openid, profile, email, groups]`,
   `authorization_policy: <the policy from step 2>`,
   `token_endpoint_auth_method: client_secret_basic`,
   `require_pkce: true`, `pkce_challenge_method: S256`.
4. Since auth now happens at the app layer, FaturaCloud's NPM proxy host
   (`fatura.<DOMAIN>` → Forward Hostname `fatura-cloud`, the container name,
   over the shared `proxy` network — not an IP or published port) needs
   **no** `auth_request` snippet at all — a plain proxy host is the entire
   routing-side setup. There is no Traefik anywhere in this topology to
   configure a middleware on.
5. This is a named, parameterized recipe (swap `fatura-cloud` for
   `stbvirement`/`openledger` and the app's own redirect URI) — document it
   in `homelab-auth`'s own docs so onboarding the other two apps later is a
   repeat of these same steps, not a redesign.

## Verification

**Automated (no live IdP needed):** a Go test using `httptest.Server` to
stand up a minimal fake OIDC provider (discovery doc + JWKS + token/userinfo
endpoints serving controlled claims) — covers: successful login mints a JWT
with the right role; `state` mismatch is rejected; `nonce` mismatch is
rejected; a token with the wrong `aud` is rejected; an expired token is
rejected; a user without the admin group gets `role: user`; a second login
for an existing SSO email updates role but not `displayName`; a login for an
`isActive = 0` user is rejected.

**Manual, end-to-end against real Authelia:**
1. Bring up `homelab-auth` locally (e.g. `/etc/hosts` entries mapping
   `fatura.dev.local` / `auth.dev.local` to `127.0.0.1`, `DOMAIN=dev.local`
   in `.env`) plus FaturaCloud configured with matching `OIDC_*` env vars.
2. Fresh incognito browser to `https://fatura.dev.local/login` → click
   "Sign in with SSO" → redirected to Authelia → log in against lldap →
   redirected back into FaturaCloud, landed on `/`, not `/login`.
3. Confirm the JIT-provisioned user in the Users admin page, correct
   `displayName`/`role` based on lldap group membership.
4. Remove the test user from the `admins` lldap group, log out and back in
   via SSO → confirm role flips to `user` on the new login (not immediately
   — this is the documented 24h-token trade-off).
5. Remove the test user from `fatura-users` entirely → confirm Authelia's
   `authorization_policy` blocks the login attempt before it reaches
   FaturaCloud.
6. Confirm local login (`POST /api/auth/login` + existing flow) is
   completely unaffected in both `OIDC_ISSUER_URL` set and unset states.
7. Confirm the SSO button is absent, `GET /api/auth/oidc/enabled` reports
   `false`, and `GET /api/auth/oidc/login` 503s, when `OIDC_ISSUER_URL` is
   unset.
8. **Specifically test the token-exchange hop** (step 2 succeeding through
   the browser redirect is not sufficient proof this works) — a failure here
   looks like: the SSO button redirects to Authelia, login succeeds, Authelia
   redirects back to FaturaCloud, but the callback then fails/loops rather
   than landing on `/`. Check FaturaCloud's container logs for the
   `oidcCallback` failure reason (logged server-side per the fail-closed
   design above) — if it's a connection error reaching `auth.<DOMAIN>`, the
   `extra_hosts` NAT-hairpin fix (see Docker/deployment above) either isn't
   configured or `NPM_LAN_IP` is wrong.

## Critical files

- `go.mod` / `go.sum` (new deps)
- `api/oidc.go` (new)
- `api/users.go` (new `provisionOrSyncUser`)
- `api/auth.go` (`authProvider` field)
- `api/router.go` / `main.go` (new config fields, public routes, env vars)
- `src/routes/login.tsx` (SSO button)
- frontend `/auth/callback` route (new)
- `src/api/index.ts` (`CurrentUser.authProvider`)
- `docker-compose.oidc.yml` (OIDC env vars + the `extra_hosts` NAT-hairpin fix)
- `homelab-auth/deploy/authelia/configuration.yml` (separate repo)
- `homelab-auth/docker-compose.yml` (separate repo — no Traefik; Authelia/lldap publish their own ports)
- `deploy.md` (pointer section to this doc)
