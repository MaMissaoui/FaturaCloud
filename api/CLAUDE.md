# api/ — Claude notes

Loaded when working under `api/`. `api/router.go` is the authoritative route list; project-wide rules (including the authorization summary) are in the root `CLAUDE.md`.

## Authorization
- **Platform admin vs. organization roles.** `users.isPlatformAdmin` gates only global actions: users, backups, restore, countries (`platformAdminProtected`). It grants no organization access and has no all-organizations view. `GET /api/organizations` returns the caller's memberships (`GetUserOrganizations`), and `createOrganization` makes only the creator an admin. Recovery for an organization with no admin: an existing org admin grants by email, or a direct `INSERT INTO organization_users`. The legacy `users.role` column is used only for OIDC group sync and display.
- **Wrappers.** Each resolves the target organization (`pathOrgID(param)` or a lookup closure such as `fiscalYearOrgID`) and checks `GetOrganizationRole`:
  - `orgMemberProtected`: any member.
  - `orgRoleProtected`: admin/power_user/general or the listed domain roles. An empty list means the org-wide roles only.
  - `orgAdminProtected`: admin only (org delete/reset, members, loan import, the org activity history).
  - `orgRoleAdminProtected`: admin plus the listed role (fiscal-year close and both GL exports, with `accounting`).

  They share `orgAuthorized` (`orgMember`/`orgRole`/`orgRoleAdmin`/`orgAdmin`). `requireOrgMember`/`requireOrgRole` are the in-handler equivalents for body-organization `POST` creates, called after `decodeJSON`.
