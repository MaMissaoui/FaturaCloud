# FaturaCloud Re-Audit — Delta Sweep (v3.54.1 → v3.58.3 + #428–#431)

**This is a delta audit, not a full-repo sweep.** It covers everything that
landed on `main` between `v3.54.1` (where the
[v3.54 delta](audit-plan-2026-09-24-v3.54.md) stopped) and `dcb13e1` (the
last lint clean-up PR, #431): 38 commits across 160 files, excluding
catalogs and docs. Numbering continues at **F156**; no document under `docs/`
uses F156 or later.

What the delta contains:

- **Cash Book payment products** (#396, #425): `Payment.products`/
  `wholeInvoice` in `db/payment.go`, `GET /api/payments/{id}/invoice-lines`
  (`db/payment_invoice_lines.go`) and the Payment history popover.
- **Organization time zone** (#402, migration `0092`): `db/timezone.go`,
  day/month/year reads across exports, numbering, e-invoices and cash reports.
- **Quantity-only inventory valuation** (#405, migration `0093`), the **stock
  count upload** (#406, `db/mass_data_stock.go`), **Cash Book stock-out** (#407,
  migration `0094`, `db/invoice_stock.go`) and the numbering fix for orgs with
  no invoice format (#408).
- **Product families** (#412, migrations `0095`/`0096`) and **series-based
  product codes** (#411, #422).
- **Session fixes** (#416, #419, #421): `logoutAtom`, the no-organization
  screen, forgetting the remembered organization on a 401.
- **Lint clean-up** (#428–#431): `useLoadOnPath`, `useFetch`, render-time
  state resets; `oxlint` from 63 warnings to 0.
- Smaller: org reset warning (#399), drawer layout (#400), seed-demo retail
  stock (#410), runner pin (#427).

**Baseline health** at `dcb13e1`, measured before writing this document:

| Check                          | Result                                                                                                                                                           |
| ------------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `go vet ./...`                 | Pass                                                                                                                                                             |
| `gofmt -l .`                   | Pass                                                                                                                                                             |
| `go test -race -count=1 ./...` | Pass (all packages)                                                                                                                                              |
| `pnpm lint` (tsc + oxlint)     | Pass, **0 warnings** (was 63 at `v3.58.3`)                                                                                                                       |
| `pnpm test`                    | Pass (99/99, 16 files)                                                                                                                                           |
| `pnpm build`                   | Pass                                                                                                                                                             |
| `pnpm extract`                 | 0 missing in de/fr                                                                                                                                               |
| `pnpm audit --prod`            | No known vulnerabilities                                                                                                                                         |
| `govulncheck ./...`            | No called vulnerabilities. `-show verbose` lists one module-level advisory, `GO-2026-5932` (`x/crypto/openpgp` unmaintained, no fix); the package isn't imported |

**Production data check** (read-only `sqlite3 -readonly` against the pimi-01
volume, 2026-09-29, schema version 96, not dirty):

| Check                                                                 | Result                                                                                       |
| --------------------------------------------------------------------- | -------------------------------------------------------------------------------------------- |
| `organizations.timezone`                                              | Atlas Moto: `Africa/Tunis`; ELECTRO MISSAOUI: `Africa/Tunis`; **Ben Salah: NULL** (see F156) |
| Cash Book sales (`movesStock = 1`)                                    | 1,245                                                                                        |
| Sent/paid sales with a stock line but no stock-out movement           | 0                                                                                            |
| Stock-enabled products whose `stockQuantity` ≠ sum of their movements | 0                                                                                            |
| Stock-enabled products below zero                                     | 0                                                                                            |
| Product families                                                      | 0 (ELECTRO's 15 are pending the manager's review, by decision)                               |

**How this document was produced.** One sequential pass by the authoring
model, with no parallel sweeps. **#428–#431 were written and reviewed by the
same model in the same session**, so this is not an independent review of
them; they were browser-verified screen by screen when they merged. Labels:

- **Confirmed** means reproduced: F156 from the production row plus the code
  path; F157 by running the server's own day arithmetic across zones.
- **Code read** means established by reading the code, with the server-side
  half already covered by an existing test (F158), or by reading the router
  (F159).

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
- F159 needs a product decision before any code is written.
- Setting Ben Salah's time zone on production (F156) is an owner action, not
  part of any PR. **Never touch Société ELECTRO MISSAOUI.**
- The 2026-09-19, 2026-09-24 and v3.54 "Explicitly excluded" lists still bind,
  and so does this document's own list at the end.

**Status:** 6 findings (F156–F161), none fixed yet.

---

## Severity overview

| #    | Finding                                                                                                                                                                          | Area                  | Severity | Phase |
| ---- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------- | -------- | ----- |
| F156 | Organizations created anywhere but the Organizations drawer get no time zone, so they read calendar days in UTC. That's the bug #402 fixed. Live on prod: Ben Salah              | Dates / correctness   | Medium   | 1     |
| F157 | A day picked in the Cash Book register or the Daily Cash Movements report is read as the next day for zones at UTC+12 and beyond (New Zealand, Fiji, Samoa, Tonga, Kiribati)     | Dates / correctness   | Low      | 1     |
| F158 | The Cash Book offers serialized (and, under perpetual valuation, uncosted) stock products that the server refuses. The cashier finds out only at checkout                        | Cash Book UX          | Low      | 2     |
| F159 | Payments reads now return sales invoice content (product names, quantities, line amounts) to every member, including roles the section guard keeps out of Sales                  | Authorization / scope | Low      | 2     |
| F160 | CLAUDE.md drift: the root file misstates the valuation switch rule, and `src/CLAUDE.md` still describes the pre-#429 fetch pattern and never mentions `useFetch`/`useLoadOnPath` | Docs                  | Low      | 3     |
| F161 | Lint is at 0 warnings but CI doesn't fail on new ones, so the clean-up can erode silently                                                                                        | CI                    | Low      | 3     |

**Already tracked, no F-number:** #432, the detail pages that bounce from
`/…/new` back to the previous document (found during #430's browser checks;
the code predates this delta).

---

## Phase 1

### 1.1 — F156: organizations created outside the drawer have no time zone (Medium, Confirmed)

**What happens.** Only `src/routes/organizations/index.tsx`'s edit drawer
prefills a zone for a new organization (`openNew` → `defaultTimezone()`).
Every other creation path sends none, and the server deliberately doesn't
infer one from the country (migration `0092`'s backfill was one-time):

- `src/routes/organizations/new.tsx`: the first-organization screen for a new
  user **and** the target of the header's "New organization" option.
  `handleSubmit` posts `CreateOrganization` with no `timezone`.
- `cmd/seed-demo/masterdata.go`: the `db.CreateOrganizationRequest` has no
  `Timezone`.
- Any API client.

An organization with no zone reads calendar days in UTC (`orgLocation`). A
date picked in a DatePicker is stored as local midnight, so for any zone east
of UTC it is read as the previous day. That's the exact symptom #402 fixed:
PDFs/Excel exports, `{day}`/`{month}`/`{year}` number tokens, FEC/DATEV and
the daily cash buckets all show the day before.

**Amplifier.** `db/einvoice.go`'s `formatMillis` keeps its older ±12h
rounding for a no-zone org. For a Tunis (UTC+1) date picked as 29 Sep
(stored 28 Sep 23:00 UTC), the PDF says **28 Sep** but the e-invoice says
**29 Sep**: the two documents for the same invoice disagree.

**Evidence.** On production, `Établissement Ben Salah Électroménager`
(`country_code = 'TN'`) has `timezone = NULL`. It was re-seeded on
2026-09-27, after `0092` ran, through seed-demo. The two older orgs got
`Africa/Tunis` from the backfill.

**Fix.**

1. `organizations/new.tsx`: send `timezone: defaultTimezone()` (the same
   helper the drawer uses). Consider adding the drawer's "Time zone" select to
   this form so the value is visible, not only implied.
2. seed-demo: set `Timezone` on the org request (`Africa/Tunis` for the
   Tunisian scenarios).
3. Test: a component test on `new.tsx` asserting the request carries a zone.

**Owner action (not part of the PR).** Set Ben Salah's time zone to
`Africa/Tunis` in the Organizations drawer on production. It's a demo org;
ELECTRO MISSAOUI already has its zone and must not be touched.

### 1.2 — F157: a picked day reads as the next day at UTC+12 and beyond (Low, Confirmed)

**What happens.** Day-based queries send `calendarDayMs` (`src/utils/date.ts`:
UTC noon of the picked date). The server decodes it with
`floorToDay(ms, loc)`, i.e. "which day is this instant in the org's zone".
UTC noon is the same calendar day only for offsets strictly below +12h. At
+12h and beyond it's already the next day.

Reproduced with the server's own arithmetic, for a picked 2026-09-29:
`Africa/Tunis`, `America/Los_Angeles` and `Pacific/Pago_Pago` → 29 Sep
(correct). `Pacific/Auckland` (NZDT, +13 since 27 Sep), `Pacific/Apia`,
`Pacific/Tongatapu` and `Pacific/Kiritimati` → **30 Sep**. New Zealand standard
time (+12) and Fiji (+12) hit it too, since UTC noon is their midnight.

**Scope.** Every `calendarDayMs` consumer:

- `GetDailyCashMovements` (`db/gl_reports.go`): the Daily Cash Movements
  report and the Cash Book register panel's Opening/In/Out/Closing.
- `GetCashMovementDetails` (`db/cash_movement_details.go`): the Cash Book's
  per-transaction table.
- `GenerateDailyCashMovementsExport` (`db/report_export.go`), which calls both.

The reporting pages (Revenue Trend, Sales by Client/Product, Purchases by
Vendor, Tax Summary) send browser-local `startOf/endOf("day")` instants and
decode no day server-side, so they are out of scope.

**Fix.** Decode a `calendarDayMs` by its **UTC date**, then build local
midnight from that date:
`t := time.UnixMilli(ms).UTC(); time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)`.
Put it in `db/timezone.go` as a named helper (e.g. `calendarDayStart`) and use
it in both functions. Test with `Pacific/Kiritimati`, `Pacific/Auckland` and
`Africa/Tunis`. `floorToDay` stays correct for its other job, bucketing
stored instants.

Real exposure today is nil (every prod org is in UTC+1), which is why this is
Low.

---

## Phase 2

### 2.1 — F158: the Cash Book offers products it will refuse (Low, Code read)

**What happens.** `src/routes/cash-book.tsx`'s `sellableProducts` only drops
`category === "component"`. The server's stock-out
(`resolveInvoiceStockLines`, `planInvoiceStockOut` in `db/invoice_stock.go`)
refuses a sale with a 409 for:

- a **serialized** stock product (the counter has no serial picker); and
- under **perpetual** valuation, a stock product with **no cost basis**.

Both refusals are covered by `TestCashSaleRefusesSerializedAndNonPositiveStockLines`
and `TestCashSalePerpetualWithoutCostBasisIsRejectedBeforeAnyWrite`, and the
messages say what to do. But the cashier only learns it after filling the
whole sale, at checkout. The picker's `onSelect` already warns about negative
stock, so there is a natural place for these warnings.

**Fix.** Exclude serialized stock products from the counter's picker, or keep
them but disabled with the reason. For perpetual organizations, warn on
selection when the product has no cost basis, using the same rule the server
uses. Don't block: quantity-only orgs never need a cost.

### 2.2 — F159: payments reads now carry sales invoice content (Low, Code read, product decision)

**What happens.** The section guard (`api/sections.go`) keeps `purchasing`
and `accounting` members out of Sales routes, but deliberately leaves every
payments route at membership level, because the screens those roles keep
(PaymentPanel, the Cash Book) read payments. This delta made two of those
membership-level reads return sales invoice content:

- `GET /api/organizations/{orgId}/payments`: each payment's `products` (the
  names of what it paid for).
- `GET /api/payments/{id}/invoice-lines` (new): each paid invoice's lines,
  with product, SKU, quantity and line amount.

A `purchasing` member can therefore read sales line items that
`GET /api/invoices/{id}/line-items` refuses them.

**Options.** (1) Accept it under the existing "payments stay membership-level"
rule and say so in `api/sections.go`'s file comment, as F150 was accepted. (2)
Section-gate `/payments/{id}/invoice-lines` to Sales ∪ Cash Book and blank
`products` for other roles. Option 1 is the likely answer, since both roles can
already see the payment amounts and invoice numbers; it just needs to be
written down.

---

## Phase 3

### 3.1 — F160: CLAUDE.md drift (Low)

- **Root `CLAUDE.md`, "Inventory valuation is per organization":** "the mode
  can only change before any stock is recorded" is wrong. The rule in
  `checkInventoryValuationSwitch` is asymmetric: perpetual → quantity-only is
  allowed with stock movements as long as nothing is posted to the Inventory
  account (the documented escape route for uncosted stock), and quantity-only
  → perpetual is refused once any movement exists. Replace the sentence with
  that.
- **`src/CLAUDE.md`, `src/routes/reporting/` entry:** still says the pages
  follow "the `ap-aging.tsx`/`profit-and-loss.tsx` pattern of fetching
  directly in a `useEffect`". Since #429 they fetch through `useFetch`. Nothing
  in `src/CLAUDE.md` mentions `src/hooks/useFetch.ts` or
  `src/hooks/useLoadOnPath.ts`. Add a short "Data loading" note: list pages
  use `useLoadOnPath`; reports, per-record counts and drawer lookups use
  `useFetch` (null key = nothing to load, `reload` for Retry/refresh); state
  that belongs to the shown record resets during render on the changed value,
  not in an effect. Without it, the next page written will reintroduce the
  pattern #428–#431 removed.

### 3.2 — F161: nothing keeps lint at zero (Low)

`package.json`'s `lint` is `tsc --noEmit && oxlint src/`, and CI runs
`pnpm lint`. `oxlint` exits 0 on warnings, so a new
`react(set-state-in-effect)` passes CI. Now that the count is 0, add
`--deny-warnings` to the script (or `"--max-warnings", "0"`), so the next
warning fails the build where it's introduced.

---

## Verified without a finding

- **New routes' authorization.** Product families: list/update/delete use
  `orgMemberProtected` with an id→org resolver, and create checks
  `requireOrgMember` inline; all four are in `api/cross_org_test.go`'s
  tripwires. Membership-level matches units of measure and the documented
  master-data rule. The stock count import/export sit under
  `stock-movements`, so the Inventory section guard applies, the same as manual
  stock movements. `/payments/{id}/invoice-lines` resolves the org from the
  payment (see F159 for scope).
- **Cross-org family ids.** `CreateProduct`/`UpdateProduct` call
  `checkProductFamilyOwnership`; `familyId` is three-state on update, so an
  older client or a 15-column sheet can't wipe it.
- **Cash Book stock-out state machine.** Stock presence follows sent/paid;
  draft/cancelled reverses exactly what was posted (net per product, never
  re-derived from current lines); sent↔paid moves nothing and doesn't
  re-validate products; a concurrent reversal is caught in-tx; delete and
  re-lining are refused while stock stands, including zero-total sales; and
  `DeleteStockMovement` refuses a sale's own movements. The COGS reversal is
  dated `invoice.Date`, the same convention as every other reversal path.
- **Valuation switch rule.** The code's asymmetric rule is sound (only the
  docs are wrong, F160). Every posting path uses the org's default Inventory
  account, so checking that account is sufficient.
- **Stock count upload.** Absolute counts; each row re-reads the product, so a
  product listed twice doesn't double-move; blank skips, only a literal 0
  empties; serialized products are refused.
- **Time-zone adoption.** Every document-number generator
  (`GenerateNextDocumentNumberTx`, `PreviewNextDocumentNumber`,
  `CreateCashSale`) renders date tokens in the org's zone. The remaining
  `.Year()`/`.Month()` calls operate on already-localized times.
  `time/tzdata` is embedded.
- **Payment invoice lines.** `loanLinesQuery` has no state filter, so paid and
  cancelled invoices keep their lines in the panel.
- **Org reset and its warning.** `product_families` is in the master-data
  reset list and in `usage-breakdown.ts`, whose full `Record` type fails the
  type-check if a count is added without a group.
- **Foreign keys.** The DSN sets `foreign_keys(1)`, so deleting a family
  nulls `products.familyId` (tested).
- **#428–#431** (see the note on independence above): browser-verified at
  merge, per page and per drawer. The `useFetch` null-key regression the
  pre-merge review caught is fixed and has a test that fails without the fix.

## Watch list: suspicions, not confirmed

- `GetPayments` loads the product names of every invoice the organization
  has ever received a payment for, on each call, alongside an already
  unpaginated payments list. Fine at today's volume (1,245 Cash Book sales on
  ELECTRO); revisit if the Cash Book screen slows down.
- Migration `0096` would fail on a database holding two families whose names
  differ only in ASCII case. Production passed it; any other deployment that
  ran `0095` for a while could hit it.

## Explicitly excluded: do not re-raise

- **Re-uploading an old stock count file resets stock to that count.** That's
  inherent to absolute counts, and the upload's design documents it.
- **Reversal entries dated on the source document's date** (a closed period
  refuses the reversal): codebase-wide convention, not specific to Cash Book
  COGS.
- **Frontend dates shown in the viewer's browser zone** rather than the
  org's: pre-existing and accepted with #402.
- **Server error messages in English only:** accepted product decision (root
  `CLAUDE.md`).
- Everything the 2026-09-19, 2026-09-24 and v3.54 documents exclude.
