# Changelog

All notable changes to FaturaCloud will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [2.0.0] - 2026-07-19

A follow-up audit (security, UI, performance, dependency freshness) building on the
1.2.x backend hardening, a reduction of the supported-language set, and a rework of
how sessions are authenticated. Two breaking changes relative to 1.2.x: the
supported-language reduction and the move to cookie-based session authentication
(see Removed and Security).

### Removed
- **BREAKING:** supported languages reduced to English, German, and French. The eight other locales (en-GB, Estonian, Finnish, Greek, Dutch, Portuguese, Swedish, Ukrainian) and their translation catalogs have been removed. A user who had selected one of the removed languages falls back to English on next load — no data, API, or configuration change is required

### Security
- **BREAKING:** session authentication now uses an httpOnly, `SameSite=Lax` cookie (`fc_token`) instead of a JWT in `localStorage` with an `Authorization: Bearer` header. Because page JavaScript can no longer read the token, an XSS flaw can no longer exfiltrate a session. State-changing requests (POST/PUT/PATCH/DELETE) now require a custom `X-CSRF-Protection` header, which the browser will only attach for same-origin requests — a stateless CSRF defense. The OIDC callback sets the cookie and redirects to `/` rather than passing the token in a URL fragment. **Migration:** all existing sessions are invalidated on deploy and users simply log in again; any non-browser API client must switch from the `Authorization` header to the cookie plus the `X-CSRF-Protection` header
- HSTS: on HTTPS requests the server now sends `Strict-Transport-Security` (`max-age=63072000; includeSubDomains`), so once a browser has loaded the app over HTTPS it refuses to downgrade to plain HTTP. The header is only emitted for requests that arrived over HTTPS, so plain-HTTP LAN deployments are unaffected
- Request bodies are now size-capped (10 MiB) on every JSON endpoint, including the unauthenticated login endpoint, closing a memory-exhaustion vector — previously only the database-restore upload was bounded
- Invoice state is validated server-side against the allowed set on create and on the state-change endpoint, and can no longer be set through a plain `PUT` — mirroring the order/delivery hardening from 1.2.x
- Issued JWTs are now bound to this application via issuer/audience claims, enforced on every request, so a token minted for another service that happens to share the signing secret is rejected (invalidates outstanding tokens once — users simply log in again)
- Source maps are no longer shipped in the deployed image: the Go binary embedded ~14 MB of maps that exposed the full original TypeScript source at `/assets/*.js.map`. They are now uploaded to Sentry at build time and deleted from the artifact
- `jotai-devtools` (which pulled in a vulnerable `jsondiffpatch`) moved to dev-only dependencies, taking the advisory out of the production dependency tree

### Added
- Overdue invoices are now visually flagged in the invoices list: a sent (unpaid) invoice past its due date shows its due date in a danger color with an "Overdue" tooltip
- Per-account login rate limiting: login attempts are now throttled per email address in addition to per source IP, so rotating source addresses can no longer grind a single account's password unchecked
- Dependency vulnerability scanning in CI — `govulncheck` on the Go module (call-graph aware) and `pnpm audit --prod` on the frontend

### Changed
- German and French translations are now complete (zero untranslated strings against the English source)
- Much faster initial load: the frontend is now route-code-split, so the login screen no longer downloads the PDF-rendering stack, drag-and-drop, and every settings page up front — the main bundle dropped from ~1.19 MB to ~190 kB gzipped, and the PDF engine loads only when an invoice is opened
- List tables (invoices, orders, deliveries, clients, products, tax rates, organizations, users) are now paginated instead of rendering every row at once
- Upgraded Ant Design 5 → 6
- Typed the frontend API layer and list-page table callbacks against shared domain models, removing pervasive `any` and catching several latent null-handling bugs at compile time (internal; no behavior change)
- Dependency upgrades: react-pdf 9→10 (with pdfjs 4→5, picking up upstream PDF.js security fixes), react-router 7→8, TypeScript 5.9→7, and routine minor/patch bumps across Lingui, Sentry, Vite, and the Go modules

### Fixed
- Invoice state is now consistent across the app: the list filter, the state dropdown, and the details page share one vocabulary (draft / sent / paid / cancelled). Previously a cancelled invoice could render as a raw untranslated tag, the list filter offered a dead "Confirmed" option and no way to filter "Sent", and the state labels went stale after switching language
- The organizations list no longer ships each organization's logo image — a potentially multi-MB payload that was re-downloaded on every login/refresh for organizations the user wasn't even viewing

## [1.2.6] - 2026-07-12

### Fixed
- OIDC login no longer silently demotes the last active admin: `provisionOrSyncUser` re-syncs a user's role from the IdP's `groups` claim on every SSO login, and previously did so unconditionally — a token missing the claim (or not mapping to `OIDC_ADMIN_GROUP`) could strip the only admin account of its role with no safeguard, unlike the local admin-user endpoints which already refuse to demote the last admin. The SSO sync path now applies the same guard: it logs a warning and leaves the role untouched instead

