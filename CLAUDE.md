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
- `zod` — schema validation
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

### API Routes

```
# Public
GET  /api/version
POST /api/auth/login
POST /api/auth/logout

# Auth (JWT required)
GET  /api/auth/me

# OIDC SSO — public; login is entirely absent/off unless OIDC_ISSUER_URL is set
GET  /api/auth/oidc/enabled
GET  /api/auth/oidc/login
GET  /api/auth/oidc/callback

# Users (admin only)
GET    /api/users
POST   /api/users
GET    /api/users/{id}
PUT    /api/users/{id}
DELETE /api/users/{id}

# Backup
GET  /api/backups
POST /api/backups
POST /api/backups/{name}/restore
GET  /api/backup/config
PUT  /api/backup/config
POST /api/restore                         multipart upload to replace DB

# Organizations
GET    /api/organizations
POST   /api/organizations
GET    /api/organizations/{id}
PUT    /api/organizations/{id}
DELETE /api/organizations/{id}             admin only — cascade-deletes clients/invoices/orders/deliveries
GET    /api/organizations/{id}/logo        raw image bytes, sniffed Content-Type — logo isn't in the org JSON
POST   /api/organizations/{id}/logo        multipart upload, 2 MB cap
DELETE /api/organizations/{id}/logo

# Clients
GET    /api/organizations/{orgId}/clients
POST   /api/clients
GET    /api/clients/{id}
PUT    /api/clients/{id}
DELETE /api/clients/{id}
GET    /api/clients/{id}/invoice-count

# Vendors
GET    /api/organizations/{orgId}/vendors
POST   /api/vendors
GET    /api/vendors/{id}
PUT    /api/vendors/{id}
DELETE /api/vendors/{id}               refused with 409 while purchasing documents reference it
GET    /api/vendors/{id}/document-count

# Purchase Orders
GET    /api/organizations/{orgId}/purchase-orders
GET    /api/organizations/{orgId}/purchase-orders/next-number
POST   /api/purchase-orders
GET    /api/purchase-orders/{id}
GET    /api/purchase-orders/{id}/line-items
PUT    /api/purchase-orders/{id}
PATCH  /api/purchase-orders/{id}/status
GET    /api/purchase-orders/{id}/received-quantities
DELETE /api/purchase-orders/{id}

# Inbound Deliveries (goods receipts)
GET    /api/organizations/{orgId}/inbound-deliveries
GET    /api/organizations/{orgId}/inbound-deliveries/next-number
POST   /api/inbound-deliveries
GET    /api/inbound-deliveries/{id}
GET    /api/inbound-deliveries/{id}/line-items
PUT    /api/inbound-deliveries/{id}
PATCH  /api/inbound-deliveries/{id}/status
DELETE /api/inbound-deliveries/{id}

# Incoming Invoices (vendor bills)
GET    /api/organizations/{orgId}/incoming-invoices
POST   /api/incoming-invoices
GET    /api/incoming-invoices/{id}
GET    /api/incoming-invoices/{id}/line-items
GET    /api/incoming-invoices/{id}/match      3-way match, computed on read
PUT    /api/incoming-invoices/{id}
PATCH  /api/incoming-invoices/{id}/state      blocked by an unresolved variance
DELETE /api/incoming-invoices/{id}

# Invoices
GET    /api/organizations/{orgId}/invoices
POST   /api/invoices
GET    /api/invoices/{id}
GET    /api/invoices/{id}/line-items
PUT    /api/invoices/{id}
PATCH  /api/invoices/{id}/state
DELETE /api/invoices/{id}

# Tax Rates
GET    /api/organizations/{orgId}/tax-rates
POST   /api/tax-rates
GET    /api/tax-rates/{id}
PUT    /api/tax-rates/{id}
DELETE /api/tax-rates/{id}
GET    /api/tax-rates/{id}/usage-count

# Products
GET    /api/organizations/{orgId}/products
POST   /api/products
GET    /api/products/{id}
PUT    /api/products/{id}
DELETE /api/products/{id}
GET    /api/products/{id}/stock-movements

# Stock Movements
GET    /api/organizations/{orgId}/stock-movements
POST   /api/stock-movements
DELETE /api/stock-movements/{id}

# Orders
GET    /api/organizations/{orgId}/orders
POST   /api/orders
GET    /api/orders/{id}
GET    /api/orders/{id}/line-items
GET    /api/orders/{id}/delivered-quantities
PUT    /api/orders/{id}
PATCH  /api/orders/{id}/status
DELETE /api/orders/{id}

# Outbound Deliveries
GET    /api/organizations/{orgId}/deliveries
GET    /api/organizations/{orgId}/deliveries/next-number
POST   /api/deliveries
GET    /api/deliveries/{id}
GET    /api/deliveries/{id}/line-items
PUT    /api/deliveries/{id}
PATCH  /api/deliveries/{id}/status
DELETE /api/deliveries/{id}
```

