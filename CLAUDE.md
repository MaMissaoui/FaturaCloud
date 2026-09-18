# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview
FaturaCloud is a web-based invoicing application. It runs as a single Docker image: a Go HTTP server that serves an embedded React frontend and exposes a REST API backed by SQLite.

## Architecture
- **Frontend**: React 19 with TypeScript and Vite 8
- **UI Framework**: Ant Design components
- **State Management**: Jotai atoms for reactive state
- **Backend**: Go `net/http` REST API — no framework, uses Go 1.26 method+path routing
- **Database**: SQLite via `modernc.org/sqlite` + `jmoiron/sqlx`
- **Styling**: SCSS with Ant Design theming
- **Internationalization**: LinguiJS with .po files in src/locales/
- **PDF Generation**: @react-pdf/renderer (client-side, no server involvement)

## Key Technologies
- Go `net/http` with Go 1.26 enhanced mux (method + path variables, e.g. `GET /api/clients/{id}`)
- React Router 7 (BrowserRouter) for client-side navigation with SPA fallback in the Go server
- Jotai for state management with atoms in src/atoms/
- LinguiJS for i18n with macros for translations
- SQLite with migrations in db/migrations/ (auto-applied on startup via golang-migrate)
- `modernc.org/sqlite` — CGO-free SQLite driver
- `go-nanoid` — 21-character IDs matching the database convention
- `decimal.js` — precise decimal arithmetic for all financial calculations
- `@dnd-kit` — drag-and-drop for invoice line item reordering
- `@sentry/react` — frontend error tracking
- `oxlint` + `oxfmt` — linting and formatting (replaces ESLint)
- `coreos/go-oidc` + `golang.org/x/oauth2` — OIDC SSO login (Authorization Code + PKCE), provider-agnostic (Authelia, Keycloak, Auth0, …)

## Development Commands
```bash
# Start the Go backend (API on :8080)
go run .

# Start the frontend dev server (proxies /api to :8080)
pnpm dev

# Build the frontend only
pnpm build

# Type-check + lint (tsc --noEmit first, then oxlint src/)
pnpm lint

# TypeScript type-check only
pnpm type-check

# Format source files
pnpm format

# Check formatting without writing
pnpm format:check

# Preview production build locally
pnpm preview

# Extract translation strings
pnpm extract

# Build and run with Docker Compose
docker compose up --build
```

## API — Frontend ↔ Backend

The frontend calls the Go REST API via `src/api/index.ts`. All typed functions live there and are imported from `src/api` throughout the app.

```ts
import { GetClients, CreateClient } from "src/api"
const clients = await GetClients(organizationId)  // GET /api/organizations/{id}/clients
```

The base fetch wrapper lives in `src/api/client.ts`. Auth is a JWT in an **httpOnly `fc_token` cookie** the browser sends automatically (not localStorage/Bearer — page JS can't read it, so XSS can't steal the session). The wrapper sends `credentials: "same-origin"` and a custom `X-CSRF-Protection` header on every request; the server's `csrfRequired` middleware rejects state-changing requests (POST/PUT/PATCH/DELETE) that lack it (a stateless CSRF defense that works because there's no permissive CORS, backed by `SameSite=Lax` on the cookie). `login` sets the cookie and returns only `{user}`; `logout` POSTs to the server so it can expire the cookie; the OIDC callback sets the cookie and redirects to `/` (no token in the URL). All API errors throw `Error(message)` so callers catch them normally.

Routes are registered in `api/router.go` (the authoritative list); route-level behavior notes and the full authorization model are in `api/CLAUDE.md`.

All handlers return JSON. Errors use `{"error": "message"}`.

