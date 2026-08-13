# Changelog

All notable changes to FaturaCloud will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [3.7.3] - 2026-08-13

Routine dependency-currency maintenance from a security audit; no
user-facing behavior change.

### Security
- Bumped `golang.org/x/crypto`, `golang.org/x/text`, and
  `modernc.org/sqlite` to their latest patch versions.
- Refreshed npm dependencies to latest, including `pdfjs-dist` (5→6) and
  `@babel/core` (7→8).

## [3.7.2] - 2026-08-09

### Fixed
- The v3.7.1 spacing fix between the wordmark and version label was too
  subtle to read as a visible gap once rendered with the actual version
  string; widened it further.

## [3.7.1] - 2026-08-09

### Fixed
- The sidebar's version label sat flush against the FaturaCloud wordmark
  with no visible gap; increased the spacing between them.

## [3.7.0] - 2026-08-09

### Added
- The sidebar now shows the running app version next to the FaturaCloud
  wordmark, fetched from the existing `GET /api/version` endpoint.
- A new organization created with Country "Germany" or "France" now seeds a
  curated starter chart of accounts drawn from that country's real,
  citable statutory numbering — SKR04 (Standardkontenrahmen 04) for
  Germany, Plan Comptable Général for France — instead of the generic
  placeholder chart every organization got before. SKR04 codes double as
  DATEV account numbers, so the existing DATEV exporter works out of the
  box for a German organization. Every other country is unaffected.

## [3.6.1] - 2026-08-09

Follow-up to a security/dependency/i18n audit of the Reporting menu and
recent GL work (`docs/audit-plan-2026-08-09.md`).

### Fixed
- The Reporting menu's endpoints (Revenue Trend, Sales by Client/Product,
  Purchases by Vendor, Tax Summary) could run an unbounded aggregate over an
  organization's entire document history if called with no date range.
  A missing start date now defaults to a 5-year lookback instead of staying
  unbounded.
- Reporting and dashboard chart tooltips showed the raw field name
  ("revenue"/"spend") instead of a translated label.
- The Revenue Trend report's table view rendered the month column as a raw
  "YYYY-MM" string instead of a locale-formatted month name.

### Security
- Pinned all GitHub Actions in CI/release workflows to commit SHAs instead
  of floating version tags.
- Bumped the Docker runtime base image from Alpine 3.21 to 3.24.

## [3.6.0] - 2026-08-09

A new Reporting menu with sales/purchasing analytics, deliberately kept
separate from Accounting's GL-derived reports since the two are computed
from different sources and aren't expected to reconcile without that
context.

### Added
- **Reporting menu**: Revenue Trend, Sales by Client, Sales by Product,
  Purchases by Vendor, and Tax Summary — computed directly from source
  documents (invoices, incoming invoices) with a real date range and no
  top-10 cap, rather than the Dashboard widget's fixed rolling window.
- **Tax Summary**: output VAT (sales) vs input VAT (purchases), grouped by
  tax rate, including zero-rated/exempt turnover that a GL posting would
  otherwise omit.

### Changed
- The Dashboard's revenue/top-client/top-product queries now share the same
  underlying functions as the new Reporting endpoints, so the two never
  drift apart.

## [3.5.0] - 2026-08-09

A full double-entry general ledger, closing the gap between FaturaCloud's
sales/purchasing documents and real bookkeeping — chart of accounts through
statutory export, plus inventory/COGS integration.

### Added
- **General ledger core**: chart of accounts, journals, fiscal years/periods,
  and journal entries with post/reverse. A single balance-enforcing choke
  point guarantees every posted entry balances and blocks posting into a
  closed fiscal year.
- **Auto-posting**: invoices and vendor bills post and reverse GL entries
  automatically on state change. A payments ledger supports partial
  payments, one payment settling multiple documents, and realized FX
  gain/loss computed against each document's own frozen exchange rate.
- **Reports**: trial balance, profit & loss, balance sheet, AR/AP aging, and
  inventory valuation — all computed live from journal lines, never cached.
- **Fiscal year closing**: rolls revenue/expense accounts into retained
  earnings (irreversible, admin only).
- **Statutory export**: France FEC and Germany DATEV Buchungsstapel (EXTF),
  both admin-only downloads gated by fiscal year; DATEV's format was
  cross-verified against two independent open-source implementations plus a
  real example file.
- **Inventory / COGS integration**: goods receipts accrue GRNI (Goods
  Received Not Invoiced); vendor bills clear it and capitalize stock-tracked
  purchases to Inventory instead of expensing them immediately; shipments
  recognize COGS (weighted-average cost, or specific-identification for
  serialized units); manual stock adjustments post a GL trace. The new
  Inventory Valuation report compares the GL's Inventory balance against
  independently computed stock value.