All handlers return JSON. Errors use `{"error": "message"}`.

## File Structure
- `main.go` — entry point; opens DB, seeds first admin, mounts API router, serves embedded `dist/`
- `api/router.go` — wires all routes onto `*http.ServeMux`; wraps protected routes in `authMiddleware`
- `api/helpers.go` — `writeJSON`, `writeError`, `decodeJSON`
- `api/middleware.go` — JWT `authMiddleware` (also re-checks the user is still active on every request, so deactivating/deleting a user revokes access immediately rather than waiting for their token to expire), `adminOnly`, per-IP login rate limiter
- `api/auth.go` — login, logout, me handlers
- `api/oidc.go` — OIDC SSO: login redirect (Authorization Code + PKCE), callback (ID token verification, JIT provisioning), issues the same JWT local login does
- `api/users.go` — user CRUD handlers (admin only); also `provisionOrSyncUser`, the JIT-provision/role-resync used by OIDC login
- `api/{domain}.go` — HTTP handlers per domain (clients, vendors, invoices, organizations, orders, deliveries, …)
- `db/vendor.go` / `api/vendors.go` — vendor master data (the purchasing counterpart to clients). `DeleteVendor` is guarded by `GetVendorDocumentCount` and returns `ErrVendorInUse` (409) rather than letting a foreign key fail as an opaque 500 — each purchasing phase adds its own subquery to that count
- `api/utility.go` — version, backup download, restore upload, scheduler
- `db/` — Go database layer (SQLite connection, migrations, CRUD per domain)
- `db/migrations/` — SQL migration files (`*.up.sql`), applied automatically on startup
- `src/api/client.ts` — base fetch wrapper; sends the httpOnly auth cookie (`credentials: same-origin`) + the `X-CSRF-Protection` header
- `src/api/index.ts` — typed API functions, one per REST endpoint
- `src/atoms/` — Jotai state atoms; import from `src/api`
- `src/atoms/auth.ts` — `currentUserAtom`, `isAuthenticatedAtom`, `isAdminAtom`
- `src/atoms/delivery.ts` — delivery list, detail, status, and delete atoms
- `src/atoms/vendor.ts` — vendor list, detail, and delete atoms (mirrors `client.ts`, including the `emails` JSON-string ↔ array conversion)
- `src/routes/vendors.tsx` + `src/components/vendors/form.tsx` — vendors list with a `Drawer` form on the same page (the clients pattern — no detail route)
- `src/types/purchase-order.ts` — the frontend single source of truth for purchase order status (`PURCHASE_ORDER_STATUSES`, `purchaseOrderStatusColor`, `purchaseOrderStatusLabel`, `purchaseOrderTransitions`). Unlike orders/deliveries, the transition matrix lives next to the statuses so the "must stay in sync with the Go map" pairing is visible in one place
- `src/routes/purchase-orders.tsx` + `src/routes/purchase-orders/details.tsx` — purchase order list and detail/edit pages
- `src/types/incoming-invoice.ts`, `src/atoms/incoming-invoice.ts`, `src/routes/incoming-invoices.tsx`, `src/routes/incoming-invoices/details.tsx` — vendor bills with the 3-way match panel (no PDF: these are received, not issued)
- `src/types/inbound-delivery.ts`, `src/atoms/inbound-delivery.ts`, `src/routes/inbound-deliveries.tsx`, `src/routes/inbound-deliveries/details.tsx` — goods receipts (no PDF: these are internal)
- `src/components/purchase-orders/purchase-order-pdf.tsx` — purchase order PDF (with prices; sent to the vendor). Takes an `i18n` prop and translates, following `invoices/pdf.tsx` rather than the hardcoded-English order/delivery PDFs
- `src/routes/` — main application pages
- `src/routes/login.tsx` — login page (public, redirects to `/` on success); shows an "Sign in with SSO" button when `GET /api/auth/oidc/enabled` reports true
- `src/routes/deliveries.tsx` — outbound deliveries list
- `src/routes/deliveries/details.tsx` — delivery detail/edit page
- `src/routes/orders/details.tsx` — order detail/edit page
- `src/routes/organizations/index.tsx` — organizations list page (standalone, not under Settings); the edit drawer's Logo card (shown only for an existing org, not while creating one) is the STBvirement-style pattern: a plain `<img>` against the `/logo` URL with a local cache-busting key, not the data-URI atom the settings page and PDFs use, since this drawer can be editing an org other than the currently-selected one
- `src/components/` — reusable React components
- `src/components/deliveries/delivery-note-pdf.tsx` — delivery note PDF (no prices)
- `src/components/orders/order-confirmation-pdf.tsx` — order confirmation PDF (with prices)
- `src/components/orders/delivery-note-pdf.tsx` — legacy delivery note from orders (kept for reference)
- `src/components/feedback-modal.tsx` — Sentry user feedback modal
- `src/layouts/base.tsx` — main application layout with sidebar and header
- `src/types/` — shared TypeScript type definitions
- `src/utils/` — lingui.tsx (i18n setup), sentry.ts, currency.ts, currencies.tsx, countries.tsx, date.ts, invoice.ts
- `src/locales/` — translation files (.po format)
- `Dockerfile` — multi-stage build: node (frontend) → golang (backend + embed) → alpine
- `docker-compose.yml` — single service, `/data` volume for SQLite
- `docker-compose.oidc.yml` — overlay enabling OIDC SSO against homelab-auth's Authelia via Nginx Proxy Manager (no Traefik — see `docs/oidc-sso.md`); merge with `-f docker-compose.yml -f docker-compose.oidc.yml`
- `docs/oidc-sso.md` — OIDC SSO design doc: generic provider-agnostic pattern, FaturaCloud-specific implementation, security model, Authelia-side client setup