## File Structure
- `main.go` — entry point; opens DB, seeds first admin, mounts API router, serves embedded `dist/`
- `Dockerfile` — multi-stage build: node (frontend) → golang (backend + embed) → debian (runtime, carries `libreoffice-calc` for PDF export — see the Docker section)
- `docker-compose.yml` — single service, `/data` volume for SQLite
- `docker-compose.oidc.yml` — overlay enabling OIDC SSO against homelab-auth's Authelia via Nginx Proxy Manager (no Traefik — see `docs/oidc-sso.md`); merge with `-f docker-compose.yml -f docker-compose.oidc.yml`
- `docs/oidc-sso.md` — OIDC SSO design doc: generic provider-agnostic pattern, FaturaCloud-specific implementation, security model, Authelia-side client setup
- `docs/inventory-cogs-integration.md` — the design note Phase 7 (GRNI on receipt, COGS on shipment, now **shipped**, PR #77) implements: why weighted-average cost (`recomputeAverageCostTx`) vs. immutable posted GL entries is safer than it first looks, the policy decisions it settled (no-cost-basis shipments 409, serialized specific-identification, manual adjustments get a GL trace, GRNI/bill matching nets by quantity-first, long-run reconciliation is a visibility report not a guarantee — see `GetInventoryValuation` below), and the finding that pre-Phase-7 bill posting (`resolveExpenseAccount`) expensed stock-tracked purchases immediately with no inventory capitalization, which `resolvePurchaseAccount` now corrects. Implemented in sub-phases 7a–7f (PR #77), plus two findings surfaced grounding the plan against the code — the accounting-defaults backfill not reaching already-migrated organizations, and cancelling a billed receipt corrupting GRNI — both fixed as part of the same PR (see the `db/account.go` and `inbound_deliveries.status` entries below)

## Cross-Cutting Invariants
These apply on both sides of the stack; the detail lives in the subdirectory files below.
- **Money** is integer cents everywhere in storage, the API, and atoms — only the form layer converts (input × 100 → store, stored ÷ 100 → display). Server rounds with `roundCents`; totals are re-validated server-side with exact rationals.
- **Dates** are Unix timestamps in milliseconds; **ids** are 21-char nanoids (every `Create*` generates one when `req.ID == ""`).
- **Document statuses/states** are defined twice and must stay in sync: the Go set/transition map in `db/<doc>.go` and the frontend source of truth in `src/types/<doc>.ts`. Status changes go through `PATCH …/status` (or `/state`) only, never `PUT`.
- **Posted journal entries are immutable** — reversed, never edited or deleted; every posting path goes through `allocateAndFinalizeEntryTx`.
- **Authorization gap (Phase C deferred):** any authenticated user can read/write every organization's data through the ~130 `protected()` routes. Only global actions (`platformAdminProtected`) and org-admin actions (`orgAdminProtected`) are gated. Full model: `api/CLAUDE.md`.
- **Modal/Drawer forms** never use module-level Jotai atoms for local state — use `useState` (the mask gets orphaned and freezes the UI).

## Where the Rest of the Rules Live
Subdirectory `CLAUDE.md` files load automatically once Claude reads a file in that directory. For planning work that touches no files yet, read the relevant one explicitly.
- `db/CLAUDE.md` — database schema conventions, status machines, stock/GL/costing rules, and per-file notes for every `db/*.go` (GL posting, payments, reports, exports, xlsx templates, fiscal close, …)
- `api/CLAUDE.md` — authorization model (platform admin vs. org admin, middleware), route-level notes, per-file notes for `api/*.go`
- `src/CLAUDE.md` — Jotai state conventions, sidebar/settings navigation, per-file notes for routes, atoms, and components
- `cmd/seed-demo/CLAUDE.md` — the demo-data seeding tool

## Internationalization
- Uses LinguiJS with macro-based extraction
- Translation files in .po format under src/locales/
- Default locale configuration in src/utils/lingui.tsx
- Supports 3 locales: en, de, fr (the set lives in `lingui.config.ts` `locales`; the language switcher, antd/dayjs locale wiring in `src/app.tsx`, and `dynamicActivate` in `src/utils/lingui.tsx` all derive from or match it). de and fr are fully translated; en is the source locale
- **Server-side error messages are deliberately English-only.** Every `db/*.go` validation/409 message (`newValidationError(...)`, `errors.New(...)`) is plain English with no translation path — the frontend's `message.error(error instanceof Error ? error.message : t\`fallback\`)` pattern only translates the generic fallback half; the specific server message always displays in English regardless of locale. This was flagged and explicitly accepted as a product decision (see `docs/audit-plan-2026-08-09.md`) rather than built out into a Go-side message-key/translation table — don't re-flag it as a bug or attempt the translation system without a fresh product decision to do so.

## Docker
```bash
# Build and run
docker compose up --build

# Build image only
docker build -t fatura-cloud .

# Run with explicit volume (bind-mounted subfolder, not a named volume —
# container runs as uid:gid 1000:1000, so ./data must be owned by that)
docker run -p 8080:8080 -v ./data:/data fatura-cloud
```

The `Dockerfile` is a three-stage build:
1. **frontend** (node:22-alpine) — `RUN corepack enable` then `pnpm install --frozen-lockfile`/`pnpm build`. Corepack (bundled with Node 22) reads `package.json`'s `packageManager` pin and fetches that exact pnpm release; the earlier `npm install -g pnpm` instead grabbed pnpm's single-executable-application build, whose per-platform `@pnpm/exe.*` native binary is an `optionalDependency` npm doesn't reliably install — it failed outright on at least one arm64 host with `no @pnpm/exe.linux-arm64 native binary was found for this host`. Corepack's shim has no such gap
2. **backend** (golang:1.26-alpine) — copies `dist/` and embeds it via `//go:embed all:dist`, compiles binary
3. **runtime** (debian:bookworm-slim) — copies only the binary plus `libreoffice-calc` and its own dependencies

**Runtime stage is Debian, not Alpine — this changed once, and why matters for not reverting it.** Issue #115's custom Excel document templates were originally going to add real PDF output (`db/pdf_convert.go` shelling out to `soffice --headless --convert-to pdf`, via `libreoffice-calc`) on an Alpine runtime stage, but that attempt was reverted: `libreoffice-calc` crashed on startup there (`terminate called after throwing an instance of 'com::sun::star::uno::RuntimeException'`) with every standard workaround tried (`SAL_USE_VCLPLUGIN=svp`, `SAL_DISABLE_SKIA=1`, installing a JRE, installing `gcompat`) — a long-standing, unresolved Alpine/musl LibreOffice packaging fragility, not something fixable from this app's side. (Along the way, the *size* estimate for `libreoffice-calc` on Alpine was also found to be wrong — an initial ~140-150MB, based on third-party examples that turned out stale for that Alpine version, was actually ~1.42GB when measured directly; that number stopped mattering once the package turned out not to run at all.) The runtime stage was later switched to `debian:bookworm-slim`, where `libreoffice-calc` runs cleanly headless as the same non-root `1000:1000` user with no workarounds needed — verified directly (not just by package-manager success) by building the real multi-stage image on both amd64 and arm64 (the latter matching a Raspberry Pi deployment target), running the container, and hitting `GET /api/invoices/{id}/export?format=pdf` end-to-end against a real invoice: valid PDF, sub-second conversion, well inside `pdfConvertTimeout`. The trade is size: `apt-get install --no-install-recommends libreoffice-calc fonts-liberation fonts-dejavu-core` (scoped to Calc's own dependency tree, not the full LibreOffice suite; `fonts-liberation` for Arial/Times/Courier metric compatibility with whatever an org's uploaded template specifies) adds roughly 630MB, taking the shipped image from ~80MB to ~710MB — accepted deliberately, once, rather than the earlier "get it working later" deferral. `db/pdf_convert.go`/`ConvertXLSXToPDF` needed **no code changes** for this — it was already complete and tested, written to work wherever `soffice` is on `PATH` regardless of which OS put it there. Custom-template export offers both Excel and PDF buttons in the UI (`src/routes/invoices/details.tsx`) as a result.

Pass `--build-arg VERSION=<tag>` to inject a version string (accessible via `GET /api/version`); the frontend build stage also uses it as the Sentry release name (see below).

Two Sentry-related build inputs are optional and deliberately excluded from the published GHCR image (`.github/workflows/docker.yml`), so pulling that image never sends crash reports to this project's Sentry account by default:
- `--build-arg VITE_SENTRY_DSN=<dsn>` — bakes a DSN into the frontend build, enabling error reporting. `docker-compose.yml` passes this through from a `VITE_SENTRY_DSN` var in your own `.env` for `docker compose up --build`.
- `--secret id=sentry_auth_token,env=SENTRY_AUTH_TOKEN` (BuildKit secret, not a build-arg — keeps the token out of image layers/history) — uploads source maps for that release to Sentry (`org: mohamed-ali-missaoui`, `project: faturacloud` in `vite.config.ts`). CI supplies it from the `SENTRY_AUTH_TOKEN` repo secret; skipped silently if absent.

Source maps are never shipped in the deployed artifact: `build.sourcemap` is `"hidden"` and the Sentry plugin's `filesToDeleteAfterUpload` removes every `dist/**/*.map` after the build (uploaded to Sentry first when a token is present, deleted regardless when not). The Go server embeds `dist/` via `//go:embed all:dist`, so this keeps original source out of the public `/assets/` and out of the binary — maps live only inside Sentry.

## Environment Variables
- `PORT` — HTTP port for the Go server (default `8080`)
- `JWT_SECRET` — secret key for signing JWT tokens; defaults to `"dev-secret-change-me-in-production"` — **must be set in production**
- `ADMIN_EMAIL` — email for the initial admin user created on first startup (default: `admin@fatura.cloud`)
- `ADMIN_PASSWORD` — password for the initial admin user (default: `admin`) — **change in production**
- `TRUSTED_PROXIES` — comma/space-separated IPs or CIDRs (e.g. `172.20.0.0/16`) of reverse proxies allowed to set `X-Forwarded-For` **and `X-Forwarded-Proto`**. Unset (default): the login rate limiter always keys on the direct TCP peer and `Secure`/HSTS decisions always see plain HTTP, so every client behind a reverse proxy shares one rate-limit bucket and (until this is set) sessions won't get `Secure` cookies — set this to your proxy's address when deploying behind one. Only ever list proxies that are the sole path to the app; an untrusted peer's `X-Forwarded-For`/`X-Forwarded-Proto` is always ignored. `api.IsTrustedProxyPeer`/`api.IsHTTPS` (`api/auth.go`) are the shared implementation both `clientIP` (rate limiting) and every `Secure` cookie/HSTS decision (`setAuthCookie`, `setOIDCStateCookie`, `main.go`'s `securityHeaders`) now go through — previously, `X-Forwarded-Proto` was trusted unconditionally regardless of peer
- `VITE_SENTRY_DSN` — frontend build-time; enables Sentry error tracking when set (see Docker section above for how to pass it in). Unset means Sentry is fully off regardless of `VITE_SENTRY_ENABLED`
- `VITE_SENTRY_ENABLED=true` — force-enables Sentry error tracking in dev (defaults off outside production); has no effect without `VITE_SENTRY_DSN` also set
- `VITE_JOTAI_DEVTOOLS_ENABLED=true` — enables Jotai DevTools in dev mode
- `OIDC_ISSUER_URL` — enables OIDC SSO login when set (Authelia or any standards-compliant provider); unset/empty means the feature is fully disabled, no route reachable, local login unaffected
- `OIDC_CLIENT_ID` / `OIDC_CLIENT_SECRET` / `OIDC_REDIRECT_URL` — OIDC client credentials and this app's own callback URL (must exactly match what's registered with the provider)
- `OIDC_SCOPES` — space-separated (default `openid profile email groups`)
- `OIDC_EMAIL_CLAIM` / `OIDC_NAME_CLAIM` / `OIDC_GROUPS_CLAIM` — ID token claim names to read (defaults `email` / `name` / `groups`) — override for providers that name claims differently
- `OIDC_ADMIN_GROUP` — group value in the groups claim that maps to the FaturaCloud `admin` role (default `admins`)

