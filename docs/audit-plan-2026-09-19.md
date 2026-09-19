# FaturaCloud Re-Audit — Full-Repo Sweep (2026-09-19)

Fresh full-repo audit at commit `604d94e` (`v3.27.2-1-g604d94e`, CHANGELOG's
pending `3.28.0`). Unlike the 2026-09-14 delta audit (F70–F95, scoped to the
v3.20.0 tranches), this is a whole-repo pass across both sides of the stack:
Go `api/` (63 files), the `db/` layer (122 files, 82 migrations), the React
frontend (`src/`, 156 ts/tsx files), the GitHub workflows, the Dockerfile and
compose files, and every `CLAUDE.md`. It continues the numbering at **F96**.

The 2026-09-14 plan is fully remediated (`main` reached `278d3bb`; the two
follow-ups, F94/F95, shipped as #251/#252). Since then, ~73 commits / 195 files
landed the features this sweep had to cover from scratch because they had never
been audited: the **Cash Book** (counter-side cash/loan sales, register
movements, daily-movement and loan-status reports), the **six-role
organization redesign**, **country-derived number formatting**, **generalized
document numbering**, **Excel mass-maintenance** for seven master-data tables,
the `cmd/seed-demo` retail scenario, and the module-by-module UI-audit
remediation (`#279`–`#291`). Those features are where most of this document's
findings live; the rest are older classes this sweep re-examined because no
prior audit had ever covered them.

Baseline health is green, measured before any change: `go vet ./...`,
`gofmt -l .`, `go build ./...`, `go test ./...` (all packages), `tsc --noEmit`,
`oxlint src/` (warnings only — the pre-existing `react(set-state-in-effect)`
set), `pnpm test` (22/22), and `pnpm build` all pass. There are **zero open
GitHub issues**, so this document is the backlog. `go.mod` passes
`govulncheck` with only the CI-allowlisted Excelize advisory outstanding.

**How this document was produced, and how much to trust it.** Seven parallel
domain sweeps (security, API, DB schema/perf, DB business-logic, frontend
reliability, i18n/a11y/perf, CI/docs). Every **High** finding and the highest-
impact **Medium**s were then re-read directly against the source by the
authoring model, and the reproduction steps in F96 were executed (not just
reasoned about). Findings below are marked **Confirmed** when the mechanism was
read in code, and **Suspected** where a live trigger needs a proxy/IdP/timing
condition. None of the Low items were individually re-verified line-by-line;
they are grouped and cited so a remediation pass can check them cheaply.

**Instructions for the executing model:**
- This document is an audit, not a remediation — no code was changed while
  producing it.
- Work phase by phase. One feature branch + PR per phase (never push to `main`
  directly). Conventional commits, no Claude attribution lines.
- After every phase: `go vet ./... && go test -race ./...` and
  `pnpm lint && pnpm build` must pass.
- F96, F97, F111 are the three findings worth scheduling first regardless of
  phase order — each is either a full loss of function or silent data loss.
- Update the `CLAUDE.md` sections whose documented behaviour a task changes.
  Phase 5 *is* the doc correction; do not skip it because it looks cosmetic —
  F132 is a security-model doc that is actively wrong.
- The "Explicitly excluded — do not re-raise" section at the end is binding:
  those were checked and dismissed with a reason.

**Status:** 44 findings (F96–F139). Not yet remediated.

---

## Severity overview

| # | Finding | Area | Severity | Phase |
|---|---------|------|----------|-------|
| F96 | `EnsureFirstAdmin` never sets `isPlatformAdmin` — a fresh install's administrator is not a platform admin, disabling user management, backups, restore and countries | Security / bootstrap | **High** | 1.1 |
| F97 | `allocateAndFinalizeEntryTx` rejects lines on `isActive = 0` accounts for *every* posting path including reversal and fiscal-year close — deactivating an account permanently blocks correcting a posted entry or closing the year | Ledger integrity | **High** | 1.2 |
| F111 | The product form's inline BOM fetch has no `.catch`; a transient failure leaves `bom: []` and the next product save replaces a real recipe with nothing | Frontend / data loss | **High** | 2.1 |
| F132 | Root `CLAUDE.md` and `api/CLAUDE.md` still describe Phase C as deferred and "any authenticated user can read/write every organization" — the membership/role gating shipped | Docs / security model | Medium-High | 5.1 |
| F98 | Last-org-admin / last-platform-admin guards are non-transactional read-then-write; concurrent demotions can leave zero admins | Concurrency | Medium | 1.3 |
| F99 | The document-number counter can be rewound by a stale Settings save (and by a transient read error), silently reissuing numbers with no unique index behind them | Data integrity | Medium | 1.4 |
| F100 | The Cash Book's per-transaction detail list and its export query a zero-width `[dayMs, dayMs]` window, so the table is effectively always empty | Correctness | Medium | 1.5 |
| F101 | A cash sale with no cash-register account configured silently posts to Bank; every seeded chart wires `cash` to Bank and no real org seeds a register account | Accounting | Medium | 1.6 |
| F102 | `createTaxRate` and `createDelivery` map `*ValidationError` to 500 "internal error" — the create-side recurrence of F72 | Correctness / UX | Medium | 1.7 |
| F103 | `DeleteInvoice`/`DeleteIncomingInvoice`/`DeleteOrder`/`DeleteDelivery` re-check status/posted-entry outside the delete's transaction and carry no status guard on the `DELETE` | Concurrency | Medium | 1.8 |
| F104 | `POST /api/payments` and `/payments/{id}/void` are not domain-role gated while the adjacent cash-book routes are — any member can post and reverse GL-settling payments | Authorization | Medium-Low (decision) | 1.9 |
| F105 | `GetDocumentNumberSetting` swallows every non-`ErrNoRows` error as "counter = 0"; `GenerateNextDocumentNumberTx` defaults to 1 on any error | Correctness | Low | 1.10 |
| F106 | A `cashbook`-only member can create arbitrary clients (and their invoices) through `POST /api/cash-sales`' `newClient` path | Authorization | Low | 1.11 |
| F107 | OIDC discovery runs on `context.Background()` while holding `oidcMu`, on public endpoints | Availability | Low-Medium | 1.12 |
| F108 | OIDC links/takes over accounts purely by email, and the `email_verified` guard is skipped when the claim is not a JSON bool | Security (IdP-trust) | Low | 1.13 |
| F109 | The 24h JWT is not revocable on logout or password change | Security (defense-in-depth) | Low | 1.14 |
| F110 | The login rate limiter keys on the leftmost (client-controllable) `X-Forwarded-For` entry once the peer is trusted | Security | Low | 1.15 |
| F112 | Inbound-delivery serial capture is still double-submittable (F86's fix landed only on production orders) | Frontend | Medium | 2.2 |
| F113 | A failed stock movement toasts an error but still closes the drawer, discarding the operator's entry | Frontend | Medium | 2.3 |
| F114 | The Bill-of-Materials list renders a failed summaries fetch as "every product has no recipe" | Frontend | Medium | 2.4 |
| F115 | AP/AR Aging, P&L, Balance Sheet and Daily Cash Movements render fetch failures as legitimate zero/empty states | Frontend | Medium | 2.5 |
| F116 | `useUnsavedChangesWarning` only guards tab-close/refresh — in-app navigation discards dirty document forms with no prompt | Frontend | Medium | 2.6 |
| F117 | The product drawer's "Open in Bill of Materials screen" link discards unsaved edits (the units-of-measure link got F88's guard, this one did not) | Frontend | Medium | 2.7 |
| F118 | Detail pages show an infinite skeleton on fetch failure (F89's fix was page-scoped to production orders) | Frontend | Low | 2.8 |
| F119 | Unhandled list-fetch rejections and swallowed save errors leave stale/empty tables and a drawer that cannot explain itself | Frontend | Low | 2.9 |
| F120 | Un-cancelled fetch races, a null-deref in `getFormattedNumber`, and Products formatting by viewer locale instead of the org's country | Frontend | Low | 2.10 |
| F121 | `minimum_fraction_digits` means "exact digits and round to them" server-side but "minimum digits" client-side — exports and UI disagree on the same amount | Accounting / formatting | Medium | 3.1 |
| F122 | `GetIncomingInvoiceMatchSummaries` is an N+1: one multi-query match per PO-linked bill on every list load | Performance | Medium | 3.2 |
| F123 | Country money-format drift: Austria's separator differs from the frontend's Intl, and a sub-unit negative zero-decimal amount renders `-0` | Formatting | Low | 3.3 |
| F124 | `DeleteTaxRate`'s usage guard omits `journal_lines.taxRateId`, and the tripwire test cannot see the column | Data integrity | Low | 3.4 |
| F125 | `OrganizationUsageCount` (the delete/reset warning) omits every table added since it was written | Correctness | Low | 3.5 |
| F126 | Three unindexed hot predicates: `journal_entries.reversalOfEntryId`, `payments.bankAccountId`/`date`, `cash_movements.counterAccountId` | Performance | Low | 3.6 |
| F127 | `<html lang>` is hardcoded `en` and never updated at runtime — a WCAG 3.1.1 failure for every de/fr user | Accessibility | Medium | 4.1 |
| F128 | Hardcoded / never-extracted strings: brand swatch names, the document-number validator message, the `{m}` month abbreviation — plus `.toFixed()` quantity separators and Imports bypassing `minimum_fraction_digits` | i18n | Low | 4.2 |
| F129 | Accessibility cluster: an unnamed icon-only BOM delete button, unlabelled role `Select`s, placeholder-only filters, a colour-only insufficient-stock cue | Accessibility | Low-Medium | 4.3 |
| F130 | Fourteen list pages still use a module-level `searchAtom` + unmemoized filter with no debounce; three pages use the correct pattern | Frontend consistency | Low | 4.4 |
| F131 | Misc UI consistency: two settings tables never show a loading state; clickable rows are not keyboard-operable; a Cash Book row and its nested button both fire the same handler | Frontend consistency | Low | 4.5 |
| F133 | Root `CLAUDE.md` says PDF generation is client-side `@react-pdf/renderer` with "no server involvement" — it is server-side Excel-template + LibreOffice | Docs | Medium | 5.2 |
| F134 | The entire Cash Book, Document Numbering and org-role-redesign surface is undocumented in every `CLAUDE.md` | Docs | Medium | 5.3 |
| F135 | Role-gating docs are wrong: GL export/fiscal close are documented "org admin only" (now admin-or-accounting), `db/CLAUDE.md` still says `role CHECK admin/user` and "15 `default*AccountId`", and `PUT /api/organizations/{id}` is documented as `protected` (now org-member) | Docs | Medium | 5.4 |
| F136 | The "Adding a New API Endpoint" recipe shows only `protected`/`platformAdminProtected`/`orgAdminProtected`, omitting the wrappers the route-coverage test enforces | Docs / security | Medium | 5.5 |
| F137 | CI never builds the Docker image; breakage is only found on a release tag | CI | Medium | 5.6 |
| F138 | Base images are tag-pinned only, not digest-pinned, despite Actions being SHA-pinned | Docker / supply chain | Medium | 5.7 |
| F139 | Ops/doc nits cluster: no healthcheck, compose doesn't pass `VERSION`, `pnpm audit --prod` skips the build chain, `govulncheck@latest` unpinned, `package.json` version `0.0.0`, stale sidebar/route-note lists, a stale DATEV comment, a dangling seed-demo README reference, React Router 7 vs 8 | Ops / docs | Low | 5.8 |

---

## 1. Backend correctness, ledger integrity, authorization

### 1.1 — F96: a fresh install's administrator is not a platform admin

`EnsureFirstAdmin` (`api/users.go:494-496`) inserts the first user with
`role = 'admin'` but **omits `isPlatformAdmin`**:

```go
INSERT INTO users (id, email, passwordHash, displayName, role, createdAt)
VALUES (?, ?, ?, 'Administrator', 'admin', ?)
```

Migration `0069_add_organization_users.up.sql:30` backfills `isPlatformAdmin = 1`
only for rows that already exist when it runs: `UPDATE users SET isPlatformAdmin = 1
WHERE role = 'admin'`. Migrations run inside `db.NewDatabase`, before
`api.EnsureFirstAdmin` is called (`main.go:93-94`), so on a **brand-new** database
the backfill sees zero users and the later insert gets the column's
`DEFAULT 0`. `authMiddleware` re-reads `isPlatformAdmin` from the DB on every
request (`api/middleware.go:82-91`), and `platformAdmin` gates on that column —
not on `role`.

Verified by reproduction against the real `db.NewDatabase` + `EnsureFirstAdmin`
order: `fresh-install first admin: role="admin" isPlatformAdmin=0`.

**Consequence.** On every fresh deployment — the normal path for the shipped
image — the administrator cannot reach any `platformAdminProtected` route: user
management, backups, restore, countries all return 403. There is no in-app
recovery (the platform-admin bootstrap path is deliberately unrecoverable by
design), so the operator must edit SQLite directly. Existing installs that
predate migration `0069` were backfilled and are unaffected; this is
specifically the fresh-install and "new admin row" path. Note the same
`INSERT` shape is the only user-creating path that bypasses migration 0069's
backfill, so a second bootstrap path is unaffected.

**Fix direction:** set `isPlatformAdmin = 1` explicitly in `EnsureFirstAdmin`'s
`INSERT` (and add a regression test that runs the real open→migrate→seed order
against a temp DB and asserts the seeded admin can pass `platformAdmin`).

### 1.2 — F97: deactivating an account blocks reversal and fiscal close

`allocateAndFinalizeEntryTx` (`db/journal_entry.go:359-373`) rejects **any**
entry containing a line on an account with `isActive = 0`, with the comment
"shouldn't receive new postings, from any path — manual entry, auto-post,
payments, or a fiscal-year close". But the same choke point is on the
**reversal** path: `reverseEntryTx` builds flipped lines reusing the original
entry's exact `accountId`s (`db/gl_posting.go:88-101`) and then calls
`allocateAndFinalizeEntryTx` at `db/gl_posting.go:107`. It cannot distinguish
"a new posting" from "the reversal of an old one".

Every reversal of a posted entry flows through `reverseEntryTx` — invoice/bill
cancel (`UpdateInvoiceState`/`UpdateIncomingInvoiceState`), payment void,
delivery/receipt cancel (COGS/GRNI reversal), production-order cancel. And
`CloseFiscalYear` posts its zeroing entry through `postAutoEntryTx`
(`db/fiscal_year_closing.go:141`), which also reaches
`allocateAndFinalizeEntryTx`, building a line for every revenue/expense account
with activity that year.

**Consequence.** Once an account that any posted entry references is
deactivated (`UpdateAccount` permits the `isActive` toggle regardless of posted
history — `db/account.go:202-208` — unlike its `type`/`isGroup` guards), those
entries can never be reversed and the fiscal year can never be closed. The
failure is a 409 "cannot post to an inactive account" with no hint that
deactivating the account caused it, and the only workaround is to reactivate a
deliberately-retired account. This defeats the core invariant that posted
entries are immutable and corrected only by reversal.

**Fix direction:** the active-account check belongs on the *posting* path, not
the reversal/closing path — e.g. a boolean parameter on
`allocateAndFinalizeEntryTx` (or a separate `allocateAndFinalizeReversalTx`),
or refuse the `isActive → 0` transition while the account has posted
`journal_lines`. Add a test that posts, deactivates, then reverses and closes.

### 1.3 — F98: last-admin guards are non-transactional (org and platform)

`isLastOrgAdmin` (`db/organization_user.go:255-273`) is called by
`AddOrganizationUser` (`:137-168`), `UpdateOrganizationUserRole` (`:172-196`)
and `RemoveOrganizationUser` (`:200-219`), each followed by a **separate**
write — no transaction, no re-check under a conditional `WHERE`. The platform
side is the same shape: `checkNotSoleOrgAdmin`/`countActiveAdmins`
(`api/users.go:250`, `:344`, `:70`). `SetMaxOpenConns(1)` serializes individual
statements, not a read-modify-write, and the handlers hold only a shared
`RLock`.

Two concurrent demotions of the only two admins can both observe "one other
admin exists", both pass, and both commit — leaving an organization (or the
platform) with zero admins and no in-app recovery. This is the same TOCTOU
class F48/F74 fixed for document deletes; the membership paths were never
converted.

**Fix direction:** put the guard and the write in one `Beginx()` transaction
with a re-count against `tx`, or use a conditional write whose `RowsAffected`
proves the count still holds.

### 1.4 — F99: document numbers can be silently reissued

Two compounding defects in `db/document_number.go`:

1. **`UpdateDocumentNumberSetting` is a read-modify-write across two
   statements** (`:159-188`): it reads the current counter, then upserts
   `counter = excluded.counter`. `SetMaxOpenConns(1)` serializes the two
   statements but not the pair, so a `GenerateNextDocumentNumberTx` that
   commits between them is overwritten by the stale read.
2. **The client always re-sends its mount-time counter.** The Settings →
   Document Numbering form sends every type's `counter` loaded once on mount
   (`src/routes/settings/document-numbering.tsx:187-221`), even when the user
   only changed a format. An admin who opens the page, while another user
   creates an order, then saves a format edit rewinds the counter.

There is **no unique index** on the number columns
(`orders`/`purchase_orders`/`outbound_deliveries`/`inbound_deliveries`/
`production_orders`), so the duplicate is silent.

**Fix direction:** compute the next counter atomically in SQL (or lock the row
in a transaction) and stop treating the client's counter as authoritative — the
PUT should carry format-only changes, not a counter snapshot. See also F105.

### 1.5 — F100: the Cash Book detail list queries a zero-width window

`GetCashMovementDetails` (`db/cash_movement_details.go:58`) treats its
`[startDate, endDate]` arguments as a literal inclusive range — `p.date >= ?
AND p.date <= ?` (`:101`) and the same on `cash_movements` (`:119`). But every
caller passes the same UTC-midnight value twice:
`src/routes/cash-book.tsx:247-250` (`utcDayMs(selectedDate)`, defined at `:228`
as `Date.UTC(y, m, d)`), and `db/report_export.go:100-104` for the xlsx/pdf
export. Only a row stamped exactly `00:00:00.000 UTC` matches. Withdrawals are
stamped `Date.now()` (`cash-book.tsx:349`) and cash sales default to the current
time, so basically nothing matches.

Meanwhile `GetDailyCashMovements` floors both ends to whole UTC days
(`db/gl_reports.go:412-413`, `floorToUTCDay` at `:365`) — so the summary cards
show correct totals while the detail table beneath them is empty, and the
export reproduces the gap. The existing unit test passes only because its
fixture creates the movement at exactly the filter value
(`db/cash_movement_details_test.go:117`).

There is a second, compounding offset bug for non-UTC orgs: the cash-sale
`DatePicker` yields local midnight while the bucketing is UTC, so a transaction
can land in the previous UTC day's summary and in no day's detail.

**Fix direction:** floor both endpoints inside `GetCashMovementDetails` the way
`GetDailyCashMovements` does (or pass `[dayStart, dayStart+86_399_999]`), and
reconcile local-date entry with UTC-day bucketing. Add a test whose creation
timestamp differs from the filter timestamp.

### 1.6 — F101: cash sales fall back to Bank when no register account is set

`CreateCashSale` resolves the counter account as
`defaultCashRegisterAccountId → defaultCashAccountId → 409`
(`db/cash_sale.go:130-137`), and the frontend mirrors that fallback
(`src/routes/cash-book.tsx:527-534`). But `seedDefaultChartOfAccounts` wires the
`cash` role — and therefore `defaultCashAccountId` — to the **Bank** account in
every template (`db/account.go:325,371,406,573`), and
`defaultCashRegisterAccountId` is seeded only by `cmd/seed-demo` for its own
demo orgs, never for a real new organization. So the fallback always resolves to
Bank and the guard meant to prevent it never fires.

**Consequence:** every cash sale debits Bank instead of the till; the register
balance/report is hidden entirely while its account is unset, so the user gets
no signal; Bank is overstated by all cash revenue. The code comment at
`cash-book.tsx:520-527` states this is exactly the outcome it is avoiding.

**Fix direction:** require `defaultCashRegisterAccountId` for the Cash Book path
(409 naming the setting) instead of falling back to Bank, and/or seed a register
`cashRegister` role in the chart.

### 1.7 — F102: create-side validation errors surface as 500

F72 fixed the *update* handlers; the *create* side of two domains was left
behind:

- `createTaxRate` (`api/tax_rates.go:40`) calls `writeInternalError`, but
  `db.CreateTaxRate` returns `*db.ValidationError` for an invalid category code
  (`db/tax_rate.go:152`) and for a missing output/input tax account (`:121,130`).
- `createDelivery` (`api/deliveries.go:49`) calls `writeInternalError`, but
  `db.CreateDelivery` propagates `*db.ValidationError` from `requireShippableOrder`
  (`db/delivery.go:178,189`), `checkDeliveryHeaderFKOwnership` (`:198,207`) and
  `replaceDeliveryLineItemsTx` (`:787,799`).

A direct API client gets `500 {"error":"internal error"}` and the server logs an
internal failure where the intended answer is a 409 with the real business
message. `TestCreateRouteOrgChecksArePresent` does not check the error writer.

**Fix direction:** switch both to `writeMutationError`, and sweep every
`create*` handler for the same shape.

### 1.8 — F103: four delete paths lost the status re-check

`DeleteInvoice` (`db/invoice.go:603-637`), `DeleteIncomingInvoice`
(`db/incoming_invoice.go:584-607`), `DeleteDelivery` (`db/delivery.go:737-752`)
and `DeleteOrder` (`db/order.go:511-544`) read status / probe for a posted entry
**before** the delete, and the `DELETE` itself carries no status guard or
tx-level re-check. A concurrent `PATCH …/status`/`…/state` (e.g. `draft → sent`
posts AR/AP; `draft → shipped` posts stock + COGS) that commits in the gap lets
the delete remove a document whose journal entry / stock movements are now
posted. Those source ids are polymorphic with no FK, so the rows are orphaned
and the GL keeps a balance for a document that no longer exists.

`DeleteProductionOrder` and `DeleteInboundDelivery` were converted to the
canonical status-guarded `DELETE` in #244; these four were not.

**Fix direction:** `DELETE … WHERE id = ? AND status = 'draft'` (or the
appropriate terminal-status refusal) with a `RowsAffected` check, mirroring the
fixed sites.

### 1.9 — F104: payments are not domain-role gated (decision needed)

`POST /api/payments` is plain `protected` (`api/router.go:677`) and
`POST /api/payments/{id}/void` is `orgMemberProtected` (`:682`), while the
adjacent money/ledger operations are role-gated: cash sales and cash movements
require `cashbook` (`:678-679`), journal entries require `accounting`
(`:661-663`). `CreatePayment` posts settlement entries and `VoidPayment`
**reverses a posted GL entry**; neither is listed in the maintained
`domainRouteRoles` table (`api/cross_org_test.go`).

**Consequence:** a `sales`/`purchasing`/`cashbook`-only member can record or
reverse financial settlement in the org. This may be deliberate (payments span
AR and AP), but it is materially inconsistent with the role model the redesign
just shipped and should be an explicit decision, not an omission.

**Fix direction:** either add payments to a role set (likely `accounting`, with
`cashbook` for the cash-register path) or record the decision that payments are
intentionally cross-domain.

### 1.10 — F105: document-number reads swallow every error

`GetDocumentNumberSetting` (`db/document_number.go:140-142`) returns
`{Format: defaultFormat, Counter: 0}, nil` for **any** query error, not just
`sql.ErrNoRows`; `GenerateNextDocumentNumberTx` (`:234-237`) has the same
`if err == nil` shape and defaults the counter to 1. A transient DB error can
therefore reset a real counter and reissue numbers, or make the settings screen
report plausible-but-wrong values at HTTP 200.

**Fix direction:** distinguish `sql.ErrNoRows` from a real error and propagate
the latter.

### 1.11 — F106: `cashbook` can create clients and invoices

`POST /api/cash-sales` is gated to `admin|general|cashbook`
(`api/cash_sale.go:14`), but `CreateCashSale` inserts a `clients` row
(`db/cash_sale.go:317-331`) and an `invoices` row (`:362-374`) — writes that
their own routes gate to `sales` (`api/clients.go:35`). The cash-sale's invoice
is inherent, but the arbitrary `newClient` path is a pure sales-domain write.

**Fix direction:** decide whether the `cashbook` role should be able to create
new clients; if not, require the client to pre-exist or gate the create branch
more tightly.

### 1.12 — F107: OIDC discovery has no timeout under the lock

`ensureOIDC` takes `oidcMu.Lock()` and then calls
`oidc.NewProvider(context.Background(), …)` (`api/oidc.go:61-68`), on the
public `GET /api/auth/oidc/login` and `/callback` routes (`api/router.go:71-73`).
A hung IdP pins the request goroutine (the server's `WriteTimeout` cannot cancel
a `context.Background()`), and every other SSO request blocks behind the mutex.
Local login is unaffected.

**Fix direction:** use `context.WithTimeout` for discovery, and avoid holding
`oidcMu` across the network call.

### 1.13 — F108: OIDC email-based account linking

`provisionOrSyncUser` (`api/users.go:423-446`) treats `users.email` as the
identity anchor, so an IdP-asserted email matching an existing account takes
that account over (and re-syncs its role). The `email_verified` guard
(`api/oidc.go:184-201`, `:413`) only fires when the claim is a Go `bool`; a
string `"true"`/`"false"` — which some IdPs emit — skips it, and an absent claim
is treated as "nothing to check".

This is mostly IdP trust, which the documented threat model already accepts, but
the type-assertion skip is a concrete weakness worth tightening.

**Fix direction:** accept both bool and string spellings of `email_verified`,
and decide an explicit account-linking policy (e.g. only link to an account that
was itself OIDC-created, or require an admin grant).

### 1.14 — F109: JWT not revoked on logout or password change

`logout` only expires the cookie (`api/auth.go:235-240`); the 24h JWT carries no
`jti`/version, and `authMiddleware` has no revocation check. Changing a
password (`api/users.go:270-276`, `:311-316`) does not invalidate existing
tokens. Deactivation *is* honored immediately, which limits the gap; there is
no in-app exfiltration path (`httpOnly`, no localStorage).

**Fix direction:** a per-user `tokenVersion` column re-checked in
`authMiddleware`, bumped on password change and logout-all.

### 1.15 — F110: rate limiter trusts the leftmost `X-Forwarded-For`

`clientIP` (`api/auth.go:134-151`) keys on `strings.SplitN(xff, ",", 2)[0]`
once the TCP peer matches `TRUSTED_PROXIES`. nginx/NPM's
`$proxy_add_x_forwarded_for` **preserves** a client-supplied header and appends
the real IP, so the leftmost value is attacker-controlled — enabling distributed
password-spraying past the per-IP bucket and a bounded login DoS by filling the
bucket map. The per-email bucket (10/min) still caps single-account brute force.

**Fix direction:** use the rightmost address the trusted proxy appended (or
`X-Real-IP`), not the leftmost.

---

## 2. Frontend reliability

### 2.1 — F111: the product form silently wipes a BOM on fetch failure

The dedicated BOM drawer got F84's guard; the **product form's** inline copy did
not. `src/components/products/form.tsx:106-120` fetches the recipe with
`GetProductBOM(productId).then(...)` and **no `.catch`**; on failure `form.bom`
stays at `[]` (from reset/initial values). `handleSubmit` then unconditionally
calls `ReplaceProductBOM(productId, values.bom ?? [])` for any finished product
(`:211-220`), so the *next save for any reason* — a price edit, a description
tweak — replaces a real recipe with nothing. Switching between finished
products quickly can also land a previous product's lines via the stale `.then`.

This is the same data-loss class the prior audit rated most serious (F84), at
the second entry point into the same `PUT /api/products/{id}/bom`.

**Fix direction:** mirror the drawer's fix — catch, surface, disable Save while
the BOM load is unresolved or failed, and add a per-product cancellation flag.

### 2.2 — F112: inbound serial capture still double-submittable

`SerialCaptureModal` supports a `confirming` prop
(`src/components/stock/serial-capture-modal.tsx:41,156`), and production orders
pass it (`production-orders/details.tsx:710`) — F86's fix. The inbound receipt
omits it (`src/routes/inbound-deliveries/details.tsx:296-300`, handler
`:633-639`), so OK stays enabled through `await applyStatusChange(...)`. A second
click fires a second `PATCH …/status`; the F48 re-check rejects it, so stock is
not double-added, but the user sees an error toast on top of the success.

**Fix direction:** pass `confirming` and guard the handler with an in-flight
flag, matching the production-order path.

### 2.3 — F113: failed stock movement discards the entry

`createStockMovementAtom` (`src/atoms/stock.ts:41-45`) catches and returns
`null` instead of rethrowing, but `src/components/stock/movement-form.tsx:67-107`
calls `handleClose()` regardless. A 409 (insufficient stock, missing cost basis,
serialized validation) toasts and then closes the drawer, losing everything the
operator typed.

**Fix direction:** rethrow (or check the returned value) and keep the drawer
open on failure, the convention every other mutation form now follows.

### 2.4 — F114: BOM list reports failure as missing data

`src/routes/bill-of-materials.tsx:54-62` runs `Promise.all([...]).finally(...)`
with no `.catch`; a failed `GetBOMSummaries` leaves `summaries = {}`, and every
finished product renders the "None — no recipe defined yet" warning badge
(`:144-153`). This is the F85 failure mode on the page whose entire job is
showing recipe status.

**Fix direction:** add a `failed` state + error alert + retry, as the
production-order page already does.

### 2.5 — F115: accounting reports render fetch failures as zero

AP Aging (`src/routes/accounting/reports/ap-aging.tsx:29`), AR Aging (`:29`),
Profit & Loss (`profit-and-loss.tsx:51-54`), Balance Sheet (`balance-sheet.tsx:31-32`)
and Daily Cash Movements (`daily-cash-movements.tsx:66-71`) all reset to
`null`/`[]` on error with no `failed` flag, alert or retry. The render then
shows `€0.00` and "No outstanding bills/invoices" or "No activity in this
range". The Reporting pages and Inventory Valuation got the correct `failed`
state + Retry (`inventory-valuation.tsx:26-58`); these did not.

**Consequence:** a transient 500 tells a collections/AP user their receivables
or payables are zero — the most dangerous possible wrong answer.

**Fix direction:** port the `failed` + `Alert action={Retry}` pattern.

### 2.6 — F116: unsaved-changes warning doesn't guard in-app navigation

`useUnsavedChangesWarning` (`src/hooks/useUnsavedChangesWarning.ts:4-12`) only
registers a `beforeunload` handler; there is no `useBlocker`/route guard
anywhere, and the app uses declarative `<BrowserRouter>`
(`src/app.tsx:373`). Used by invoices/orders/purchase-orders/deliveries/
inbound-deliveries/incoming-invoices detail pages and the imports form. Clicking
any sidebar/header link discards dirty edits silently — the hook's name implies
protection it does not provide on the client side.

**Fix direction:** add a router-level blocker (React Router's `useBlocker`, or a
navigation guard) or rename the hook to what it actually does.

### 2.7 — F117: product drawer's BOM link discards unsaved edits

F88 added a confirmation to the units-of-measure link
(`src/components/products/form.tsx:488-507`) but not to the sibling "Open in
Bill of Materials screen" link (`:535-541`), whose `e.stopPropagation()` only
stops the Select popup closing. Since the drawer's visibility is router state
(`get(location.state, "productModal")`), the drawer unmounts and all typed
fields are lost without a prompt.

**Fix direction:** apply the same `isFieldsTouched()` + `Modal.confirm` guard.

### 2.8 — F118: detail pages spin a skeleton forever on failure

`invoices/details.tsx:401-412`, `orders/details.tsx:274-283`,
`purchase-orders/details.tsx:359-368`, `deliveries/details.tsx:320-329`,
`inbound-deliveries/details.tsx:325-334`, `incoming-invoices/details.tsx:322-331`
catch the atom error, return `null`, and then render `<Skeleton active />`
unconditionally when the document is null — for both loading and failure. F89
fixed only the production-order page (`production-orders/details.tsx:502-523`).

**Fix direction:** distinguish `hasError` from loading and show an alert +
retry.

### 2.9 — F119: unhandled list rejections / swallowed save errors

- `src/routes/organizations/index.tsx:117-135` — `fetchOrgs` is `try/finally`
  with no `catch`; failure leaves an empty table with no error.
- `src/routes/organizations/index.tsx:344-347` — `handleSubmit`'s `catch {}`
  swallows save failures with **no toast at all**; `:183-204`'s `openEdit`
  `catch {}` opens a blank edit drawer.
- `src/routes/products.tsx:109-116`, `src/routes/inventory.tsx:190-196` —
  `.then(...).finally(...)` without `.catch`, so a failed list fetch leaves
  stale rows and an unhandled rejection.
- `src/routes/login.tsx:29` — `GetOidcEnabled().then(...)` unhandled; the SSO
  button silently never appears.

**Fix direction:** surface the errors (toast/alert) and set a failed state
rather than leaving stale or empty UI.

### 2.10 — F120: fetch races, a null-deref, and Products locale drift

- `src/routes/cash-book.tsx:243-267` — `refreshDailyMovement`/`refreshLoanStatus`
  apply whichever response resolves last, so rapidly changing the register date
  or loan-status customer can leave stale rows. Same shape in
  `src/components/products/bom-editor-drawer.tsx:214-227` and the
  `src/routes/reporting/*` pages. No cancellation flags.
- `src/utils/currencies.tsx:176-182` — `getFormattedNumber` dereferences
  `organization.minimum_fraction_digits` after explicitly treating
  `organization` as nullable two lines above; a null org (fetch failure / no
  org selected) throws inside a table cell. Callers include
  `invoices/index.tsx:213`, `incoming-invoices.tsx:188`.
- `src/routes/products.tsx:40-48,280-292` — Products formats money with the
  viewer's `i18n.locale`, bypassing `numberFormatLocale(organization?.country_code)`
  that every other screen uses, so a Tunisian org viewed in English shows a
  different separator convention than the rest of the app.

**Fix direction:** request-id/cancellation guards for the races, optional
chaining for the deref, and route Products through `formatOrgCents`.

---

## 3. Accounting correctness and performance

### 3.1 — F121: `minimum_fraction_digits` means two different things

`db/format_money.go:103-107` treats the organization's
`minimumFractionDigits` as **exact** digits — it replaces the currency's digit
count and rounds to it. The frontend (`src/utils/currencies.tsx:85-98,113-130`)
forwards the same value as Intl `minimumFractionDigits`, leaving
`maximumFractionDigits` to default to the currency's own. Setting USD/EUR to
"0 decimal places" therefore renders `$12.5` on screen (Intl clamps max to 2)
while the server export/PDF renders `13` (rounded to whole units). Verified
against Intl directly.

**Fix direction:** pass both `minimumFractionDigits` and `maximumFractionDigits`
from the frontend, or define the server value as a true minimum and keep at
least the currency's digits.

### 3.2 — F122: incoming-invoice match summaries are an N+1

`GetIncomingInvoiceMatchSummaries` (`db/incoming_invoice_match.go:266-285`)
loads every PO-linked bill id, then loops calling `GetIncomingInvoiceMatch`,
which itself runs two queries plus three per linked line (`:88-92,106-110,129-135`).
Called on every incoming-invoices list load (`api/incoming_invoices.go:58`).
Cost is ≈ N·(2+3L) serialized queries. The established batch pattern
(`GetBillOfMaterialsSummaries`, `GetImportSummaries`) is a single set-based
query.

**Fix direction:** compute the summaries in one set-based query, as the sibling
list endpoints do.

### 3.3 — F123: money-formatting drift

- **Austria separator.** `db/format_money.go:50` gives `"AT": {decimal: ",", group: " "}`,
  but ICU 78's `de-AT` EUR grouping is `"."` (verified), so an Austrian org's
  exports (`1 234 567,89`) disagree with every on-screen amount
  (`1.234.567,89`). The table's own comment claims the two agree. `pt-PT`,
  `pl-PL`, `ru-RU` (NBSP grouping) are similar, though those are near-invisible
  and browser-ICU-dependent.
- **`-0`.** `db/format_money.go:110-141` can render `-0 JPY` for a sub-unit
  negative amount at 0 decimal places, because the sign test at `:139` runs
  after the `digits <= 0` branch may have rounded `whole` away.

**Fix direction:** regenerate the server separator table from the same ICU data
the frontend uses, and suppress the sign when the rendered magnitude is zero.

### 3.4 — F124: `DeleteTaxRate` misses a reference

`db/tax_rate.go:263-267`'s usage count omits `journal_lines.taxRateId` (a real
FK with `ON DELETE SET NULL`, migration `0052`). A rate referenced only by a
manual journal entry can be deleted, nulling the historical link (the DATEV
export's BU-key join is the visible casualty). The tripwire test
(`db/db_test.go:3284-3292`) matches a column literally named `taxRate`, so it
cannot see `taxRateId`.

**Fix direction:** add the reference to the count and widen the tripwire's
column matcher.

### 3.5 — F125: `OrganizationUsageCount` is stale

`db/organization.go:601-623` omits `production_orders`, `imports`,
`cash_movements`, `product_serial_numbers`, `document_number_settings` and the
BOM/version tables, though its doc comment claims to report what a reset
cascade-deletes. The actual reset *is* complete
(`TestResetOrganizationDataCoversEveryOrganizationScopedTable` passes), so this
only understates the confirmation dialog.

### 3.6 — F126: three unindexed hot predicates

- `journal_entries.reversalOfEntryId` (`db/journal_entry.go:121-124`) — every
  journal-entry detail view full-scans the largest append-mostly table.
- `payments.bankAccountId` and `payments.date` (`db/cash_movement_details.go:101`)
  — the daily cash report grows with total payments, not the day's activity.
- `cash_movements.counterAccountId` (migration `0080`, used by
  `db/account.go:252`) — minor today, grows with till volume.

**Fix direction:** add the three indexes in a migration.

---

## 4. i18n, accessibility, UI consistency

### 4.1 — F127: `<html lang>` is always `en`

`index.html:2` hardcodes `<html lang="en">` (also baked into `dist/`), and
nothing updates it: `dynamicActivate` (`src/utils/lingui.tsx:25-37`) and
`app.tsx:154-168` switch lingui/dayjs/antd but never write
`document.documentElement.lang`. A de/fr user's page reports English to assistive
tech — a WCAG 3.1.1 failure affecting the whole app, fixed with one assignment.

### 4.2 — F128: hardcoded strings and locale-blind formatting

- **Never extracted:** brand swatch names and "Default"
  (`src/components/organizations/brand-color-picker.tsx:9-17,69`); the
  document-number validator message
  (`src/utils/document-number.ts:30`, shown at
  `settings/document-numbering.tsx:90-96`); the `{m}` preview/help month
  (`src/utils/document-number.ts:59`, hardcodes `toLocaleString("en")`).
- **`.toFixed()` separators:** quantities/percentages in
  `src/routes/inventory.tsx:70,396`, `products.tsx:316`,
  `production-orders/details.tsx:340,343,623`, `dashboard.tsx:358`,
  `components/imports/form.tsx:480` render `2.5`/`1.5%` with a period where de/fr
  expect a comma.
- **Imports bypasses the org setting:** `src/routes/imports.tsx:82-87` and
  `components/imports/form.tsx:470-476` call `formatCents`, not
  `formatOrgCents`, so they ignore `minimum_fraction_digits`.

The catalogs themselves are clean: en/de/fr all 1244 entries, 0 untranslated —
F92's backlog is fully closed and the CI drift gate is live. These are strings
the extractor cannot see.

### 4.3 — F129: accessibility cluster

- `src/components/products/bom-fields.tsx:59` — the only icon-only `<Button>`
  in `src/` with no `aria-label`/`title`; it is the destructive "remove
  component" control.
- `src/components/organizations/organization-members-panel.tsx:76-84,110-121` —
  per-row and add-member role `Select`s have no accessible name or row
  association; the email `Input` relies on a placeholder.
- Placeholder-only filters/search (`src/components/page-header.tsx:39-47` and
  the new-tranche screens) expose no name once a value is set.
- `src/routes/deliveries/details.tsx:527` conveys "requested exceeds available"
  by colour alone.

### 4.4 — F130: two search standards still coexist

Fourteen list pages (`clients`, `orders`, `invoices/index`, `vendors`,
`deliveries`, `imports`, `purchase-orders`, `inbound-deliveries`,
`incoming-invoices`, `accounting/chart-of-accounts`, and four under `settings/`)
use a module-level `searchAtom` plus an inline/unmemoized filter with no
debounce and no row memoization — every keystroke writes a global atom and
re-renders every visible row, and the atom leaks filter state across navigation.
Three pages built in the same period (`production-orders`, `units-of-measure`,
`bill-of-materials`) use the correct `useState` + `useMemo` shape, and the
paginated `products`/`inventory` lists debounce server-side. F91 named the
inconsistency; three pages were migrated, fourteen were not.

### 4.5 — F131: misc UI consistency

- `src/routes/settings/tax-rates.tsx:74` and `countries.tsx:85` render their
  `Table` with no `loading` prop (siblings pass it) — a flash of "No data".
- Clickable rows without keyboard affordances: `invoices/index.tsx:153-156`,
  `products.tsx:220-224`, `bill-of-materials.tsx:106-110`, `cash-book.tsx:673-689`.
- `src/routes/cash-book.tsx:673-689` — the row `onClick` and the nested "Select"
  button both fire `selectClient`, causing a duplicate fetch.
- `/organizations` eagerly pulls the whole `@react-pdf/renderer` engine
  (`organization-edit-drawer.tsx:31` → `layouts.ts:3-4` → `pdf.tsx`) into a
  1.27 MB route chunk (~462 KB gzip) only to read a 3-option layout list; the
  registry is orphaned for invoices since the PDF unification. Route-lazy, so
  first paint is unaffected, but it is a large avoidable admin-screen download.

---

## 5. Documentation, CI, ops

### 5.1 — F132: the authorization-model docs are materially false (Medium-High)

Root `CLAUDE.md:94` states: *"Authorization gap (Phase C deferred): any
authenticated user can read/write every organization's data through the ~130
`protected()` routes. Only global actions … and org-admin actions … are gated."*
`api/CLAUDE.md:7` repeats that Phase C "is explicitly deferred" and that
`GET /api/organizations` still returns every organization.

The code no longer matches. `api/router.go` now registers **37** plain
`protected(`, **128** `orgMemberProtected(`, **35** `orgRoleProtected(`,
**3** `orgRoleAdminProtected(` and **6** `orgAdminProtected(` routes;
`listOrganizations` uses `GetUserOrganizations`; migration `0081` widened roles
to six. The route-coverage tripwires
(`TestPhaseCRouteCoverage`, `TestCreateRouteOrgChecksArePresent`,
`TestDomainRoleRouteCoverage`) enforce this.

**Why this is serious:** a developer following the false doc would register new
routes with plain `protected()` and reintroduce the cross-org gap, and would
mis-threat-model the whole app. It also hides that the Phase C deferral it
describes has been substantially closed.

**Fix direction:** rewrite the root and `api/` authorization sections to describe
the membership/role model as shipped, name the wrappers, and state precisely
what (if anything) remains ungated.

### 5.2 — F133: the PDF architecture claim is inverted

Root `CLAUDE.md:16` says **"PDF Generation: @react-pdf/renderer (client-side, no
server involvement)"**. Every invoice/order/PO/delivery/receipt/bill PDF is in
fact generated server-side via the Excel-template engine + headless LibreOffice
(`api/router.go` export routes, `Dockerfile:66-96`); `src/CLAUDE.md:50-51` says
the client-side path was removed and the remaining `pdf.tsx`/`pdf-tunisia.tsx`
are orphaned. The same root file also self-contradicts elsewhere (its Docker
section describes the LibreOffice runtime). This is the top-level architecture
description, so it actively misleads any PDF work.

