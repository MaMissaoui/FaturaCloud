# api/ — Claude notes

Loaded only when working under `api/`. The authoritative route list is `api/router.go`; project-wide rules are in the root `CLAUDE.md`.

## Authorization model

**Authorization model (per-organization roles, migration `0069`).** `users.isPlatformAdmin` (orthogonal to the legacy `role` column, which stays only for OIDC group-resync/display) gates the handful of genuinely global actions that have no natural per-org owner: user management, backups, DB restore, countries. Every other admin-grade action — deleting/resetting an organization, closing a fiscal year, the GL exports — is **org-scoped**: it requires the caller to be an `admin`-role member of *that organization* via the new `organization_users` table, not a platform admin. `api/middleware.go`'s `authMiddleware` re-derives both `isActive` and `isPlatformAdmin` fresh from the DB on every request (never trusted from the JWT, which carries neither) — `platformAdmin` middleware gates the global routes, `orgAdmin(resolve orgIDResolver)` gates the org-scoped ones by resolving which organization a request targets (a path value via `pathOrgID`, or a DB lookup closure like `fiscalYearOrgID`) and checking `GetOrganizationRole` for it. **Phase C — gating the remaining ~130 `protected()` routes on membership, so a user only sees organizations they belong to — is explicitly deferred**, matching this codebase's habit of naming a scope boundary rather than half-building it (see `/Users/mam/.claude/plans/stateful-riding-eagle.md`'s own phasing): today, any authenticated user can still read/write every organization's data through those routes, `GET /api/organizations` still returns every organization rather than the caller's memberships, and `createOrganization` grants only the creator an admin membership (nobody else). A platform admin created after this shipped has **zero** organization memberships and no self-service way to grant themselves one (`/members` write routes are org-admin gated, deliberately — no platform-admin bypass, since "an org's own admins control its membership" was the explicit design decision over an all-orgs admin view); recovery today is either an existing org admin granting them by email, or a direct `INSERT INTO organization_users` against the database.

## Route notes

Routes with a behavior worth knowing before calling or changing them (every other route is plain CRUD — read `api/router.go`).

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

**Document Templates (issue #115) — per-org, per-document-type Excel export template overrides**
- `GET /api/organizations/{orgId}/document-templates/{documentType}` — the org's uploaded override, or the embedded default if none
- `POST /api/organizations/{orgId}/document-templates/{documentType}` — multipart upload, validated by actually parsing it with excelize
- `DELETE /api/organizations/{orgId}/document-templates/{documentType}` — reverts to the embedded default
- `GET /api/organizations/{orgId}/document-templates/{documentType}/orientation` — {orientation: "" | "portrait" | "landscape"}, "" meaning no override
- `PUT /api/organizations/{orgId}/document-templates/{documentType}/orientation` — sets the org's page orientation for this document type
- `DELETE /api/organizations/{orgId}/document-templates/{documentType}/orientation` — reverts to no override (the template's own authored page setup)

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

**Accounting — Fiscal Years / Periods**
- `POST /api/fiscal-years/{id}/close` — org admin only, irreversible — see Database section

**Accounting — Journal Entries**
- `DELETE /api/journal-entries/{id}` — draft only — reverse a posted entry instead

**Accounting — GL Export**
- `GET /api/organizations/{orgId}/gl-export/fec` — org admin only — France FEC
- `GET /api/organizations/{orgId}/gl-export/datev` — org admin only — Germany DATEV Buchungsstapel EXTF

## Files

- `api/router.go` — wires all routes onto `*http.ServeMux`; wraps protected routes in `authMiddleware`. `platformAdminProtected` gates the global-admin routes (users, backups, restore, countries); `orgAdminProtected(method, pattern, resolve orgIDResolver, handlerFn)` gates the org-scoped admin routes (org delete/reset, fiscal-year close, GL exports, organization members) — `resolve` is either `pathOrgID(param)` (orgId is already in the path) or a DB-lookup closure like `fiscalYearOrgID` for a route that doesn't carry it directly
- `api/helpers.go` — `writeJSON`, `writeError`, `decodeJSON`
- `api/middleware.go` — JWT `authMiddleware` re-derives `isActive` **and** `isPlatformAdmin` fresh from the DB on every request (so deactivating/deleting a user, or revoking their platform-admin flag, takes effect immediately rather than waiting for their token to expire — neither is trusted from the JWT, which carries only `UserID`/`Email`/`Provider`). `platformAdmin` middleware checks `isPlatformAdmin`; `orgAdmin(resolve orgIDResolver)` resolves the target organization and checks `GetOrganizationRole` for an `admin` membership — both take one short-lived `dbMu.RLock()` (the resolve and the role check share it, since a resolver like `fiscalYearOrgID` also hits the DB and running it unprotected would race a concurrent `/api/restore` swap), unlike `withDB`-wrapped handlers which hold theirs for the whole request. Per-IP login rate limiter also lives here
- `api/auth.go` — login, logout, me handlers
- `api/oidc.go` — OIDC SSO: login redirect (Authorization Code + PKCE), callback (ID token verification, JIT provisioning), issues the same JWT local login does. `oidcCallback` also checks the standard `email_verified` claim — rejects only when it's present and `false`; absent is treated as "nothing to check" since Authelia doesn't always emit it
- `api/users.go` — user CRUD handlers (platform admin only); also `provisionOrSyncUser`, the JIT-provision/role-resync used by OIDC login. the JIT-provisioning branch's `bcrypt.GenerateFromPassword` (~50-100ms at DefaultCost, via the new `hashRandomPassword` helper) runs with **no** `dbMu` held — a brief `RLock`'d pre-check decides whether hashing is even needed (returning user vs. first login), then the write section re-checks under the write lock regardless. Before this, the hash ran inside `dbMu`'s write lock, and since `dbMu` is the global RWMutex every `withDB`-wrapped handler RLocks for its whole request, every OIDC first-login stalled all other API traffic for that duration
- `api/{domain}.go` — HTTP handlers per domain (clients, vendors, invoices, organizations, orders, deliveries, …)
- `api/utility.go` — version, backup download, restore upload, scheduler. `runScheduler`'s "already backed up today" gate is now `lastSuccessDate`, single-goroutine local state set only after `db.Backup` actually returns success — the old file-existence check could be fooled by a partial file a failed `VACUUM INTO` left behind, silently treating a corrupt backup as done instead of retrying. `applyRetention` only deletes files sharing the `fatura-` prefix (`backupFilePrefix`) every backup this app creates — scheduled or manual — owns, rather than any file in `backupDir` past the cutoff by modtime alone. Reviewed and deliberately left unchanged: the restore-upload path (`restoreDatabase`/`swapDatabase`) still auto-migrates any uploaded file that passes `integrity_check` + has a `users` table to the running schema version with no version check — a footgun for an admin restoring an unexpectedly-old backup, not fixed here because refusing the migration would break restoring a legitimate older backup, the entire point of having one