## [3.4.1] - 2026-08-03

### Fixed
- Product unit words ("hour", "day", "week", "month", "piece") were
  hardcoded in the product form's Unit dropdown and re-displayed raw on
  the products list and inventory grid regardless of the active locale —
  they bypassed LinguiJS extraction entirely rather than being untranslated
  entries in the catalog. Now translate correctly in German and French,
  same as everywhere else in the app. Two hardcoded error messages in the
  invoice PDF-preview panel and a hardcoded "Tax" fallback label got the
  same fix.

## [3.4.0] - 2026-08-03

Per-unit serial number tracking for stock-enabled products, plus a currency
field layout fix across purchase orders and goods receipts.

### Added
- **Serial number tracking**: a stock-enabled product can now be marked as
  individually serialized. Manual stock movements, goods receipts, and
  deliveries for a serialized product require exact serial numbers on
  in/out movements instead of a bare quantity, posting one stock movement
  per physical unit rather than one aggregate row per line. A product's
  serial history is available per unit, with in-stock status computed from
  its latest movement rather than stored. Toggling serialization is
  blocked in either direction while a product's stock is non-zero, since
  either direction would strand the registry against untracked stock.

### Fixed
- Purchase orders and goods receipts gave the Currency field its own
  dedicated row alongside the (usually hidden) exchange rate fields — when
  the document currency matched the organization's, that row rendered
  nothing but a lone dropdown with most of the row empty. Currency now
  shares its header row with the document's other fields on every
  document page, and the exchange rate row only appears when there's
  actually a conversion to show.

## [3.3.0] - 2026-08-02

Closes the last gap in 3.2.0's multi-currency rollout, tightens a couple of
delete guards, and fixes a backlog of missing/broken translations.

### Added
- **Order exchange rate**: orders got a currency column in 3.2.0 but were
  missed when `exchangeRate`/`exchangeRateDate` were added to every other
  document — a foreign-currency order had no way to record its conversion to
  the organization's currency. Orders now go through the same rate
  validation/prefill as purchase orders and incoming invoices.

### Fixed
- Deleting a stock movement generated by a shipped delivery or a received
  goods receipt bypassed those documents' own cancel-first guards and could
  desync `stockQuantity`/average cost from a document that still claims it
  happened. Blocked, same as deleting the document itself.
- A paid incoming invoice (vendor bill) could be deleted outright, silently
  dropping it from other invoices' double-billing check against the same
  order/receipt line — mirrors the existing guard on sales invoices.