### 5.3 — F134: Cash Book / Document Numbering / role redesign are undocumented

No `CLAUDE.md` mentions the Cash Book, Document Numbering, `cash_sale`,
`cash_movement`, `document_number`, `defaultCashRegisterAccountId`, or migration
`0081`. Yet the `db/CLAUDE.md` contract promises "per-file notes for every
`db/*.go`", and the root file promises the same for `src/`. Five-plus new db
files and the whole Cash Book route/screen have no map. This is a large,
behaviour-bearing (GL-posting) feature invisible to the documented architecture.

### 5.4 — F135: role-gating docs are wrong

- `api/CLAUDE.md:88,94-95,99` and the route notes say fiscal-year close and GL
  export are "org admin only". They are now `orgRoleAdminProtected(…,
  ["accounting"])` (`api/router.go:642,721-722`) — admin **or** accounting.
- `db/CLAUDE.md:53` still says `organization_users … role CHECK admin/user`;
  migration `0081` widened it to `admin|general|sales|purchasing|accounting|cashbook`.
- `db/CLAUDE.md:52` says "The 15 `default*AccountId` columns"; migration `0079`
  added a 16th (`defaultCashRegisterAccountId`) and calls itself the 16th.
- `db/CLAUDE.md:52` says `PUT /api/organizations/{id}` is `protected`; it is now
  `orgMemberProtected` (`api/router.go:168`).

