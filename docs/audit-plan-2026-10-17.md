# FaturaCloud Re-Audit — Full Sweep (2026-10-17)

**This is a whole-repo audit, not a delta.** It covers `main` at `e253f31`
(v3.67.4) across UI, performance, security, data integrity, clean code, CI
and ops. Numbering continues at **F167**; no document under `docs/` uses
F167 or later.

The previous audits were deltas (2026-09-24, v3.54, 2026-09-29, 2026-10-11)
or a code-read full sweep (2026-09-19). This one leans on **running the app**:
a scratch server seeded with two production-sized organizations, timed API
calls, a browser sweep of every route, and throwaway probes against the
server. Most findings below were reproduced, not only read.

**Baseline health** at `e253f31`, measured before writing this document:

| Check                          | Result                                                                                             |
| ------------------------------ | -------------------------------------------------------------------------------------------------- |
| `go vet ./...`                 | Pass                                                                                               |
| `gofmt -l .`                   | Pass                                                                                               |
| `go test -race -count=1 ./...` | Pass (api 33s, db 28s locally)                                                                     |
| Go coverage                    | api 43.7%, db 75.4% of statements                                                                  |
| `staticcheck` (v0.8.2)         | 4 findings, all dead code or a dead store (F188); no bugs                                          |
| `deadcode -test`               | 3 unreachable functions (F188)                                                                     |
| `pnpm lint` (tsc + oxlint)     | Pass, 0 warnings                                                                                   |
| `pnpm test`                    | Pass (132/132, 22 files)                                                                           |
| `pnpm build`                   | Pass                                                                                               |
| `pnpm extract`                 | 0 missing in de/fr; tree stays clean                                                               |
| `pnpm audit --prod`            | No known vulnerabilities                                                                           |
| `govulncheck ./...`            | No called vulnerabilities (one module-level advisory in a package that isn't imported, as before)  |
| Browser route sweep            | 45 routes × 2 widths (1440×900, 390×844): no horizontal overflow, no console error, no error state |

**How this document was produced.** One sequential pass by the authoring
model, with no parallel sweeps and no subagents. The test bed:

- A scratch server built from `e253f31` (`HOME` pointed at a scratch
  directory, so it never touched the local dev database), seeded with
  `cmd/seed-demo` for 18 months: **Audit Moto** (moto scenario, Tunisia/TND:
  4,988 invoices, 5,837 payments, 642 purchase orders, 380 production
  orders) and **Audit Retail** (retail scenario: 5,340 invoices, 8,975
  payments, 495 cash movements, 30 opening loans). Production's demo
  organizations are about this size.
- API timings with `curl`, browser measurements with Playwright, and
  index, foreign-key and row-count queries on a read-only copy of the scratch
  database.
- **All timings are from a Mac.** The Raspberry Pi in production will be
  several times slower. Nothing was measured on the Pi.

Labels:

- **Confirmed** means reproduced against the scratch server (or, for F171,
  against production's public static files). Every probe was throwaway: probe
  users, invoices and the uploaded template were deleted afterwards. The
  product deletions (F167, F170) and a tax-rate change (F168, reverted) were
  left in the scratch database, which is discarded.
- **Code read** means established by reading the code.
- **Decision** means the code does what it was written to do, but the
  outcome needs an owner decision.

No code was changed while producing this document. Production was not read,
apart from fetching the public `index.html`, `/api/version` and one JavaScript file's
response headers.

**Instructions for the executing model:**

- This document is an audit, not a remediation. Wait for the owner's
  go-ahead before starting any phase.
- **Phase 0 comes first:** the owner runs the production queries below. Two
  fixes (the unique number index in F169, the email migration in F174) fail
  on production data that already breaks the new rule, so their shape
  depends on those counts.
- Use one feature branch and PR per phase, and never push to `main`. Use
  conventional commits with no Claude attribution lines.
- After every phase, `go vet ./... && go test -race ./...` and
  `pnpm lint && pnpm build` must pass, and `pnpm extract` must report 0
  missing.
- F170 and F186 need a product decision before any code is written (the
  options are under each finding). F167's archive flag and F168's
  "new rate instead of edit" flow are UI changes the owner should see first.
- **Never touch Société ELECTRO MISSAOUI.**
- The 2026-09-19, 2026-09-24, v3.54, 2026-09-29 and 2026-10-11 "Explicitly
  excluded" lists still bind, and so does this document's own list at the
  end.

**Status (2026-10-17):** Phase 1 (F167–F169) merged in #515, F169's unique
index in #516, Phase 2 (F170 with option (a), F178) in its own PR. Owner
decision pending: F186 (which Cash Book layout stays).

---

## Severity overview

| #    | Finding                                                                                                                                                                 | Area                    | Severity          | Phase |
| ---- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------- | ----------------- | ----- |
| F167 | Deleting a product erases its whole stock history and serial registry while the posted GL entries stay; the Inventory account no longer matches the stock               | Ledger integrity        | **High**          | 1     |
| F168 | Changing an in-use tax rate's percentage rewrites the VAT of every past document in the Tax Summary, re-exported documents and e-invoices                               | Report integrity        | **High**          | 1     |
| F169 | Invoice numbers are typed in the browser and never checked for uniqueness; no document type has a unique number index                                                   | Numbering integrity     | Medium-High       | 1     |
| F170 | Every role, `cashbook` included, can change organization-wide configuration (tax rates, products, templates, numbering) through the API                                 | Authorization           | Medium (Decision) | 2     |
| F171 | Static assets and API responses are served with no caching and no compression, in production too: every visit re-downloads 1.6–2.9 MB of JavaScript                     | Performance             | Medium            | 3     |
| F172 | After every Cash Book sale or collection the page re-downloads the organization's whole payment history and loan register, one request after another                    | Performance / Cash Book | Medium            | 3     |
| F173 | Excel uploads are opened with excelize's 16 GB unzip limit: a 1 MB file inflated to 248 MB, held 570 MB of extra memory, and a template upload leaks a 248 MB temp file | Security / availability | Medium            | 4     |
| F174 | Email addresses are case-sensitive everywhere: a different capitalization fails sign-in, and two accounts can differ only by case                                       | Auth / UX               | Medium            | 4     |
| F175 | A session ends exactly 24 hours after sign-in, mid-shift, and the next request throws away whatever the user was typing                                                 | Auth / UX               | Medium            | 4     |
| F176 | Invoices, payments and journal entries are returned whole: 3.2 MB, 5.3 MB and 10.4 MB on the retail data, growing without limit                                         | Performance             | Low-Medium        | 3     |
| F177 | One SQLite connection serves every read and write, so a few heavy list reads delay a counter sale by 100–450 ms (Mac)                                                   | Performance             | Low-Medium        | 3     |
| F178 | `seed-demo --reset` deletes any organization the account can see whose name matches, test flag or not                                                                   | Ops safety              | Low               | 2     |
| F179 | #512's group-readable backups directory only applies to a directory created after the upgrade                                                                           | Ops                     | Low               | 5     |
| F180 | The CSP still allows WebAssembly, `data:`/`blob:` fetches and blob workers for a PDF stack that no longer runs; four packages and a util are unused                     | Security hardening      | Low               | 4     |
| F181 | Ops/CI cluster: no health check, compose ignores `VERSION`, unpinned `govulncheck`, no graceful shutdown, base images never refreshed, runtime stage untested in CI     | Ops / CI                | Low               | 5     |
| F182 | The activity history reads its "before" snapshot outside the change and before authentication                                                                           | Activity history        | Low               | 4     |
| F183 | The chart library is 1.5 MB (451 KB gzipped) for one column chart and four bar charts                                                                                   | Performance             | Low               | 3     |
| F184 | UI polish cluster: an orphaned aging card, chart axis decimals, cut-off placeholders, 3-decimal TND inputs over 2-decimal storage, duplicate fetches                    | UI                      | Low               | 6     |
| F185 | Six document detail pages repeat the same 650–1,200-line shell; 287 uses of `any` with the rule switched off                                                            | Clean code              | Low               | 6     |
| F186 | Two Cash Book layouts have been maintained side by side since 2026-09-30                                                                                                | Clean code / UX         | Low (Decision)    | 6     |
| F187 | The CLAUDE.md files total 274 KB (db/CLAUDE.md alone 137 KB), read like a changelog, and have drifted from the code                                                     | Docs                    | Low               | 6     |
| F188 | Dead Go code: three unreachable functions, an unused field, a dead store                                                                                                | Clean code              | Low               | 6     |
| F189 | The Cash Book's sale assembly (gross-to-net conversion and totals) has no frontend test                                                                                 | Tests                   | Low               | 6     |

---

## Phase 0 — production checks (owner runs these)

Run against a read-only copy of the pimi-01 database, as on 2026-10-11.
Each answer changes how a later fix is written.

**Results (2026-10-17, schema 104, a temporary copy deleted afterwards).**
Every check came back clean: no duplicate or blank invoice numbers, no
opening-loan reference colliding with an invoice number, no duplicates in the
five counter-numbered documents, no emails differing only by case, no product
deletion and no tax-rate percentage change in `audit_events`, and the backups
directory is `0775`. F169's unique index (migration `0106`) therefore applies
with no cleanup; the other five number columns can get theirs the same way.

```sql
-- F169: duplicate invoice numbers (opening loans use register references)
SELECT o.name, i.number, COUNT(*) AS n
FROM invoices i JOIN organizations o ON o.id = i.organizationId
WHERE COALESCE(i.origin, '') <> 'opening' AND COALESCE(i.number, '') <> ''
GROUP BY i.organizationId, i.number HAVING n > 1;

-- F169: the same for the five other numbered documents
SELECT 'orders', organizationId, orderNumber, COUNT(*) FROM orders
  WHERE COALESCE(orderNumber, '') <> '' GROUP BY 2, 3 HAVING COUNT(*) > 1
UNION ALL SELECT 'purchase_orders', organizationId, orderNumber, COUNT(*) FROM purchase_orders
  WHERE COALESCE(orderNumber, '') <> '' GROUP BY 2, 3 HAVING COUNT(*) > 1
UNION ALL SELECT 'outbound_deliveries', organizationId, deliveryNumber, COUNT(*) FROM outbound_deliveries
  WHERE COALESCE(deliveryNumber, '') <> '' GROUP BY 2, 3 HAVING COUNT(*) > 1
UNION ALL SELECT 'inbound_deliveries', organizationId, deliveryNumber, COUNT(*) FROM inbound_deliveries
  WHERE COALESCE(deliveryNumber, '') <> '' GROUP BY 2, 3 HAVING COUNT(*) > 1
UNION ALL SELECT 'production_orders', organizationId, orderNumber, COUNT(*) FROM production_orders
  WHERE COALESCE(orderNumber, '') <> '' GROUP BY 2, 3 HAVING COUNT(*) > 1;

-- F169: blank or missing numbers (if any, the unique index must be partial)
SELECT 'invoices', COUNT(*) FROM invoices WHERE COALESCE(number, '') = ''
UNION ALL SELECT 'orders', COUNT(*) FROM orders WHERE COALESCE(orderNumber, '') = ''
UNION ALL SELECT 'purchase_orders', COUNT(*) FROM purchase_orders WHERE COALESCE(orderNumber, '') = ''
UNION ALL SELECT 'outbound_deliveries', COUNT(*) FROM outbound_deliveries WHERE COALESCE(deliveryNumber, '') = ''
UNION ALL SELECT 'inbound_deliveries', COUNT(*) FROM inbound_deliveries WHERE COALESCE(deliveryNumber, '') = ''
UNION ALL SELECT 'production_orders', COUNT(*) FROM production_orders WHERE COALESCE(orderNumber, '') = '';

-- F174: accounts whose emails collide once trimmed and lowercased
SELECT lower(trim(email)), COUNT(*) FROM users
GROUP BY lower(trim(email)) HAVING COUNT(*) > 1;

-- F167 / F168: product deletions and tax-rate percentage edits since the
-- activity history shipped (v3.65.0, 2026-10-06). Earlier ones left no trace.
SELECT createdAt, organizationId, userEmail, entityLabel
FROM audit_events WHERE resource = 'products' AND method = 'DELETE';
SELECT createdAt, organizationId, userEmail, entityLabel, changes
FROM audit_events WHERE resource = 'tax-rates' AND method = 'PUT'
  AND changes LIKE '%"percentage"%';

-- F172 / F176: how big the lists already are
SELECT o.name,
  (SELECT COUNT(*) FROM payments p WHERE p.organizationId = o.id) AS payments,
  (SELECT COUNT(*) FROM invoices i WHERE i.organizationId = o.id) AS invoices,
  (SELECT COUNT(*) FROM journal_entries j WHERE j.organizationId = o.id) AS entries
FROM organizations o;
```

Also note each organization's Inventory Valuation report "Difference" today
(Accounting ▸ Inventory Valuation), to compare against after F167.

And on the host: `stat -c '%a %U:%G' /srv/containers/fatura-cloud/data/backups`
(F179: `750` is right, `700` means Syncthing still can't read the backups).

Every query above was run read-only against the scratch database before
this document was written: all run cleanly; the duplicate, blank-number and
email checks return nothing there, and the two history queries return
exactly this audit's probes. If the blank-number check returns anything on
production, F169's unique indexes must be partial (`WHERE number <> ''`).

---

## Phase 1 — ledger, report and numbering integrity

### 1.1 — F167: deleting a product erases its stock history (High, Confirmed)

**What happens.** `DeleteProduct` (`db/product.go:480`) is a bare
`DELETE FROM products WHERE id = ?` with no usage check. The schema cascades
it:

| Child table                                                    | On delete |
| -------------------------------------------------------------- | --------- |
| `stockMovements`                                               | CASCADE   |
| `product_serial_numbers`                                       | CASCADE   |
| `bill_of_materials` (both sides), `bill_of_materials_versions` | CASCADE   |
| every line-item table, `production_orders`                     | SET NULL  |

So one delete removes the product's whole stock-movement history and its
serial registry. The posted GL entries those movements fed (GRNI, COGS,
production, adjustments) stay, because posted entries are never touched. The
Inventory account then carries value with nothing behind it, permanently.

**Reproduced.** On Audit Moto, the Inventory Valuation report read GL
30,603,113.80 TND against a computed 30,461,741.12 TND (difference
141,372.68). Deleting "Atlas Roadster 650cc" (214 units at 5,649.42 TND, 44
movements) as an admin answered 200; afterwards the movements were gone and
the difference was **1,350,348.56 TND**. The same delete also worked for a
`cashbook` member through the API (F170).

**Reach.** Not just the API: the product drawer offers **Delete** on every
product behind a plain "Are you sure?" (`src/components/products/form.tsx:347`),
and admin, power user, general, sales and purchasing users all have the
Products screen. Every other referenced master record is guarded: clients
(`clientReferencingTables`), vendors, accounts, tax rates and imports all
refuse a delete while in use. Products are the one gap.

**Knock-on effects (Code read).** Cancelling a delivery, receipt or
production order for the deleted product later finds no product on its
lines (`SET NULL`), so the stock reversal skips it while the GL reversal
still runs. A Cash Book sale's stock-out (`invoices.movesStock`) is restored
by summing its movements, which are gone.

**Fix.**

1. Refuse the delete (409, naming what references it) when the product has
   any stock movement, serial, document line, production order or BOM use,
   the same shape as `GetTaxRateUsageCount`/`DeleteTaxRate`. A tripwire test
   over the live schema, like `TestTaxRateUsageCountCoversEveryReference`.
2. Products have **no archive or active flag** (columns checked), so users
   would have no way to retire a product. Add `products.isArchived`
   (migration), hide archived products from pickers, keep them in history
   and reports, and offer **Archive** where Delete is refused.
3. The UI shows Delete only for an unused product, as the tax-rate form
   already does.

Phase 0 sizes any past damage: the activity history has every product
deletion since 2026-10-06; anything earlier left no trace beyond the
Inventory Valuation report's difference.

### 1.2 — F168: editing a tax rate rewrites history (High, Confirmed / Code read)

**What happens.** Line items store only a tax rate's **id**, never its
percentage. `UpdateTaxRate` (`db/tax_rate.go:229`) lets the percentage
change at any time (`percentage = COALESCE(?, percentage)`), with no usage
check; only _deleting_ an in-use rate is refused. Everything that recomputes
VAT from a stored document reads the **current** percentage:

| Reader                                           | Where                                       | Label     |
| ------------------------------------------------ | ------------------------------------------- | --------- |
| Tax Summary report                               | `db/sales_reports.go:301`, `:330`           | Confirmed |
| Re-exported documents' VAT recap and rate column | `db/xlsx_export.go:388`, `:532`, `:583-584` | Code read |
| E-invoice XML                                    | `db/einvoice.go:115`, `:271`, `:355`        | Code read |
| Validation of a later edit to an old invoice     | `db/invoice_totals.go:128-132`              | Code read |

The posted GL is safe (entries are immutable), so after an edit the GL and
these outputs disagree.

**Reproduced.** On Audit Retail, a `cashbook` member changed the default
rate "Taux normal" from 19% to 7% (200). The 18-month Tax Summary then
reported that rate's output VAT as **843,137.68 TND** on a base of
12,044,822.71 TND, where the invoices were issued and posted at 19% (about
2,288,516 TND). The rate was set back to 19% afterwards.

**Reach.** The Settings ▸ Tax rates form keeps the percentage editable on a
rate in use (it only disables Delete, `src/components/tax-rates/form.tsx:128`),
and every role except `cashbook` has the Settings gear. A real VAT change
(19% → 20%, say) is exactly the moment someone will edit the existing rate.

**Fix.** Refuse a percentage change on a rate in use (409, the
`UpdateAccount` type-change precedent), and make the form say so: "this rate
is used by N documents — create a new rate and make it the default". The new
rate then applies to new documents only.

Snapshotting the percentage onto each line is the more thorough fix, but its
backfill would copy **today's** percentages, which on production may already
be wrong (Phase 0's `audit_events` query shows edits since 2026-10-06 only).
If chosen, do it after the refusal, and backfill from the posted GL tax
lines where one exists rather than from `taxRates`.

### 1.3 — F169: invoice numbers aren't unique (Medium-High, Confirmed / Code read)

**What happens.**

- `CreateInvoice` (`db/invoice.go:264`) stores whatever `number` the browser
  sends, then advances `invoice_number_counter`. The invoice form prefills the
  number when it **opens** and leaves it editable. Two people creating
  invoices at the same time get the same number; anyone can type an existing
  one.
- There is no unique index on `invoices.number` (only a plain index,
  `idx_invoices_number`) or on any other document number. F99 (2026-09-19)
  already noted "no unique index behind it" for the five counter-numbered
  types; its fix stopped the counter rewind but added no index, so that part
  is still open.
- Cash Book sales are fine: `CreateCashSale` numbers them server-side inside
  the transaction (`db/cash_sale.go:361-418`).

**Reproduced:** the API accepts a duplicate. Two `POST /api/invoices` with
the number of an existing paid invoice (`FAC-2025-0001`) both answered 201,
leaving three invoices with that number. Both probes were deleted. The
two-people-at-once path (both forms prefilled with the same number) is
**Code read**: it follows from the prefill and the missing check, but two
browsers were not raced.

Invoice numbers are normally expected to be unique and sequential; the owner
should confirm the exact obligation for Tunisian invoices.

**Fix.**

1. Number invoices server-side inside `CreateInvoice`'s transaction, as
   `CreateCashSale` already does (`generateDocumentNumber`); the form shows
   "assigned on save" or a preview. A user-typed number only for an explicit,
   documented override, refused if taken.
2. A unique index on `(organizationId, number)` for invoices (excluding
   `origin = 'opening'`, whose register references are unique among
   themselves already) and on the five other number columns. **Phase 0
   first:** existing duplicates make the migration fail, and need a decision
   (renumber drafts, keep sent ones) before it can run.

---

## Phase 2 — access decision and operator safety

### 2.1 — F170: every role can change organization-wide configuration (Medium, Decision, Confirmed)

**What happens.** The role redesign gated writes for clients (`sales`),
vendors (`purchasing`) and accounts (`accounting`) with `orgRoleProtected`.
These writes stayed `orgMemberProtected`, so **every** role may call them,
`cashbook` and `accounting` included:

- products: `PUT`, `DELETE`, and the Excel import
- tax rates, payment terms, units of measure, product families: create,
  update, delete, import
- document templates (upload/delete) and page orientation
- document numbering (`PUT .../document-number-settings/{type}`, counter
  included)
- stock movements and the stock count upload (section-guarded to Inventory)

`api/sections.go`'s comment explains why these stay membership-level: the
screens a restricted role keeps **read** them. The writes came along with
the reads without a separate decision.

**Reproduced** as a `cashbook` member of Audit Retail: set the order counter
back to 1 (200), changed the default VAT rate 19% → 7% (200), deleted a
stocked product (200). The same member correctly got 403 on the invoices
list. The probe user was deleted.

**How it relates to the exclusions.** `PUT /api/organizations/{id}` at
member level is excluded (2026-09-19) and is **not** re-raised here. The
2026-09-24 exclusion covers master data staying membership-level for
**reads** under the section guard. Neither decided the writes, and F167/F168
make them far more consequential.

**Options (owner decision).**

- **(a)** Gate these writes to `admin`/`power_user`/`general` plus the
  domain role that owns them: products to `sales`/`purchasing`; tax rates
  and payment terms to `accounting`; templates, numbering, units and
  families to the org-wide roles only. Reads unchanged. Add them to
  `domainRouteRoles` so the tripwire covers them.
- **(b)** Keep them open, and rely on F167/F168's guards to remove the
  dangerous cases.
- **(c)** (a) for tax rates, templates and numbering only.

(a) matches how clients, vendors and accounts are already handled.

**Decision (owner, 2026-10-17): (a).** Implemented with the screens a role
can't save made view-only (no New, Upload, Save, Delete or Archive). Stock
movements needed no change: the Inventory section guard already limits them
to the org-wide roles. The bill of materials stays as it was, limited to
admin/power_user by its own section.

### 2.2 — F178: `seed-demo --reset` deletes by name only (Low, Code read)

`setupOrganization` (`cmd/seed-demo/masterdata.go:148-160`) lists the
organizations the account belongs to and deletes the first whose name equals
`--org-name`, without checking `isTest`. The tool runs against production
next to real organizations; a mistyped `--org-name` with `--reset` deletes a
real one, and the account that runs it may well be an admin of it.

**Fix.** Refuse `--reset` unless the matched organization has `isTest`, and
print the id and ask for `--confirm-reset <id>`. Two lines of code for an
irreversible operation on production.

**Fixed (2026-10-17)** as written, plus a refusal when more than one
organization carries the name.

---

## Phase 3 — performance

The server-side query times are small on the Mac: the worst endpoint is the
retail Dashboard at 171 ms; most lists take 1–90 ms. The cost is in **what
crosses the network and how often**. Order: F171 and F172 first (cheap,
8–10× less data), then F176/F177 once the Pi has been measured.

### 3.1 — F171: no caching and no compression (Medium, Confirmed)

**What happens.** `spaHandler` (`main.go:186`) serves the embedded `dist/`
through `http.FileServerFS`. An `embed.FS` has no modification times, so
there is no `Last-Modified`, no `ETag`, no `Cache-Control`, and no
compression. API JSON is not compressed either.

- **Production confirmed** (public files only): `GET /assets/index-B9egfpBM.js`
  through the reverse proxy (openresty) returns `content-length: 404064`, no
  `content-encoding` even with `Accept-Encoding: gzip, br`, and no cache
  headers. The proxy adds nothing.
- With no validator and no freshness, the browser downloads every script
  again on every full load. Measured in the browser: about **1.6 MB** of
  JavaScript for `/invoices`, **2.9 MB** for `/dashboard`, **1.7 MB** for
  `/cash-book`, on every visit.
- gzip at level 6 would cut the JavaScript from 4.07 MB to 1.35 MB, and the
  big JSON lists 8–10×: invoices 3.1 MB → 339 KB, payments 5.1 MB → 653 KB,
  loan status 1.7 MB → 314 KB.

**Fix.**

1. `/assets/*` (content-hashed names): `Cache-Control: public,
max-age=31536000, immutable`. `index.html` and SPA fallbacks:
   `Cache-Control: no-cache`.
2. Pre-compress assets at build time (`.br`/`.gz` next to each file, served
   by `Accept-Encoding`), so the Pi never compresses the same file twice.
3. gzip API responses above ~1 KB with a small middleware (e.g.
   `github.com/klauspost/compress/gzhttp`), skipping the binary
   exports/backups that are already compressed.

**Where the compression goes matters.** The activity history learns a
created document's id from the first 2 KB of the response body, captured by
the response writer `RequestLogger` installs (`api/request_log.go:166`,
F164's fallback). Compression must therefore wrap the **outermost** handler
in `main.go`, outside `api.RequestLogger(...)`, and never inside
`NewRouter`'s wrappers. Placed inside, the capture sees compressed bytes and
history rows silently lose their `entityId`; the existing audit tests use
`NewRouter` alone and would not notice.

Enabling compression in the proxy instead is possible, but the app is also
run without one (the default `docker-compose.yml` publishes 8080 directly),
so the app should do it itself.

### 3.2 — F172: the Cash Book re-downloads everything after each sale (Medium, Confirmed)

**What happens.** After `CreateCashSale` succeeds, `handleSubmit`
(`src/components/cash-book/use-cash-book.ts:680-694`) awaits, one after
another: all clients, all products, the day's register, the **whole loan
register** (`GetLoanStatus`, every loan line ever) and the **organization's
whole payment history** (`GetPayments`), only to filter it to inbound
payments of one customer on screen (`:305-319`). The **Record** button stays busy
until all of it arrives (`setSubmitting(false)` is in `finally`). A loan
collection does the same for loans, register and payments (`:738-740`).

**Measured** on Audit Retail with a real counter sale in the new layout: 6
follow-up requests, about **7.3 MB** (clients 217 KB, products 65 KB, loan
status 1.76 MB, payments 5.3 MB), 0.4 s on localhost. Opening the Cash Book
costs the same downloads. Over a shop's connection to the Pi this is seconds
per sale, and it grows with every payment ever recorded.

**Fix.**

1. Release the button as soon as the sale is recorded; refresh in the
   background, in parallel (`Promise.all`).
2. Scope the reads: `GET .../payments?clientId=&direction=inbound` (or a
   limit) instead of the whole history; loan status for the customer being
   served (the old layout already scopes it with `scopeLoanStatusToClient`).
3. Merge what the server already returns (`CashSaleResult` has the client,
   invoice and payment) instead of re-fetching clients; refresh products
   only for the lines sold.

F171's compression shrinks the rest further.

### 3.3 — F176: the big lists are returned whole (Low-Medium, Confirmed)

Only stock movements are paginated server-side (`api/stock.go:16-17`).
Everything else returns the full history:

| List (Audit Retail) | Rows   | JSON    | Server time (Mac) |
| ------------------- | ------ | ------- | ----------------- |
| Journal entries     | 20,748 | 10.4 MB | 68 ms             |
| Payments            | 8,975  | 5.3 MB  | 91 ms             |
| Invoices            | 5,340  | 3.2 MB  | 24 ms             |
| Loan status         | —      | 1.8 MB  | 119 ms            |

Each full load of a list page pays this again (F171), and the sizes only
grow. **Pagination is not a drop-in change**: the Invoices and Incoming
invoices pages compute their chip counts and their Unpaid/Overdue/Paid
filters client-side over the full list (`useStatusChips`, the outstanding
filters), so they need server-side counts first. (Their "owed" headlines
already come from the server's client and vendor summaries.) Fix after F171/F172: server-side paging and
filters with counts for journal entries and payments first (the largest,
and nothing on screen needs all of them at once), then invoices.

**Phase 3 status (2026-10-17).** F171, F172 and F176 (journal entries) are
done in the Phase 3 PR. Measured on a scratch retail organization:
- the main script is 404 KB → 126 KB gzipped;
- the payments list is 447 KB → 56 KB;
- a journal entries page is 12.5 KB against 1 MB for the whole list;
- a second visit transfers no JavaScript;
- a counter sale frees its button when the POST answers, then makes five small parallel requests (about 9 KB) instead of 7.3 MB in series.

Payments are not paged: their only screen, the Cash Book, now asks for inbound payments of one customer (stacked layout), and the loan register layout needs every customer's to group stalled balances. Invoices wait for server-side chip counts, as above. F177 and F183 wait for a measurement on the Pi.

### 3.4 — F177: one connection serializes every request (Low-Medium, Confirmed on the Mac)

`db.SetMaxOpenConns(1)` (`db/db.go:50`) puts every read and write through
one SQLite connection. WAL mode would let reads run alongside the writer,
but they queue instead. Measured: a 0.4 ms request
(`GET .../accounts`) took **107–447 ms** while 12 journal-entry and payment
list requests were in flight. On the Pi a till sale waits behind someone
opening Journal Entries.

Two invariants depend on the single connection and must survive any change:
`allocateAndFinalizeEntryTx`'s `MAX(entryNumber)+1` (race-free only because
writes serialize), and the convention that a `d.DB` read inside an open
transaction deadlocks rather than reading outside it. So writes stay on one
connection. The option is a second, read-only pool (`mode=ro`, a few
connections) used only by GET handlers that open no transaction.
**Measure on the Pi first**; F171/F172/F176 remove most of the heavy reads
that cause the queueing.

### 3.5 — F183: the chart library (Low, Confirmed)

`@ant-design/plots` builds a 1.50 MB chunk (451 KB gzipped) for one
`Column` chart on the Dashboard and four `Bar` charts on the reporting
pages. Admins land on Invoices, not the Dashboard, so only those pages pay
it, and with F171 it becomes a one-time cost per release. A lighter library
or plain SVG bars would cut it; worth doing only if Pi load times call for
it.

---

## Phase 4 — security and auth hardening

### 4.1 — F173: Excel uploads can be decompression bombs (Medium, Confirmed / extrapolated)

**What happens.** Every upload is opened with `excelize.OpenReader` and no
options (`db/mass_data.go:132`, `db/loan_import.go:354`,
`db/document_template.go:77`, and every export re-opens a stored template at
`db/xlsx_export.go:144`). excelize v2.11's defaults are
`UnzipSizeLimit` 16 GB and `UnzipXMLSizeLimit` 16 MB (`go doc`): a worksheet
up to 16 GB is extracted, spilling to the temp directory past 16 MB. The
8 MB upload cap limits the compressed size only.

**Reproduced** with a generated 1.0 MB workbook whose one sheet inflates to
247.6 MB (400,000 rows × 16 cells):

- Products Excel import (any member): 7 s of work, server memory from about
  302 MB to a **871 MB** peak, and a 35 MB JSON report of 399,999 row errors.
- Document template upload (any member): **accepted** (204) and stored as
  the organization's invoice template. Each invoice export then spent 6 s
  inflating it before failing (409).
- `UploadDocumentTemplate` never closes the workbook it opens, so **each**
  template upload left a 247.6 MB `excelize-*` temp file behind (the import
  path closes its workbook and left none). On the Pi that's the SD card.

At the 8 MB cap the same ratio is about 2 GB of XML and several GB of
memory, which would exhaust the Pi. That last figure is an extrapolation;
only the 1 MB case was run. All probe files and the template were removed.

**Fix.** One helper that every caller uses:
`excelize.OpenReader(r, excelize.Options{UnzipSizeLimit: …,
UnzipXMLSizeLimit: …})` plus `defer f.Close()`, and a row cap per import
(e.g. 20,000 rows, reported as an error). Also cap the number of row errors
a report carries. **Set the limit from measured sizes, not a guess:** export
the seeded organizations' largest mass-data sheets (products, clients,
accounts) and the stock count, take the loan-register template and the
embedded document templates, read their unzipped totals with `unzip -l`,
and use a generous multiple of the largest (a few tens of MB is the likely
outcome).

**Phase 4 status (2026-10-17).** F173, F174, F175, F180 and F182 are done in
the Phase 4 PR.
- F173's limits were set from measured sizes: the largest real workbook (the products export of the quarter-scale retail demo) unzips to 114 KB, so the limit is 64 MB total and 16 MB per sheet in memory.
- F174 folds ASCII only, so Go and SQLite's `lower()` agree.
- F175 renews a token after 12 h, up to 7 days after sign-in.
- F182 keeps the concurrency window, as the plan allowed.

### 4.2 — F174: emails are case-sensitive (Medium, Confirmed)

`users.email` is `TEXT NOT NULL UNIQUE` with the default (case-sensitive)
collation, and every lookup is `WHERE email = ?`: sign-in
(`api/auth.go:234`), adding a member (`api/organization_users.go:87`), SSO
linking and provisioning (`api/users.go:418`, `:432`). Nothing lowercases or
trims on write.

**Reproduced:** created `Probe.Case@Example.com`; signing in as
`probe.case@example.com` → 401, and with a leading space → 401. A second
account `probe.case@example.com` was accepted (201). Both deleted.

**Why it matters.** Phone keyboards capitalize the first letter, so a
cashier signing in on a tablet fails with "invalid credentials". An SSO
provider that returns `Ali@Shop.tn` for a local `ali@shop.tn` creates a
second, empty account instead of linking.

**Fix.** Normalize (`strings.ToLower(strings.TrimSpace(...))`) on every
write and lookup; a migration that lowercases existing emails, guarded by
Phase 0's collision query (two accounts that collide need a manual merge
first); then `UNIQUE` on `lower(email)`.

### 4.3 — F175: the session ends mid-shift and drops unsaved work (Medium, Code read)

The JWT and its cookie live exactly 24 hours from sign-in
(`api/auth.go:304`, `:373`) and are never renewed. When it expires, the next
request gets 401, and `request()` (`src/api/client.ts:25-38`) replaces the
page with `/login`: a half-entered counter sale or an unsaved invoice is
lost, and sign-in lands on the role's home page rather than where the user
was.

**Fix.** Sliding renewal: when an authenticated request carries a token
older than, say, 12 hours, re-issue the cookie with the same `tokenVersion`
(revocation still works). On 401, keep the path (`/login?next=`) and return
there after sign-in. Optionally warn before redirecting while a form is
dirty (`useUnsavedChangesWarning` knows).

### 4.4 — F180: the CSP still allows what the removed PDF stack needed (Low, Code read)

`main.go:149-153` allows `'wasm-unsafe-eval'`, `connect-src data: blob:` and
`worker-src blob:`, justified in its comment by `@react-pdf`'s in-app preview.
That preview was removed on 2026-09-08. Nothing the app ships imports
`@react-pdf/renderer`, `react-pdf`, `pdfjs-dist` or `qrcode` (the build has
no PDF chunk). Only the orphaned `src/components/invoices/pdf.tsx` and
`pdf-tunisia.tsx` import `@react-pdf/renderer`; whether to delete those is
the owner's open decision and is not raised here.

**Independent of that decision:**

- Drop `'wasm-unsafe-eval'`, `data:`/`blob:` from `connect-src` and
  `worker-src blob:` from the CSP; check the login, Dashboard, exports and
  Sentry still work.
- Remove the unused `react-pdf`, `pdfjs-dist` (and its `pnpm-workspace.yaml`
  catalog pin), `qrcode`, `@types/qrcode`, and `src/utils/sepa-qr.ts`, which
  nothing imports.
- Fix the stale comment at `src/app.tsx:44-46` ("PDF stack … pulled in by
  invoice details") and drop the `/invoices/:id/pdf` route
  (`src/app.tsx:288`), which renders the details page.

### 4.5 — F182: the activity history's "before" snapshot (Low, Code read)

`auditMux` wraps the whole route, authentication included
(`api/audit.go:97-140`). For every change request it reads the document's
label, state and full row **before** the handler runs, under a separate read
lock:

- A concurrent edit between that read and the handler's own transaction is
  recorded as part of this request's change (the 2026-10-11 "Not covered"
  note, now confirmed by reading).
- An unauthenticated `PUT`/`DELETE` costs those reads before the 401.

**Fix.** Skip the snapshot unless the request is authenticated (check the
cookie claims first). Accept the concurrency window, or take the "before"
inside the handler's transaction for the few routes where it matters.

---

## Phase 5 — ops and CI

### 5.1 — F179: #512 only reaches a new backups directory (Low, Code read)

#512 creates the backups directory as `0750` (`main.go:44`), but
`os.MkdirAll` leaves an **existing** directory's mode alone. An install
upgraded from before #512 keeps a `0700` directory, and the group can't
enter it even though new snapshots are `0640`. Phase 0's `stat` shows
whether production's was fixed by hand.

**Fix.** `os.Chmod(backupDir, backupDirMode)` after `MkdirAll` (ignoring
"operation not permitted" on a host-owned directory), plus a test.

**Phase 5 status (2026-10-17).** F179 and F181 are done in the Phase 5 PR, except the move off bookworm, which stays a plan (see below).
- The dev-dependency audit runs report-only. Three advisories are open, each pinned by its parent: jotai-devtools > jsondiffpatch, @lingui/cli > micromatch > braces, oxfmt > tinypool. In-range updates fixed the rest.
- govulncheck is pinned to v1.8.0.

**Bookworm** is not moved in this PR. Debian 12's regular security support ended in June 2026, and it is now on Debian LTS (through June 2028, as far as known here). Whether LTS covers `libreoffice-calc` was not checked; that package is what the image is for.

The move to `debian:trixie-slim` is a deliberate change of its own:
1. switch the digest-pinned base;
2. rebuild;
3. check that `libreoffice-calc`, `fonts-liberation` and `fonts-dejavu-core` still install;
4. export a real invoice PDF from the running container and look at it, per this repo's rule.

`runtime-image.yml` then checks it every week. Dependabot proposes digest updates within bookworm, but not the suite change.

### 5.2 — F181: ops and CI cluster (Low, Code read)

- **No health check.** There's no `HEALTHCHECK`, no `/healthz`, and the
  compose files have no `healthcheck:`. F139 (2026-09-19) listed this and
  its PR (#298) didn't address it. `restart: unless-stopped` can't restart a
  wedged process.
- **`docker-compose.yml` doesn't pass `VERSION`** as a build arg (the other
  F139 leftover), so a local `docker compose up --build` reports `dev`.
- **CI's Docker job builds `--target backend` only** (`ci.yml:256`), so a
  broken runtime stage (the LibreOffice `apt-get`) is found only on a
  release tag.
- **`govulncheck@latest`** is still unpinned (`ci.yml:147`), and
  `pnpm audit` still checks `--prod` only (`ci.yml:205`).
- **No graceful shutdown.** `srv.ListenAndServe()` with no signal handling:
  `docker stop` cuts in-flight requests and the background loops, and
  `defer database.Close()` never runs. SQLite's WAL makes this safe for
  data, but an export or backup in progress just dies. Use
  `signal.NotifyContext` + `srv.Shutdown`.
- **Pinned images never refresh.** Base images are digest-pinned (since
  2026-09-20) and actions SHA-pinned, but there's no Dependabot or Renovate,
  so nothing proposes updates. The runtime is `debian:bookworm-slim`;
  bookworm has been oldstable since Debian 13, so confirm its remaining
  security-support window and plan the move to `trixie-slim` (re-verify PDF
  export in the image, per this repo's rule).

---

## Phase 6 — clean code, UI polish, docs, tests

**Phase 6 status (2026-10-17).** F184, F185 (the shared shell), F188 and F189 are done in the Phase 6 PR. F187's rewrite is its own PR, so the docs can be reviewed apart from the code: the five CLAUDE.md files went from 287 KB to 79 KB of current rules, with every backticked identifier checked against the code both ways. Still open:
- **F185's typing:** each page's form values typed, then `typescript/no-explicit-any` turned on directory by directory.
- **F184's antd icon names:** antd names its icon buttons in English ("search", "close-circle"). That is library behavior, with no app-level hook short of replacing each icon.
- **F186:** both layouts are kept, by the owner's decision of 2026-10-17.
- **F188's legacy tables:** production had no rows in `timeEntries`/`projects`/`tags` (checked 2026-10-17); migration `0109` drops them.

### 6.1 — F184: UI polish cluster (Low, Observed in the browser)

The route sweep was clean (no overflow, no errors, at 1440 and 390 px), and
the Cash Book, Dashboard and Clients screens read well on both. Small items:

- **Dashboard:** the fifth aging card ("More than 90 days late") wraps
  alone onto a second row at 1440 px; the revenue chart's y-axis prints
  full decimals ("2 000 000,00 DT") and its month labels are vertical.
- **Placeholders cut off:** the Cash Book search ("…IBAN, or ident" in the
  old layout at 1440 px) and the Clients search ("Name, phone, ID or ta…").
  French is longer still.
- **TND money inputs show three decimals** (the invoice Discount reads
  "0,000") while storage is two-decimal cents, so a third decimal is rounded
  away silently on save. The millime limitation is documented; the input
  shouldn't invite it.
- **Duplicate fetches:** a full load of `/invoices` requests
  `invoices/outstanding`, `clients/summary` and `/api/version` twice each.
- **Accessibility:** the header's organization switcher has no label, the
  date-range filters are placeholder-only ("From"/"To", "Start date"), and
  antd's icon buttons are named by their English icon name ("search",
  "close-circle") in every language.
- A remembered organization id that no longer exists logs a 404 and a
  console error before the app recovers.

### 6.2 — F185: duplicated document detail pages (Low, Measured)

The six document detail pages repeat the same shell: dirty tracking, the
"save before exporting" Excel/PDF buttons, the save shortcut, the
unsaved-changes warning, the sticky footer and the status actions.

| Page                                        | Lines | `isDirty` | `any` |
| ------------------------------------------- | ----- | --------- | ----- |
| `src/routes/invoices/details.tsx`           | 1,206 | 8         | 20    |
| `src/routes/purchase-orders/details.tsx`    | 841   | 7         | 29    |
| `src/routes/incoming-invoices/details.tsx`  | 791   | 7         | 15    |
| `src/routes/inbound-deliveries/details.tsx` | 669   | 7         | 18    |
| `src/routes/deliveries/details.tsx`         | 658   | 7         | 24    |
| `src/routes/orders/details.tsx`             | 656   | 7         | 19    |

A fix to one (the F112/F113-style regressions of earlier audits) has to be
repeated five times. Extract a `useDocumentForm` hook (dirty flag, save
shortcut, unsaved warning, rethrow-on-failure submit) and a
`DocumentExportButtons`/`DocumentFooter` pair; type the form values per
document. Across `src/` there are 287 uses of `any`, with
`typescript/no-explicit-any` off (`.oxlintrc.json`); turn it on as a warning
per directory as each page is typed. `.oxlintrc.json` also ignores a
`src-tauri/**` that doesn't exist.

### 6.3 — F186: two Cash Book layouts (Low, Decision)

The old and new Cash Book layouts have been switchable per browser since
v3.60.0 (2026-09-30), with the choice pending with the owner. Every Cash
Book change is tested against both. In this audit both worked; the new
layout's counter and till side by side read better at 1440 px, and both
were fine at 390 px. **Decision:** pick one, then delete the other's files
and the switch (`src/CLAUDE.md` names what to remove).

### 6.4 — F187: the CLAUDE.md files (Low, Measured)

| File                      | Size   |
| ------------------------- | ------ |
| `db/CLAUDE.md`            | 137 KB |
| `src/CLAUDE.md`           | 67 KB  |
| `api/CLAUDE.md`           | 32 KB  |
| `CLAUDE.md`               | 30 KB  |
| `cmd/seed-demo/CLAUDE.md` | 9 KB   |

These load into every agent session that touches the directory: about 40k
tokens for `db/` alone. Much of it is history ("as of 2026-09-09…", "found
and fixed…", "a real bug caught live") that git already keeps, and it
drifts:

- Root `CLAUDE.md` counts "the 30 plain `protected()` routes"; `router.go`
  has 31.
- Root `CLAUDE.md` describes `floorToDay` as the function "for stored
  instants", and `db/CLAUDE.md` names it as how the Cash Book register and
  `GetCashMovementDetails` bucket days; nothing calls it any more
  (`db/timezone.go:93`, F188).
- `db/CLAUDE.md` describes `AccountNormalBalance` as part of the accounts
  API; it's unreachable (F188).

**Fix.** Rewrite each file as current rules and pointers (aim: a quarter of
today's size), move the narrative to commit messages or a `docs/` design
note, and keep the per-file lists to one line each.

### 6.5 — F188: dead Go code (Low, staticcheck + deadcode)

- `api/request_log.go:75` `requestID` — unreachable
- `db/timezone.go:93` `floorToDay` — unreachable (see F187)
- `db/account.go:52` `AccountNormalBalance` — unreachable
- `db/templates/gen/tunisia_layout.go:37` field `secondPartyLabel` — unused
- `db/product_bom.go:330` `nextVersionNumber := 1` / `+1` — dead store; the
  real number is computed inside the transaction

Also check the `timeEntries`, `projects` and `tags` tables (desktop-era
leftovers in `legacyUnusedTables`) for a drop migration.

### 6.6 — F189: the Cash Book's sale assembly is untested (Low, Code read)

`netCentsFor`, the gross-to-net conversion and the totals that must agree
with `validateInvoiceTotals` to the cent live in
`src/components/cash-book/use-cash-book.ts`. Only `loan-register-model.ts`
has a test. A mismatch shows up as a 409 at the counter. Extract the sale
assembly into a pure function and table-test it against the server's
rounding cases (half-cent VAT, a discount, a free line).

---

## Watch list — not findings

- **Inventory Valuation difference on a fresh seed.** Before any probe,
  Audit Moto's GL Inventory exceeded the computed stock value by 141,372.68
  TND (0.46%). Landed-cost and production rounding residuals are documented
  as expected; whether they explain this much was not checked. Audit Moto
  is now contaminated by F167's probe, so re-seed before investigating.
- **Debian bookworm's support window** (F181) was not verified.
- **Excel-template exports at volume:** each PDF export starts a LibreOffice
  process with no concurrency limit (`db/pdf_convert.go`); several at once
  on the Pi were not tried.

---

## Explicitly excluded: do not re-raise

Each was checked during this sweep and dismissed with a reason.

- **A JWT under `fc_token` in the browser's `localStorage`.** Found in the
  test browser's profile; it predates the httpOnly cookie (F26) and no
  current code reads or writes it. It expired long ago.
- **antd icon-only buttons "without a name".** The automated check flagged
  them, but each carries its icon's `aria-label`; only the English wording
  is a finding (F184).
- **The other cascading foreign keys.** Clients → invoices, tax rates →
  line items, documents → their lines, payments → applications: every
  parent delete is guarded by a usage or status check. Products are the
  only unguarded one (F167).
- **#508 and #509** (the 2026-10-11 fixes): `setAuditEntity` names the
  invoice and the user; `agingBucketFor` clamps negative day counts
  (`calendarDaysBetween` returns 0 or more). Both read correct.
- **The OIDC flow.** State, nonce, PKCE, a signed single-use state cookie,
  a bounded discovery timeout and the `email_verified` handling are as the
  earlier audits left them. Only the case-sensitive email (F174) is new.
- **Upload size caps.** Every multipart route has a `MaxBytesReader`; F173
  is about what a small file expands to, not the cap.
- **Index coverage.** Every hot predicate checked has an index; query time
  is not where the cost is (Phase 3).
- **The Modal/Drawer-on-module-atom sweep** (2026-10-11 "Not covered"): no
  file that renders a `Modal` or `Drawer` declares a module-level atom.
- Everything the 2026-09-19, 2026-09-24, v3.54, 2026-09-29 and 2026-10-11
  documents exclude, notably: English-only server errors,
  `PUT /api/organizations/{id}` at member level, the shared Dashboard
  payload, dates shown in the viewer's browser zone, and the orphaned
  React-PDF invoice components.

## Not covered

Named so a later pass knows where to start:

- **PDF export.** No `soffice` locally; the LibreOffice path and the
  template layouts were not exercised. Per this repo's rule, that needs the
  Docker image.
- **The Raspberry Pi.** Every timing here is from a Mac.
- **A live SSO sign-in**, the database restore path, and backup scheduling.
- **The browser in French/German and in dark mode**, and keyboard-only use
  of the Cash Book beyond the search list.
- **seed-demo** beyond `--reset` (F178).

## Verification

Beyond the per-phase gates:

- **F167:** a DB test that a product with a movement, a serial, a document
  line or a BOM use can't be deleted (409, naming the reference) and an
  unused one can; a schema tripwire over every table referencing
  `products`. Archive a product and confirm it leaves the pickers but stays
  on old documents, the Inventory page and the valuation report.
- **F168:** a DB test that changing an in-use rate's percentage is refused
  and an unused one's isn't; in the app, follow the "new rate" path and
  confirm the Tax Summary for the old period is unchanged.
- **F169:** concurrent `CreateInvoice` calls get distinct numbers; a
  duplicate number is refused (409); the migration runs on a copy of
  production after Phase 0's cleanup.
- **F170:** extend `domainRouteRoles`/`TestDomainRoleRouteCoverage` for the
  chosen gates; as a `cashbook` member, the probes above answer 403.
- **F171:** `curl -I` an asset: `Cache-Control: … immutable` and
  `Content-Encoding: br|gzip`; `index.html` is `no-cache`; a second browser
  load transfers no JavaScript (Playwright `transferSize` 0). A test that
  sends a create through the **full `main.go` handler chain** with
  `Accept-Encoding: gzip` and checks the history row still names its
  document (guards the placement rule above). Then the same `curl` checks
  against production after deploy.
- **F172:** record a counter sale and a collection; the button frees right
  after the POST and no request fetches the full payment history.
- **F173:** the 1 MB bomb is refused (or row-capped) on every upload route,
  memory stays flat, and no `excelize-*` file is left in the temp directory.
- **F174:** sign in with different capitalization and a trailing space;
  creating a second account differing by case is refused.
- **F175:** with a short token lifetime in a test, an active session is
  renewed; an expired one returns to the same page after sign-in.
- **F177:** only after measuring on the Pi.