## [1.2.5] - 2026-07-12

A full security and robustness audit of the backend, database layer, and Docker/CI
setup, plus everything shipped as untagged v1.2.3/v1.2.4 builds.

### Added
- CI workflow that runs `go vet`, `go test -race`, and the frontend lint/build on every push and pull request — previously the only workflow was the tag-triggered Docker image publish
- Content-Security-Policy header on all responses; backup files/directories are now created with owner-only permissions instead of world-readable

### Fixed
- Eliminated a data race on the database handle: every request now holds a read lock for its duration, and a failed database restore no longer leaves the app running with a nil database handle — it validates the upload before swapping and rolls back to the pre-restore safety backup on failure instead of bricking the process
- Delivery and order status can now only change through their dedicated PATCH endpoints, with transitions validated server-side; `PUT` requests can no longer set `status` directly (which previously bypassed stock adjustments and allowed shipping the same delivery twice), and line items of an already-shipped delivery can no longer be edited
- Deactivating or deleting a user now revokes their existing session immediately instead of leaving their token valid until it expires (up to 24h later)
- `updateUser`: a role-only change is no longer silently dropped, role is validated, an admin can no longer demote or deactivate their own account, and updating a nonexistent user now returns 404 instead of an empty 200
- Login no longer leaks via response timing whether an email is registered; new users now require a valid email and an 8+ character password
- Delivery creation and line-item replacement are now transactional; delivery numbers are derived from the highest existing number instead of a row count, so deleting a draft delivery can no longer cause the next one to collide with it
- Database errors during user management are now handled and logged instead of silently discarded; "email already exists" is only returned for an actual conflict, not any insert failure
- API error responses no longer echo raw JSON-decoder messages back to the client
- Added the missing down-migration for the orders table
- Unmatched `/api/*` paths now return a JSON 404 instead of the SPA's `index.html`; directory paths under the embedded static assets no longer render a listing
- Restricted organization deletion — which cascade-deletes all of its clients, invoices, orders, and deliveries — to admins
- Invoice `total`/`taxTotal`/`subTotal` are now recomputed and validated server-side against the line items before every create/update and rejected on mismatch, instead of trusting client-supplied totals verbatim
- Docker images were missing `public/` (logos, favicons), which fell through to the SPA fallback; completed the English, French, and German translation catalogs, including strings added by OIDC SSO login that were never extracted

### Changed
- Docker builds now cross-compile the frontend/backend stages instead of QEMU-emulating them, cutting CI build time from ~26 minutes to under a minute

## [1.2.2] - 2026-07-05