### 5.5 — F136: the new-endpoint recipe omits the enforced wrappers

Root `CLAUDE.md:156-165` documents only `protected`,
`platformAdminProtected` and `orgAdminProtected`. The codebase's authoritative
wrappers also include `orgMemberProtected`, `orgRoleProtected` and
`orgRoleAdminProtected`, and the coverage test fails any new org route that
doesn't use one. Following the documented recipe produces an ungated route (or
a failing test with no context in the doc).

### 5.6 — F137: CI never builds the image

`.github/workflows/ci.yml` has no `docker build`/`docker compose config` step;
`docker.yml` runs only on `push: tags: v*`. A broken Dockerfile, dropped build
arg or compose error merges to `main` unvalidated and is first discovered at
release time. For a product whose sole artifact is the image, that is the one
artifact with no PR gate.

**Fix direction:** add a `docker build` (and `docker compose config`) job to CI
on PRs.

### 5.7 — F138: base images are not digest-pinned

`Dockerfile:8,50,78` pin `node:22-alpine`, `golang:1.26-alpine`,
`debian:bookworm-slim` by tag only. The repo SHA-pins every GitHub Action, so
this is an inconsistency in the same supply-chain model; mutable tags also make
rebuilds non-reproducible.

### 5.8 — F139: ops/doc nits cluster

