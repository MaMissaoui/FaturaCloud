# FaturaCloud Security Audit (2026-10-23)

**This is a security-only audit of `main` at `6241535` (v3.71.1).** It looks
at authorization, sessions, injection, uploads, data exposure, denial of
service, and the Docker and CI setup. It does not cover UI, performance or
clean code. Numbering continues at **F190**; no document under `docs/` uses
F190 or later.

**How this document was produced.**

- A read-only code sweep by OpenCode (`opencode-go/deepseek-v4.1-flash`,
  plan agent, no edits) against a brief listing ten security areas, the
  binding exclusion lists and the product decisions.
- A separate pass by the authoring model over the same areas.
- A second, independent OpenCode sweep (`opencode-go/kimi-k2.7-code`, same
  brief, run on `main` without this document). It reported seven items.
  One is F192, found independently. The other six were dismissed (see
  "Explicitly excluded") or are already on the watch list. It did not find
  F190, F191, F193 or F194.
- Every finding below was checked against the code by the authoring model.
  Of OpenCode's eight, three were dismissed or moved to the watch list on
  evidence (`TRUSTED_PROXIES`, API `Cache-Control`, the template vector; see
  "Explicitly excluded"), and two were downgraded (F191 from High, F193 from
  Medium).
- Probes, all throwaway:
  - a `db` test (deleted afterwards, never committed) reproducing F190,
    F191, F193 and F196 on a scratch database;
  - the released `ghcr.io/mamissaoui/fatura-cloud:3.70.0` image, used to
    convert a crafted workbook with the same `soffice` arguments as
    `db/pdf_convert.go` (formula vector) and to start the server on a
    scratch `/data` volume (F195).
- homelab-deploy's fatura-cloud templates were read for `TRUSTED_PROXIES`.
  Only key names were read, never values.

Production was not read. No code was changed.