### Fixed
- `docker-compose.yml` now bind-mounts a `./data` subfolder next to the compose file instead of a named Docker volume, so the SQLite database and backups are easy to find, back up, and copy between hosts; the container's non-root user is now a fixed uid:gid (1000:1000) so the host directory can be chowned to match ahead of time — see `deploy.md` for setup and migration steps
- The login rate limiter no longer collapses into one shared bucket for every client behind a reverse proxy (it previously keyed on the direct TCP peer, which is the proxy's own address for every request). A new `TRUSTED_PROXIES` env var lets it read the real client address from `X-Forwarded-For`, but only when the direct peer matches a configured trusted proxy, so an untrusted client can't spoof the header to dodge rate limiting

## [1.2.1] - 2026-07-04

### Added
- Sentry error tracking is now wired to a real project (DSN via the `VITE_SENTRY_DSN` build-arg, off by default) instead of a dummy placeholder; the published GHCR image ships without a DSN so third-party deployments don't report into this project's Sentry account. Source-map upload (`vite.config.ts`) now tags releases with the same version string the running app reports, so uploaded maps actually match reported events — previously they were always tagged `"development"` since `GITHUB_SHA` never reached the Docker build

## [1.2.0] - 2026-07-04

### Added
- Sidebar groups (Sales, Inventory, Master Data) are now collapsible/expandable, matching the existing Settings behavior — the active group auto-expands based on the current page
- Two new stock movement types for recording physical stock count / assessment discrepancies directly: "Stock count — surplus found" and "Stock count — shortage found", alongside the existing generic Adjustment type
- OIDC single sign-on: FaturaCloud can now authenticate against Authelia or any standards-compliant OIDC provider (Authorization Code + PKCE), with local email/password login always kept as a fallback. Off by default (`OIDC_ISSUER_URL` unset); see `docs/oidc-sso.md`
- `docker-compose.oidc.yml` simplified to route through Nginx Proxy Manager directly (no Traefik in the homelab-auth stack anymore); adds an `extra_hosts`/`NPM_LAN_IP` fix for a NAT-hairpin issue in the OIDC token-exchange call — see `docs/oidc-sso.md`'s Docker/deployment section
- Deliveries created from an order now pre-fill line items with the outstanding (not-yet-delivered) quantity per line, so a single order can be fulfilled across multiple full or partial deliveries
- Marking a delivery as shipped validates and reduces inventory for stock-tracked products, rejecting the transition with a descriptive error if stock is insufficient; cancelling an already-shipped delivery restores it via a reversing stock movement, both referenced by the delivery number
- Standalone deliveries (not linked to any order) can now pick a product per line item and get the same stock validation and movements as order-linked deliveries
- Order line items show a "Delivered X / Y" indicator reflecting quantity already fulfilled across all deliveries
- Deleting a shipped or delivered delivery is blocked — cancel it instead, which restores stock
- Products now require a unique product code (SKU); the New Product form proposes one from the product name and adjusts it automatically if it collides with an existing code. Existing products without a code were backfilled. The code is now shown wherever products are selected (orders, deliveries, stock movements, inventory) and on order confirmation / delivery note PDFs
- Invoice line items now have a product picker that fills in description, unit price, and default tax rate, matching orders and deliveries

### Fixed
- Creating an organization without a code no longer fails with a database constraint error
- The invoice PDF (both the download button and the in-place preview) always failed to render because it referenced font files that don't exist in the repo; it now uses the same built-in fonts as the order/delivery PDFs
- An invoice PDF with no due date set showed "Invalid Date" instead of a blank/dash

## [1.0.0] - 2026-06-23

Initial release of FaturaCloud — a web-based invoicing application that runs as a
single Docker image (Go HTTP server + embedded React frontend + SQLite).

### Features

#### Authentication & Users
- JWT authentication (HS256, 24-hour expiry) with Bearer token stored in `localStorage`
- Login page with per-IP rate limiting (10 attempts per minute)
- User management — admin-only page to create, edit, deactivate, and delete users
- Two roles: `admin` (full access) and `user` (standard access)
- First admin auto-created on startup from `ADMIN_EMAIL` / `ADMIN_PASSWORD` env vars

#### Organizations
- Full CRUD with a standalone list page and drawer form
- Fields: name, code (short uppercase identifier), email, phone, address, VAT, IBAN, BIC, logo
- Formatting preferences: currency, decimal places, date format, invoice number format, due days, overdue charge
- Multiple organizations supported; active org selected from header dropdown

#### Clients
- Full CRUD with search and sortable table
- Fields: name, code, email, phone, address, VAT, IBAN, BIC

#### Invoices
- Full invoice lifecycle: `draft` → `sent` → `paid` (cancel at any stage)
- Configurable invoice number format (e.g. `#{number}`, `{year}-{number}`, `{clientCode}-{number}`)
- Line items with description, quantity, unit price, tax rate, and drag-and-drop reordering
- Per-line tax rates with support for multiple rates on one invoice
- Overdue charge percentage field
- Client-side PDF generation via `@react-pdf/renderer` with logo, parties, line items, tax breakdown
- In-place PDF preview (view mode)
- Invoice duplication
- Cancel invoice action

#### Orders
- Full order lifecycle: `draft` → `confirmed` → `shipped` → `delivered` (cancel at any stage)
- Line items with product lookup (auto-fills description and unit price), quantity, unit price
- Order confirmation PDF export
- "New delivery" button links directly to the outbound delivery form pre-filled with the order

#### Outbound Deliveries
- Linked to orders (one order → many deliveries for partial fulfilment)
- Status: `draft` → `shipped` → `delivered` (cancel at any stage)
- Line items: description, quantity, unit — no prices shown on delivery documents
- Auto-generated delivery numbers (`DEL-0001`, `DEL-0002`, …)
- Delivery note PDF export (with signature areas, no prices)

#### Products & Inventory
- Products entity: physical products and services with name, price, stock tracking
- Stock movements with signed-delta storage (positive = in, negative = out/adjustment)
- `stockQuantity` always recomputed as `SUM(quantity)` across all movements
- Inventory page: record stock in, out, and adjustments with notes

#### Tax Rates
- Per-organization tax rates with name and percentage
- One rate can be marked as default and applied automatically to new line items

#### Backup & Restore
- Manual SQLite snapshot download
- File-based restore via multipart upload
- Automatic backup scheduler — configurable hour and retention count
- Backup history page — list stored backups with size, date, and one-click named restore

#### UI & UX
- Sidebar navigation grouped into: **Sales** (Invoices, Outbound Deliveries, Orders), **Inventory**, **Master Data** (Clients, Products, Organizations), **Settings**
- All create/edit forms use right-side Ant Design drawers
- Settings pages use Card-based layout
- Logged-in user shown in header with logout button; admin badge for admin users

#### Internationalisation
- 11 locales: English (en), English UK (en-GB), German (de), Estonian (et), Finnish (fi), French (fr), Greek (el), Dutch (nl), Portuguese (pt), Swedish (sv), Ukrainian (uk)
- LinguiJS with `.po` translation files; locale stored in `localStorage`

#### Infrastructure
- Single Docker image: three-stage build (node → golang → alpine)
- SQLite database; migrations applied automatically on startup
- `GET /api/version` returns the build version (injected via `--build-arg VERSION`)
- Sentry frontend error tracking and user feedback modal