See `docs/oidc-sso.md` for the full design, security model, and the matching Authelia-side setup.

## Adding a New API Endpoint

**Go side** — add a handler method in the relevant `api/{domain}.go` file, then register the route in `api/router.go`:
```go
protected("GET", "/api/things/{id}", h.getThing)
// or for a genuinely global admin-only action (no natural per-org owner):
platformAdminProtected("DELETE", "/api/things/{id}", h.deleteThing)
// or for an org-scoped admin action:
orgAdminProtected("DELETE", "/api/organizations/{orgId}/things/{id}", pathOrgID("orgId"), h.deleteThing)
```

**Frontend side** — add a typed function in `src/api/index.ts`:
```ts
export const GetThing = (id: string) => get<Thing>(`/things/${id}`)
```

Then import and call it from atoms or components as needed.

## Committing
- Use conventional commit format: `<type>: <description>`
- Types: feat, fix, docs, style, refactor, perf, test, chore, ci, revert, hotfix
- Breaking changes: add `!` before `:` (e.g., `feat!: remove status endpoint`)
- First line under 72 chars, present tense, imperative mood
- Never include "Generated with Claude Code" or "Co-Authored-By" attribution
- Split into multiple commits when changes span different modules/concerns or mix types
- Stage all changes if none are already staged