**Status (2026-10-24):** Phase 1 (F190, F191's first step, F193, F196) merged in #546. F192 and F195 fixed in the Phase 2–3 PR. Open: F191's second step (needs a production query first), F194's rule (owner decision). F196's same-organization case: the API now ignores a client-sent movement id.

Labels:

- **Confirmed** means reproduced by a probe (named under each finding).
- **Code read** means established by reading the code.

**Instructions for the executing model:**

- This document is an audit, not a remediation. Wait for the owner's
  go-ahead before starting any phase.
- Use one feature branch and PR per phase, and never push to `main`. Use
  conventional commits with no Claude attribution lines.
- After every phase, `go vet ./... && go test -race ./...` and
  `pnpm lint && pnpm build` must pass, and `pnpm extract` must report 0
  missing.
- **Never touch Société ELECTRO MISSAOUI.**
- The 2026-09-19, 2026-09-24, v3.54, 2026-09-29, 2026-10-11 and 2026-10-17
  "Explicitly excluded" lists still bind, and so does this document's own
  list at the end.

---

## Severity overview

| #    | Finding                                                                                  | Area               | Severity | Label     | Phase |
| ---- | ---------------------------------------------------------------------------------------- | ------------------ | -------- | --------- | ----- |
| F190 | One Daily Cash Movements request with a wide date range exhausts the server's memory     | Denial of service  | High     | Confirmed | 1     |
| F191 | A manual journal entry can claim any document and stop its posting, in any organization  | Ledger integrity   | Medium   | Confirmed | 1     |
| F192 | PDF exports start any number of LibreOffice processes at once                            | Denial of service  | Medium   | Code read | 2     |
| F193 | The organization's default GL accounts accept another organization's account             | Cross-organization | Low      | Confirmed | 1     |
| F194 | Anyone who knows an email can keep that account locked out of password sign-in           | Authentication     | Low      | Code read | 2     |
| F195 | The live database is created world-readable                                              | Ops                | Low      | Confirmed | 3     |
| F196 | A stock movement named after another organization's sale moves that organization's stock | Cross-organization | Medium   | Confirmed | 1     |

No earlier finding was found to have regressed. OpenCode's sweep checked
F167, F168, F169, F170, F173, F174, F175, F178, F179, F181 and F182 against
the code; the authoring model spot-checked F167 (`productReferencingTables`
guard), F169 and F174 (migrations `0106`–`0108`), F173 (`openXLSX` limits)
and F175 (renewal constants) and found them in place. The others rest on
OpenCode's read.

---

## Phase 1 — ledger, cross-organization and the report span

### 1.1 — F190: one report request exhausts the server's memory (High, Confirmed)

**What happens.** `GET /api/organizations/{orgId}/reports/daily-cash-movements`
(`api/router.go:773`, membership-level; the `report-cashbook` section admits
`general`, `accounting` and `cashbook`, `api/sections.go:76`) reads
`startDate` and `endDate` with `parseInt64Param` (`api/helpers.go:108`),
which accepts any int64. `GetDailyCashMovements` (`db/gl_reports.go:409`)
checks only that the end isn't before the start. `dailyCashMovements` then
appends one row per calendar day between them (`db/gl_reports.go:466`), and
the handler encodes the whole slice as JSON. Nothing caps the span.

**Probe.** On a scratch database, a 1,000-year span built 365,244 rows and
allocated 88 MB in 29 ms. A 10,000-year span built 3,652,426 rows and
allocated **1,040 MB** in 0.3 s, before JSON encoding. Allocation grows
linearly, so a 100,000-year span needs about 10 GB.

**Attack.** A `cashbook` member picks one of their organization's cash
accounts (the account list is membership-level) and sends one request with
`startDate=0` and an `endDate` a few hundred thousand years out. The
container is OOM-killed, every organization on the instance loses in-flight
work, and the request can be repeated after the restart. `WriteTimeout`
(300 s) doesn't help: it bounds the socket, not the handler's allocation.

The Dashboard's till panel calls the same function with a fixed two-day
range (`db/dashboard_panels.go:110`), so it isn't affected.

**Fix.** Refuse a span over a fixed limit (for example 366 days) with a
409 in `GetDailyCashMovements`, before any allocation. The Cash Book screen
asks for a day or a short range, so a year is generous. Check the other
`startDate`/`endDate` reports for the same shape: this is the only per-day
loop over a request range in `db/`.

### 1.2 — F191: a manual journal entry can claim any document (Medium, Confirmed)

**What happens.** `CreateJournalEntryRequest` carries `sourceDocumentType`
and `sourceDocumentId` (`db/journal_entry.go:77-78`). `createJournalEntry`
(`api/journal_entries.go:66`) decodes the body straight into it, and
`CreateJournalEntry` stores both fields unchecked
(`db/journal_entry.go:303-311`). No caller needs them: the frontend never
sends them, and every automatic posting goes through `postAutoEntryTx`
instead.

Posting is skipped for any document that already has a live posted entry.
`findPostedEntryForSourceDocumentTx` (`db/journal_entry.go:214`) looks the
document up by type and id **across every organization**, and migration
`0059`'s unique index has no `organizationId` either. `UpdateInvoiceState`
(`db/invoice.go:703-725`) posts only when that lookup finds nothing, and
reverses whatever it finds when the document leaves a posted state.

**Probe.** Two organizations on a scratch database:

- **Same organization:** an `accounting` member of A creates and posts a
  1-cent entry naming A's own draft invoice. When the invoice is sent, its
  real entry (AR, revenue and tax) is never posted. The invoice shows as
  sent with only the claim's two lines behind it.
- **Across organizations:** A's member claims B's draft invoice the same
  way. When B sends it, B's ledger gets **0** lines for it. When B moves it
  back to draft, the server reverses **A's** entry, in A's ledger: B's
  action writes into A's books.

**Attack.** The same-organization case needs no foreign id: an `accounting`
member can make any of their organization's sales or purchases disappear
from the ledger while the document itself looks normal. No screen offers
this, so it bypasses the "posted entries are reversed, never edited"
discipline without leaving an obvious trace. The cross-organization case
needs the victim document's 21-character id. That id can't be guessed, but
a former member may still have it. The same applies to every source type
(`incoming_invoice`, `invoice_cogs`, `production_order`, …), because the
type is a free string.

**Fix.** Stop accepting `sourceDocumentType`/`sourceDocumentId` on manual
entries (tag them `json:"-"` on the request, or clear them in the handler).
Then add `organizationId` to `findPostedEntryForSourceDocumentTx` and to
`0059`'s index in a new migration, after a production query confirms no
entry already names a document of another organization (Phase 0 style).
`CreateJournalEntryRequest` is the only request type carrying these fields,
and journal entries have no update route, so the first step closes this
entry point. It is not the only client-chosen `sourceDocumentId`: a manual
stock movement's is its request `id` (F196).

### 1.3 — F193: the default GL accounts accept another organization's account (Low, Confirmed)

**What happens.** `UpdateOrganization` builds the SET clause for the 16
`default*AccountId` columns straight from the request
(`db/organization.go:582-614`). The foreign key checks only that the account
exists. Every other account reference in `db/` goes through
`requireSameOrg`: products (`db/product.go:292`), tax rates
(`db/tax_rate.go:124`), payments (`db/payment.go:368`), cash movements
(`db/cash_movement.go:107`) and journal lines. `allocateAndFinalizeEntryTx`
doesn't check either.

**Probe.** Setting A's `defaultArAccountId` to B's AR account was accepted.
After A sent an invoice, `GetAccountUsageCount` on B's account returned 3
(`db/account.go:235` counts `journal_lines` with no organization filter). So
B can no longer delete or retype its own account.

**Impact.** It needs a foreign account id, which can't be guessed. The GL
reports filter by the entry's organization (`db/gl_reports.go:362`), so B's
reports don't show A's lines. The harm is a locked account in B, A's
postings landing on an account that isn't A's, and B's account label shown
in A's reports and activity history (`db/audit_changes.go:72`).
`PUT /api/organizations/{id}` itself stays membership-level, by the
2026-09-19 decision. Only the missing ownership check is the finding.

**Fix.** For each non-empty value, `GetAccount` it and call `requireSameOrg`
before building the SET clause, outside any transaction. Extend
`TestCreateAccountRejectsCrossOrgReferences`'s family in
`db/org_ownership_test.go`.

### 1.4 — F196: a stock movement named after another organization's sale (Medium, Confirmed)

Found while checking whether F191's first step closes every path. Added
after the rest of this document.

**What happens.** `POST /api/stock-movements` (`api/router.go:593`, any
member through `requireOrgMember`) accepts an `id` in the body.
`CreateStockMovement` uses it as the batch's `sourceDocumentId`
(`db/stock.go:263`), and for a non-serialized product also as the row's
primary key. Invoice ids live in another table, so nothing stops it from
equalling a Cash Book sale's id. A sale's posted stock is then summed by
`sourceDocumentId` alone (`postedInvoiceStockTx`, `db/invoice_stock.go:141`),
with no organization filter.

**Probe.** Org B sells 2 washers at the counter. A member of org A creates a
manual "in" movement of 7 of A's own washers with `id` set to B's sale id.
The server then reports B's sale as standing at −2 of B's washer and +7 of
A's. When B cancels the sale, it puts back exactly what it thinks the sale
moved: +2 of B's washer and **−7 of A's washer, as a movement inside org
B**. A's stock drops from 12 to 5 through B's action.

**Attack.** It needs B's sale id (21 characters, not guessable), the same
precondition as F191's cross-organization case. The impact is a write into
another organization's stock and a foreign product's movement in the
victim's books. In the same organization, a member can also attach a
manual movement to their own sale, which the sale's cancellation then
reverses, and `DeleteStockMovement` refuses to delete
(`db/stock.go:460`).

**Fix.** Scope `postedInvoiceStockTx` to the invoice's organization. For the
same-organization case, refuse a client-chosen movement id that equals an
existing document's id, or ignore client ids for stock movements (an owner
decision: `Create*` taking a client id is a repo-wide convention).

