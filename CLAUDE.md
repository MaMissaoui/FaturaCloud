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

The base fetch wrapper lives in `src/api/client.ts`. It attaches the JWT Bearer token from `localStorage` to every request. All API errors throw `Error(message)` so callers catch them normally.

### API Routes

```
# Public
GET  /api/version
POST /api/auth/login
POST /api/auth/logout

# Auth (JWT required)
GET  /api/auth/me

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
DELETE /api/organizations/{id}

# Clients
GET    /api/organizations/{orgId}/clients
POST   /api/clients
GET    /api/clients/{id}
PUT    /api/clients/{id}
DELETE /api/clients/{id}
GET    /api/clients/{id}/invoice-count

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
- `api/middleware.go` — JWT `authMiddleware`, `adminOnly`, per-IP login rate limiter
- `api/auth.go` — login, logout, me handlers
- `api/users.go` — user CRUD handlers (admin only)
- `api/{domain}.go` — HTTP handlers per domain (clients, invoices, organizations, orders, deliveries, …)
- `api/utility.go` — version, backup download, restore upload, scheduler
- `db/` — Go database layer (SQLite connection, migrations, CRUD per domain)
- `db/migrations/` — SQL migration files (`*.up.sql`), applied automatically on startup
- `src/api/client.ts` — base fetch wrapper; attaches JWT Bearer token from `localStorage`
- `src/api/index.ts` — typed API functions, one per REST endpoint
- `src/atoms/` — Jotai state atoms; import from `src/api`
- `src/atoms/auth.ts` — `currentUserAtom`, `isAuthenticatedAtom`, `isAdminAtom`
- `src/atoms/delivery.ts` — delivery list, detail, status, and delete atoms
- `src/routes/` — main application pages
- `src/routes/login.tsx` — login page (public, redirects to `/` on success)
- `src/routes/deliveries.tsx` — outbound deliveries list
- `src/routes/deliveries/details.tsx` — delivery detail/edit page
- `src/routes/orders/details.tsx` — order detail/edit page
- `src/routes/organizations/index.tsx` — organizations list page (standalone, not under Settings)
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

## Database
SQLite is accessed from Go via `jmoiron/sqlx`. All schema migrations live in `db/migrations/` as `*.up.sql` files and run automatically on every startup. The database file is located at:
- **Docker**: `/data/sqlite.db` (mount a volume at `/data`)
- **Local dev (macOS)**: `~/Library/Application Support/FaturaCloud/sqlite.db`
- **Local dev (Linux)**: `~/.config/FaturaCloud/sqlite.db`

Schema conventions:
- Primary keys are 21-character nanoid strings
- Monetary values stored as integer cents — the form layer converts (user input × 100 → store; stored ÷ 100 → display); atoms and API pass cents through unchanged
- Dates stored as Unix timestamps in milliseconds
- Organization logo stored as BLOB (raw bytes) — Go's `encoding/json` marshals `[]byte` as base64; the frontend calls `atob`/`btoa` accordingly
- `products.type` is `"product"` | `"service"` (default `"service"`)
- `products.sku` (labeled "Product code" in the UI) must be unique per organization — enforced by a `UNIQUE(organizationId, sku)` index, not a DB-level `NOT NULL` (SQLite can't add that retroactively without a table rebuild); required-ness is enforced in `api/products.go` and the frontend form instead. The New Product form proposes a code derived from the name, deduplicated against other products in the org
- `stockMovements.quantity` is a **signed delta**: positive = stock in, negative = stock out/adjustment; `products.stockQuantity` is always `SUM(quantity)` over all movements and is recomputed inside a transaction on every insert/delete — never update it directly
- `invoices.state` is unconstrained text; common values: `"draft"` | `"sent"` | `"paid"` | `"cancelled"`
- `orders.status` is `"draft"` | `"confirmed"` | `"shipped"` | `"delivered"` | `"cancelled"`; transitions enforced client-side via `STATUS_TRANSITIONS` in `src/routes/orders/details.tsx`
- `orderLineItems.unitPrice` stored as integer cents; `orderLineItems.quantity` stored as REAL (supports fractional quantities)
- `outbound_deliveries.status` is `"draft"` | `"shipped"` | `"delivered"` | `"cancelled"`; transitions enforced client-side in `src/routes/deliveries/details.tsx`
- `outbound_delivery_line_items` has no price columns — delivery notes never show prices
- `outbound_delivery_line_items.productId` links a delivery line to a stock-tracked product — set directly (standalone deliveries) or auto-resolved server-side from `orderLineItemId` when omitted (`db.replaceDeliveryLineItems`); this is the only field `db.getShippableStockLines` uses to decide which lines affect inventory
- Marking a delivery `"shipped"` (`db.UpdateDeliveryStatus`) validates every stock-enabled product line against `products.stockQuantity` and rejects the transition if any line is short; on success it inserts `"out"` `stockMovements` referenced by `deliveryNumber`. Cancelling an already-`shipped` delivery inserts reversing `"in"` movements. Deleting a `shipped`/`delivered` delivery is rejected — cancel it instead
- `db.GetOrderDeliveredQuantities(orderID)` sums delivered quantity per `orderLineItemId` across non-cancelled deliveries, used to prefill a new delivery from an order with only the outstanding quantity per line (supports full or partial fulfilment)
- `invoiceLineItems.taxRate` has an `ON DELETE CASCADE` foreign key to `taxRates(id)` — deleting a tax rate still referenced by any invoice line item would silently strip those line items off existing invoices. `db.DeleteTaxRate` guards against this via `GetTaxRateUsageCount` and returns `ErrTaxRateInUse` (surfaced as 409) instead of deleting; the frontend only offers deletion for unused tax rates

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
- **Inventory**: Inventory
- **Master Data**: Clients → Products → Organizations
- **Settings**: Invoice, Tax Rates, Backup, Users (admin only)

## Internationalization
- Uses LinguiJS with macro-based extraction
- Translation files in .po format under src/locales/
- Default locale configuration in src/utils/lingui.tsx
- Currently supports 11 locales: en, en-GB, de, et, fi, fr, el, nl, pt, sv, uk

## Docker
```bash
# Build and run
docker compose up --build

# Build image only
docker build -t fatura-cloud .

# Run with explicit volume
docker run -p 8080:8080 -v fatura_data:/data fatura-cloud
```

The `Dockerfile` is a three-stage build:
1. **frontend** (node:22-alpine) — runs `pnpm build`, outputs `dist/`
2. **backend** (golang:1.26-alpine) — copies `dist/` and embeds it via `//go:embed all:dist`, compiles binary
3. **runtime** (alpine:3.21) — copies only the binary, minimal footprint

Pass `--build-arg VERSION=<tag>` to inject a version string (accessible via `GET /api/version`).

## Environment Variables
- `PORT` — HTTP port for the Go server (default `8080`)
- `JWT_SECRET` — secret key for signing JWT tokens; defaults to `"dev-secret-change-me-in-production"` — **must be set in production**
- `ADMIN_EMAIL` — email for the initial admin user created on first startup (default: `admin@fatura.cloud`)
- `ADMIN_PASSWORD` — password for the initial admin user (default: `admin`) — **change in production**
- `VITE_SENTRY_ENABLED=true` — force-enables Sentry error tracking in dev (defaults off outside production)
- `VITE_JOTAI_DEVTOOLS_ENABLED=true` — enables Jotai DevTools in dev mode
- `GITHUB_SHA` — injected by CI for Sentry release tracking; resolves to `"development"` locally

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
