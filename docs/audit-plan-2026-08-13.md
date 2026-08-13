# FaturaCloud Audit — Concurrency, Ledger Integrity, Frontend Reliability (2026-08-13)

Fresh audit at commit `8106e27` (`v3.7.3`), scoped to concurrency/ledger
integrity, accounting data-validation gaps, frontend reliability, and
performance/ops — no UI/perf sweep beyond what those areas surfaced. The
2026-08-09 plan (F42–F47) is **fully implemented**; F45 remains the standing
product decision (English-only server error text, accepted — do not re-flag).
This plan continues the numbering at **F48** and covers what changed since:
the SKR04/PCG country chart templates (PR landed in v3.7.0) and the dependency
bumps through v3.7.3.

**Instructions for the executing model:**
- Work phase by phase. One feature branch + PR per phase (never push to main
  directly). Conventional commits, no Claude attribution lines.
- After every phase: `go vet ./... && go test -race ./...` and
  `pnpm lint && pnpm build` must pass.
- Update CLAUDE.md sections whose documented behavior a task changes.
- This document is an audit, not a remediation — no code was changed while
  producing it.

**Status:** Phase 1 (F48–F50) fixed, PR open. Phase 2 (F51–F54) fixed, PR
open (#97 — F53 decided and pushed as a follow-up commit after the PR was
first opened; see the PR #97 comment for what changed and why). Phases 3–4
not started.
Triage notes from the executing session: F53 was initially left open as a
product decision (changing `PreviouslyInvoiced` to `approved`/`paid`-only),
then decided and fixed after the fact — see 2.3. F54's fix is scoped to the
rounding change only; the plan's suggested "sanity check that stored value
is within one cent" is skipped as speculative validation for a case the
rounding itself eliminates. F63 will be reduced to a documentation note
rather than a behavior change — refusing to auto-migrate a schema-mismatched
upload would break restoring a legitimate older backup, which the plan
itself already flags as "a footgun rather than a threat."

---

## Severity overview

| # | Finding | Area | Severity | Phase | Status |
|---|---------|------|----------|-------|--------|
| F48 | Double-posting class: `journal_entries(sourceDocumentType, sourceDocumentId)` has no unique index and the posted-entry lookup runs **outside** the transaction in every state-change path | Concurrency / Ledger | High | 1.1 | Fixed |
| F49 | `CloseFiscalYear` snapshots activity before the tx; the status flip lacks `WHERE status='open'` → two concurrent closes post two closing entries | Concurrency / Ledger | Medium | 1.2 | Fixed |
| F50 | `DeleteJournalEntry` deletes in a non-transactional statement without `AND status='draft'` → a concurrent post between read and delete removes a posted entry | Concurrency | Low | 1.3 | Fixed |
| F51 | `UpdateAccount` freely retypes accounts / toggles `isGroup` with posted history (retroactive reclassification); `parentId` not validated | Accounting integrity | Medium | 2.1 | Fixed |
| F52 | Overlapping fiscal years/periods allowed; `resolveFiscalPeriodForDate` resolves with `LIMIT 1` and no `ORDER BY` | Accounting integrity | Low | 2.2 | Fixed |
| F53 | Match report counts **draft** bills in `PreviouslyInvoiced` while GRNI clearing counts only `approved`/`paid` — the two disagree about what has "billed" | Accounting integrity | Low | 2.3 | Fixed |
| F54 | `int64(float64)` truncation of `unitPrice`/`unitCost` toward zero vs. decimal-rounded totals | Accounting integrity | Low | 2.4 | Fixed |
| F55 | Invoice PDF preview leaks a resize listener and an object URL per generation; leftover `console.log` debug output ships in prod | Frontend | Medium | 3.1 | Open |
| F56 | Product/tax-rate save failures close the form silently — setter swallows the error, `handleClose()`/`navigate()` still runs | Frontend | Medium | 3.2 | Open |
| F57 | Users page uses module-level Jotai atoms for drawer state — violates the documented rule that risks an orphaned-mask freeze | Frontend | Low | 3.3 | Open |
| F58 | No index on `journal_entries.fiscalYearId` — `CloseFiscalYear`, both GL exports, and report group-bys scan by it | Performance | Low | 4.1 | Open |
| F59 | `provisionOrSyncUser` holds `dbMu` write lock during bcrypt — every OIDC callback stalls all API traffic ~50–100 ms | Performance | Low | 4.2 | Open |
| F60 | Scheduler retries a failing backup every minute for the whole scheduled hour; `applyRetention` deletes stray files by modtime | Ops | Low | 4.3 | Open |
| F61 | `CreateInvoice` inserts client-supplied `req.ID` as-is — empty string becomes an empty-string PK | Ops | Low | 4.4 | Open |
| F62 | `SeedInventoryAccountingDefaultsForAllOrganizations` gate skips an org whose `defaultInventoryAccountId` is set manually but the other three are NULL | Ops | Low | 4.5 | Open |
| F63 | Restore path applies pending migrations to an arbitrary uploaded SQLite file that passes `integrity_check` + a `users` table | Security (admin-only) | Low | 4.6 | Open |
| F64 | `isHTTPS`/`requestIsHTTPS` trust `X-Forwarded-Proto` unconditionally — Secure cookies/HSTS on a misconfigured plain-HTTP deployment | Ops | Low | 4.7 | Open |
| F65 | OIDC never checks the `email_verified` claim before JIT-provisioning a user | Security | Low | 4.8 | Open |

---

## Phase 1 — Concurrency & ledger integrity (P1)

### 1.1 The double-posting class (F48) — High

`journal_entries` has no uniqueness constraint on
`(sourceDocumentType, sourceDocumentId)`, and every posting path looks up the
existing entry *before* its transaction begins. With `db.SetMaxOpenConns(1)`
(`db/db.go:36`) each statement is serialized, but read-modify-write flows are
not: two concurrent identical requests both pass the same check and each post
a full entry.

Concrete sites (all read before `Beginx()`, act inside it):

- `UpdateInvoiceState` (`db/invoice.go:430-449`) — `FindPostedEntryForSourceDocument` at
  line 430 runs before the tx at line 464. Two concurrent `draft→sent` calls
  each see "no posted entry", each post `postAutoEntryTx` → AR and revenue
  **double-counted**. Same shape in `UpdateIncomingInvoiceState`
  (`db/incoming_invoice.go`) for both the post and the reverse branch.
- `UpdateDeliveryStatus` (`db/delivery.go`) — the stock-availability check runs
  pre-tx; two concurrent `draft→shipped` calls can both pass it and insert
  double `"out"` movements + double COGS.
- `UpdateInboundDeliveryStatus` (`db/inbound_delivery.go`) — the GRNI-cleared /
  in-stock checks run pre-tx; a concurrent cancel can reverse movements and a
  GRNI entry another tx already reversed.
- `VoidPayment` (`db/payment.go`) — the original-payment read is pre-tx and
  `reverseEntryTx` never re-checks the original's status; a concurrent double
  void reverses the reversal.
- `CreatePayment` (`db/payment.go`) — the overpay check computes `remaining`
  pre-tx; two concurrent payments can over-settle an invoice.

**Fix direction:** the durable fix is one partial unique index
`CREATE UNIQUE INDEX ... ON journal_entries(sourceDocumentType, sourceDocumentId)
WHERE status='posted' AND reversalOfEntryId IS NULL` — `FindPostedEntryForSourceDocument`
already filters on exactly that predicate, so the index is a faithful encoding
of "the live entry for a document". That turns the double-post from a silent
corruption into a 500 on the loser. Second, move each state/status/existing-entry
re-check *inside* the existing tx (each path already has one) so the check and
the act share a connection. Both together, in that order. Note: the `CreatePayment`
overpay path needs a `SUM`-inside-tx re-check, not an index — don't force it
into the same shape.

**Fixed** (branch `audit/phase1-concurrency-ledger`): migration `0059` adds the
partial unique index. `findPostedEntryForSourceDocumentTx` (tx-capable twin of
`FindPostedEntryForSourceDocument`) is re-checked inside the transaction in
`UpdateInvoiceState`/`UpdateIncomingInvoiceState`; `UpdateDeliveryStatus`/
`UpdateInboundDeliveryStatus` re-read `status` via `tx` right after `Beginx()`
and abort if it no longer matches the pre-tx read their decisions were made
against; `reverseEntryTx`'s final `UPDATE` now carries
`WHERE status = 'posted'` with a `RowsAffected` check — the single choke point
for every reversal path, closing `VoidPayment`'s race directly; `CreatePayment`
re-verifies every application's remaining balance via a new
`getDocumentAmountPaidTx` SUM inside its own transaction, per the note above.
Regression tests in `db/concurrency_test.go` (`TestConcurrentUpdateInvoiceStateDoesNotDoublePost`,
`TestConcurrentVoidPaymentDoesNotDoubleReverse`, `TestConcurrentCreatePaymentDoesNotOverpay`)
were confirmed to reliably fail against the pre-fix code (verified by
temporarily reverting the fix and re-running) and pass after.

### 1.2 CloseFiscalYear snapshot-before-tx (F49)

`db/fiscal_year_closing.go:67-123`: `GetFiscalYear` (status check), the
revenue/expense snapshot `SELECT`, and the retained-earnings lookup all run
before `Beginx()` at line 123. A posting committed in that window is excluded
from the closing entry — its revenue/expense never rolls to retained earnings,
silently unbalancing the year. And the final `UPDATE fiscal_years SET status='closed'`
(line 143-145) has no `WHERE status='open'` re-check, so two concurrent closes
each post their own closing entry.

**Fix direction:** move the snapshot `SELECT` and the org/retained-earnings read
inside the tx (against `tx`, not `d.DB`), and add `AND status='open'` to the
flip `UPDATE` with a `RowsAffected` check. The `GetFiscalYear`/`GetOrganization`
prefix can stay outside — the tx-level re-check on the flip is what makes the
whole thing idempotent.

**Fixed**: the activity snapshot now runs against `tx`; the flip `UPDATE`
carries `WHERE status = 'open'` with a `RowsAffected` check. Regression test
`TestConcurrentCloseFiscalYearPostsExactlyOneClosingEntry` confirmed to fail
pre-fix (multiple closing entries posted) and pass post-fix.

### 1.3 DeleteJournalEntry status TOCTOU (F50)

`db/journal_entry.go:336-351`: reads status (`GetJournalEntry`), then issues a
plain `DELETE FROM journal_entries WHERE id = ?` with no `AND status='draft'`
and no transaction. A concurrent `PostJournalEntry` between the two statements
deletes a posted entry, violating "posted entries are never deleted" and FEC's
gapless `entryNumber` requirement (the freed number then gets reused).

**Fix direction:** `DELETE FROM journal_entries WHERE id = ? AND status = 'draft'`
and check `RowsAffected == 0` → return "only a draft can be deleted" 409.

**Fixed** as specified. Regression test `TestConcurrentDeleteJournalEntryDoesNotDeleteAPostedEntry`
added, but empirically does not reliably reproduce the race via plain
goroutine scheduling even pre-fix (50 runs, zero failures) — the vulnerable
window is a single-statement gap, too narrow for scheduling alone to hit
consistently. Kept as a regression guard for the invariant regardless; the
fix matches the same choke-point pattern proven to close the race in every
other Phase 1 test.

---

## Phase 2 — Accounting data-validation gaps (P2)

### 2.1 Account retyping with history (F51)

`UpdateAccount` (`db/account.go:133-153`) applies `type`/`isGroup` changes
unconditionally. Changing an account that has posted `journal_lines` from
`expense` to `revenue` — or making a leaf account a group header —
retroactively reclassifies every past posting in the balance sheet/P&L, and
groups are "never postable" only by enforcement at *post* time, not at *edit*
time. `DeleteAccount` guards with a usage count; `UpdateAccount` has no
equivalent. `parentId` is also inserted raw — a dangling parent is an opaque
FK 500, not a clean validation.

**Fix direction:** refuse `type`/`isGroup` changes on an account that has any
`journal_lines` (reuse `GetAccountUsageCount`, which already exists and is
schema-tested); validate `parentId` exists when non-null; consider refusing
`isGroup=1` while child accounts reference it.

**Fixed** (branch `audit/phase2-accounting-validation`): `UpdateAccount` now
refuses `type`/`isGroup` changes when `GetAccountUsageCount > 0`; refuses
`isGroup=0` while the account still has child accounts; validates a non-null
`parentId` exists and isn't the account's own id. Regression tests in
`db/account_test.go` (`TestUpdateAccountRejectsRetypeWithPostedHistory`,
`TestUpdateAccountAllowsRetypeWithoutHistory`,
`TestUpdateAccountRejectsMakingAnAccountWithChildrenALeaf`,
`TestUpdateAccountValidatesParentID`).

### 2.2 Overlapping fiscal years / periods (F52)

`CreateFiscalYear`/`CreateFiscalPeriod` (`db/fiscal_period.go:85-141`) validate
only `endDate > startDate`. Nothing stops a second fiscal year spanning the
same dates. `resolveFiscalPeriodForDate` (`db/fiscal_period.go:171-178`) then
resolves with `LIMIT 1` and no `ORDER BY` — with overlapping years, which
year a posting lands in is nondeterministic. Periods aren't checked to fall
inside their year either.

**Fix direction:** reject a new year/period whose range overlaps an existing
open one for the same org (a closed prior year legitimately overlaps the new
year's start — allow overlap only against closed years); add
`ORDER BY startDate DESC` (or an explicit "most recent open" preference) to the
resolution query so even a data anomaly resolves deterministically.

**Fixed**: `CreateFiscalYear` rejects overlap against another open year for
the org (closed years exempt); `CreateFiscalPeriod` requires its range inside
its year and rejects overlap against sibling periods; both
`resolveFiscalPeriodForDate` lookups gained `ORDER BY startDate DESC`.
Regression tests in `db/fiscal_period_test.go`
(`TestCreateFiscalYearRejectsOverlapWithOpenYear`,
`TestCreateFiscalYearAllowsOverlapWithClosedYear`,
`TestCreateFiscalPeriodRejectsRangeOutsideItsYear`,
`TestCreateFiscalPeriodRejectsOverlapWithSiblingPeriod`,
`TestResolveFiscalPeriodForDateIsDeterministicUnderOverlap` — the last one
seeds two overlapping open years directly, bypassing the new guard, to prove
the `ORDER BY` still resolves a pre-existing anomaly deterministically).

### 2.3 Match report vs. GRNI clearing disagree on "already billed" (F53)

`db/incoming_invoice_match.go:126` counts `PreviouslyInvoiced` as `state != 'cancelled'`
— i.e. **drafts included** — while GRNI clearing (`db/gl_posting.go:640`)
counts only `state IN ('approved','paid')`. Two consequences: a draft bill's
quantities can push a *second* bill into `over_received`/`quantity_variance`
and block its `approved` transition (drafts are savable, so this is reachable),
and the match panel's `PreviouslyInvoiced` number can disagree with what the
GL actually netted when a linked bill is still a draft.

**Fix direction:** align both to one definition. The GRNI side is the accounting
truth (a draft has no AP obligation yet) — make the match report's
`PreviouslyInvoiced` count only `approved`/`paid` too, and document the choice
in `db/incoming_invoice_match.go`'s top comment alongside the existing "computed
on read" note. Check `incoming_invoice_match_test.go` for coverage of the
draft-bill case before changing it.

**Fixed, after a deliberate product decision.** Initially left open in this
session because it changes a number visible in the match panel. Decided
2026-08-13: aligned `PreviouslyInvoiced` to `approved`/`paid` only, matching
`grniClearedQtyForPOLine`'s existing definition. The double-billing guard
this exists for still holds where it actually matters — matching is
computed on read, so the draft→approved transition re-evaluates it, and a
second bill still can't reach `approved` while a first one already has for
the same goods (`TestIncomingInvoiceDoubleBillingDetected` covers this and
was unaffected by the change, since its first invoice is approved before
the second is checked). What changes is that a still-draft bill — which has
no AP obligation and might be edited or deleted before ever being approved
— no longer produces a spurious variance against a sibling bill. New
regression test:
`TestIncomingInvoiceMatchIgnoresDraftBillsInPreviouslyInvoiced`
(`db/db_test.go`), confirmed to fail pre-fix (reported
`PreviouslyInvoiced=10` from a draft) and pass post-fix. See
`db/incoming_invoice_match.go` and CLAUDE.md's 3-way matching bullet.

### 2.4 Float64→int64 truncation on line-item prices (F54)

`db/incoming_invoice.go:575`, `db/inbound_delivery.go:763`, `db/order.go:209,282`,
`db/purchase_order.go:299` store `int64(item.UnitPrice)` / `int64(*item.UnitCost)`
— truncation toward zero. The frontend normally sends whole cents (rounded via
`unitsToCents`), so this is latent, not active: a fractional-cent value arriving
through JSON (e.g. a unit price derived from a division that lands at `x.9` after
float round-trip) stores a value that disagrees with `validateInvoiceTotals`'s
decimal-rounded totals by up to a cent per line. `db/invoice.go:74` has the same
float64 field for the sales path — verify whether its insert rounds or truncates.

**Fix direction:** round with the same `ROUND_HALF_UP` semantics the totals use
(e.g. `int64(math.Round(x))`, or better, `decimal` → int64 at the API boundary
in one place per domain), and add a tiny sanity check that stored `unitPrice`
is within one cent of the requested float.

**Fixed**, scoped to the rounding change only. `db/invoice.go:74`'s sales path
was checked: it passed the raw `float64` straight into the `INSERT` with no
Go-level conversion at all, relying on SQLite's own INTEGER-affinity
coercion — which, per SQLite's documented behavior, only converts a REAL to
INTEGER when the conversion is lossless, and stores it as REAL otherwise.
That's a *different* bug from truncation (it doesn't round or truncate at
all, it just stores the fractional value in principle), so it got the same
fix as the other four sites: a new `roundCents` helper (`db/invoice_totals.go`)
used everywhere a `float64` cents value reaches an `INSERT`. The plan's
suggested "sanity check that stored value is within one cent of the
requested float" was **not** added — that would be validating a scenario the
rounding itself eliminates by construction, against CLAUDE.md's rule against
speculative validation for something that can't happen. Regression tests:
`TestRoundCents` (unit-level, `db/invoice_totals_test.go`) and
`TestIncomingInvoiceLineItemRoundsFractionalUnitPrice` (integration-level,
confirms the stored value rounds rather than truncates end-to-end).

---

## Phase 3 — Frontend reliability (P2)

### 3.1 Invoice PDF preview resource leaks + debug output (F55)

`src/routes/invoices/details.tsx`:

- **Resize listener leak** (lines 120-149): the callback ref stores the cleanup
  on `(node as any)._cleanup`, but the unmount branch reads
  `containerRef._cleanup` — the ref **callback function** itself, which never
  receives the property. `window.removeEventListener("resize", ...)` never runs;
  every preview mount/unmount leaks a listener + closure.
- **Object URL leak** (lines 186-218): the `useEffect` with `[]` deps closes over
  the initial `pdfUrl === null`, so the cleanup's `if (pdfUrl)` is always false.
  No object URL is ever revoked — each generated preview leaks one, and they
  accumulate across generations within the same page session too.
- **Debug output** (lines 154, 168-175): `console.log("Sidebar state changed:",
  siderCollapsed)` and the "Sidebar change measurements" block ship in the
  production bundle. Line 270 also has dead state: `const [, setSubmitting] =
  useState(false);`.

**Fix direction:** replace the callback-ref cleanup with a proper
`useEffect(() => { ... addEventListener ...; return () => removeEventListener }, [])`
bound to a stable node via the ref; generate the PDF in a `useEffect` keyed on
the inputs with cleanup that revokes the *current* `pdfUrl` (track it in a ref or
revoke the previous URL before each new `setPdfUrl`); delete the two `console.log`
blocks and the dead state.

### 3.2 Failed product/tax-rate saves close the form silently (F56)

`src/components/products/form.tsx:129-141` and `src/components/tax-rates/form.tsx:83-90`
do `await setProduct(...)` / `await setTaxRate(...)` and then unconditionally
`handleClose()` / `navigate("/settings/tax-rates")`. The atoms' setters
(`src/atoms/product.ts:45-72`, `tax-rate.ts` — same shape) catch every error,
toast it via `message.error`, and **never rethrow**. Result: on a failed save
(duplicate SKU, FK violation, server 500), the drawer closes and the user's
input is silently lost after a transient toast.

**Fix direction:** make these two setters rethrow after toasting (the
account/journal-entry setters already set that precedent — `src/atoms/account.ts:62-66`
pairs rethrow with try/catch in the form), or check a returned success boolean
before closing. Whichever is chosen, verify the two forms actually stay open on
failure and keep the toast.

### 3.3 Users page module-level Jotai drawer atoms (F57)

`src/routes/settings/users.tsx:35-37` declares `searchAtom`, `drawerOpenAtom`,
`editingIdAtom` at module scope and drives the Drawer's open state with them —
the exact pattern CLAUDE.md forbids ("never use Jotai module-level atoms for
local UI state inside Modal or Drawer forms — the mask gets orphaned and freezes
the UI"). Same module-scope search atoms exist on the list pages (invoices,
clients, vendors, orders, deliveries, purchase-orders, inbound-deliveries,
incoming-invoices, countries, chart-of-accounts, trial-balance) — those persist
filter state across navigation, which is a smell but not the freeze risk; the
**drawer** one is the actionable item.

**Fix direction:** convert the three users-page atoms to `useState`/`useRef`.
The list-page search atoms can stay if intentional, but consider scoping them to
the route component for consistency.

---

## Phase 4 — Performance & ops (P3)

### 4.1 Missing index on `journal_entries.fiscalYearId` (F58)

Migration 0052 created `journal_entries` without an index on `fiscalYearId`.
`CloseFiscalYear`, both GL exports (`export_fec.go`, `export_datev.go`), and the
report group-bys all filter by it — on a large ledger each is a full scan of a
table that is append-mostly and never compacted. One
`CREATE INDEX ... ON journal_entries(fiscalYearId)` in a new migration.

### 4.2 OIDC JIT-provision holds the DB write lock during bcrypt (F59)

`api/users.go:284-301` (`provisionOrSyncUser`): `h.dbMu.Lock()` is held across a
`bcrypt.GenerateFromPassword` for newly provisioned OIDC users (~50-100 ms at
DefaultCost). Since `dbMu` is a global RWMutex guarding every `h.db` access in
`withDB`-wrapped handlers, every OIDC first-login stalls **all** API traffic.
Non-OIDC deployments are unaffected (feature disabled).

**Fix direction:** hash the password *before* taking the lock (it doesn't touch
the DB), or scope the lock to the minimal read/upsert window around it.

### 4.3 Scheduler retry / retention behavior (F60)

`api/utility.go`:
- `runScheduler` (288-320): the "already backed up today" gate is the backup
  file's existence, so if a backup fails (disk full, transient IO), the loop
  retries every minute for the entire scheduled hour — 60 failed `VACUUM INTO`
  attempts, each potentially competing with live reads.
- `applyRetention` (322-343): deletes **any** file in `backupDir` older than the
  cutoff by modtime, regardless of name — a stray log or editor artifact left in
  the directory is silently destroyed.

**Fix direction:** gate on a "succeeded today" in-memory marker (reset at the
next midnight/hour boundary) rather than file existence, and have retention
match only the `fatura-*` prefix it owns.

### 4.4 `CreateInvoice` accepts client-supplied empty ID (F61)

`db/invoice.go` inserts `req.ID` without the `if req.ID == "" { req.ID, _ =
gonanoid.New() }` guard every other create path has. A client sending
`{"id": ""}` creates an invoice with an empty-string primary key; a second such
request 500s on PK collision. Low impact (frontend always sends a nanoid) but a
one-line divergence from the established convention.

### 4.5 Inventory-defaults backfill gate gap (F62)

`db/account.go:581-592`: `SeedInventoryAccountingDefaultsForAllOrganizations` is
gated only on `defaultInventoryAccountId IS NULL`. An org that set that one
column manually (e.g. via the accounting card) while leaving the other three NULL
is skipped forever. Given the seeding populates all four together, gate on *any*
of the four being NULL, or on all four.

### 4.6 Restore applies migrations to uploaded DBs (F63)

`api/utility.go` `swapDatabase` calls `db.NewDatabase(h.dbPath)` (line 251),
which runs every pending migration against the restored file. For a legitimate
backup that's correct (it came from this app); but the **upload** path
(`restoreDatabase`) only validates `integrity_check` + a `users` table before
executing our own migration SQL against an arbitrary file. Admin-only, so this
is a footgun rather than a threat — but an admin restoring a database from an
older *schema* version (or a file that merely *looks* like one) gets it migrated
without consent. Consider recording the schema version (e.g. in
`schema_migrations`) and refusing to auto-migrate an uploaded file whose version
differs from the running one.

### 4.7 `X-Forwarded-Proto` trust for Secure/HSTS (F64)

`api/oidc.go:315-319` (`isHTTPS`) and `main.go:168-173` (`requestIsHTTPS`) treat
an `X-Forwarded-Proto: https` header as authoritative. A misconfigured proxy
(or any peer allowed to set the header when the app is directly exposed) makes
the server set `Secure` cookies and HSTS on a plain-HTTP deployment, locking
clients out of their session. Not remotely exploitable (browsers don't send the
header unprompted) but worth the same treatment as `X-Forwarded-For`: only honor
it from a `TRUSTED_PROXIES` peer. At minimum, document the constraint in the
env-var section.

### 4.8 OIDC `email_verified` not checked (F65)

`api/oidc.go` verifies signature/issuer/audience/nonce but never inspects the
`email_verified` claim before JIT-provisioning a user with the claimed email.
Providers configured with permissive email-verification policies (or a
misconfigured IdP) could hand this app an unverified address. A claim to check
when present (`verified == true`, skipping only when the claim is absent, since
Authelia doesn't always emit it). Low severity — the provider is operator-chosen
and trusted.

---

## Explicitly verified and OK (don't "fix")

Re-checked at `8106e27`; no findings:

- **Tests, lint, build all green** — `go vet ./...`, `go test -race ./...`,
  `pnpm lint`, `pnpm build` clean at this commit.
- **Supply chain** — `govulncheck ./...`: 0 vulnerabilities (1 unreachable
  advisory in a required-but-uncalled module); `pnpm audit --prod
  --audit-level high`: clean; `pnpm outdated`: nothing outstanding — the two
  previously-held majors (`@babel/core` 7→8, `pdfjs-dist` 5→6) have since
  landed (`@babel/core` is on 8, `react-pdf` is on 10 with `pdfjs-dist` 6.2).
  CI actions remain SHA-pinned with version comments (F43 holds).
- **SQL injection** — every query bound-parameterized; the only dynamic SQL
  (`GetTrialBalance`/`getAccountActivity` in `db/gl_reports.go`) is built from
  internal constants + `?` placeholders.
- **dbMu discipline** — `withDB` RLock on every protected handler; restore
  routes correctly take the write lock themselves; `authMiddleware` acquires
  the RLock *before* the handler, so no deadlock/nesting; no nil-`h.db` race
  in the scheduler.
- **Restore safety** — `integrity_check` + `users`-table validation, pre-restore
  safety backup, `recoverFromSafety` rollback with a hard stop if both fail.
- **Backups** — `VACUUM INTO` + `chmod 0600`; consistent single-file snapshot.
- **Auth** — HS256 allowlist, `iss`/`aud` binding, per-request active-user
  re-check, IP+email rate limiting (trusted-proxy-aware), decoy bcrypt against
  email-enumeration timing, CSRF header + SameSite=Lax on login/logout too,
  HSTS conditional on HTTPS. OIDC state/nonce/PKCE all in an HMAC-signed,
  HttpOnly, single-use cookie; signature/issuer/audience/nonce all verified.
- **Arithmetic** — every financial computation (`validateInvoiceTotals`, GL line
  building, GRNI/COGS, average-cost replay, FX conversion) uses `math/big`
  rationals; no float drift in the paths that matter.
- **Exporters** — single-query + in-memory grouping, no N+1; FEC/DATEV column
  layout and sanitization hold.
- **Frontend hygiene** — zero `dangerouslySetInnerHTML`; no sensitive data in
  localStorage (httpOnly cookie auth only); every `src/api/index.ts` function
  matches a real route in `api/router.go`; `pnpm extract` produces only
  line-reference churn in the `.po` files, no missing/added messages; de/fr
  catalogs 832/832, 0 missing, 0 fuzzy.
- **No cross-org authorization is a deliberate whole-app property**, not a
  regression — re-confirmed, still documented for operators, not re-filed.
- `dist/` and `node_modules/` are gitignored and untracked; source maps never
  ship in the artifact (F21 holds).

## Verification checklist (after any fix phase)

1. `go vet ./... && go test -race ./...` and `pnpm lint && pnpm build` — clean.
2. `govulncheck ./...` and `pnpm audit --prod --audit-level high` — clean.
3. `pnpm extract` — no content diff (line-reference churn only);
   `de`/`fr` catalog stats — 0 missing, 0 fuzzy.
4. Manual: for F55, open an invoice PDF preview, navigate away and back a few
   times, confirm no listener/URL growth (or just that the fix compiles and the
   preview still resizes with the sider); for F56, attempt a product save with a
   duplicate SKU and confirm the drawer stays open.
5. For F48/F49 specifically: a concurrency test (two goroutines issuing the same
   state change / close simultaneously) must leave exactly one posted entry /
   one closing entry. `TestJournalEntryConcurrentPost`-style coverage belongs in
   the same PR as the fix.
