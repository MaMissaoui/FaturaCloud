# FaturaCloud Re-Audit — Delta Sweep (v3.53.1 → v3.54.0)

**This is a delta audit, not a full-repo sweep.** It covers everything that
landed on `main` between `73737ea` (`v3.53.1`, where the
[2026-09-24 plan](audit-plan-2026-09-24.md) stopped) and `4903bda`
(`v3.54.0`): 26 non-merge commits across 52 files, excluding catalogs and
docs. Almost all of it is that plan's own remediation. This sweep checks
whether those fixes are complete and whether they introduced anything new.
Numbering continues at **F150**. **F151 is skipped**, because older documents
already use that number for an unrelated item (see
`docs/audit-plan-2026-09-14.md`), and a duplicate would send lookups to the
wrong place.

What the delta contains:

- **Per-line Cash Book loan settlement** (#384, migration `0090`):
  `db/cash_sale_payment.go`, the new
  `POST /api/cash-sales/{id}/payments`, `allocateInvoiceLines`/`allocateCapped`
  in `db/dashboard.go`, the per-line modal in `src/routes/cash-book.tsx`, and
  invoice numbers on the Cash Book tables and exports.
- **Phase 1 fixes** (#386): the client import's counter fields
  (`massDataColumnAware`), discount-aware Tax Summary and Sales by Product
  (`invoiceNetFactorExpr`), the UBL e-invoice's document-level allowance, and
  French `invariableBeforeMille`.
- **Phase 2 and F147** (#387, #388): payment buttons gated by role,
  `failOnCompileError`, doc fixes, and role-aware dashboard widgets.
- **Watch-list follow-ups** (#389): the loan tracker's `lineAppCount` term, and
  default payment terms and units of measure seeded in one transaction.
- **Amount in words in the organization's language** (#390, migration
  `0091`): `organizations.documentLanguage`, `db/amount_in_words_lang.go`, and
  the drawer select.

**Baseline health** at `4903bda`, measured before any change:

| Check                          | Result                                                                                                                                                                                                                                                                                                                                                                   |
| ------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `go vet ./...`                 | Pass                                                                                                                                                                                                                                                                                                                                                                     |
| `gofmt -l .`                   | Pass                                                                                                                                                                                                                                                                                                                                                                     |
| `go test -race -count=1 ./...` | Pass (all packages)                                                                                                                                                                                                                                                                                                                                                      |
| `pnpm type-check`              | Pass                                                                                                                                                                                                                                                                                                                                                                     |
| `oxlint src/`                  | Warnings only: the pre-existing `react(set-state-in-effect)` set                                                                                                                                                                                                                                                                                                         |
| `pnpm test`                    | Pass (74/74, 10 files)                                                                                                                                                                                                                                                                                                                                                   |
| `pnpm build`                   | Pass                                                                                                                                                                                                                                                                                                                                                                     |
| `govulncheck ./...`            | No called vulnerabilities (the local v1.8.0 binary and `@latest` agree). The Excelize advisory `GO-2026-6452` that CI allowlists was amended on 2026-09-24 to "fixed in v2.11.0", the version already in use, so it no longer matches and the allowlist entry is dead. `-show verbose` lists three module-level `golang.org/x/crypto` advisories that are not called (see F155) |

**How this document was produced.** One sequential pass by the authoring
model, with no parallel sweeps. **Confirmed** means reproduced:

- F150 through the real router with a real `cashbook` member.
- F152 through the real `GenerateEInvoice` path, with invoices that
  `validateInvoiceTotals` accepted.

The brute-force search that found F152's cases, and every reproduction test,
were throwaway and have been deleted.

**Instructions for the executing model:**

- This document is an audit, not a remediation. No code was changed while
  producing it.
- Use one feature branch and PR per phase, and never push to `main`. Use
  conventional commits with no Claude attribution lines.
- After every phase, `go vet ./... && go test -race ./...` and
  `pnpm lint && pnpm build` must pass.
- F150 needs a product decision before any code is written (see its options).
  F152 needs external EN 16931 validation before its fix ships.
- The 2026-09-19 and 2026-09-24 "Explicitly excluded" lists still bind, and so
  does this document's own list at the end.

**Status:** 5 findings (F150, F152–F155), all resolved by PR. The owner's
decisions are recorded:

- **F150:** accepted as intended (option 1). The counter may collect any open
  receivable. Documented in #394, with no behavior change.
- **F152:** fixed with option 1, the smaller change, in #393. It is still
  unvalidated against BR-CO-17 with an external validator.
- **F153–F155:** fixed in #394.

---

## Severity overview

| #    | Finding                                                                                                                                                                                                                            | Area                          | Severity | Phase |
| ---- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------- | -------- | ----- |
| F150 | The Cash Book loan tracker and the new line-payment route cover every sent invoice in the organization, not just Cash Book sales. A `cashbook` member can't open an ordinary invoice, but can settle it into the register, and the server then marks it Paid | Authorization / product scope | Medium   | 2     |
| F152 | The UBL e-invoice discount split is incomplete (F142). `PayableAmount` can differ from the invoice total by a cent, and with four or more VAT categories one allowance can be negative                                                  | E-invoicing / compliance      | Medium   | 1     |
| F153 | `TaxSummary`'s doc comment is now attached to `invoiceNetFactorExpr`, leaving the type undocumented                                                                                                                                | Docs                          | Low      | 3     |
| F154 | `GET /api/clients/{id}/open-invoices` has had no caller since `10e721c` (pre-delta). Its comment still describes the removed `PaymentPanel` flow                                                                                    | Dead code                     | Low      | 3     |
| F155 | Dependency hygiene: Excelize is marked `// indirect` in `go.mod` despite being imported directly, CI's `GO-2026-6452` allowlist entry is dead, and `x/crypto` has three uncalled module-level advisories fixed in v0.56.0                | Dependencies / CI             | Low      | 3     |

---

## Phase 1

### 1.1 — F152: the e-invoice discount split can disagree with the invoice (Medium, Confirmed)

**Where:** `db/einvoice.go`, `buildUBLInvoice`, the allowance loop added by
F142 (#386).

**What happens.** Each VAT category's allowance is its proportional share of
the discount, rounded to the cent, and the last category takes the remainder.
Each category's tax is then computed from `taxable − rounded share`. The
invoice itself does it differently. `validateInvoiceTotals`,
`buildInvoiceGLLines` and the frontend's `allocateDiscount` compute each
group's tax from the **exact** rational net (`gross − D·gross/S`) and round
only the tax. The two paths round at different points, so they don't always
agree:

- **Payable amount differs by a cent.** Take 1234.56 at 19% plus 456.78 at 7%,
  with a 10.02 discount. `validateInvoiceTotals` accepts tax 264.97 and total
  **1946.29**. `GenerateEInvoice` emits
  `<cbc:PayableAmount currencyID="EUR">1946.28</cbc:PayableAmount>`. A
  brute-force sweep of two-category invoices at realistic amounts found such
  cases for many discount values, not as isolated edge cases. The customer's
  e-invoice then asks for a different amount than the invoice, its PDF and the
  GL.
- **Negative allowance.** With categories weighted 30/30/30/10 (19%, 7%, 5% and
  16%, all category S) and a 0.05 discount, each of the first three shares
  (0.015) rounds up to 0.02. The last category's remainder is then **-0.01**,
  emitted as `<cbc:Amount currencyID="EUR">-0.01</cbc:Amount>` on an allowance
  (`ChargeIndicator=false`). That category's taxable amount ends up above its
  own line total. The payable amount happened to match in this case.

**Why F142's test missed it.** `TestGenerateEInvoiceCarriesInvoiceDiscount`
splits 8.00 over 30.00/10.00, which divides exactly, so no rounding happens.

**Fix direction.** There are two options. Neither can be validated here:
`db/CLAUDE.md` notes there is no EN 16931 validator (e.g. KoSIT) in this
environment, and conformance with BR-CO-17 (category tax = taxable × rate,
rounded) and its tolerance can't be checked locally.

1. **Keep the invoice as the reference.** Emit rounded allowances, but take
   each category's `TaxAmount` from the exact net, as the invoice does.
   `PayableAmount` then always equals `invoice.Total − fiscalStampAmount`.
   The e-invoice deliberately doesn't emit the Tunisian fiscal stamp (see
   `db/CLAUDE.md`'s `db/einvoice.go` entry); whether it should is a separate,
   undecided question. For the negative case,
   allocate the cent shares by largest remainder, so no share goes negative
   and no category exceeds its line total. Risk: a category's
   `TaxAmount` may differ by a cent from `TaxableAmount × rate`. Whether
   BR-CO-17's tolerance accepts that must be checked with an external
   validator.
2. **Round the discount shares once, everywhere.** Allocate the discount in
   cents by largest remainder, and use those same cents in
   `validateInvoiceTotals`, `buildInvoiceGLLines`, `buildTaxBreakdownRows`,
   `invoiceNetFactorExpr` and `allocateDiscount`. Every consumer then agrees
   by construction, and BR-CO-17 holds exactly. Risk: this is a much larger
   change. Existing stored `taxTotal`s were validated under the exact rule,
   so re-validating an old invoice (`UpdateInvoice` with totals) could start
   failing. That needs a compatibility decision.

**Regression test.** Sweep discounts over at least two- and four-category
invoices. For each, assert `PayableAmount == invoice.Total − fiscalStampAmount`
(or sweep only stamp-free invoices), that the
allowances are all non-negative and sum to `AllowanceTotalAmount`, and that
no category's allowance exceeds its line total.

---

## Phase 2

### 2.1 — F150: the loan tracker and line payments aren't limited to Cash Book sales (Medium, Confirmed)

**Where:** `db/dashboard.go` (`GetLoanStatus`, `loanLinesQuery`),
`db/cash_sale_payment.go` (`CreateCashSalePayment`), `api/router.go:679`.

**Reproduction** through the real router. The organization has a `cashbook`
member and an ordinary invoice, created with `CreateInvoice` (not
`POST /api/cash-sales`) and sent:

| Call by the cashier                                          | Result                                            |
| ------------------------------------------------------------ | ------------------------------------------------- |
| `GET /api/invoices/{id}`                                     | 403 (section guard)                               |
| `GET /api/organizations/{orgId}/reports/loan-status`         | 200, and it lists the invoice's line              |
| `POST /api/cash-sales/{id}/payments` for the full amount     | 201. The invoice is now **paid**                  |

**What predates this delta, and what it adds.**

- **Pre-existing:** `GetLoanStatus` has no notion of where an invoice came
  from. Its "was ever a loan" filter keeps any invoice with no payment
  (`appCount = 0`), so every unpaid ordinary invoice in the organization
  already appeared in the Cash Book's loan tracker, with its customer,
  products and amounts.
- **Added by #384:** the tracker's per-line **Record payment** now works for
  those invoices too. `CreateCashSalePayment` checks only that the invoice is
  sent or paid, in the organization's currency, and posted. A `cashbook`
  member can therefore record a cash receipt into the register against any
  customer invoice, and the server flips it to **paid**.
- **This widens F104.** `POST /api/payments` was made an accounting-tier
  action in `v3.29.0`, and the new route is a narrower (cash-only,
  inbound-only, register-only) path around that decision.
- **The docs are wrong.** `db/CLAUDE.md` says the state change is "the one
  place an invoice state is derived from payments, scoped to this
  purpose-built flow". In practice it applies to any invoice.

**This is a product decision, not a clear bug.** Collecting any customer's
open invoice at the counter may be exactly what the shop wants. Options:

1. **Accept and document.** The Cash Book is the counter's view of every open
   receivable. Correct `db/CLAUDE.md` and the route notes, and consider
   renaming "Loan status" in the UI accordingly.
2. **Mark Cash Book invoices and scope both the tracker and the route.** Add a
   column saying where an invoice came from, set by `CreateCashSale`, then
   filter `GetLoanStatus` on it and refuse the route for other invoices. The
   hard part is the **backfill**: nothing in today's schema reliably
   identifies an existing Cash Book invoice. `dueDate = date` and the
   register payment are heuristics that ordinary invoices can also match.
   Existing rows would need a "not known" value, the tracker would have to
   include those, or someone would have to classify them once.
3. **Scope only the route.** Keep the tracker as a read-only view of
   receivables, and allow line payments only on marked invoices. This has the
   same backfill problem as option 2, and a tracker that lists invoices the
   cashier can't act on.

---

## Phase 3

### 3.1 — F153: `TaxSummary` lost its doc comment (Low)

**Where:** `db/sales_reports.go`, around the new `invoiceNetFactorExpr`.

#386 inserted `invoiceNetFactorExpr` and `invoiceNetFactor` directly below
the long comment that documents the Tax Summary (rounding order, the
deliberate divergences from the GL, the discount paragraph). godoc now
attaches that whole block to `invoiceNetFactorExpr`, and `type TaxSummary`
has no doc comment. The discount paragraph also runs on into "Base itself is
summed-then-rounded …" on the same line.

**Fix:** move the two new declarations below `type TaxSummary`, or put a
blank line and an explicit comment between the blocks. Rewrap the paragraph.

### 3.2 — F154: a dead open-invoices endpoint (Low, pre-existing)

**Where:** `db/dashboard.go` (`GetClientOpenInvoices`), `api/cash_sale.go`,
`api/router.go:237`, `src/api/index.ts` (`GetClientOpenInvoices`).

The Cash Book stopped calling `GetClientOpenInvoices` in `10e721c`, before
this delta, and nothing else calls it. Its doc comment still explains a
filter choice in terms of "this screen's PaymentPanel", a flow #384 removed.
It also uses `paymentsNonVoided` where the loan tracker uses
`paymentsPosted`. The two are equivalent today, since payments are only
`posted` or `voided`.

**Fix:** remove the route, the handler, the DB function, the TypeScript
function and their route-coverage entries. Alternatively, if the route is
kept for API clients, correct its comment.

### 3.3 — F155: dependency hygiene (Low)

- **`go.mod`:** `github.com/xuri/excelize/v2` is listed `// indirect`,
  although `db/` imports it directly. It has been that way since `f222de9`
  (#115). `go mod tidy -diff` moves it into the direct block.
- **CI allowlist:** `.github/workflows/ci.yml` allowlists `GO-2026-6452`. That
  advisory now reads "fixed in v2.11.0", the version in use, so the entry
  matches nothing. Remove it, along with the comment block that explains it.
- **`golang.org/x/crypto` v0.55.0:** `GO-2026-6355`, `GO-2026-6354` and
  `GO-2026-5932` show only at module level; no vulnerable symbol is called.
  Bump to v0.56.0 anyway, so these don't mask a future called one.

---

## Verified without a finding

- **Migration `0090`'s `ON DELETE SET NULL` on `invoiceLineItemId`.** Invoice
  line items are deleted and reinserted on every line edit, which would null
  the line targeting. But a line edit is refused while the invoice has a
  posted GL entry, and cancelling an invoice (which reverses that entry) is
  refused while payments are still applied (`"void them first"`). Only
  applications of voided payments can lose their line, and nothing counts
  those.
- **`CreateCashSalePayment` concurrency.** It re-checks the line balance
  inside the transaction. The Paid flip uses the in-transaction paid figure,
  and `SetMaxOpenConns(1)` serializes two payments on different lines.
- **The new route's permissions.** It is in the section guard's cash-sales
  section, `wantSectionRoutes` and `domainRouteRoles`, and allows
  admin/power_user/general/cashbook. The `accounting` role isn't in that
  list: it records payments through `POST /api/payments`, which is a fact
  about the current wiring rather than a recorded decision.
  `PAYMENT_WRITE_ROLES` on the frontend
  matches the server for `POST /api/payments` and void.
- **The client import (F145).** Every `UpdateClientRequest` field is now a
  sheet column, so no client field is silently replaced any more.
- **The Tax Summary and Sales by Product discount (F141, F143).**
  `round(grossTax × factor)` equals the GL's exact net × rate. The
  per-invoice subquery only runs when a discount exists (`CASE`), and the
  tables it reads are indexed.
- **The printed VAT recap** (`buildTaxBreakdownRows`) takes tax from the exact
  net, so it agrees with the invoice, unlike F152.
- **The loan tracker's new `lineAppCount` subquery** is indexed through
  `payment_applications_document`.
- **Report exports with the new Invoice column** keep
  `FitToWidth = 1`, so the wider loan-status sheet still prints one page
  wide.
- **`documentLanguage`.** It is validated on create and update, in every
  explicit column list in `db/organization.go`, backfilled by `0091` (tested),
  and French output is unchanged.
- **Seeding transactions** (#389). The count check runs before `Beginx()`,
  and nothing reads through `d.DB` inside the transaction.

## Watch list: suspicions, not confirmed

- **The Tunisia layout's amount-in-words row has a fixed height.** Found while
  verifying #390: at nine-digit totals the sentence wraps and the second line
  runs into the box border, in French as well as German. It is a template
  limit, not a language one. Unverified whether real Tunisian totals ever get
  that large.
- **German wording for 1,000,001.** It is spelled "Eine Million ein EUR".
  Grammatical, but a native reviewer may prefer a different form. It's a
  style question.
- **The Cash Book payment modal is filled before it mounts.** `openPayment`
  calls `payLineForm.setFieldsValue` before the `destroyOnHidden` modal's
  `Form` mounts. The prefill was seen working during #384's browser
  verification, but it relies on the form store surviving the unmount. If
  the prefilled amount ever shows blank, use the Form's `initialValues`
  instead.

## Explicitly excluded: do not re-raise

- **The dashboard's shared payload.** F147's recorded decision was option (a):
  a role-aware presentation over the shared API payload. Restricted roles
  can still fetch all dashboard data through the API. That is known and
  accepted.
- **The amount-in-words line uses the template's own labels.**
  `documentLanguage` changes only the amount-in-words sentence, by design
  (#390). An English sentence on the French Tunisia layout is expected.