- **Section guard** (`api/sections.go`, mirroring `src/layouts/role-menu.ts`'s `ROLE_MENU`): `routeAllowedForRole` runs after every membership/role check and 403s a restricted role on routes its UI hides. These stay membership-level because the screens restricted roles keep cross-reference them:
  - master data and accounts reads (writes use the `accounting-write` section);
  - every payments route;
  - the dashboard, exchange rates, templates and numbering;
  - organizations and members, auth, users, backups and restore.

  Exceptions inside those:
  - `payment-products` (general/sales/cashbook; F159): `GET /api/payments/{id}/invoice-lines` and the payment-history export. `listPayments` empties `products` for other roles.
  - `client-balances` (general/sales/accounting/cashbook) and `vendor-balances` (general/purchasing/accounting): the client and vendor summaries. `getProductSummary` strips movement counterparties by the same rule (`TestProductSummaryCounterpartiesByRole`).
- **Configuration writes** (F170): products by sales/purchasing, tax rates and payment terms by accounting, and units of measure, product families, document templates and orientation, numbering by the org-wide roles only. Mass-data imports are gated like the table's own `PUT`. `CONFIG_WRITE_ROLES` (`src/atoms/organization.ts`) is the frontend twin; keep the two in step (`TestConfigurationWritesAreRoleGated`).
- **Tripwires.** Every new route must be classified, or the build fails:
  - `api/cross_org_test.go`: `TestPhaseCRouteCoverage`, `TestDomainRoleRouteCoverage` (`domainRouteRoles`), `TestCreateRouteOrgChecksArePresent` (`createRouteDomainRoles`), `exemptRoutes`;
  - `api/sections_test.go`: `TestSectionRouteCoverage` (`wantSectionRoutes` or `TestSharedRoutesAreUnrestricted`) and `TestSectionRoleAccess`, which pins the role matrix.
- **Read pool** (F177): the heavy list and report GET handlers (invoices, incoming invoices, payments, journal entries, the dashboard, the summaries, aging, cash reports and loan status, the GL and sales reports, the BOM overview) call `h.db.ReadOnly().GetX(...)`, so a counter sale never queues behind them. A new heavy read-only GET may do the same. Never route a handler that writes or opens a transaction: the pool is `query_only`.
- **Locking.** The wrappers take one short `dbMu.RLock()` covering both the resolve and the role check (a resolver hits the DB and must not race a restore swap). `withDB` handlers hold the read lock for the whole request. **LibreOffice exports are registered directly on `mux`, not `withDB`, and call `h.orgMember(...)` inline**, so a conversion never runs under the request-long lock. The restore routes call `platformAdmin` inline.

## Auth and sessions
- `authMiddleware` re-reads `isActive`, `isPlatformAdmin` and `tokenVersion` from the DB on every request. The JWT carries only `UserID`/`Email`/`Provider`/`TokenVersion`/`SessionStart`. A `tokenVersion` mismatch is a 401. Logout and a password change bump `users.tokenVersion`, which revokes every session.
- **Renewal** (F175): a token older than `tokenRenewAfter` (12 h) gets a fresh cookie from `renewSessionIfDue`, with the same `tokenVersion` and set through `setAuthCookie`, until `maxSessionAge` (7 days) after `sessionStart`. The frontend's resume flow is `src/utils/session-resume.ts`.
- **Emails** (F174): every email write and lookup goes through `db.NormalizeEmail` (trim, ASCII lower case, matching migration `0107`). A new lookup must too.
- `clientIP` walks `X-Forwarded-For` from the right, skipping trusted proxies (`IsTrustedProxyPeer`). `IsHTTPS` decides `Secure` and HSTS. There is a per-IP login rate limiter.
- `api/oidc.go`: Authorization Code + PKCE, JIT provisioning (`provisionOrSyncUser`). `email_verified` rejects only when present and false. The bcrypt hash for a first login runs with **no** `dbMu` held (`hashRandomPassword`); the write re-checks under the lock.

## Route behavior
Plain CRUD isn't listed; read `api/router.go`.
- `GET /api/healthz`: public. Runs `SELECT 1` with a 2 s timeout and answers 200 or 503. The Docker `HEALTHCHECK` runs `fatura-cloud healthcheck`.
- `POST /api/client-errors`: any signed-in user (`exemptRoutes`). Logged as `event=client_error`, never stored, limited to 20 per user per 10 minutes, always 204.
- Users (platform admin): delete or deactivation is refused (409) for the sole admin of any organization (`GetOrganizationsWhereSoleAdmin`).
- Members (org admin): the email must be an existing user, and the last-admin guard applies. `GET …/my-role` and `…/my-roles` are open to any signed-in user.
- Organizations: the logo is served raw (sniffed type) and uploaded as multipart with a 2 MB cap. `DELETE` cascades the organization's data.
- Invoices: a blank `number` is assigned on the server; a taken number is a 409 (F169). `GET …/e-invoice` returns UBL. `GET …/{id}/export?format=xlsx|pdf` exists for invoices, incoming invoices, orders, POs, outbound and inbound deliveries, plus the Cash Book reports; it is a 503 only where `soffice` is missing.
- Incoming invoices: `GET …/match` is computed on read. `PATCH …/state` is blocked by an unresolved variance.
- Products: `DELETE` is a 409 with history (F167). `GET …/usage-count` returns `{count, usage}`, and the drawer offers Delete or Archive from it. `PUT` takes `isArchived` (omitted keeps). Listing takes `archived=exclude|only` and `stock=out|low`. `PUT …/bom` takes `batchSize` (omitted or 0 inherits the latest). BOM version restore creates a new version.
- Tax rates: `PUT` is a 409 for a percentage change on a rate used by documents (F168). `GET …/usage-count` returns `{count, documentLines}`: `count` blocks Delete, `documentLines` locks the percentage.
- Units of measure: `DELETE` is unconditional (products `SET NULL`).
- Payments: `GET …/payments` takes `direction` and `clientId` (F172). `POST /api/payments` and `…/void` are accounting-tier (F104), deliberately, like journal post/reverse.
- **Cash Book:**
  - `POST /api/cash-sales` and `POST /api/cash-movements` are plain `protected()` with `requireOrgRole(…, "cashbook")`. A cash sale may create its walk-in client (`newClient`) in the same transaction. That cross-domain write is accepted on purpose: don't tighten it to `sales` without deciding where that client gets created. Prices arrive net (the screen converts from gross).
  - `POST /api/cash-sales/{id}/payments` is `orgRoleProtected` with `cashbook` and settles one invoice line. It is deliberately separate from `POST /api/payments`: it can't pay vendors, void, or take FX. Any sent or paid invoice qualifies (F150). It follows the same `cashBookPaymentMethods` rule as a sale: cash to the register, card/transfer in `cash_and_bank` to the default bank account.
  - The report routes (`daily-cash-movements`, `cash-movement-details`, `loan-status`) are membership-level. Their day ranges arrive as `calendarDayMs`. Exports: daily movements take `accountId` + `date`, loan status `clientId` + `openOnly=true` (or `register=open|stale|settled`), payment history `clientId`.
- Loan register import (org admin): `POST …/loan-imports` takes `file`, `cutoverDate` and `dryRun` (only `"false"` imports) and always answers 200 with the report. `DELETE /api/loan-imports/{id}` undoes a batch (409 once collected).
- Document templates: reads are membership-level; upload, reset and orientation writes are org-wide roles. An unset orientation is `""`.
- Document numbering: an unknown type is a 400. A never-saved type returns the default with `hasOverride: false`. `counter` is optional on `PUT`.
- Mass data: `GET|POST /api/organizations/{orgId}/{table}/export|import`. An import answers with a report and is never a 4xx for a bad row. The stock count upload sits under `stock-movements`.
- Summaries (`…/summary`, `bom-overview`, `bom/summary`) are a 409 when `masterDataSummaries` is off. `GET …/invoices/outstanding` and `…/incoming-invoices/outstanding` (`api/outstanding_documents.go`) feed the list filters from `GetReceivableAging`/`GetPayableAging`, because a state can lag its balance.
- Journal entries: listing pages only when `limit` is sent (≤ 500, with `offset`, `sort` and `order`; answer `{data, total}`), otherwise a plain array. `DELETE` works on drafts only.
- Fiscal-year close is irreversible. Production order `DELETE` works on drafts only (a status-guarded `DELETE`, like `DeleteInboundDelivery`'s).

## Files
- `api/helpers.go`: `writeJSON`, `writeError`, `decodeJSON`. `writeInternalError` logs the error and answers `internal error (ref <request id>)`.
- `api/request_log.go`: `RequestLogger`, one `slog` line per `/api` request (route pattern, status, duration, `X-Request-ID`, IP, user/org/role from `requestInfo`). It recovers panics. Levels: 5xx error, 403/429 warn, changes and other 4xx info, reads debug; `noteSecurityEvent` raises to warn. **It never logs bodies, query strings, cookies or headers** (`TestRequestLogLeavesOutPrivateData`). See `docs/logging.md`.
- `api/audit.go`: `NewRouter`'s `mux` is an `auditMux` that wraps every change route except `/api/auth/*` and `/api/client-errors`; keep calling `mux.Handle`, which the coverage tests parse. After a 2xx it writes one `audit_events` row; a failed write is logged and never fails the request.
  - The target comes from `auditTarget`. A create names its document via `setAuditEntity`, or by leading its response with the document's `"id"` (`TestAuditCreateRoutesNameTheirDocument`).
  - Snapshots go through `db.AuditSnapshot`; `auditSnapshotTarget` points logo changes at the organization. The "before" row is read only for a request carrying a session this server signed (`hasSignedSessionToken`, F182).
  - Read routes: `GET /api/organizations/{orgId}/audit-events` (org admin) and `GET /api/audit-events` (platform admin), with `userId`/`entityId`/`from`/`to` filters, a `before=<createdAt>:<id>` cursor (`next` in the answer) and `limit` ≤ 500.
- `api/background.go`: the background tasks (backup scheduler, history pruning, login rate-limit sweep) run on a context through `Background`/`sleepCtx`. `NewRouterContext` lets `main.go` stop and wait for them on shutdown; `NewRouter` stays for tests.
- `api/utility.go`: version, backup and restore, and the scheduler.
  - The scheduler marks a day done only after a successful `db.Backup` (`lastSuccessDate`).
  - `applyRetention` deletes only `backupFilePrefix` files.
  - Restore migrates any upload that passes `integrity_check` to the current schema with no version check. This is accepted, so older backups stay restorable.
