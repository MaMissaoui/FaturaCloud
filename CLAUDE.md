# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview
FaturaCloud is a web-based invoicing application. It runs as a single Docker image: a Go HTTP server that serves an embedded React frontend and exposes a REST API backed by SQLite.

## Architecture
- **Frontend**: React 19, TypeScript, Vite 8, Ant Design, SCSS. State lives in Jotai atoms (`src/atoms/`). Routing uses React Router 8 (BrowserRouter), with an SPA fallback in the Go server.
- **Backend**: Go `net/http`, no framework, using Go 1.26 method+path routing (`GET /api/clients/{id}`).
- **Database**: SQLite through `modernc.org/sqlite` (CGO-free) and `jmoiron/sqlx`. Migrations in `db/migrations/` run on startup (golang-migrate).
- **i18n**: LinguiJS macros, .po catalogs in `src/locales/`.
- **Libraries**: `go-nanoid` (ids), `decimal.js` (all frontend money math), `@dnd-kit` (line reordering), `@sentry/react`, `oxlint` + `oxfmt` (lint and format), `coreos/go-oidc` + `golang.org/x/oauth2` (OIDC SSO, any provider).
- **Documents and PDF**: a document's Excel and PDF are its organization's Excel template, filled on the server (`db/xlsx_export*.go`) and converted with headless LibreOffice (`db/pdf_convert.go`). Each of the six document types has two embedded layouts, chosen org-wide by `organizations.documentLayout`:
  - `default`: the generic layout;
  - `tunisia`: the Tunisian "Facture" layout, generated from `db/templates/gen/tunisia_layout.go`.

  An uploaded per-type override (`document_templates`) wins over both. Details are in `db/CLAUDE.md`.

## Development Commands
```bash
go run .              # backend, API on :8080
pnpm dev              # frontend dev server, proxies /api to :8080
pnpm build            # frontend build
pnpm lint             # tsc --noEmit, then oxlint --deny-warnings
pnpm type-check
pnpm test             # vitest
pnpm format           # oxfmt; pnpm format:check to verify
pnpm extract          # extract translation strings (run after changing UI text)
docker compose up --build
```
Start a scratch server with `HOME` set to a scratch directory, or it opens your real dev database.

## API — Frontend ↔ Backend
Typed API functions live in `src/api/index.ts` and are imported from `src/api`:
```ts
import { GetClients, CreateClient } from "src/api"
const clients = await GetClients(organizationId)  // GET /api/organizations/{id}/clients
```
`src/api/client.ts` is the fetch wrapper.
- Auth is a JWT in the **httpOnly `fc_token` cookie** (never localStorage or Bearer). The wrapper sends `credentials: "same-origin"` and an `X-CSRF-Protection` header.
- `csrfRequired` rejects a POST/PUT/PATCH/DELETE without that header. This stateless CSRF defense works because there's no permissive CORS, and the cookie is `SameSite=Lax`.
- `login` returns only `{user}`. `logout` POSTs so the server can expire the cookie and revoke sessions. The OIDC callback sets the cookie and redirects to `/`.
- API errors throw `Error(message)`. Handlers return JSON, and errors are `{"error": "message"}`.

`api/router.go` is the authoritative route list. Route behavior and the full authorization model are in `api/CLAUDE.md`.

## File Structure
- `main.go`: the entry point. It opens the DB, seeds the first admin, and mounts the router through `serverHandler`. Its `healthcheck` subcommand is the image's HEALTHCHECK, and it shuts down gracefully on SIGTERM.
  - `compressAPIResponses` (`compress.go`) wraps **outside** `api.RequestLogger`, never inside, or the activity history loses created ids (`TestServerHandlerKeepsActivityHistory`).
  - `static.go` serves the embedded `dist/`. Hashed `/assets/*` are immutable for a year (a missing one is a 404, never the SPA shell); everything else is `no-cache` with an ETag.
  - `securityHeaders` sets the CSP: same-origin, no `wasm-unsafe-eval`, no blob/data fetches.
- `Dockerfile`: node → golang → debian (see Docker). `docker-compose.yml`: one service, `/data` volume. `docker-compose.oidc.yml`: the OIDC overlay (`docs/oidc-sso.md`).
- `cmd/seed-demo/`: demo data through the real API.
- `docs/`: `logging.md` (reading the server log on the Pi), `oidc-sso.md`, `inventory-cogs-integration.md` (the GRNI/COGS design), and the audit plans (`audit-plan-*.md`).