- German and French translations had gone stale since before 3.2.0 (extraction
  wasn't re-run after that release): the org data-reset feature, the
  exchange-rate fields on purchase orders/incoming invoices, and the order/
  delivery status labels all fell back to raw English. Backfilled, plus three
  form fields that bypassed translation entirely.
- The delivery note and order confirmation PDFs never actually used the i18n
  they imported — every label was hardcoded English regardless of locale.
  Both now translate like the invoice and purchase order PDFs.

## [3.2.0] - 2026-08-02

Multi-currency support across master data and documents, plus a per-organization
data reset tool.

### Added
- **Multi-currency support**: organizations get a currency + decimal-precision
  setting (precision defaults from the currency, e.g. JPY → 0, BHD → 3, still
  user-overridable); clients get a default currency (mirroring vendors); orders,
  purchase orders, and incoming invoices all get a currency picker (incoming
  invoices' free-text currency field is now a proper picker). Invoices, purchase
  orders, incoming invoices, and goods receipts gain `exchangeRate`/
  `exchangeRateDate` fields (1 unit of document currency = `exchangeRate` units
  of organization currency), manually entered and frozen at save time, prefilled
  from the last rate used for that currency pair. Goods-receipt unit costs
  convert to organization currency before feeding `stockMovements`/average
  cost; 3-way matching and dashboard aggregates (revenue, outstanding, top
  clients/products) now compare/sum in organization-currency terms rather than
  mixing raw amounts across currencies — resolving the mixed-currency
  limitation called out in the 3.1.0 Dashboard notes above.
- **Per-organization data reset** (Organizations page, admin only): a "Danger
  zone" section with **Master data** (clients, vendors, products, tax rates)
  and **Transactional data** (invoices, orders, deliveries, purchase orders,
  goods receipts, incoming invoices, stock movements) checkboxes to wipe an
  organization's records without deleting the organization itself. Selecting
  master data always includes transactional data, since clients cascade onto
  invoices and vendors are referentially protected against purchasing
  documents. Resetting transactional data also zeroes the invoice number
  counter and product stock quantities.

### Fixed
- Invoice detail page formatted every amount using the organization's currency
  instead of the invoice's own.

## [3.1.0] - 2026-07-27

Tier 4 of the UI-consistency plan: server-side scale for Products/Inventory,
and a new Dashboard page.

### Added
- **Dashboard** (`/dashboard`) — a new standalone top-level page (not nested
  in a sidebar group) with revenue over time, outstanding invoices aged into
  buckets (Current, 1-30, 31-60, 61-90, 90+ days), stock valuation, and top
  clients/products by revenue, over a selectable 3/6/12/24-month window.
  "Revenue" is defined consistently as `sent`/`paid` invoices only. First use
  of a charting library (`@ant-design/plots`) in this app, chosen for its
  built-in antd dark-mode theming. Known limitation: totals sum naively
  across an organization's invoices regardless of currency — there is no
  currency-conversion logic anywhere in this codebase, so a mixed-currency
  organization's totals are not currency-converted.
- Server-side pagination, search, and sorting for the Products and Inventory
  (stock movements) list pages, replacing full-table client-side fetch and
  filtering — the five line-item product pickers elsewhere in the app
  (invoices, orders, deliveries, purchase orders, inbound deliveries)
  continue to receive the complete, unpaginated product list unaffected.

## [3.0.0] - 2026-07-26

A large release spanning e-invoicing groundwork, a PDF redesign, and a full UI
consistency pass across the app (master-data forms, settings, and all six
document detail pages now share the same layout and line-item table
components). **One breaking change**: the free-text address field is gone
(see Removed).

### Removed
- **BREAKING:** the free-text `address` field on clients, organizations, and
  vendors has been removed from the API and the database — structured
  `street`/`house_number`/`postal_code`/`city`/`country_code` fields (added
  for e-invoicing) are now the single source of truth everywhere (forms,
  lists, PDFs). **Migration:** on upgrade, any existing free-text address with
  no structured data yet is best-effort backfilled into `street`; this is a
  one-time backfill, not a parser, so review addresses on multi-line legacy
  records after upgrading. The column drop cannot be reversed without data
  loss
- Duplicate Logo card removed from Settings → Invoice — Organizations → edit
  already covers the same upload/delete for every organization, and the
  removed card's caption wrongly implied a per-invoice override that never
  existed

### Added
- **Country-aware e-invoicing** — `GET /api/invoices/{id}/e-invoice` renders
  an invoice as an EN 16931 UBL 2.1 XML document, profile resolved from the
  **buyer's** country (falling back to the seller's when unset): `"DE"` gets
  XRechnung 3.0 (CustomizationID + mandatory buyer reference), every other
  country gets a generic EN 16931 core profile rather than a guessed CIUS.
  Peppol BIS Billing 3.0, Tunisia's TEIF/TTN, and ZUGFeRD are explicitly out
  of scope — see `CLAUDE.md` for why. No EN 16931 validator is available in
  this environment; validate externally before relying on this for real B2G
  submission
- **Redesigned PDF documents** — invoice, purchase order, order confirmation,
  and delivery note now share a dark header block, boxed party cards, and a
  dark table header with striped rows. Invoices additionally get a SEPA
  payment QR code (EPC069-12 "GiroCode"), shown for EUR invoices on an
  organization with an IBAN set
- **Organization logo** moves to dedicated `GET/POST/DELETE
  /api/organizations/{id}/logo` endpoints (raw bytes, sniffed content type,
  2 MB cap) instead of a base64 blob inside the organization JSON, and gains
  an upload card on the Organizations list edit drawer, not just Settings
- **Country activation picklist** (Settings → Countries, admin-only) — every
  ISO 3166-1 alpha-2 country can be individually activated; the Client,
  Vendor, and Organization country fields become a `Select` scoped to the
  active set instead of free text, with names resolved via
  `Intl.DisplayNames` (no translation table to maintain)
- **Product is now required on new line items** across invoices, purchase
  orders, orders, and both delivery types (incoming invoices excluded — they
  have no manual product picker). Applies only to newly added lines; existing
  saved lines aren't retroactively required to have one. Invoice line items
  also now actually persist their product link (`invoiceLineItems` was
  missing the column entirely, silently discarding it on save)
- **Unified line-item table** — all six document detail pages (invoices,
  orders, deliveries, purchase orders, inbound deliveries, incoming invoices)
  now share one config-driven `LineItemsTable` component instead of six
  hand-rolled copies that had drifted apart on column order, labels, and
  affordances. Cell inputs stay borderless until hovered or focused; the
  drag-to-reorder handle (invoices only) moved into the row-number column,
  visible on row hover or keyboard focus
- **Responsive layout** for master-data drawers (clients, vendors, products),
  Settings → Invoice, and every document detail page's header fields — none
  of these reflowed below their designed desktop width before; they now stack
  to fewer columns down to phone width

### Changed
- Invoices now lead with **Product**, matching purchase orders, orders, and
  both delivery types (previously the only document leading with Description)
- Toasts and confirm dialogs (`message.*`, `Modal.confirm`) now render with
  the active theme — previously always light-styled regardless of dark mode,
  since antd's static APIs can't see `ConfigProvider`'s theme context

### Fixed
- Settings → Invoice's Save button is now reachable via a sticky footer,
  matching every other edit screen — previously the only Save button in the
  app with no sticky affordance, scrolling out of view below ~750px viewport
  height
- Delivery line items are now frozen (matching the server's existing rule)
  once a delivery is shipped or delivered — previously the UI let you edit
  and hit Save only to get a 409, with no prior indication the fields were
  off-limits
- Order and delivery confirmation PDF pages had the same infinite-fetch-loop
  bug already fixed for invoices/purchasing pages in 2.2.1/2.2.2, just not yet
  hit by testing — same cause, same `loadable()`-based fix, now replaced
  repo-wide by a non-deprecated userland implementation

## [2.2.2] - 2026-07-25

### Fixed
- Purchase order, goods receipt, and incoming invoice detail pages had the
  same infinite fetch loop as the v2.2.1 invoice fix (#31), just not yet
  confirmed by reproduction — opening an existing record hung indefinitely
  instead of rendering. Same cause and same fix, applied to all three

## [2.2.1] - 2026-07-25

### Fixed
- Invoice details page (`/invoices/{id}`) hung indefinitely on load, stuck
  re-fetching the invoice and its line items in an infinite loop instead of
  ever rendering. Caused by reading an async Jotai atom in a way that
  suspended the app's top-level route boundary on every update

## [2.2.0] - 2026-07-25

Adds the buy-side mirror of FaturaCloud's existing sell side: vendors, purchase
orders, goods receipts, and incoming invoices with 3-way matching. Non-breaking.

### Added
- **Vendors** — new master data alongside clients, with the same fields (address,
  emails, VATIN, …) plus default currency and payment terms. A vendor can't be
  deleted while any purchase order, goods receipt, or incoming invoice still
  references it
- **Purchase orders** — draft → confirmed → received → cancelled, server-numbered
  (`PO-0001`, …), with a translated PDF (with prices) to send to the vendor. Each
  line shows quantity received to date against quantity ordered
- **Goods receipts** (inbound deliveries) — recording a receipt posts incoming
  stock movements and, unlike shipping stock out, needs no availability check;
  cancelling a received receipt reverses those movements but is blocked if the
  goods have since been shipped out on an outbound delivery, which would drive
  stock negative. Receipts can be created standalone or prefilled from a purchase
  order's outstanding quantities
- **Moving-average product costing** — `products.unitCost` is now derived by
  replaying a product's full stock-movement history (`db/product_cost.go`)
  instead of being a hand-typed number nobody could trust. Costed receipts move
  the weighted average; uncosted inflows and all outflows move at the current
  average without changing it. A product with no costed purchase history keeps
  whatever cost was typed into it
- **Incoming invoices** (vendor bills) with **3-way matching** — each line is
  compared against what was ordered and what was actually received, computed on
  read so it can never go stale. Approving an invoice is blocked while any line
  has an unresolved variance (over-billed, over-received, price mismatch)
  unless explicitly overridden with a required reason; a draft can still be
  saved with variances so they can be investigated first. Match tolerances are
  configurable per organization under Settings → Invoice
- New **Purchasing** sidebar group: Purchase Orders → Goods Receipts → Incoming
  Invoices

## [2.1.0] - 2026-07-25

### Security
- Bumped `react-router` 8.2.0 → 8.3.0, patching a CSRF bypass that allowed action execution before the framework's 400 response ([GHSA-qwww-vcr4-c8h2](https://github.com/advisories/GHSA-qwww-vcr4-c8h2))
- Response security headers now match STBVirement's baseline: added `Permissions-Policy` and a CSP `form-action 'self'` directive, and switched `Referrer-Policy` to `strict-origin-when-cross-origin`

### Changed
- Header layout unified with STBVirement: the organization selector moved to the left (next to the sidebar toggle), and the right-side controls now appear in the same order on both apps — theme toggle, language, then user/logout

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