**Other consumers of `sourceDocumentId`, checked.** The product summary's
joins (`db/product_summary.go:200-204`) already match on the movement's
organization, and the serialized shipment, receipt and production lookups
(`db/delivery.go:630`, `db/inbound_delivery.go:722`,
`db/production_order.go:642`) pin a `productId`, which is
organization-specific. Left as they are: `DeleteStockMovement`'s
cash-sale check (`db/stock.go:460`, which only blocks the planting
organization's own movement) and the global `stock_movement` GL lookup
(`db/stock.go:445`), which F191's second step scopes.

---

## Phase 2 — resource limits

### 2.1 — F192: no limit on concurrent LibreOffice conversions (Medium, Code read)

**What happens.** Every PDF export calls `ConvertXLSXToPDF`
(`db/pdf_convert.go:35`), which starts its own `soffice` process with its
own profile, for up to 60 s. It has 7 callers (the six document exports in
`api/document_templates.go` and the report exports in
`api/report_export.go`) and no semaphore, pool or per-user limit anywhere. The export routes sit outside `withDB` on
purpose, so nothing else serializes them. This was the 2026-10-17 watch
list's "Excel-template exports at volume" entry. It is a finding now
because any member can reach it. Both OpenCode sweeps raised it
independently (the second rated it High; Medium here until the Pi's
limit is measured).

**Attack.** A member with any export route (for example `sales` on invoice
PDFs, `cashbook` on the daily movement export) fires a few dozen PDF exports
at once. Each `soffice` takes hundreds of MB, so the Raspberry Pi runs out
of memory and CPU for every organization.

**Fix.** A buffered-channel semaphore around `ConvertXLSXToPDF` (1–2 slots
on the Pi, configurable). A request that can't get a slot within a short
wait gets a 503 with a retry message, rather than queueing unboundedly.

**Not measured:** how many conversions the Pi survives.

### 2.2 — F194: anyone can lock an account out of password sign-in (Low, Code read)

**What happens.** Login is limited per IP and per email, 10 attempts a
minute each (`api/auth.go:25-26`). The per-email bucket is consumed before
the password is checked (`api/auth.go:234`), whatever the source IP. So 10
bad attempts a minute for a victim's email, from a single IP, keep the
victim's own correct sign-ins at 429.

**Impact.** Limited where users sign in through SSO, which doesn't use this
path. The 2026-09-19 audit (F110) relied on the per-email bucket against
password guessing, so dropping it isn't the answer. No earlier document
weighs the lockout side of it.

**Fix (options for the owner).** Count only failures in the per-email
bucket, and let a correct password through for an IP that hasn't itself
exceeded the IP bucket. Or keep it as is and accept the trade-off; if so,
add it to the exclusions.

---

## Phase 3 — ops

### 3.1 — F195: the live database is world-readable (Low, Confirmed)

**What happens.** SQLite creates `sqlite.db`, `-wal` and `-shm` with the
process umask (`0022` in the image). Only backups are restricted
(`BackupFileMode` 0640, `db/db.go:110`; the backups directory is 0750).
F15 (2026-07-12) fixed the same problem for backups and the config file
only; the live database was not part of it.

**Probe.** The 3.70.0 image started on a scratch `/data` creates
`sqlite.db`, `sqlite.db-wal` and `sqlite.db-shm` as **0644**, next to a
`backups/` directory at 0750.

**Impact.** Whether another host user can read it depends on the bind-mounted
directory's mode. `deploy.md` creates `./data` without setting one. On a
host where `./data` is world-traversable, any local account or process
reads every organization's data and the password hashes.

**Fix.** The database files should end up 0640, the same mode and reason
as `BackupFileMode` (a group ACL grant for the backup user must keep
working, so not 0600). SQLite recreates `-wal` and `-shm` on every start,
so either chmod all three after opening on every startup, or set the
process umask to `0027` early in `main.go`, which covers the database,
WAL, SHM and temp files in one place. Have `deploy.md` create `./data` as 0750.

### Hardening notes, not findings

- **`ADMIN_PASSWORD`:** in production `main.go` refuses an unset value
  (`main.go:80-86`) but accepts any other, including `admin`, the default.
  The 2026-07-12 audit judged these startup guards sound, and the value only
  matters on a first start (`EnsureFirstAdmin` seeds an empty `users` table
  only), so this is a hardening note: refusing the default value costs one
  line.
- **The binary's owner:** the runtime image chowns `/app` to the user it
  runs as, so a compromised process could replace its own binary. A
  root-owned `/app` with the binary read-only to `fatura` costs nothing.

---

## Watch list — not findings

- **Template parts LibreOffice wasn't tested with.** The formula vector is
  closed (see below), but a template carrying `xl/externalLinks/`,
  `xl/vbaProject.bin` or a connection part was not converted. Uploading a
  template needs an org-wide role. If it's ever tested, use a real `.xlsx`
  as the link target and a listener reachable over IPv6 from the container.
  The second sweep rated this High from the code alone; without a
  conversion that shows LibreOffice acting on such a part, it stays here.
- **A pixel-bomb logo.** The logo upload checks the sniffed type and a 2 MB
  cap, not the dimensions. A 2 MB PNG can decode to gigabytes, and
  LibreOffice decodes it on every export that places `{{organization.logo}}`.
  Not tried.
- **No `Cache-Control` on API JSON.** JSON responses carry no `Last-Modified`
  or `ETag`, so browsers and NPM don't heuristically cache them. Worth a
  `no-store` only if a caching layer is ever added in front of `/api`.

## Explicitly excluded: do not re-raise

- **`WEBSERVICE`/`FILTERXML` and `file://` lookups in an uploaded template.**
  Converted in the 3.70.0 image (LibreOffice from Debian trixie): the
  `WEBSERVICE` cells came out as `Err:540` (LibreOffice's "external content
  disabled") and `INDIRECT('file:///…')` as `#REF!`. This rests on
  LibreOffice's default, not on app configuration; re-check on a LibreOffice
  major upgrade. (OpenCode's suspected "templates handed to LibreOffice
  unexamined" is reduced to this and the watch list.)
- **`TRUSTED_PROXIES` missing behind the proxy.** Production sets it from
  NPM's proxy-network subnet (homelab-deploy's
  `env/fatura-cloud/.env.j2`), and `docker-compose.oidc.yml:38` passes it.
- **`PUT /api/organizations/{id}` at member level**, including its GL
  defaults, invoice numbering and inventory valuation, which F170's role
  gates don't reach. Excluded on 2026-09-19 and again on 2026-10-17. Only
  F193's missing ownership check is new.
- **SQL built with string formatting.** Every dynamic `ORDER BY` column comes
  from a whitelist map with a boolean direction (`db/journal_entry.go:130`,
  `db/stock.go:180`, `db/product.go:223`); table and column names come from
  Go literals.
- **The `soffice` command line**: fixed arguments, fixed file names, a
  per-call temp directory and profile, no shell, a 60 s timeout.
- **The logo as a script carrier**: the upload must sniff as `image/*`, it is
  served with that sniffed type, and `nosniff` is set.
- **Backup restore by name**: `filepath.Base` plus a `filepath.Rel`
  containment check, platform admin only.
- **The post-sign-in return path**: `safeResumePath` accepts same-origin
  paths only; the OIDC callback redirects only to `/` or `/login?error=…`.
- **JWT, cookie and session handling**: HS256 pinned, issuer and audience
  checked, `tokenVersion`/`isActive` re-read per request, `HttpOnly`,
  `SameSite=Lax`, `Secure` over HTTPS. Production refuses a default or short
  `JWT_SECRET`.
- **Login responses**: email normalized, a decoy bcrypt compare for unknown
  emails, one "invalid credentials" message.
- **CSRF and body limits**: every state-changing route, including those
  registered directly on `mux`, goes through `csrf`; every JSON body through
  `MaxBytesReader`.
- **Secrets in logs and history**: request logs never record bodies, query
  strings, cookies or headers; the activity history skips `auditSecretColumn`
  matches and masks IBAN/RIB.
- **Frontend HTML injection**: no `dangerouslySetInnerHTML`, `innerHTML` or
  `eval` in `src/`.
- **CI workflows**: no `pull_request_target`, no untrusted event text in
  `run:` steps; the Sentry token is a BuildKit secret.
- **Formula injection in the FEC export** (raised by the second sweep). The
  2026-09-19 audit verified Excel-formula injection clean. FEC is a
  regulated file read by tax tools, and prefixing a `'` would change the
  values it must carry.
- **Unbounded list endpoints** (second sweep): F176 (2026-10-17).
- **No `permissions:` block in `ci.yml` and `runtime-image.yml`** (second
  sweep). The repository's default workflow token is read-only
  (`default_workflow_permissions: read`, checked with `gh api`), and it
  can't approve pull requests.
- **Unpinned `apt-get install` versions in the Dockerfile** (second sweep).
  The base image is digest-pinned. Pinning Debian package versions would
  also hold back their security updates.
- Everything the earlier documents exclude.

## Not covered

- **The Raspberry Pi.** F190's and F192's limits were measured or reasoned
  on a Mac, not on the Pi.
- **The routes end to end over HTTP.** F190, F191, F193 and F196 were reproduced
  at the `db` layer; the route wrappers and sections were read, not called.
- **The template parts and the logo bomb** on the watch list.
- **A live SSO sign-in** and the restore path, as before.
- **Production's host**: the mode of the bind-mounted `./data` (F195).

## Verification

Beyond the per-phase gates:

- **F190:** a `db` test that a span over the limit is a 409 before any rows
  are built, and that a one-day and a one-year request still return their
  rows; the Cash Book's daily view still loads.
- **F191:** a `db` test that a manual entry can't set the source fields (or
  that they're ignored), and a two-organization test that sending B's
  invoice posts B's AR line even when A's ledger holds an entry naming it.
  Run the production query for cross-organization source references before
  the index migration.
- **F193:** `UpdateOrganization(A, {defaultArAccountId: <B's account>})` is a
  `ValidationError`; A's own account and `""` (clear) still work.
- **F196:** a movement in org A named after org B's sale leaves A's stock
  untouched when B cancels, and org B holds no movement of A's product.
- **F192:** in the runtime image, 20 simultaneous PDF exports leave at most
  the configured number of `soffice` processes, the rest get a 503, and
  other requests stay responsive.
- **F194:** with the chosen rule, 10 bad attempts for an email from one IP
  don't stop a correct sign-in from another.
- **F195:** a fresh container's `sqlite.db`, `-wal` and `-shm` are 0640;
  backups and restore still work, and the backup user still reads backups.