## Cross-Cutting Invariants
These apply on both sides of the stack; the detail lives in the subdirectory files.
- **Money** is integer cents in storage, the API and atoms; only the form layer converts. The server rounds with `roundCents` and re-validates totals with exact rationals.
- **Dates** are Unix milliseconds. **Ids** are 21-char nanoids (every `Create*` generates one when `req.ID == ""`). A stored date is the moment of entry or a picked local midnight. **Server code that turns a timestamp into a day, month or year must use the organization's zone**: `orgLocation`/`organizationLocation` (`db/timezone.go`; unset = UTC), never `.UTC()`, `time.Local` or SQLite `strftime`. Day-based queries send `calendarDayMs` (`src/utils/date.ts`, UTC noon of the picked date), which the server decodes with `calendarDayStart`.
- **Document statuses/states** are defined twice and must stay in sync: the Go map in `db/<doc>.go` and `src/types/<doc>.ts`. Status changes go through `PATCH …/status` (or `/state`), never `PUT`.
- **Posted journal entries are immutable**: reversed, never edited or deleted. Every posting goes through `allocateAndFinalizeEntryTx`.
- **Inventory valuation** is per organization (`organizations.inventoryValuation`). `perpetual` posts GRNI/COGS/adjustments and refuses an uncosted stock-out. `quantity_only` posts none of them. Every inventory posting path checks `isQuantityOnlyInventory`; switching rules are in `db/CLAUDE.md`.
- **Authorization is org-scoped**:
  - **Wrappers:** `orgMemberProtected` (any member), `orgRoleProtected` (admin/power_user/general or the listed domain roles), `orgRoleAdminProtected` (admin plus the listed role), `orgAdminProtected` (org admin), `platformAdminProtected` (global actions only: users, backups/restore, countries).
  - **Roles** (`0081`/`0086`): `admin`, `power_user`, `general`, `sales`, `purchasing`, `accounting`, `cashbook`. `users.isPlatformAdmin` grants no organization access.
  - **Section guard:** `api/sections.go` (twin of `src/layouts/role-menu.ts`) 403s a restricted role on the sections its UI hides.
  - **Configuration writes** (F170): products by sales/purchasing, tax rates and payment terms by accounting, units, families, templates and numbering by the org-wide roles.
  - **Outside the wrappers:** body-organization `POST` creates (`requireOrgMember`/`requireOrgRole` after `decodeJSON`), self-limiting routes, and the LibreOffice export and restore routes registered on `mux` (they check inline so a conversion never runs under `withDB`'s lock).
  - **Tripwires:** `api/cross_org_test.go` and `api/sections_test.go`'s `TestSectionRouteCoverage` fail the build for any unclassified route.
- **Modal/Drawer forms** never use module-level Jotai atoms for local state — use `useState` (the mask gets orphaned and freezes the UI).
- **Lint stays at zero warnings.** `pnpm lint` runs `oxlint --deny-warnings`, so CI fails on any new warning — fix it (see `src/CLAUDE.md`'s Data loading section for the `set-state-in-effect` patterns) rather than suppressing it.

## Product decisions not to revisit
- **Server error messages are English only.** Every `db/*.go` validation message is plain English. The frontend translates only its generic fallback. Accepted as a product decision (`docs/audit-plan-2026-08-09.md`): don't re-flag it or build a translation table without a new decision.
- **The Cash Book may collect any open invoice** of the organization, not only Cash Book sales (F150). A Cash Book sale takes its own stock out, and stock may go negative. Serialized products are refused at the counter.
- **Invoice numbering stays on the `organizations` columns**, outside `document_number_settings` (which covers orders, POs, deliveries, receipts and production orders).
- **Both Cash Book layouts stay** until the owner picks one (F186).
- **The runtime image is Debian, not Alpine**: `libreoffice-calc` crashes on Alpine/musl with every known workaround. Don't revert. The ~630 MB it adds is accepted.

## Where the Rest of the Rules Live
Subdirectory `CLAUDE.md` files load automatically once Claude reads a file in that directory. For planning work that touches no files yet, read the relevant one explicitly.
- `db/CLAUDE.md` — schema conventions, status machines, stock/GL/costing rules, Cash Book, templates and exports
- `api/CLAUDE.md` — authorization model, sessions, route behavior, per-file notes for `api/*.go`
- `src/CLAUDE.md` — state and data-loading conventions, forms, navigation and roles, layout rules, per-screen notes
- `cmd/seed-demo/CLAUDE.md` — the demo-data seeding tool

## Internationalization
- Locales en (source), de and fr, all fully translated. The set lives in `lingui.config.ts`; the switcher, the antd/dayjs wiring in `src/app.tsx` and `dynamicActivate` in `src/utils/lingui.tsx` follow it.
- Run `pnpm extract` after changing UI strings and fill in de/fr. When several branches touch strings, merge them one at a time and re-extract after each.

## Docker
```bash
docker compose up --build
docker build -t fatura-cloud .
# bind-mounted ./data owned by 1000:1000 (the container user)
docker run -p 8080:8080 -v ./data:/data fatura-cloud
```
- Stages:
  1. **frontend** (node:24-alpine, the LTS line: Node 25 dropped corepack): `corepack enable` (pnpm from `packageManager`; `npm install -g pnpm` fails on arm64), then `pnpm install --frozen-lockfile` and `pnpm build`.
  2. **backend** (golang:1.26-alpine): embeds `dist/` (`//go:embed all:dist`).
  3. **runtime** (debian:trixie-slim): the binary plus `libreoffice-calc fonts-liberation fonts-dejavu-core`, running as `1000:1000`.
- `--build-arg VERSION=<tag>` sets `GET /api/version` and the Sentry release; compose passes `VERSION` (default `dev`).
- `GET /api/healthz` backs the HEALTHCHECK. On SIGTERM, in-flight requests get 25 s and the background loops stop through `api.NewRouterContext`; compose sets `stop_grace_period: 30s`.
- CI builds the `backend` target on every PR. `.github/workflows/runtime-image.yml` runs the whole image (health, PDF conversion, graceful stop) on image changes, weekly and on demand. Dependabot updates weekly.
- Sentry inputs are optional and left out of the published GHCR image (`.github/workflows/docker.yml`), so the public image never reports to this project:
  - `--build-arg VITE_SENTRY_DSN=<dsn>` (compose reads it from your `.env`);
  - `--secret id=sentry_auth_token,env=SENTRY_AUTH_TOKEN` (a BuildKit secret, never a build-arg) uploads source maps (Sentry org and project are in `vite.config.ts`; CI uses the `SENTRY_AUTH_TOKEN` secret, skipped when absent).
- Source maps never ship: `build.sourcemap` is `"hidden"` and `filesToDeleteAfterUpload` deletes `dist/**/*.map`.

## Environment Variables
- `PORT` (default `8080`); `LOG_LEVEL` (`debug`/`info`/`warn`/`error`) and `LOG_FORMAT` (`text`/`json`). See `docs/logging.md`.
- `JWT_SECRET` (default `dev-secret-change-me-in-production`, **must be set in production**); `ADMIN_EMAIL`/`ADMIN_PASSWORD` for the first admin (defaults `admin@fatura.cloud`/`admin`, change in production).
- `TRUSTED_PROXIES`: the IPs or CIDRs of reverse proxies allowed to set `X-Forwarded-For` and `X-Forwarded-Proto`. Unset, the rate limiter keys on the TCP peer and cookies aren't `Secure` behind a proxy. List only proxies that are the sole path to the app. `api.IsTrustedProxyPeer`/`api.IsHTTPS` are the one implementation behind `clientIP`, `setAuthCookie`, `setOIDCStateCookie` and HSTS.
- `VITE_SENTRY_DSN` (build time; unset = Sentry off), `VITE_SENTRY_ENABLED=true` (Sentry in dev), `VITE_JOTAI_DEVTOOLS_ENABLED=true`.
- `OIDC_ISSUER_URL` enables SSO (unset = fully off). Also `OIDC_CLIENT_ID`/`OIDC_CLIENT_SECRET`/`OIDC_REDIRECT_URL` (must match the provider exactly), `OIDC_SCOPES` (default `openid profile email groups`), `OIDC_EMAIL_CLAIM`/`OIDC_NAME_CLAIM`/`OIDC_GROUPS_CLAIM`, and `OIDC_ADMIN_GROUP` (default `admins`). See `docs/oidc-sso.md`.

## Adding a New API Endpoint

**Go side** — add a handler method in the relevant `api/{domain}.go` file, then register the route in `api/router.go`:
```go
// an ordinary org-scoped route — any member of the resolved organization:
orgMemberProtected("GET", "/api/organizations/{orgId}/things", pathOrgID("orgId"), h.listThings)
// an org-scoped mutation limited to admin/power_user/general or one of the given domain roles:
orgRoleProtected("PUT", "/api/things/{id}", thingOrgID, []string{"sales"}, h.updateThing)
// an org-scoped admin-tier action (admin plus the given role):
orgRoleAdminProtected("POST", "/api/fiscal-years/{id}/close", fiscalYearOrgID, []string{"accounting"}, h.closeFiscalYear)
// or for a genuinely global admin-only action (no natural per-org owner):
platformAdminProtected("DELETE", "/api/things/{id}", h.deleteThing)
// or for an org-scoped admin action:
orgAdminProtected("DELETE", "/api/organizations/{orgId}/things/{id}", pathOrgID("orgId"), h.deleteThing)
```

An org-scoped **`POST` create** route whose `organizationId` lives in the JSON body can't use an org resolver (it would run before `decodeJSON`), so it uses plain `protected()` plus an inline `h.requireOrgMember(...)` / `h.requireOrgRole(..., "<role>")` in the handler. The route-coverage tripwires in `api/cross_org_test.go` (`TestPhaseCRouteCoverage`, `TestDomainRoleRouteCoverage`, `TestCreateRouteOrgChecksArePresent`) parse `router.go` and fail the build for any new route that uses none of the wrappers or isn't explicitly classified — so following this recipe is required, not optional. A new route also needs a place in `api/sections.go` (`TestSectionRouteCoverage`).

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
