# api/ — Claude notes

Loaded only when working under `api/`. The authoritative route list is `api/router.go`; project-wide rules are in the root `CLAUDE.md`.

## Authorization model

**Authorization model (per-organization roles, migrations `0069`/`0081`).** `users.isPlatformAdmin` (orthogonal to the legacy `role` column, which stays only for OIDC group-resync/display) gates the handful of genuinely global actions that have no natural per-org owner: user management, backups, DB restore, countries. Every other admin-grade action — deleting/resetting an organization, closing a fiscal year, the GL exports — is **org-scoped**: it requires an appropriate role in *that organization* via `organization_users`, not a platform admin. Since migrations `0081`/`0086` those roles are seven: `admin`, `power_user` (added by `0086` as a copy of `general`'s original full access) and `general` (the renamed old `user`) — all three full read/write everywhere non-admin-gated, except that the **`general` role's UI** drops Accounting, Imports and Bill of Materials (`src/CLAUDE.md`, frontend-only) — plus the narrow writers `sales`/`purchasing`/`accounting`/`cashbook`; **reads stay membership-level for every role**. `api/middleware.go`'s `authMiddleware` re-derives both `isActive` and `isPlatformAdmin` fresh from the DB on every request (never trusted from the JWT, which carries neither), and four org-scoped wrappers sit on top of it, each resolving which organization a request targets (a path value via `pathOrgID`, or a DB lookup closure like `fiscalYearOrgID`) and checking `GetOrganizationRole` for it: `orgMemberProtected` (any membership — the bulk of the table), `orgRoleProtected` (admin/power_user/general or the listed domain role, for mutations), `orgAdminProtected` (admin only), and `orgRoleAdminProtected` (admin plus the listed role — fiscal-year close and both GL exports use it with `accounting`). **Issue #141 Phase C has shipped**: `GET /api/organizations` now returns only the caller's own memberships (`GetUserOrganizations`) — including for a platform admin, who has no all-orgs view — and the ordinary data routes are all membership/role gated. What a `protected()` wrapper does **not** cover is the body-organization `POST` create routes (organizationId lives in the JSON, gated inline by `requireOrgMember`/`requireOrgRole` after `decodeJSON`), the self-limiting/global routes (`GET /api/auth/me`, `GET`/`POST /api/organizations`, the `my-role`/`my-roles` routes, `GET /api/countries/active`), and the export/restore routes registered directly on `mux`. A platform admin is still not automatically a member of any organization: `createOrganization` grants only the creator an admin membership, and the `/members` write routes are org-admin gated with no platform-admin bypass ("an org's own admins control its membership" was the explicit design decision over an all-orgs admin view); recovery is an existing org admin granting them by email, or a direct `INSERT INTO organization_users` against the database.

## Route notes

Routes with a behavior worth knowing before calling or changing them (every other route is plain CRUD — read `api/router.go`).

**Auth**
- `POST /api/auth/logout` — clears the cookie AND revokes every session for that user by bumping `users.tokenVersion`, so all previously-issued JWTs (including one held elsewhere) fail `authMiddleware`'s `claims.TokenVersion != stored` check on their next use instead of staying valid until expiry. Password change (`PUT /api/users/{id}` with `password`) bumps it too.

**Users (platform admin only)**
- `DELETE /api/users/{id}` — refused with 409 if the target is the sole admin member of any organization

**Backup**
- `POST /api/restore` — multipart upload to replace DB

**Organizations**
- `POST /api/organizations` — creator is auto-granted an 'admin' organization_users row
- `DELETE /api/organizations/{id}` — org admin only — cascade-deletes clients/invoices/orders/deliveries
- `GET /api/organizations/{id}/logo` — raw image bytes, sniffed Content-Type — logo isn't in the org JSON
- `POST /api/organizations/{id}/logo` — multipart upload, 2 MB cap

**Organization Members (per-org roles, migration 0069)**
- `GET /api/organizations/{orgId}/my-role` — NOT org-admin gated — any authenticated user may ask their own role
- `GET /api/organizations/{orgId}/members` — org admin only
- `POST /api/organizations/{orgId}/members` — org admin only, {email, role} — email must belong to an existing user account
- `PUT /api/organizations/{orgId}/members/{userId}` — org admin only, {role} — 409 if it would demote the sole admin
- `DELETE /api/organizations/{orgId}/members/{userId}` — org admin only — 409 if it would remove the sole admin

**Vendors**
- `DELETE /api/vendors/{id}` — refused with 409 while purchasing documents reference it

**Imports (F114 — consolidated China shipments purchase orders link to)**
- `GET /api/imports/{id}/summary` — computed on read — committed PO value, landed cost rate
- `DELETE /api/imports/{id}` — refused with 409 while a purchase order still links to it

**Purchase Orders**
- `GET /api/purchase-orders/{id}/export` — fills the org's Excel template (upload or embedded default) and returns .xlsx; ?format=pdf converts it with headless LibreOffice — same mechanism and same "not wrapped in withDB" reasoning as the invoice export below

**Inbound Deliveries (goods receipts)**
- `GET /api/inbound-deliveries/{id}/export` — fills the org's Excel template (upload or embedded default) and returns .xlsx; ?format=pdf converts it with headless LibreOffice — same mechanism and same "not wrapped in withDB" reasoning as the invoice export above. No tax rate and no aggregate totals block — unitCost/lineTotal are shown per line only

**Incoming Invoices (vendor bills)**
- `GET /api/incoming-invoices/{id}/match` — 3-way match, computed on read
- `PATCH /api/incoming-invoices/{id}/state` — blocked by an unresolved variance
- `GET /api/incoming-invoices/{id}/export` — fills the org's Excel template (upload or embedded default) and returns .xlsx; ?format=pdf converts it with headless LibreOffice — same mechanism and same "not wrapped in withDB" reasoning as the invoice export above. Unlike purchase orders/orders, reads the invoice's own server-validated stored totals rather than recomputing them

**Invoices**
- `GET /api/invoices/{id}/e-invoice` — EN 16931 UBL XML export (country profile resolved from the buyer)
- `GET /api/invoices/{id}/export` — issue #115 — fills the org's Excel template (upload or embedded default) and returns .xlsx; ?format=pdf converts it with headless LibreOffice (see db/pdf_convert.go and the Dockerfile's runtime-stage comment) and returns .pdf — 503s only in an environment without soffice on PATH (e.g. local `go run` dev), not in the shipped image. Not wrapped in withDB

**Cash Book — counter-side point-of-sale (db/cash_sale.go, db/cash_movement.go, db/cash_movement_details.go, src/routes/cash-book.tsx)**
- `POST /api/cash-sales` — plain `protected()` with the `cashbook` role checked inline (`requireOrgRole`) — one atomic write that resolves-or-creates the client, creates the invoice, posts its GL entry, and records any upfront payment, because chaining the separate endpoints could strand an unbalanced AR entry. Counter `unitPrice`s arrive gross/tax-inclusive from the screen and are converted to net before the call. The sale ends `paid` iff `amountReceived == total`, otherwise `sent` (a loan settled later through the ordinary payment flow). Rejects a foreign currency (organization currency only) and a sale with no line items
- `POST /api/cash-movements` — same inline `cashbook` gate — records money *leaving* the register (a bank deposit or a document-less petty-cash expense) as a `cash_movements` row plus its posted two-line entry, atomically. `counterAccountType` (`"bank"`|`"expense"`) is cross-checked against the counter account's own `type`, `accountId` defaults to `organizations.defaultCashRegisterAccountId` (409 if neither is set), and the response carries the register balance after the movement
- `GET /api/organizations/{orgId}/reports/daily-cash-movements` — member-level; `accountId`/`startDate`/`endDate` query params; day-bucketed Opening/In/Out/Closing (UTC-day caveats in db/gl_reports.go)
- `GET /api/organizations/{orgId}/reports/cash-movement-details` — member-level; the per-transaction drill-down behind the daily aggregate (`accountId`/`startDate`/`endDate`), merging inbound invoice payments with `cash_movements` withdrawals
- `GET /api/organizations/{orgId}/reports/loan-status` — member-level; optional `clientId`; the standing "who owes what" report over loan-sale invoices
- `GET /api/organizations/{orgId}/reports/daily-cash-movements/export` (takes `accountId` + `date`) and `GET /api/organizations/{orgId}/reports/loan-status/export` (takes `clientId` + `openOnly=true`) — member-level `?format=xlsx|pdf`, registered directly on `mux` (not `withDB`) like every other LibreOffice export

**Document Templates (issue #115) — per-org, per-document-type Excel export template overrides**
- `GET /api/organizations/{orgId}/document-templates/{documentType}` — the org's uploaded override, or the embedded default if none
- `POST /api/organizations/{orgId}/document-templates/{documentType}` — multipart upload, validated by actually parsing it with excelize
- `DELETE /api/organizations/{orgId}/document-templates/{documentType}` — reverts to the embedded default
- `GET /api/organizations/{orgId}/document-templates/{documentType}/orientation` — {orientation: "" | "portrait" | "landscape"}, "" meaning no override
- `PUT /api/organizations/{orgId}/document-templates/{documentType}/orientation` — sets the org's page orientation for this document type
- `DELETE /api/organizations/{orgId}/document-templates/{documentType}/orientation` — reverts to no override (the template's own authored page setup)

**Document Numbering — per-org, per-document-type number format + persisted counter (db/document_number.go, migration 0082, src/routes/settings/document-numbering.tsx)**
- `GET /api/organizations/{orgId}/document-number-settings/{documentType}` — member-level; returns `{documentType, format, counter, hasOverride}`. An unknown `documentType` (not one of the five below) is a plain 400; a type the org has never saved returns its default format with counter 0 and `hasOverride: false` ("absence means default," the same convention as the document-template endpoints)
- `PUT /api/organizations/{orgId}/document-number-settings/{documentType}` — member-level; body `{format, counter?}` — `counter` is a pointer so a format-only save doesn't have to resend it. A blank format or an unrecognized `{token}` 409s; `counter` may not be negative. Tokens are `{number}`, zero-padded `{number:N}`, the date parts `{year}`/`{y}`/`{month}`/`{m}`/`{day}`, and `{clientCode}`. Covers the five types `order`/`purchase_order`/`delivery`/`inbound_delivery`/`production_order`; invoices deliberately stay on the `organizations.invoice_number_format`/`_counter` columns (Settings → Invoice), and each counter advances once per created document inside that document's own transaction (so a deleted document's number isn't reissued), replacing the old `MAX(...)+1` scan

**Mass Data (Excel download/upload for bulk maintenance) — Clients, Vendors, Products, Tax Rates, Payment Terms, Units of Measure, Chart of Accounts**
- `GET /api/organizations/{orgId}/{clients|vendors|products|tax-rates|payment-terms|units-of-measure|accounts}/export` — org-member read, same level as that table's own list route; returns a real `.xlsx`
- `POST /api/organizations/{orgId}/{...}/import` — multipart upload (`file` field), returns a JSON report (`{created, updated, failed, rows: [...]}`, never a 4xx for a bad row — see `db/mass_data.go`); role-gated identically to that table's own `PUT`/`DELETE` (`sales` for clients, `purchasing` for vendors, `accounting` for accounts; the rest are any org member) — the 3 role-gated import routes are in `domainRouteRoles`/`api/cross_org_test.go`'s tripwire alongside every other domain-role route

**Products**
- `GET /api/organizations/{orgId}/products/bom-summaries` — batch componentCount per finished product — see db/product_bom.go
- `PUT /api/products/{id}` — body also takes unitOfMeasureId — when set, overwrites the legacy free-text unit field server-side with that unit of measure's name (db/product.go's resolveProductUnit), so every existing reader of unit keeps working unchanged
- `PUT /api/products/{id}/bom` — body also takes batchSize (int) — omitted/0 inherits the latest version's, not a reset to 1, so a caller with no batch UI (the product form's card) can't stomp it — see db/product_bom.go
- `GET /api/products/{id}/bom/versions` — header list (componentCount, batchSize), newest first
- `GET /api/products/{id}/bom/versions/{versionId}` — one version's full lines, denormalized
- `POST /api/products/{id}/bom/versions/{versionId}/restore` — non-destructive — itself creates a new version

**Units of Measure — maintained per-org list for products.unitOfMeasureId (db/unit_of_measure.go, migration 0076), same shape as Payment Terms below**
- `DELETE /api/units-of-measure/{id}` — unconditional — products.unitOfMeasureId is ON DELETE SET NULL, not CASCADE, so this can never orphan or destroy a product

**Production Orders — consume a finished product's BOM, produce finished units (db/production_order.go, src/routes/production-orders.tsx)**
- `PATCH /api/production-orders/{id}/status` — body also takes serialNumbers ([]string), required when the finished product is serialized
- `DELETE /api/production-orders/{id}` — draft only — cancel a completed order instead; DELETE carries AND status='draft' (a status-guarded DELETE), as does DeleteInboundDelivery's

**Orders**
- `GET /api/orders/{id}/export` — fills the org's Excel template (upload or embedded default) and returns .xlsx; ?format=pdf converts it with headless LibreOffice — same mechanism and same "not wrapped in withDB" reasoning as the invoice export below

**Outbound Deliveries**
- `GET /api/deliveries/{id}/export` — fills the org's Excel template (upload or embedded default) and returns .xlsx; ?format=pdf converts it with headless LibreOffice — same mechanism and same "not wrapped in withDB" reasoning as the invoice export above. No price columns at all — a delivery note never shows prices, so there is no totals block

**Payments (F104)**
- `POST /api/payments` and `POST /api/payments/{id}/void` — require the `accounting` organization role (admin/power_user/general/accounting pass, via the standard role helpers). Deliberate boundary decision: a payment settles AR/AP and a void reverses a posted GL entry, so both are accounting actions, the same tier as journal-entry post/reverse.

**Accounting — Fiscal Years / Periods**
- `POST /api/fiscal-years/{id}/close` — org admin **or accounting** (`orgRoleAdminProtected`), irreversible — see Database section

**Accounting — Journal Entries**
- `DELETE /api/journal-entries/{id}` — draft only — reverse a posted entry instead

**Accounting — GL Export**
- `GET /api/organizations/{orgId}/gl-export/fec` — org admin **or accounting** (`orgRoleAdminProtected`) — France FEC
- `GET /api/organizations/{orgId}/gl-export/datev` — org admin **or accounting** (`orgRoleAdminProtected`) — Germany DATEV Buchungsstapel EXTF

**Cash Book**
- `POST /api/cash-sales` — role-gated to `admin` | `general` | `cashbook` (`requireOrgRole`, `api/cash_sale.go`). Deliberately NOT a pure invoice write: a cash-sale request may carry a `newClient` (the walk-in customer, who doesn't exist as a client yet) and the handler creates that client as part of issuing the invoice, in the same transaction. That is an accepted cross-domain write for the `cashbook` role — a cashier taking a counter sale otherwise couldn't record it at all, since the `sales`-gated `POST /api/clients` would 403 them. Do not "tighten" this to `sales` without deciding where the walk-in client gets created instead.

## Files

- `api/router.go` — wires all routes onto `*http.ServeMux`; wraps protected routes in `authMiddleware`. `platformAdminProtected` gates the global-admin routes (users, backups, restore, countries). The org-scoped routes use four wrappers, all taking a `resolve orgIDResolver` (either `pathOrgID(param)` when orgId is already in the path or a DB-lookup closure like `fiscalYearOrgID` for a route that doesn't carry it directly): `orgMemberProtected` (any membership — the bulk of the table), `orgRoleProtected` (admin/power_user/general or the listed domain role, for mutations), `orgAdminProtected` (org delete/reset, organization members), and `orgRoleAdminProtected` (fiscal-year close and both GL exports, with `accounting`)
- `api/helpers.go` — `writeJSON`, `writeError`, `decodeJSON`
- `api/middleware.go` — JWT `authMiddleware` re-derives `isActive` **and** `isPlatformAdmin` fresh from the DB on every request (so deactivating/deleting a user, or revoking their platform-admin flag, takes effect immediately rather than waiting for their token to expire — neither is trusted from the JWT, which carries only `UserID`/`Email`/`Provider`). It also re-reads `users.tokenVersion` and rejects a token whose embedded `claims.TokenVersion` no longer matches (F109) — that value *is* carried in the token (unlike `isPlatformAdmin`), since revocation is only meaningful if the minted version can be compared against the stored one. `platformAdmin` middleware checks `isPlatformAdmin`; `orgAdmin(resolve orgIDResolver)` resolves the target organization and checks `GetOrganizationRole` for an `admin` membership. `orgMember`/`orgRole`/`orgRoleAdmin` share the same `orgAuthorized` core, differing only in the allowed-role predicate (any member / admin-or-general-or-listed domain role / admin-or-listed role) and the 404-collapse-vs-403 failure shape; `requireOrgMember`/`requireOrgRole` are the from-inside-the-handler equivalents for the body-organization create routes. All of these take one short-lived `dbMu.RLock()` (the resolve and the role check share it, since a resolver like `fiscalYearOrgID` also hits the DB and running it unprotected would race a concurrent `/api/restore` swap), unlike `withDB`-wrapped handlers which hold theirs for the whole request. Per-IP login rate limiter also lives here
- `api/auth.go` — login, logout, me handlers. `clientIP` (rate-limit key) walks `X-Forwarded-For` from the RIGHT, skipping configured trusted-proxy hops, and returns the first non-trusted address (F110) — reading the leftmost entry was spoofable because proxies append rather than replace. `logout` parses the session cookie best-effort and bumps `users.tokenVersion`, revoking every session for the user; `issueTokenWithProvider` embeds the current `user.TokenVersion` (F109)
- `api/oidc.go` — OIDC SSO: login redirect (Authorization Code + PKCE), callback (ID token verification, JIT provisioning), issues the same JWT local login does. `oidcCallback` also checks the standard `email_verified` claim — rejects only when it's present and `false`; absent is treated as "nothing to check" since Authelia doesn't always emit it
- `api/users.go` — user CRUD handlers (platform admin only); also `provisionOrSyncUser`, the JIT-provision/role-resync used by OIDC login. the JIT-provisioning branch's `bcrypt.GenerateFromPassword` (~50-100ms at DefaultCost, via the new `hashRandomPassword` helper) runs with **no** `dbMu` held — a brief `RLock`'d pre-check decides whether hashing is even needed (returning user vs. first login), then the write section re-checks under the write lock regardless. Before this, the hash ran inside `dbMu`'s write lock, and since `dbMu` is the global RWMutex every `withDB`-wrapped handler RLocks for its whole request, every OIDC first-login stalled all other API traffic for that duration
- `api/{domain}.go` — HTTP handlers per domain (clients, vendors, invoices, organizations, orders, deliveries, …)
- `api/utility.go` — version, backup download, restore upload, scheduler. `runScheduler`'s "already backed up today" gate is now `lastSuccessDate`, single-goroutine local state set only after `db.Backup` actually returns success — the old file-existence check could be fooled by a partial file a failed `VACUUM INTO` left behind, silently treating a corrupt backup as done instead of retrying. `applyRetention` only deletes files sharing the `fatura-` prefix (`backupFilePrefix`) every backup this app creates — scheduled or manual — owns, rather than any file in `backupDir` past the cutoff by modtime alone. Reviewed and deliberately left unchanged: the restore-upload path (`restoreDatabase`/`swapDatabase`) still auto-migrates any uploaded file that passes `integrity_check` + has a `users` table to the running schema version with no version check — a footgun for an admin restoring an unexpectedly-old backup, not fixed here because refusing the migration would break restoring a legitimate older backup, the entire point of having one
