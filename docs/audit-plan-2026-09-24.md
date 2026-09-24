# FaturaCloud Re-Audit — Delta Sweep (2026-09-24)

**This is a delta audit, not a full-repo sweep.** It covers everything that
landed on `main` between `433e145` (the merge of #298, which closed out the
2026-09-19 plan's Phase 5) and `73737ea` (`v3.53.1`): 206 commits and 183
files. It also re-checks the F96–F139 fixes wherever this delta rewrote the
code they live in. Numbering continues at **F140**.

The 2026-09-19 plan (F96–F139) is fully remediated. Phases 1–5 shipped as
#293–#298. That document's header still says "Not yet remediated" (see F146).

What the delta contains, and what this sweep covered:
- **Focused per-role views, enforced server-side** (#375/#377): `api/sections.go`,
  the section guard in `api/middleware.go`, `power_user` (migration `0086`) and
  `src/layouts/role-menu.ts`.
- **Invoice discount (remise) and amount-in-words** (#377, migration `0088`):
  `db/invoice_totals.go`, `db/gl_posting.go`, `db/amount_in_words.go`, the
  invoice form.
- **Dual default/Tunisia document layouts** (#379, migration `0089`):
  `db/templates/gen/*`, `db/templates_embed.go`, `db/xlsx_export*.go`.
- **Cash Book iterations** (#358–#374, #382): the pick-list, the loan status by
  line item, the payment-history export, and the client counter fields (migration
  `0087`).
- **Performance indexes** (migration `0085`), the correlated-subquery rewrite of
  `db/dashboard.go`, and the UI layout audit (#380).

**Baseline health**, measured at `73737ea` before any change:

| Check | Result |
|---|---|
| `go vet ./...` | Pass |
| `gofmt -l .` | Pass |
| `go test -race -count=1 ./...` | Pass (all packages) |
| `pnpm type-check` | Pass |
| `oxlint src/` | Warnings only: the pre-existing `react(set-state-in-effect)` set |
| `pnpm test` | Pass (63/63, 10 files) |
| `pnpm build` | Pass |
| `govulncheck ./...` | Only the CI-allowlisted Excelize advisory (`GO-2026-6452`, no fix available) |

**How this document was produced.** One sequential pass by the authoring model,
with no parallel sweeps. Findings are marked **Confirmed** only when they were
reproduced: F140 through the real router with real `cashbook`, `sales`,
`purchasing` and `general` members, and F141 and F142 through the real DB layer.
F145 was confirmed by reading the SQL: the `UPDATE` statement's bare `= ?`
assignments leave no room for doubt. The throwaway reproduction tests were deleted
afterwards. The route-access sweep was systematic, not grep-based: every pattern
in `api/router.go` was run through `routeAllowedForRole` for each restricted
role. Every denied pattern was then traced to its `src/api` function and to
every page or component that calls it (see "Section-guard sweep" below).

**Instructions for the executing model:**
- This document is an audit, not a remediation. No code was changed while
  producing it.
- Use one feature branch and PR per phase, and never push to `main`. Use
  conventional commits with no Claude attribution lines.
- After every phase, `go vet ./... && go test -race ./...` and
  `pnpm lint && pnpm build` must pass.
- Schedule F140 first. It is a full loss of function for the `cashbook` role's
  core flow: blocked at the payment step since `v3.29.0` and at the
  invoice-load step since `v3.52.0`. F145 is silent data loss and should
  follow it.
- The 2026-09-19 "Explicitly excluded" list still binds, and so does this
  document's own list at the end.

**Status:** 10 findings (F140–F149). F140 fixed in #384; the rest are not yet remediated.

---

## Severity overview

| # | Finding | Area | Severity | Phase |
|---|---------|------|----------|-------|
| F140 | A `cashbook`-only member cannot settle a loan from the Cash Book. All three calls in the flow 403: `GET /api/invoices/{id}` (section guard, `v3.52.0`), `POST /api/payments` (F104's accounting gate, `v3.29.0`) and `PATCH …/state` (role redesign) | Authorization / regression | **High** | 1.1 |
| F141 | Tax Summary's output VAT ignores the invoice discount, so it overstates base and tax against the posted GL | Accounting / reporting | Medium | 1.2 |
| F142 | The UBL/Peppol e-invoice ignores the invoice discount, so `PayableAmount` exceeds the invoice total | E-invoicing / compliance | Medium | 1.3 |
| F143 | Sales-by-Product revenue is gross of the invoice-level discount and disagrees with the P&L | Reporting | Low | 1.4 |
| F144 | Amount-in-words pluralizes `cents` / `quatre-vingts` before `mille` (200 000 → "Deux Cents Mille") on a printed legal document | Print correctness | Low | 1.5 |
| F145 | A client Excel mass-maintenance **import wipes** the migration-`0087` fields (`phone2`, `phone3`, `guarantor`, `address`) on every updated row, and the export omits them | Master data / data loss | Medium | 1.6 |
| F146 | Doc drift: root `CLAUDE.md` says 8 export / 10 direct `mux.Handle` routes (actual 9 / 11), `base.tsx` still calls the cashbook redirect "not an authorization boundary", and the 09-19 plan still reads "Not yet remediated" | Docs | Low | 2.1 |
| F147 | The dashboard is role-blind: purchasing and accounting members see sales data (cashbook only through the API), and its invoice rows link to a page their route guard bounces | UX / authz intent (decision) | Low | 2.2 |
| F148 | `PaymentPanel` shows Record/Void payment controls to `sales` and `purchasing`, which F104 made 403 | UX | Low | 2.3 |
| F149 | The Document Numbering hint `{number:4}` fails Lingui's ICU compile in all three locales (printed on every `pnpm build`) | i18n | Low | 2.4 |

---

## 1. Correctness

### 1.1 — F140: the cashbook role cannot settle a loan (Confirmed, regression)

The Cash Book's loan-settlement flow (`src/routes/cash-book.tsx`) makes these calls:
1. `openPayment` calls `GetInvoice(invoiceId)` → `GET /api/invoices/{id}`
   (`:980-987`) to load the invoice into `PaymentPanel`.
2. `PaymentPanel` calls `GET /api/invoices/{id}/payments` and `POST /api/payments`.
3. `handleSettled` calls `UpdateInvoiceState(id, "paid")` →
   `PATCH /api/invoices/{id}/state` (`:1008-1019`).

`api/sections.go`'s `routeSections` classifies every `invoices/...` path as
`sectionSales` except the `/payments` suffix. `sectionRoles[sectionSales]` is
`{"general", "sales"}`, so step 1 now returns 403 for a `cashbook` member.
`openPayment` catches the error, shows "Failed to load invoice", and the panel
never opens. Before the section guard landed (`969fb42`, #377, `v3.52.0`),
`GET /api/invoices/{id}` was `orgMemberProtected`, so this is a regression.
Step 3 is a separate, **pre-existing** 403: that route has been
`orgRoleProtected(..., []string{"sales"}, ...)` since the role redesign. So even
before #377, a pure-cashbook user settling a loan in full got the "update it
manually from the invoice page" error, for a page that role cannot open.

Step 2 is blocked too, and for longer. F104's remediation (`8e321f5`, #293,
first released in `v3.29.0`) added
`requireOrgRole(w, r, req.OrganizationID, "accounting")` to `createPayment`
(`api/payments.go:45-50`). Its allowed set is admin/power_user/general/
accounting, which leaves `cashbook` out. `api/CLAUDE.md`'s "Payments (F104)"
entry records that as a deliberate accounting-tier boundary. But
`api/sections.go`'s file comment keeps every payments route shared *because*
"the Cash Book's loan settlement both read[s] and write[s] payments". The two
decisions contradict each other, and nobody reconciled them for the one role
whose screen depends on it. So a pure-cashbook user has been unable to settle a
loan since `v3.29.0`. Since `v3.52.0` the flow fails even earlier, at step 1.
Note that `POST /api/cash-sales` still records a sale's *upfront* payment,
because that path runs inside `db.CreateCashSale` and never touches
`createPayment`. Only the later settlement of a loan is broken.

Reproduced through `NewRouter` with real members of the invoice's org:

```
cashbook   GET   /api/invoices/inv-1          -> 403 {"error":"forbidden"}
cashbook   GET   /api/invoices/inv-1/payments -> 200 []
cashbook   POST  /api/payments                -> 403 {"error":"forbidden"}
cashbook   PATCH /api/invoices/inv-1/state    -> 403 {"error":"forbidden"}
sales      POST  /api/payments                -> 403 (F104, deliberate; see F148)
purchasing POST  /api/payments                -> 403 (F104, deliberate; see F148)
general    POST  /api/payments                -> passes authz (409 on the dummy body)
```

**Why the tests stayed green.** `TestSectionRoleAccess` (`api/sections_test.go`)
lists `GET /api/invoices/{id}/payments` among the shared routes but never
`GET /api/invoices/{id}`. `role-menu.test.ts` tests only the menu, not the
screens' API dependencies. Of the shared screens, only the Products form
(`bomAllowed`, F111's follow-up) was checked against the guard.

**Fix direction.** Record the reconciled decision in `api/CLAUDE.md` first.
- Add `"cashbook"` to `createPayment`'s `requireOrgRole` call. Ideally also
  check that every application targets an `invoice` document (not a bill), so
  the till role can settle customer loans but not pay vendors. Leave
  `…/void` accounting-tier.
- Let `cashbook` read a single invoice. `routeSections` already supports more
  than one section per route, so return `{sectionSales, sectionCashbook}` for
  `GET invoices/{id}` (and `…/line-items` if `PaymentPanel` ever needs it). Keep
  the list and write routes sales-only.
- Decide step 3 explicitly:
  - (a) Move the auto-`paid` transition server-side into the payment
    application, scoped to invoices created through `POST /api/cash-sales`. This
    is the cleanest fix, and it removes the client-side follow-up call that
    `db/cash_sale.go`'s doc comment already justifies.
  - (b) Allow `cashbook` on `PATCH …/state` only for the `sent → paid`
    transition.
- Add the calls above to `TestSectionRoleAccess`, and a `POST /api/payments`
  as `cashbook` case to the domain-role tests. Add an end-to-end test
  that drives them as a `cashbook` member through the real mux, mirroring
  `TestDomainRoleEnforcement`.

**Resolution (#384).** The owner
decided the Cash Book must settle loans **per invoice line**: one line per
payment, and an amount above that line's outstanding balance is rejected.
Implemented as a dedicated `POST /api/cash-sales/{id}/payments`
(`orgRoleProtected` with `cashbook`, in the cashbook section).
`db.CreateCashSalePayment` posts the cash receipt into the register and
records a `payment_applications` row carrying the new
`invoiceLineItemId` (migration `0090`). It checks the line's balance before
the transaction and again inside it, and flips the invoice to `paid` in the
same transaction once it clears. `POST /api/payments` stays accounting-tier,
and the Cash Book no longer calls `GET /api/invoices/{id}` or
`PATCH …/state`. `GetLoanStatus` now counts line-targeted payments against
their own line and spreads invoice-level ones by line amount, capped
(`allocateInvoiceLines` / `allocateCapped`). Covered by
`api/cash_sale_test.go` (a real `cashbook` member through the router) and
`db/cash_sale_payment_test.go`.

### 1.2 — F141: Tax Summary output VAT ignores the discount (Confirmed)

`getOutputTaxSummary` (`db/sales_reports.go:241-268`) computes each group's
base and tax from `SUM(ili.quantity * ili.unitPrice …)`. It never reads
`invoices.discountAmount`. The doc comment directly above it (`:218-223`) says
"There is no discount column … so quantity*unitPrice is the correct taxable
base — the same base the GL itself computes". Migration `0088` made both halves
false. The discount is invoice-level, and `buildInvoiceGLLines` subtracts each
tax group's proportional share before posting.

Reproduced with one 20% line of 2 × 10.00 and a 5.00 discount (subtotal 2000,
net 1500, tax 300, total 1800):

```
TaxSummary output: base=2000 tax=400
GL posted tax credit = 300
```

This report feeds the VAT return, so every discounted invoice overstates
output VAT. **Fix:** allocate `discountAmount` across each invoice's tax groups
exactly as `validateInvoiceTotals` / `buildInvoiceGLLines` /
`buildTaxBreakdownRows` do. Do it in Go over the per-invoice group rows, since
the proportional split with a remainder isn't comfortable in SQLite. Then
update the doc comment. Add a test with two tax rates that compares the
report's tax against the posted `journal_lines` tax credits.

### 1.3 — F142: the e-invoice ignores the discount (Confirmed)

`buildUBLInvoice` (`db/einvoice.go:184-275`) builds `LineExtensionAmount`,
`TaxExclusiveAmount`, every `TaxSubtotal` and `PayableAmount` from the line
items alone, and `einvoice.go` never mentions `DiscountAmount`. Reproduced on the
invoice from F141:

```
invoice.Total=1800  UBL TaxExclusive=20.00 TaxAmount=4.00 Payable=24.00
```

The recipient is billed 24.00 for an 18.00 invoice. The discount `Form.Item`
in `src/routes/invoices/details.tsx` is not org- or layout-gated, so every
organization that uses e-invoicing (the EU XRechnung/Peppol path) can hit this.
It is not only a Tunisian issue.

**Fix:** emit a document-level `cac:AllowanceCharge` (`ChargeIndicator=false`)
for each VAT category that carries a discount share. Set
`LegalMonetaryTotal/AllowanceTotalAmount`, make `TaxExclusiveAmount` =
`LineExtensionAmount − AllowanceTotalAmount`, and compute each `TaxSubtotal`'s
`TaxableAmount` net of its share. That satisfies EN 16931 BR-CO-13 and the
per-category BR-S-08 rules. Each document-level allowance also needs its own
`cac:TaxCategory` (BR-32) and an `AllowanceChargeReason` or reason code
(BR-33). Without them the fix swaps one validation failure for another. Test
that `PayableAmount == invoice.Total` on a discounted invoice with two rates.

The builder also ignores `fiscalStampAmount`. That only affects an
organization using both the Tunisian stamp and the EU e-invoice export, which is
unlikely. Either add the stamp as a document-level `ChargeIndicator=true`
charge or exclude it explicitly.

### 1.4 — F143: Sales-by-Product is gross of the discount (Confirmed by reading)

`GetSalesByProduct` (`db/sales_reports.go:127-140`) sums
`quantity * unitPrice`. The GL and the P&L book revenue net of the
proportional discount, so a product sold on a discounted invoice shows more
revenue here than was recognized. (Sales-by-Client uses `i.total`, which is
tax-inclusive, so the two reports never shared a basis. That is pre-existing
and documented.) A sweep of every `unitPrice` aggregate in `db/` found only this query and Tax
Summary (F141). `GetLoanStatus`'s `netLine` is used only as an allocation
weight over the invoice total, so it is already discount-correct. This is Low
because the report is a ranking, but it should
either allocate the discount per line, as `GetLoanStatus` already does with
invoice totals, or say in its UI and doc comment that it is gross of discounts.

### 1.5 — F144: French amount-in-words plural agreement (Confirmed by reading)

`frenchNumber` (`db/amount_in_words.go:84-120`) spells the thousands group
with `frenchBelow1000(thousands) + " mille"`. `frenchBelow1000` adds the plural
`s` to `cent` (and `frenchBelow100` to `quatre-vingts`) whenever the group ends
in it. French drops that `s` before `mille`. So 200 000 prints "Deux Cents
Mille" instead of "Deux Cent Mille", and 80 000 prints "Quatre-Vingts Mille"
instead of "Quatre-Vingt Mille". The `s` is still correct before `millions` and
`milliards`, which are nouns. This line is printed on a legal invoice ("Arrêtée
la présente facture à la somme de …"). `TestFrenchNumber` has no case in the
thousands band that ends in 80 or in a round hundred. **Fix:** pass a
"followed by mille" flag, and add 80 000 / 200 000 / 300 080 cases.

### 1.6 — F145: client mass-maintenance import wipes the counter fields (Confirmed by reading)

`UpdateClient` (`db/client.go:169-191`) is a full-row replace. Every column,
including `phone2 = ?, phone3 = ?, guarantor = ?, address = ?`, is a bare
assignment with no `COALESCE`. `clientsMassDataSpec.ImportRow`
(`db/mass_data_clients.go:81-92`) builds its `UpdateClientRequest` from the 18
spreadsheet columns, which predate migration `0087`. So the four counter
fields are always `nil`, and **every existing client row in an import has its
second and third phone, guarantor and address set to NULL.** The export omits
the same four columns, so an export → edit → re-import round-trip, the whole
point of mass maintenance, silently erases the Cash Book's customer-contact
data. For the Tunisia counter use case, the guarantor is the collection
contact for an open loan.

The client drawer is *not* affected. Its collapsed Cash Book panel uses
`forceRender` (`src/components/clients/form.tsx:333`), so a `PUT` always
carries the fields.

**Fix:** add the four columns to `Headers()`, the export row and `ImportRow`.
Add a round-trip test in `db/mass_data_test.go` that seeds the four fields,
exports, re-imports the unmodified workbook, and asserts they survive. Also
consider making the import merge onto `existing` rather than replace, so the
next column added to `clients` can't reopen this.

---

## 2. Docs, UX

### 2.1 — F146: doc drift

- Root `CLAUDE.md` (Cross-Cutting Invariants ▸ Authorization) says "the 10
  routes registered directly via `mux.Handle` (the 8 document/report export
  routes … and the 2 restore routes)". `api/router.go` has **11**: the 6
  document exports, the 3 Cash Book report exports (`daily-cash-movements`,
  `loan-status`, `payment-history`, the last added after that sentence was
  written) and the 2 restores. The "29 plain `protected()` routes" count is
  still correct. Check `api/CLAUDE.md` for the same numbers.
- `docs/audit-plan-2026-09-19.md`'s `**Status:**` line still says "Not yet
  remediated". Update it to point at #293–#298.
- `src/layouts/base.tsx:161-165` says the cashbook redirect is "purely a UI
  restriction — reads stay membership-level server-side, so this is not an
  authorization boundary". Since #377 the section guard *is* a server-side
  boundary for that role.
- `db/sales_reports.go:218-223`'s "There is no discount column" comment is
  false, and is fixed as part of F141.

### 2.2 — F147: the dashboard is role-blind (decision)

`GET /api/organizations/{orgId}/dashboard` is deliberately membership-level
(`api/sections.go`'s file comment), and `src/routes/dashboard.tsx` has no role
awareness. `ROLE_MENU` gives the Dashboard to `purchasing` and `accounting`.
Those roles see sales revenue, outstanding
receivables and top clients/products. The overdue-invoice rows `navigate()` to
`/invoices/{id}` (`:301-305`), which `isRouteAllowedForRole` bounces for
purchasing and accounting. (`cashbook` is redirected away from `/dashboard`
by `base.tsx`, so for that role the exposure is API-only.) Choose one of these and record the choice in
`api/CLAUDE.md`:
- (a) Keep the dashboard shared, but make its widgets and row links
  role-aware.
- (b) Split the payload per section.

Either way, stop rendering links a role can't follow.

### 2.3 — F148: PaymentPanel offers actions the role can't perform

`src/components/payments/payment-panel.tsx` has no role awareness. The
invoice and bill detail pages that `sales` and `purchasing` keep render its
Record payment and Void controls. F104's deliberate accounting-tier gate
(`api/CLAUDE.md` "Payments (F104)") then 403s both, and the user sees a
generic error. Hide or disable the controls for roles outside
admin/power_user/general/accounting (plus `cashbook` for recording, once F140
lands), using the same `useOrganizationRole` source the sidebar uses.

---

### 2.4 — F149: a translation string that doesn't compile (Confirmed)

Every `pnpm build` prints "Failed to compile catalog for locale en/de/fr" for
message `sJZbdf`, "Zero-padded sequential number (e.g. {number:4} → 0007)",
from the Document Numbering settings (`src/routes/settings/document-numbering.tsx`).
ICU MessageFormat reads `{number:4}` as a placeholder with invalid syntax, so
Lingui can't compile the string in any of the three locales, and the build
passes only because `failOnCompileError` is off. Escape the braces
(ICU `'{'number:4'}'`) or pass the token in as a value, then consider turning
`failOnCompileError` on so CI catches the next one.

---

## Section-guard sweep: method and result

Every route pattern in `api/router.go` (both the `protected*` wrappers and the
direct `mux.Handle` registrations) was passed through `routeAllowedForRole` for
`general`, `sales`, `purchasing`, `accounting` and `cashbook`. That produced
125 denied (pattern, role) pairs. Each denied pattern was mapped to its
`src/api/index.ts` function, and each function to every non-test file that
imports it. Section-owned routes, atoms and components used only inside their
own section were set aside. That left these cross-section consumers:

| Consumer | Denied call | Roles that keep the consumer | Verdict |
|---|---|---|---|
| `src/routes/cash-book.tsx` | `GetInvoice`, `UpdateInvoiceState` | `cashbook` | **F140** |
| `src/components/products/form.tsx` | `GetProductBOM`, `ReplaceProductBOM` | `general`, `sales`, `purchasing` | Handled: `bomAllowed` skips the fetch and the save |
| `src/routes/settings/gl-export.tsx` | fiscal years, FEC/DATEV | admin, accounting (route-gated) | Consistent |
| `src/routes/dashboard.tsx` | row link to `/invoices/{id}` | all | **F147** (navigation only; no API call is denied) |

No other page that a restricted role keeps calls a route its section guard
denies. Accounting pages link to no source documents.

## Regression re-check of F96–F139 in rewritten code

- **F100** (zero-width Cash Book window): still fixed server-side.
  `GetCashMovementDetails` floors `endDate` to the UTC day and makes it
  exclusive-plus-one (`db/cash_movement_details.go:79`).
- **F115 / F119 / F120** in `cash-book.tsx`: the daily movement and loan status
  refreshes keep their request-id race guards, and loan status has an explicit
  failed state. The payment-history refresh (`refreshPayments`, `:519-532`) has
  no request-id guard. It is org-keyed only, so the race can't return another
  filter's data. Not raised.
- **F116** (in-app navigation guard): present on all six editable document
  detail pages. `production-orders/details.tsx` has no editable form to guard.
- **F118** (infinite skeleton): the retry path is intact on the rewritten
  detail pages (e.g. `invoices/details.tsx:414`).
- **F111** (BOM wipe): intact, and extended with the `bomAllowed` section check.

---

## Watch list: suspicions, not confirmed

- **`GetLoanStatus` with a negative line** (a credit or return line with a
  negative `unitPrice`). The allocator clamps each weight to ≥ 1 but sums the
  raw `netLine`, so `netSum` can be smaller than the sum of the weights. Earlier
  lines can then over-allocate, leaving the last line a negative remainder. Not
  reproduced. Whether a Cash Book invoice can carry a negative line was not
  checked.
- **Amount-in-words language.** The line is always French, labelled "facture",
  and uses the currency code as the unit name for non-TND currencies ("Mille EUR
  et Dix Centimes"). The doc comment says this is deliberate for the Tunisian
  format, but the setting isn't layout-gated and the default layout also prints
  it. A de or en organization that enables it gets French. This is a product
  question, not yet a defect.
- **A single-payment loan vanishes from the loan tracker.** `GetLoanStatus`'s
  "was ever a loan" filter (`appCount = 0 OR appCount > 1 OR paid < total`)
  can't tell a zero-deposit loan settled by exactly one payment from a straight
  cash sale. Such a loan drops out of the table even with "Open only"
  unchecked. This predates line-level settlement (the same happened with one
  invoice-level repayment), but it's now likely for single-line loans.
- **Carried forward from 2026-09-19, still unverified:** TND 3-decimal
  round-trip, the non-transactional default UoM and payment-term seeding, and
  OIDC provider caching.

## Explicitly excluded: do not re-raise

- **Changing `discountAmount` on a posted invoice.** `discountAmount` is not in
  `invoiceUpdateTouchesGLFields`. But `UpdateInvoice` revalidates the effective
  totals, and `total = net + roundHalfUp(net·r)` is strictly monotonic in `net`
  for r ≥ 0. So a discount change without a matching `total` change (which *is*
  GL-guarded) always fails validation. Adding it to the guard would only be
  defense in depth.
- **Migrations `0085`–`0089`.** All `.up`/`.down` pairs are present. `0086`'s
  table rebuild recreates both `organization_users` indexes and its down
  migration maps `power_user → general`. `0087` is additive. `0089`'s rename is
  applied everywhere outside `db/migrations/` and its own test, with no stale
  `invoiceLayout` references in Go, TS or `cmd/`.
- **The new report exports** (`daily-cash-movements`, `loan-status`,
  `payment-history`). They are org-scoped through `h.orgMember(pathOrgID)`, the
  `clientId` filter only narrows an org-scoped list, the format is whitelisted
  and filenames go through `sanitizeContentDispositionFilename`.
- **`api/document_templates.go`'s delta.** It only threads the organization
  logo into the six fill calls. `fillTemplate` embeds it only when
  `imageExtension` recognizes the bytes, and it reports an `AddPictureFromBytes`
  failure as an error rather than panicking on it. The logo upload endpoint
  itself is unchanged. The govulncheck Excelize trace through
  `AddPictureFromBytes` is the existing allowlisted advisory.
- **The `db/dashboard.go` correlated-subquery rewrite.** Every outer query
  filters on `organizationId` or an already-scoped `clientId`, and the
  `fmt.Sprintf` fragments are compile-time constants, not request input.
- **Accounts reads, payments and master data staying membership-level under the
  section guard.** This is documented and intentional (the `api/sections.go`
  file comment), because kept screens cross-reference them.

---

## Remediation phases

**Phase 1 — correctness and data loss (F140–F145).** F140 goes first and
ships alone if needed. It needs the payments-vs-cashbook decision and the step-3
decision recorded here before code moves. F145 is next: a small change that
stops silent data loss. F141 and
F142 share the discount-allocation helper that `validateInvoiceTotals`,
`buildInvoiceGLLines` and `buildTaxBreakdownRows` already duplicate, so extract
it once. F143 and F144 are small.

**Phase 2 — docs, UX (F146–F148).** F147 needs a product decision first. F148
should land with or after F140, so the role list it hides against is final.

## Verification

Beyond the per-phase gates:
- **F140:** an `api/` test through `NewRouter`. A `cashbook` member of the org
  must get 200 on `GET /api/invoices/{id}` and succeed at the chosen `paid`
  path, and `sales` and `general` behavior must be unchanged. Then drive the
  real app (`go run .` + `pnpm dev`) as a `cashbook` user: record a loan sale,
  settle it in full from the Cash Book, and confirm the invoice ends up `paid`
  and drops out of the open-loan view.
- **F141 / F142:** DB-layer tests on a two-rate discounted invoice. Tax Summary
  output must equal the GL tax credits, and UBL `PayableAmount` must equal
  `invoice.Total`. For F142, also validate the XML against the XRechnung
  schematron if it is available in CI.
- **F144:** table cases in `TestFrenchNumber`.
- **F145:** a mass-data export → import round-trip that preserves all four
  fields, and a test that fails on the current code.
- **F148:** as a `sales` member, open a sent invoice and confirm no payment
  write control is offered.