- `docker-compose.yml` has no `healthcheck`, and there is no `/healthz` endpoint
  or `HEALTHCHECK`; `restart: unless-stopped` cannot restart a wedged process.
- `docker-compose.yml:10-14` does not pass `VERSION` as a build arg, so a local
  `docker compose up --build` with `VERSION=vX` tags the image `:vX` but bakes
  `main.version="dev"` and Sentry release `"dev"` (`Dockerfile:28-29`;
  `deploy.md:139-145` recommends exactly this path).
- `.github/workflows/ci.yml:184` runs `pnpm audit --prod` only, skipping the
  dev/build dependency chain the frontend job actually executes.
- `.github/workflows/ci.yml:126` runs `govulncheck@latest` and parses its
  human-readable output, so the gate is non-reproducible.
- `package.json:4` version is `0.0.0` (the app version is injected via ldflags).
- `src/CLAUDE.md:24,27` sidebar/settings lists omit Cash Book and Document
  Numbering; `api/CLAUDE.md` route notes omit every cash/numbering route.
- `api/router.go:717-720` comments that DATEV is "not implemented yet"
  immediately above its registered route.
- `cmd/seed-demo/README.md:218-219` references a CLAUDE.md cash-register note
  that does not exist.
- Root `CLAUDE.md:20` says React Router 7; `package.json` is on
  `react-router@8.3.1`.

