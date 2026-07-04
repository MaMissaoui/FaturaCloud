# Changelog

All notable changes to FaturaCloud will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
