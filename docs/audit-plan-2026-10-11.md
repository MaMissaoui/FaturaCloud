# FaturaCloud Re-Audit — Delta Sweep (#431 → v3.67.2 + #504–#506)

**This is a delta audit, not a full-repo sweep.** It covers everything that
landed on `main` between `dcb13e1` (#431, where the
[2026-09-29 delta](audit-plan-2026-09-29.md) stopped) and `015c042` (#506):
69 commits across 235 files, excluding catalogs and docs. Numbering
continues at **F162**; no document under `docs/` uses F162 or later.

What the delta contains:

- **The 2026-09-29 remediation** (#434–#436): organization time zones on
  every creation path, `calendarDayStart`, payment products restricted, lint
  at zero warnings.
- **Loan register migration** (#437–#442, migrations `0097`/`0098`): opening
  receivables (`db/opening_loan.go`) and the Excel import with undo
  (`db/loan_import.go`).
- **Cash Book** follow-ups (#444–#457, #467): the v2 layout, the loan
  register export, and the line allocation that leaves other lines alone
  (`allocateInvoiceLines`).
- **Dashboard redesign** (#462, #464, #469) and **master data summaries**
  (#476–#482, migration `0100`): client, vendor, product and BOM summaries,
  the outstanding-documents routes, the list redesigns.
- **Payments mark documents paid** (#484, migration `0101` backfill,
  `db/payment_state.go`).
- **Structured request logs and browser error reports** (#486), the
  **activity history** (#487, #489, #491, #495, #498, migrations
  `0102`/`0103`).
- **Test organization flag** (#473, migration `0099`), comma decimals (#460),
  logo `hasLogo` (#459), the sign-in user shape (#501), seed-demo coverage
  (#466, #471, #472), Go 1.26.9 (#493).
- **CI** (#504–#506): the api tests' golden database, per-commit concurrency
  on main, the docs-only skip, the release Dockerfile's version-free
  `package.json`.

**Baseline health** at `015c042`, measured before writing this document:

| Check                          | Result                                                                                                                       |
| ------------------------------ | ---------------------------------------------------------------------------------------------------------------------------- |
| `go vet ./...`                 | Pass                                                                                                                         |
| `gofmt -l .`                   | Pass                                                                                                                         |
| `go test -race -count=1 ./...` | Pass (all packages; api 26s, db 26s locally)                                                                                 |
| `pnpm lint` (tsc + oxlint)     | Pass, 0 warnings                                                                                                             |
| `pnpm test`                    | Pass (132/132, 22 files)                                                                                                     |
| `pnpm build`                   | Pass                                                                                                                         |
| `pnpm extract`                 | 0 missing in de/fr; the tree stays clean (de/fr carry one more obsolete `#~` entry than en, which is harmless)                |
| `pnpm audit --prod`            | No known vulnerabilities                                                                                                     |
| `govulncheck ./...`            | No called vulnerabilities (one module-level advisory in a package that isn't imported, as in the 2026-09-29 baseline)        |

**Production data check** (2026-10-11, run by the owner from a throwaway
read-only copy of the pimi-01 database, deleted afterwards): history rows a
create on an affected route wrote since the activity history shipped
(v3.65.0, 2026-10-06):

| Organization                           | Cash Book sales (`POST /api/cash-sales`) | Member adds |
| -------------------------------------- | ---------------------------------------- | ----------- |
| Société ELECTRO MISSAOUI               | 0                                        | 0           |
| Établissement Ben Salah Électroménager | 0                                        | 0           |

Atlas Moto has no history rows at all. F164 has therefore not reached any
production row yet, and migration `0104`'s repair is a no-op there; the fix
still matters because the first real counter sale would be recorded wrong.
The F163 count (open loans with a free line) was not taken.

**How this document was produced.** One sequential pass by the authoring
model, with no parallel sweeps. **#504–#506 were written and reviewed by the
same model in the same session**, so this is not an independent review of
them. Labels:

- **Confirmed** means reproduced: F163 with a throwaway counter sale through
  the router plus a sweep over the allocation function, F164 with throwaway
  router tests reading the history rows back.
- **Code read** means established by reading the code (F162, F165, F166).

Every probe was throwaway and has been deleted. No code was changed while
producing this document.

**Instructions for the executing model:**

- This document is an audit, not a remediation. Wait for the owner's go-ahead
  before starting any phase.
- Use one feature branch and PR per phase, and never push to `main`. Use
  conventional commits with no Claude attribution lines.
- After every phase, `go vet ./... && go test -race ./...` and
  `pnpm lint && pnpm build` must pass, and `pnpm extract` must report 0
  missing.
- F162 and F163 each need a product decision before any code is written (the
  options are under each finding).
- **Never touch Société ELECTRO MISSAOUI.**
- The 2026-09-19, 2026-09-24, v3.54 and 2026-09-29 "Explicitly excluded"
  lists still bind, and so does this document's own list at the end.

**Status:** 5 findings (F162–F166). The owner's decisions (2026-10-11):

- **F162:** option (a) — a document due today is not overdue, on every
  screen; the server buckets by calendar day in the organization's zone.
- **F163:** fix with zero weights for free lines, accepting the one-cent
  shift on existing loans that have a free line.
- **F164:** fix the history going forward **and repair the existing rows**
  (member adds exactly, cash sales by client and creation time).
- **Production data check:** approved; run by the owner (see the baseline note).

---

## Severity overview

| #    | Finding                                                                                                                                                            | Area                     | Severity | Phase |
| ---- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------ | ------------------------ | -------- | ----- |
| F164 | The activity history records the wrong document, with no label, for every Cash Book sale (the client) and every member added (the membership row)                  | Activity history         | Medium   | 1     |
| F162 | On a document's due date, the server and the master-data screens call it overdue while the invoice and bill lists don't, so two screens disagree about the same bill | Aging / consistency      | Low      | 2     |
| F163 | A free line on a Cash Book loan always sorts last and absorbs the rounding cents, so a free item shows cents owed and the real lines show cents short               | Cash Book / allocation   | Low      | 2     |
| F165 | The background loops (`runAuditPruning`, `sweepLoginBuckets`, `runScheduler`) have no test of their own; #489's "stop once closed" fix is untested                  | Tests                    | Low      | 3     |
| F166 | `db/dbtest` repeats `db/db_test.go`'s golden-database builder and its schema guard; the two copies can drift                                                        | Tests                    | Low      | 3     |

**Pending verification, no F-number:** #504's per-commit concurrency group
for pushes to `main` can only be observed when two merges land close
together; check that neither run is cancelled the next time a release PR
follows a fix.

---

## Phase 1

### 1.1 — F164: the history records the wrong document for Cash Book sales and member adds (Medium, Confirmed)

**What happens.** `api/audit.go` takes a created document's id from the
first `"id"` in the response (`createdIDPattern` over the first 2 KB,
`requestInfo.head`). `POST /api/cash-sales` answers with
`db.CashSaleResult`, whose fields are `client`, `invoice`, `payment` in that
order, so the first `"id"` is the **client's**. The history row then gets:

- `entityId` = the client's id, while `resource = "cash-sales"` maps to the
  `invoices` table (`auditDocuments` in `db/audit_event.go`);
- `entityLabel` = `""`, because no invoice has that id.

Reproduced with a throwaway router test (cash sale through `POST
/api/cash-sales`, then the newest `audit_events` row):
`invoice id ULNX…, client id h9lq…, history entityId h9lq… label ""`.

**Why it matters.** The Cash Book sale is the most frequent change in
production, and the history is the accountability record. Every sale since
the activity history shipped (#487, released v3.65.0, deployed 2026-10-06)
is recorded against the wrong document with no invoice number. Filtering the
history by the invoice (`entityId`) finds its later state changes and
payments but not the sale that created it.

**A second route has the same cause.** `POST
/api/organizations/{orgId}/members` answers with the `organization_users`
row, whose first `"id"` is the **membership's** id, while `resource =
"members"` maps to the `users` table (label: email). Reproduced the same way:
the row's `entityId` was the membership id and its label empty, so the
history can't say who was added.

Every other audited create route was checked. Each returns a document whose
first JSON field is its own `id` (payments, journal entries, every sales and
purchasing document, production orders, imports, the master data); a user
create returns a flat object; `cash-movements` (`cashMovement` first) and
`stock-movements` (`movements` first) pick the right object; the loan import
and the mass-data imports have no `"id"` key in their reports, and the loan
import sets its label explicitly. No test covers either affected row:
`api/audit_test.go` covers only `/api/cash-sales/{id}/payments`, whose id
comes from the path.

**Fix.** Let a handler name its created entity explicitly instead of relying
on response field order: a `setAuditEntity(r, id)` next to `setAuditLabel`,
which `api/audit.go` uses before falling back to the pattern. The cash-sale
handler passes `result.Invoice.ID`, the member add the added user's id. Add
a router test that creates through **every** audited `POST` create route and
asserts each row's `entityId` names the created document, so a future
wrapped response can't regress silently.

**Existing rows (owner decision).** Either leave them, or backfill with a
migration. A member-add row can be repaired exactly: its `entityId` is an
`organization_users.id`, which names the user. A cash-sale row can only be
matched by client and creation time (`requestId` isn't stored on the
invoice), which is reliable only when a client has one sale per second. The
production counts (`resource = 'cash-sales' AND route NOT LIKE
'%/payments'`, and `resource = 'members' AND method = 'POST'`) size this.

---

## Phase 2

### 2.1 — F162: "overdue" means two things on a document's due date (Low, Code read)

**What happens.** A due date picked in a DatePicker is stored as local
midnight. The server's `agingBucketFor` (`db/dashboard.go`, 2026-08-08)
treats a document as late as soon as `now > dueDate`, i.e. from the first
minute of its due date, in bucket `days1To30` with `daysOverdue = 0`. The
frontend's `daysLate` (`src/components/master-data/due-date.tsx`) documents
the opposite rule: "a document due today is not late yet — the aging
reports' rule".

The delta put both rules on screen side by side:

| Screen                                                     | Rule used                              | Due today?  |
| ---------------------------------------------------------- | -------------------------------------- | ----------- |
| Vendors list "Overdue" chip (`db/vendor_summary.go`)       | bucket ≠ `current`                     | Overdue     |
| Vendor/client summary panels, Dashboard owed panel, AR/AP aging | bucket                            | Overdue     |
| Incoming invoices list "Overdue" chip                      | `daysOverdue > 0`                      | Not overdue |
| Invoices list "Overdue" chip                               | `daysOverdue > 0`                      | Not overdue |

So on a bill's due date its vendor sits under Vendors ▸ Overdue while the
bill itself is absent from Incoming invoices ▸ Overdue. `daysOverdue` also
counts elapsed 24-hour periods, not calendar days in the organization's
zone, so it is off by one for an hour around a daylight-saving change.

**Options (owner decision).** (a) Due today is not late, everywhere: the
server buckets by calendar day in the organization's zone (`orgLocation`),
which moves due-today amounts from `days1To30` to `current` in the aging
reports and summaries. (b) Due today is late, everywhere: the lists'
chips use the bucket, and the `daysLate` comment is corrected. (a) matches
the usual meaning of a due date and the frontend's stated intent.

### 2.2 — F163: a free line on a loan absorbs the rounding cents (Low, Confirmed)

**What happens.** `allocateInvoiceLines` (`db/dashboard.go`) splits the
invoice total over its lines by net value with `mulDiv` (floor), and the
**last** line takes the remainder. Lines are ordered by
`loanLineOrder = "netLine DESC, lineId ASC"`, so a free line (net 0, given
weight 1) is **always** last. It then takes up to n−1 cents. Lines 100.00,
100.00 and a free gift come out as **99.99, 99.99, 0.02**: the gift owes
2 cents, and after the cashier settles both real lines' shown balances the
invoice still has 0.02 outstanding and doesn't turn paid until someone
"pays" the free item.

Reproduced end to end as well: a counter sale through `POST
/api/cash-sales` with lines 2 × 10.00 and 1 × 5.00 at 20% VAT plus a free
"Gift" line is accepted (201), and `GetLoanStatus` then lists the lines at
**23.99, 5.99 and 0.02** instead of 24.00, 6.00 and 0.00.

The rest of #467 held up under a throwaway sweep of 20,000 random invoices
(1–6 lines, free lines, tax, stamp-like extras, deposits, payments in random
order): **0 violations** of the invariants. Line balances always summed to
the invoice's, none went negative, and paying one line's shown balance never
moved another line.

The free-line case predates the delta (`d4ce460`, 2026-09-24) but lives in
the code #467 reworked. The production count of open loan invoices whose
last line in `loanLineOrder` has `netLine = 0` sizes it.

**Fix (owner decision on existing loans).** Give non-positive lines a
weight of **0** (falling back to equal weights only when every line is
free), and hand the remainder to the last line with a positive weight. The
cause is the free line's weight of 1 inflating the weight sum; merely moving
the remainder elsewhere would leave two identical lines 2 cents apart
(100.01 / 99.99). With zero weights the example comes out 100.00 / 100.00 /
0.00, and the same throwaway sweep over 20,000 random invoices found 0
invariant violations and no free line owing anything.

Because the allocation is computed on read, the fix moves a cent on existing
loans that have a free line: a real line already settled by direct payments
at 23.99 would then show 0.01 owed, and the free line 0. Invoice totals and
outstanding don't change. Pin the case with tests (the example above, and a
free line in an invoice with a deposit).

---

## Phase 3

### 3.1 — F165: the background loops have no test of their own (Low, Code read)

`runAuditPruning` (`api/audit.go`), `sweepLoginBuckets` (`api/auth.go`) and
`runScheduler`/`readBackupConfig` (`api/utility.go`) only act after a
`time.Sleep` of a minute or more. Their only coverage was incidental: the
api suite used to run for 3+ minutes, so their timers fired during it. Once
#504 cut the suite to ~25 s, `api` coverage dropped from 43.2% to 42.8%,
entirely in these four functions. `db.PruneAuditEvents` is tested at the db
level, but #489's fix (`runAuditPruning` returns once `db.IsClosed`) is in
the loop, and nothing exercises it.

**Fix.** Make the interval injectable (a package-level `var` or a parameter)
and add a test per loop: pruning stops once its database is closed, the
login sweep evicts expired buckets, the scheduler skips a disabled config
and backs up once per day.

### 3.2 — F166: two copies of the golden-database builder (Low, Code read)

`db/dbtest/dbtest.go` (#504) repeats `db/db_test.go`'s `goldenMigratedDB`
and `listSchemaTables` guard, because `db`'s own tests can't import a
package that imports `db`. A later change to one copy (for example a
stronger snapshot check) can silently miss the other.

**Fix.** Move the snapshot-and-verify step into one exported function in
`db` (non-test code, a few lines) that both helpers call, or have
`db_test.go` move to an external `db_test` package where it can import
`dbtest`. The first is smaller.

---

## Explicitly excluded: do not re-raise

Each was checked during this sweep and dismissed with a reason.

- **Day arithmetic in the new server code.** The loan staleness
  (`calendarDaysBetween`), the Dashboard till's today/yesterday, the loan
  import's cutover (`calendarDayStart`) and sheet dates all use the
  organization's zone. F162 is about the pre-existing aging rule, not these.
- **The new per-organization flags on creation paths.** `isTest` defaults
  off and `masterDataSummaries` on, both in the column default and in
  `CreateOrganization` (nil → default), so the drawer, `/organizations/new`,
  seed-demo and a plain API call all agree (the F156 lesson, checked).
- **Plain `InputNumber` after #460.** None left; every number field goes
  through `NumberInput`.
- **Opening loans meeting the payment-driven paid state.**
  `markPaidIfSettledTx`/`reopenIfUnsettledTx` are pure state updates with no
  posting, the same as `updateOpeningLoanState`, so no path posts revenue for
  an `origin = 'opening'` invoice. `0101`'s backfill uses the same
  non-voided-applications rule as the live code.
- **The loan import undo.** It re-checks "collected" inside its
  transaction, counts voided payments as collected (documented), reverses
  rather than deletes posted entries, and deletes a created customer only
  when none of `invoices`, `orders`, `outbound_deliveries`, `payments` or
  `journal_lines` references it, a superset of `clientReferencingTables`.
- **Activity history secrets.** Every audited column is classified in
  `auditColumnReview` (tripwired); password hashes and token versions are
  skipped, IBAN/RIB masked. Reads are org-admin / platform-admin only.
- **Request ids and browser error reports.** The request id is always
  generated server-side (a client-sent `X-Request-ID` is ignored), and the
  `client-errors` rate-limit map is swept.
- **Release PRs running the full CI twice.** Owner decision 2026-10-11:
  left as is.
- Everything the 2026-09-19, 2026-09-24, v3.54 and 2026-09-29 documents
  exclude.

## Not covered

Named so a later pass knows where to start:

- **Frontend screens in the delta** (Cash Book v2, the Dashboard redesign,
  the master data and BOM screens, the loan import screen) were not
  browser-checked in this pass; the Modal/Drawer-on-module-atom sweep was not
  run over them.
- **Performance on the Pi.** The Dashboard's loan follow-up and the master
  data summaries read whole histories per request; not measured against a
  production-sized database.
- **The activity history's before-snapshot under concurrency.** It is read
  before the handler runs, so a concurrent edit can be attributed to the
  wrong request. Not probed; a lesser issue than F164.
- **seed-demo** changes (#466, #471, #472) were skimmed only.