---

## Watch list — suspicions, not confirmed

Recorded so a future audit doesn't re-derive them. Do not fix blind.

- **Cash Book local-vs-UTC day stamping** is only half-diagnosed here (as part of
  F100). A non-UTC org's cash sale is stored at local midnight but bucketed by
  UTC day; the full set of affected reports (loan status, register balance) was
  not enumerated.
- **`formatMoneyCents` vs the frontend for 3-decimal currencies** (TND): storage
  is 2-decimal cents while TND's real subunit is the millime
  (`db/CLAUDE.md`'s Tunisia note). F121 covers the digit-count mismatch; whether
  a 3-decimal org can round-trip its *own* configured precision was not tested.
- **`SeedDefaultUnitsOfMeasureForAllOrganizations` /
  `SeedDefaultPaymentTermsForAllOrganizations` run outside a transaction** — the
  2026-09-14 audit's watch list already notes the per-org `COUNT(*)` gate makes a
  partial seed permanent. Still true.
- **OIDC `ensureOIDC` caches the provider on first success**; whether a provider
  config change requires a restart was not examined beyond F107.
- **`OrganizationUsageCount`** (F125) — the reset itself is complete; only the
  warning undercounts. Confirm before treating any blast-radius claim as
  authoritative.

---

## Explicitly excluded — do not re-raise

Each of these was checked during this sweep and dismissed with a reason.

- **English-only server error text.** Standing product decision (F45) — root
  `CLAUDE.md` says do not re-flag.
- **The 40+ `react(set-state-in-effect)` oxlint warnings.** Repo-wide,
  pre-existing, not recent-change drift.
- **0% `api/` handler test coverage as a general claim.** Measured repo-wide
  norm across 17 handler files; only the narrow F102 point stands.
- **The Phase C authorization model itself.** The code is correct and
  test-enforced; the *documentation* is the defect (F132), not the code. No new
  cross-org write hole was found.
- **`PUT /api/organizations/{id}` being org-member- rather than org-admin-gated.**
  Documented precedent; `db/CLAUDE.md` explicitly blesses it. Only the stale
  "`protected`" wording is a finding (F135).
- **The platform-admin "zero org memberships after creation, no self-service
  grant" gap.** Documented as deliberate in `api/CLAUDE.md`. Distinct from F96,
  which is about `isPlatformAdmin` never being set at all on a fresh install.
- **JWT algorithm confusion / cookie flags / CSRF / SQL injection / XSS / path
  traversal / SSRF / Excel-formula injection.** All verified clean: `HS256`
  pinned, cookies `httpOnly`+`SameSite=Lax`+conditional `Secure`, every
  state-changing route behind `csrfRequired`, all SQL parameterized, zero
  `dangerouslySetInnerHTML`, export filenames sanitized, no request-controlled
  argv.
- **`go.mod` dependency vulnerabilities.** Only the CI-allowlisted Excelize
  advisory; the `x/crypto/ssh` advisories are in an uncalled package.
- **The route-authorization coverage tripwires.** `TestPhaseCRouteCoverage`,
  `TestCreateRouteOrgChecksArePresent` and `TestDomainRoleRouteCoverage` are live
  and passing; the only gaps found are intent-level (F104/F106), not enforcement
  bugs.
- **`organization_users` being excluded from `ResetOrganizationData`.** Access
  control, deliberately not reset — documented and tripwired.
- **Migration pairing / `0081`'s table rebuild / `0080`'s CHECKs.** All 82
  `.up`/`.down` pairs present and consistent; verified against the application
  validation.
- **`mass_data*.go`** — per-row commits with a structured error report is the
  documented design; no bypass of per-table validation found.
- **`journal_entries`/`payments` transaction boundaries under
  `SetMaxOpenConns(1)`.** No `d.DB` call between `Beginx()` and `Commit` in any
  new path; no deadlock found.

---

## Remediation phases

**Phase 1 — backend correctness, ledger integrity, authz (1.1–1.15).** F96 and
F97 first: both are full losses of function (admin control plane; correction of
posted entries). F98/F99/F103 are the F48 TOCTOU class and belong in one
coherent concurrency pass. F104/F106/F108 need explicit decisions recorded in
this document before code moves.

**Phase 2 — frontend reliability (2.1–2.10).** F111 is the data-loss finding and
should lead. F112/F113 are small, contained, and regress fixes already written
elsewhere in the repo (F86). F114/F115 reuse the error-state pattern that
already exists in-tree.

**Phase 3 — accounting correctness and performance (3.1–3.6).** F121 needs a
frontend+backend change together; F122 is a self-contained query rewrite.

**Phase 4 — i18n, accessibility, UI consistency (4.1–4.5).** F127 is one line
and the highest-leverage item here.

**Phase 5 — docs, CI, ops (5.1–5.8).** F132 (authorization docs) and F134/F135
(Cash Book/role docs) belong together: the corrections are all descriptive. F137
(Docker build gate) is independent and can ship with F138.

---

## Verification

Beyond the mandatory per-phase gates (`go vet ./... && go test -race ./...`,
`pnpm lint && pnpm build`):

- **F96** — a test that runs the real `db.NewDatabase` → `EnsureFirstAdmin` order
  against a temp DB and asserts the seeded admin has `isPlatformAdmin = 1` and
  can pass `platformAdmin`. Then in a fresh container: log in and confirm
  Settings → Users is reachable.
- **F97** — DB-layer test: post an entry, deactivate one of its accounts,
  assert reversal succeeds and `CloseFiscalYear` for a year touching that
  account succeeds. Assert the 409 path is gone.
- **F111** — force the failure live (stop the backend mid-drawer-open), then
  edit and save the product; assert the recipe is unchanged. Drive the real app
  (`go run .` + `pnpm dev`), per this repo's UI-fix convention.
- **F99, F100, F101, F102, F103, F121, F124, F126** — DB-layer tests. F100
  specifically needs a fixture whose creation timestamp differs from the query
  boundary; F99 needs two concurrent saves.
- **F104** — a route-role decision recorded in `api/CLAUDE.md` and (if gated)
  added to `domainRouteRoles`.
- **F112, F113, F114, F115, F116, F117** — drive the real app; for F112/F113
  force the error path and confirm the drawer/modal does not close.
- **F127** — switch the UI to de and fr and confirm
  `document.documentElement.lang` updates.
- **F132, F133, F134, F135, F136** — after editing, re-read the `CLAUDE.md`
  files against `api/router.go` and `db/` and confirm every claimed route gate
  and role matches code.
- **F137** — a PR that deliberately breaks the Dockerfile must fail CI before
  merge.
