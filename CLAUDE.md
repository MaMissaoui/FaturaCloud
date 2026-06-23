# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview
FaturaCloud is a web-based invoicing application derived from Fatura. It runs as a single Docker image: a Go HTTP server that serves an embedded React frontend and exposes a REST API backed by SQLite.

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

The frontend calls the Go REST API via `src/api/index.ts`. Function names are intentionally identical to the old Wails bindings so the Jotai atoms only import from `src/api` instead of `wailsjs/go/main/App`.

```ts
import { GetClients, CreateClient } from "src/api"
const clients = await GetClients(organizationId)  // GET /api/organizations/{id}/clients
```

The base fetch wrapper lives in `src/api/client.ts`. All API errors throw `Error(message)` so callers catch them normally.

### API Routes
```
GET  /api/version
GET  /api/backup                          download SQLite snapshot
POST /api/restore                         multipart upload to replace DB

GET  /api/organizations
POST /api/organizations
GET  /api/organizations/{id}
PUT  /api/organizations/{id}
DELETE /api/organizations/{id}

GET  /api/organizations/{orgId}/clients
POST /api/clients
GET  /api/clients/{id}
PUT  /api/clients/{id}
DELETE /api/clients/{id}
GET  /api/clients/{id}/invoice-count

GET  /api/organizations/{orgId}/invoices
POST /api/invoices
GET  /api/invoices/{id}
GET  /api/invoices/{id}/line-items
PUT  /api/invoices/{id}
PATCH /api/invoices/{id}/state
DELETE /api/invoices/{id}

GET  /api/organizations/{orgId}/tax-rates
POST /api/tax-rates
GET  /api/tax-rates/{id}
PUT  /api/tax-rates/{id}
DELETE /api/tax-rates/{id}

GET  /api/organizations/{orgId}/products
POST /api/products
GET  /api/products/{id}
PUT  /api/products/{id}
DELETE /api/products/{id}
GET  /api/products/{id}/stock-movements

GET  /api/organizations/{orgId}/stock-movements
POST /api/stock-movements
DELETE /api/stock-movements/{id}

GET  /api/organizations/{orgId}/orders
POST /api/orders
GET  /api/orders/{id}
GET  /api/orders/{id}/line-items
PUT  /api/orders/{id}
PATCH /api/orders/{id}/status
DELETE /api/orders/{id}
```

All handlers return JSON. Errors use `{"error": "message"}`.

## File Structure
- `main.go` — entry point; opens DB, mounts API router, serves embedded `dist/`
- `api/router.go` — wires all routes onto `*http.ServeMux`
- `api/helpers.go` — `writeJSON`, `writeError`, `decodeJSON`
- `api/{domain}.go` — HTTP handlers per domain (clients, invoices, organizations, …)
- `api/utility.go` — version, backup download, restore upload
- `db/` — Go database layer (SQLite connection, migrations, CRUD per domain)
- `db/migrations/` — SQL migration files (`*.up.sql`), applied automatically on startup
- `src/api/client.ts` — base fetch wrapper
- `src/api/index.ts` — typed API functions (same names as old Wails bindings)
- `src/atoms/` — Jotai state atoms; import from `src/api`, not `wailsjs`
- `src/routes/` — main application pages
- `src/components/` — reusable React components; notable: `feedback-modal.tsx` (Sentry feedback)
- `src/layouts/base.tsx` — main application layout
- `src/types/` — shared TypeScript type definitions
- `src/utils/` — lingui.tsx (i18n setup), sentry.ts (error tracking init), currency.ts / currencies.tsx / countries.tsx, date.ts, invoice.ts
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
- `stockMovements.quantity` is a **signed delta**: positive = stock in, negative = stock out/adjustment; `products.stockQuantity` is always `SUM(quantity)` over all movements and is recomputed inside a transaction on every insert/delete — never update it directly
- `orders.status` is `"draft"` | `"confirmed"` | `"shipped"` | `"delivered"` | `"cancelled"`; transitions are enforced client-side via STATUS_TRANSITIONS in `src/routes/orders/details.tsx`
- `orderLineItems.unitPrice` stored as integer cents; `orderLineItems.quantity` stored as REAL (supports fractional quantities)

## State Management
Uses Jotai atoms pattern with:
- Storage atoms for persistence (localeAtom, siderAtom)
- Database-connected atoms for entities (clientsAtom, invoicesAtom, etc.)
- Setter atoms for database operations (setClientsAtom, etc.)
- Each domain has its own file under `src/atoms/`

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
- `PORT` — HTTP port for the Go server (default `8080`); set in Docker via `ENV PORT=8080`
- `VITE_SENTRY_ENABLED=true` — force-enables Sentry error tracking in dev (defaults off outside production)
- `VITE_JOTAI_DEVTOOLS_ENABLED=true` — enables Jotai DevTools in dev mode
- `GITHUB_SHA` — injected by CI for Sentry release tracking; resolves to `"development"` locally

## Adding a New API Endpoint

**Go side** — add a handler method in the relevant `api/{domain}.go` file, then register the route in `api/router.go`:
```go
mux.HandleFunc("GET /api/things/{id}", h.getThing)
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