## Database
SQLite is accessed from Go via `jmoiron/sqlx`. All schema migrations live in `db/migrations/` as `*.up.sql` files and run automatically on every startup. The database file is located at:
- **Docker**: `/data/sqlite.db` (mount a volume at `/data`)
- **Local dev (macOS)**: `~/Library/Application Support/FaturaCloud/sqlite.db`
- **Local dev (Linux)**: `~/.config/FaturaCloud/sqlite.db`

Schema conventions:
- Primary keys are 21-character nanoid strings
- Monetary values stored as integer cents — the form layer converts (user input × 100 → store; stored ÷ 100 → display); atoms and API pass cents through unchanged
- Dates stored as Unix timestamps in milliseconds
- Organization logo stored as BLOB (raw bytes), read and written exclusively through `GET/POST/DELETE /api/organizations/{id}/logo` (`db.GetOrganizationLogo`/`SetOrganizationLogo`) rather than as a field on the organization JSON — `db.Organization` has no `Logo` field at all (`json:"-"` alone wouldn't stop `SELECT *` from loading a multi-MB BLOB into memory on every org fetch, so it's dropped from the struct and both `GetOrganizations`/`GetOrganization` use an explicit column list instead). `GetOrganizationLogo` also decodes the legacy storage format from before this endpoint existed, where the column held the browser's full `"data:image/png;base64,..."` string as text rather than raw bytes. On the frontend, `organizationAtom` fetches the logo alongside the organization and converts it to a data URI (`FileReader.readAsDataURL`) so PDF templates and the settings page can keep reading `organization.logo` as a ready image source; the Organizations list edit drawer instead uses a plain `<img src="/api/organizations/{id}/logo">` (cookies travel automatically on a same-origin `<img>` request) since it can be editing an org other than the currently-selected one
- `products.type` is `"product"` | `"service"` (default `"service"`)
- `products.sku` (labeled "Product code" in the UI) must be unique per organization — enforced by a `UNIQUE(organizationId, sku)` index, not a DB-level `NOT NULL` (SQLite can't add that retroactively without a table rebuild); required-ness is enforced in `api/products.go` and the frontend form instead. The New Product form proposes a code derived from the name, deduplicated against other products in the org
- `stockMovements.quantity` is a **signed delta**: positive = stock in, negative = stock out/adjustment; `products.stockQuantity` is always `SUM(quantity)` over all movements and is recomputed inside a transaction on every insert/delete — never update it directly
- `invoices.state` is validated against the canonical set `"draft"` | `"sent"` | `"paid"` | `"cancelled"` (`invoiceStates` in `db/invoice.go`) on create and on `PATCH /api/invoices/{id}/state`; unknown values are rejected with a 409. Unlike orders/deliveries there's no transition matrix — invoices move freely between states (a bounced payment can send `paid→sent`). State is **not** settable via `PUT` (stripped from `UpdateInvoiceRequest`); the frontend single source of truth is `src/types/invoice.ts` (`INVOICE_STATES`, `invoiceStateColor`, `invoiceStateLabel`)
- `purchase_orders.status` is `"draft"` | `"confirmed"` | `"received"` | `"cancelled"`, enforced by a `CHECK` constraint **and** by `purchaseOrderStatusTransitions` in `db/purchase_order.go` (`PATCH /api/purchase-orders/{id}/status` only — status can't be set through `PUT`). `received`/`cancelled` are terminal. Status is never auto-advanced from received quantities; per-line fulfilment is reported separately
- `purchase_orders.vendorId` deliberately has **no `ON DELETE` clause**. Vendor referential integrity is enforced app-side by `DeleteVendor`'s guard; `ON DELETE SET NULL` would silently orphan a purchase order from its vendor and make that guard's rationale false. Any new table with a `vendorId` column must be added to `vendorReferencingTables` in `db/vendor.go` — `TestVendorDocumentCountCoversEveryReference` reads the live schema and fails otherwise
- `inbound_deliveries.status` is `"draft"` | `"received"` | `"cancelled"`, enforced by a `CHECK` constraint and by `inboundDeliveryStatusTransitions` in `db/inbound_delivery.go` (`PATCH` only). Marking a receipt `"received"` inserts `"in"` `stockMovements` (**no availability check** — stock is going up) carrying the line's `unitCost`; cancelling a received receipt inserts reversing `"out"` movements, but **only after validating that every line's quantity is still in stock** — otherwise the goods have already been shipped and reversing would drive `stockQuantity` negative. That guard has no outbound equivalent. Deleting a received receipt is rejected — cancel it instead
- `inbound_delivery_line_items.unitCost` is the deliberate divergence from `outbound_delivery_line_items` (which has no price columns): it is what feeds `stockMovements.unitCost` and, through it, the product's average cost. When a line comes from a purchase order and names neither product nor cost, `db.replaceInboundDeliveryLineItemsTx` resolves **both** from the order line
- `products.unitCost` is a **weighted average derived from `stockMovements`**, never adjusted in place — the same philosophy as `stockQuantity` being `SUM(quantity)`. `db.recomputeAverageCostTx` (`db/product_cost.go`) replays the whole history with `math/big` rationals, ordered `createdAt ASC, rowid ASC` (`createdAt` is second-resolution TEXT and ties within a transaction). Costed inflows move the average; uncosted inflows and all outflows move at the running average and leave it unchanged. **Cancelling a receipt does not restore the previous average** — a reversal removes quantity at the *current* average, which is correct weighted-average behaviour. A product with no costed inflow keeps whatever cost the user typed
- `incoming_invoices.state` is `"draft"` | `"approved"` | `"paid"` | `"cancelled"`. Like sales invoices — and unlike orders/receipts — there is **no transition matrix**, only set membership: a bounced payment can send `paid→approved`. State is settable only via `PATCH /state`, which is where 3-way matching is enforced
- **3-way matching** (`db/incoming_invoice_match.go`) compares each invoice line against the purchase order (quantity and unit price) and against goods actually received, counting what *other* non-cancelled invoices already billed for the same order line — that `PreviouslyInvoiced` term is what stops the same goods being billed twice. Per-line status is `matched` | `unlinked` | `quantity_variance` | `over_received` | `price_variance`. It is **computed on read, never stored**: a stored flag would go stale as soon as a linked receipt is cancelled. Tolerances come from `organizations.match_price_tolerance_percent` / `match_quantity_tolerance_percent` (both default 0, exposed on Settings → Invoice); comparisons use `math/big` rationals, not float64
- Matching does **not** block saving — a draft can be saved with variances so they can be investigated. It blocks the move to `approved` (and to `paid` from a non-`approved` state) unless `matchOverride = 1`, and `matchOverrideReason` is **required** when it is: a blank reason would make the override a silent bypass. `unlinked` lines are informational and never block
- `incoming_invoice_line_items` reuses `db.CreateInvoiceLineItemRequest` so incoming invoices go through the same `db.validateInvoiceTotals` as sales invoices rather than duplicating its exact-rational arithmetic; the type gained nil-safe `purchaseOrderLineItemId`/`productId` fields that stay nil on the sales path
- `taxRates` is referenced by **three** line-item tables now (`invoiceLineItems`, `incoming_invoice_line_items`, `purchase_order_line_items`), listed in `taxRateReferencingTables` in `db/tax_rate.go`. The first two cascade on delete, so a table missing from that list would let an in-use rate be deleted and silently strip line items off existing invoices — `TestTaxRateUsageCountCoversEveryReference` reads the live schema and fails if one is missing
- `orders.status` is `"draft"` | `"confirmed"` | `"shipped"` | `"delivered"` | `"cancelled"`; transitions enforced both client-side via `STATUS_TRANSITIONS` in `src/routes/orders/details.tsx` and server-side via `orderStatusTransitions` in `db/order.go` (`PATCH /api/orders/{id}/status` only — status can't be set through `PUT`, which no longer accepts a `status` field)
- `orderLineItems.unitPrice` stored as integer cents; `orderLineItems.quantity` stored as REAL (supports fractional quantities)
- `outbound_deliveries.status` is `"draft"` | `"shipped"` | `"delivered"` | `"cancelled"`; transitions enforced both client-side in `src/routes/deliveries/details.tsx` and server-side via `deliveryStatusTransitions` in `db/delivery.go` (`PATCH /api/deliveries/{id}/status` only — status can't be set through `PUT`, which no longer accepts a `status` field). Line items are frozen once a delivery is `shipped`/`delivered` — `PUT` still accepts header-field-only edits (tracking number, notes, …)
- `outbound_delivery_line_items` has no price columns — delivery notes never show prices
- `outbound_delivery_line_items.productId` links a delivery line to a stock-tracked product — set directly (standalone deliveries) or auto-resolved server-side from `orderLineItemId` when omitted (`db.replaceDeliveryLineItemsTx`, run inside the same transaction as the delivery header write in `CreateDelivery`/`UpdateDelivery`); this is the only field `db.getShippableStockLines` uses to decide which lines affect inventory
- Marking a delivery `"shipped"` (`db.UpdateDeliveryStatus`) validates every stock-enabled product line against `products.stockQuantity` and rejects the transition if any line is short; on success it inserts `"out"` `stockMovements` referenced by `deliveryNumber`. Cancelling an already-`shipped` delivery inserts reversing `"in"` movements. Deleting a `shipped`/`delivered` delivery is rejected — cancel it instead
- `db.GetOrderDeliveredQuantities(orderID)` sums delivered quantity per `orderLineItemId` across non-cancelled deliveries, used to prefill a new delivery from an order with only the outstanding quantity per line (supports full or partial fulfilment)
- `invoiceLineItems.taxRate` has an `ON DELETE CASCADE` foreign key to `taxRates(id)` — deleting a tax rate still referenced by any invoice line item would silently strip those line items off existing invoices. `db.DeleteTaxRate` guards against this via `GetTaxRateUsageCount` and returns `ErrTaxRateInUse` (surfaced as 409) instead of deleting; the frontend only offers deletion for unused tax rates
- `invoices.total`/`taxTotal`/`subTotal` are recomputed and checked server-side against line items + tax rate percentages before every create/update (`db.validateInvoiceTotals` in `db/invoice_totals.go`) and rejected with a 409 on mismatch — the frontend still does the actual computation (`src/routes/invoices/details.tsx` + `src/utils/currency.ts`, decimal.js `ROUND_HALF_UP`), this is a server-side check that it agrees. The Go side uses exact rational arithmetic (`math/big`), not float64, to avoid rounding-boundary mismatches (e.g. a 3.33 unit price at 19.5% tax lands exactly on a half-cent boundary). `UpdateInvoice` validates whenever any of `lineItems`/`total`/`taxTotal`/`subTotal` is present, filling in whichever of those a partial request omits from what's already stored — a request can't bypass the check by sending only new totals (validated against stored line items) or only new line items (validated against stored totals). A pure header-only edit (neither line items nor any total) has nothing financial to recompute and is skipped

## State Management
Uses Jotai atoms pattern with:
- Storage atoms for persistence (localeAtom, siderAtom)
- Database-connected atoms for entities (clientsAtom, invoicesAtom, etc.)
- Setter atoms for database operations (setClientsAtom, etc.)
- Each domain has its own file under `src/atoms/`

**Important**: never use Jotai module-level atoms for local UI state inside Modal or Drawer forms — the mask gets orphaned and freezes the UI. Use `useState` for all local drawer/modal state.

## Sidebar Navigation
The sidebar is grouped into collapsible submenus (click the group to expand/collapse, same behavior for all groups — the active group auto-expands based on the current route via `defaultOpenKeys` in `src/layouts/base.tsx`):
- **Sales**: Invoices → Outbound Deliveries → Orders
- **Purchasing**: Purchase Orders → Goods Receipts → Incoming Invoices
- **Inventory**: Inventory
- **Master Data**: Clients → Vendors → Products → Organizations
- **Settings**: Invoice, Tax Rates, Backup, Users (admin only)

## Internationalization
- Uses LinguiJS with macro-based extraction
- Translation files in .po format under src/locales/
- Default locale configuration in src/utils/lingui.tsx
- Supports 3 locales: en, de, fr (the set lives in `lingui.config.ts` `locales`; the language switcher, antd/dayjs locale wiring in `src/app.tsx`, and `dynamicActivate` in `src/utils/lingui.tsx` all derive from or match it). de and fr are fully translated; en is the source locale

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
1. **frontend** (node:22-alpine) — runs `pnpm build`, outputs `dist/`
2. **backend** (golang:1.26-alpine) — copies `dist/` and embeds it via `//go:embed all:dist`, compiles binary
3. **runtime** (alpine:3.21) — copies only the binary, minimal footprint

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
- `TRUSTED_PROXIES` — comma/space-separated IPs or CIDRs (e.g. `172.20.0.0/16`) of reverse proxies allowed to set `X-Forwarded-For`. Unset (default): the login rate limiter always keys on the direct TCP peer, so every client behind a reverse proxy shares one bucket — set this to your proxy's address when deploying behind one. Only ever list proxies that are the sole path to the app; an untrusted peer's `X-Forwarded-For` is always ignored
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
// or for admin-only:
adminProtected("DELETE", "/api/things/{id}", h.deleteThing)
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
