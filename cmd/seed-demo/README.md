# seed-demo

Populates a FaturaCloud "demo" organization with months of realistic, daily
business activity — sales invoices, sales orders/deliveries, purchase
orders, goods receipts, vendor bills, and payments — so there's something
that looks like a real, aged business to click through when validating a
workflow or a new feature, rather than a handful of hand-crafted rows.

It drives the real REST API (`src/api/index.ts`'s own endpoints) exactly
like a browser would: a logged-in session, the `X-CSRF-Protection` header,
real server-side validation, real GL posting. A seeding run is therefore
also a decent end-to-end smoke test of the API layer itself, not just a data
generator.

## Quick start

```bash
# 1. Have a server running somewhere (local `go run .`, or the Docker image).
# 2. From the repo root:
go run ./cmd/seed-demo \
  --base-url http://localhost:8080 \
  --admin-email admin@fatura.cloud --admin-password admin \
  --org-name "Demo Organization" \
  --months 18 --volume busy --seed 20260101
```

That creates (or, with `--reset`, replaces) an organization named
`--org-name`, seeds its master data, and then simulates one business day at
a time from `(--end-date - --months)` through `--end-date` (default: today).

A full 18-month `busy` run creates on the order of 5,000-7,000 documents
(invoices, orders, deliveries, purchase orders, receipts, bills, payments)
and takes several minutes — it's making thousands of real HTTP requests
against a real server doing real GL posting on each one, not writing
directly to the database. Use `--dry-run` to check the plan (organization,
tax rates, master-data counts, fiscal year coverage) without generating any
documents, and a short `--months` value while iterating on the tool itself.

## Flags

| Flag | Default | What it does |
|---|---|---|
| `--base-url` | `http://localhost:8080` (or `$SEED_BASE_URL`) | Server to seed |
| `--admin-email` / `--admin-password` | `admin@fatura.cloud` / `admin` (or `$SEED_ADMIN_EMAIL`/`$SEED_ADMIN_PASSWORD`/`$ADMIN_EMAIL`/`$ADMIN_PASSWORD`) | Login used to create the organization — see "Why a platform admin" below |
| `--org-name` | `Demo Organization` | Organization to create/reset/seed |
| `--country` | `Germany` | Organization's country — also picks a chart-of-accounts template (`masterdata.go`'s `orgProfiles`); `Germany` and `Tunisia` have dedicated address/VATIN/VAT-account profiles, anything else falls back to a generic one |
| `--currency` | `EUR` | Organization's functional currency (ISO 4217 code) — independent of `--country`, so e.g. `--country Tunisia --currency TND` is expected, not implied |
| `--months` | `18` | Length of the simulated history, ending at `--end-date` |
| `--end-date` | today | Last simulated day, `YYYY-MM-DD` |
| `--seed` | `20260101` | RNG seed — same seed always reproduces the same dataset |
| `--volume` | `busy` | `small` or `busy` — see `volumeProfiles` in `seeder.go`. Ignored by `--scenario retail`, which sizes itself (see "The retail scenario" below) |
| `--scenario` | `moto` | `moto` (a motorcycle assembler/manufacturer — the original scenario) or `retail` (a home-appliance retailer selling entirely through Cash Book counter sales) — see `scenario.go` and "The retail scenario" below |
| `--reset` | off | Delete an existing organization named `--org-name` first, then recreate from scratch |
| `--dry-run` | off | Print the plan (org, master data, fiscal years) and stop — no documents |
| `--progress-every` | `20` | Log a progress line every N simulated days (`0` disables) |

## What gets created

- **Organization** — `Country`/`Currency` from `--country`/`--currency`
  (default `Germany`/`EUR`, so it gets the SKR04 chart of accounts and
  every account this tool needs, e.g. the VAT accounts tax rates require;
  `Tunisia` is the other country with a dedicated profile — see
  `masterdata.go`'s `orgProfiles`), 14-day payment terms (30 for Tunisia).
- **Tax rates** — Standard (19%), Reduced (7%), Zero-rated (0%), each wired
  to the SKR04 chart's output/input VAT accounts (codes `3800`/`1400`) —
  without that wiring, sending an invoice 409s with "no output tax account
  configured".
- **Fiscal years/periods** — one full calendar year (12 open monthly
  periods) per year the simulated range touches. Deliberately left **open**
  forever — see "What this deliberately doesn't do" below.
- **Vendors, clients** — counts from the chosen `--volume` profile, with
  company/person names, cities, phone numbers, and VATIN format drawn from
  the `countryLocale` `--country` resolves to (`catalog.go`'s `localeFor`)
  — `Germany` and `Tunisia` have dedicated locales matching their
  `orgProfiles` entry, so a Tunisia-seeded organization's clients and
  vendors read as Tunisian too, not German placeholder data with a
  Tunisian address bolted onto the organization alone; any other
  `--country` falls back to Germany's.
  **Products** are fixed regardless of `--volume` (`catalog.go`): 10
  generic services, 25 "finished" motorcycles (5 model lines × 5
  displacement classes, `buildFinishedMotorcycleCatalog`), and 275
  "component" parts assembled into them (55 part templates × the same 5
  displacement classes, `buildComponentCatalog`) — `products.category` is
  set on every physical good, and `sales.go`/`purchasing.go` each filter by
  it (sales excludes "component", purchasing excludes "finished") so a
  vendor is never asked to supply a finished vehicle and a client is never
  sold a bare part. This models "Atlas Moto Assemblage SARL"-style
  businesses specifically — a from-scratch catalog for a different kind of
  business would mean replacing `productCatalog` and the two filters above,
  not tuning a knob.
- **Daily simulation** (`seeder.go`'s `Run`, one pass per calendar day).
  Invoice/bill due dates and the "Net N days" payment-terms text both
  follow the organization's own `orgProfile.dueDays` (14 for Germany, 30
  for Tunisia) rather than a hardcoded number, so AR/AP aging buckets a
  document lands in stay consistent with what the organization's own
  settings actually say:
  - **Direct sales invoices** — the bulk of the volume. Created draft, sent,
    then a randomly chosen fate: paid in full (most), paid via two partial
    payments, left outstanding (so AR aging has something to show), or
    cancelled shortly after issuing.
  - **Sales orders → deliveries → invoices** — a smaller weekly cadence that
    actually moves stock: order → confirm → (a few days later) deliver
    (checked against this tool's own on-hand tracking, so it never asks the
    server to ship more than it has) → invoice for what shipped.
  - **Purchase orders → goods receipts → vendor bills → payments** — order →
    confirm → (a few days later, sometimes a partial shipment) receive
    (posts the GRNI accrual, raises stock) → (a few days later) bill,
    linking each line back to its PO line item for the 3-way match (~12% of
    bills get a deliberate small price or quantity variance, exercising the
    match-override workflow) → approve → a randomly chosen fate mirroring
    the AR side (paid in full, paid via two partials, or left outstanding
    for AP aging — same small ~5% never-paid share as AR).
  - **Assembly** (`production.go`) — the piece that makes the "finished"
    motorcycles actually sellable, not just priced-and-catalogued. Drives
    the app's real Production Order feature: `assembleBatch` creates a
    draft order for a batch (against a curated, representative subset of
    one displacement class's "component" products — an "Engine block",
    "Frame chassis", "Tire", ... — not all 55 per class, see
    `production.go`'s own comment for why that turned out unworkable) then
    marks it `completed` (`POST /api/production-orders` +
    `PATCH .../status`), the same two-call flow the frontend's
    Create/"Mark as completed" buttons drive. The server snapshots the
    product's own BOM into the order and resolves the produced cost from
    what was actually consumed — no invented assembly-labor markup — and
    enforces a real stock-availability check at completion; a 409 there (this
    tool's own on-hand estimate having drifted from server truth) skips that
    batch rather than failing the run. A dedicated weekly procurement pass
    (`maybeRestockAssemblyComponents`) keeps those specific components
    supplied on a short, predictable lead time, independent of ordinary
    purchasing's random restocking of the same products — without it,
    getting all of even a small BOM in stock at once via uniform random
    purchasing alone was too unreliable to depend on. Before this feature
    existed, every seeded organization's Profit & Loss showed real revenue
    but permanently empty expenses: `sales.go`'s stock-fulfilment path only
    ever shipped a finished good when it had on-hand stock, and nothing ever
    gave it any.
  - **Imports** (F114, `imports.go`) — roughly once a month, a consolidated
    shipment is created (freight/customs cost in the organization's own
    currency, per the real F114 design — see CLAUDE.md). For the following
    ~3 weeks, 1-2 purchase orders a week are placed directly against it
    (up to 5 POs per import), each from one of the fixed overseas (China)
    vendors `setupForeignVendors` seeds and priced in that vendor's own
    currency (USD) — see "What this deliberately doesn't do" below for the
    multi-currency scope this narrowly exercises. A domestic restock never
    links to an import; an import-linked PO is never from a local vendor.
    Linking is what makes a receipt against that PO spread the import's
    landed cost across it (`db/gl_posting.go`'s `applyLandedCost`) and what
    the Imports list's "Purchase orders"/"Committed value" columns have
    something to show.

Everything multi-step is coordinated by `scheduler.go`'s day-keyed task
queue — see its doc comment for the mechanism every generator uses to say
"come back and do this later."

## The retail scenario (`--scenario retail`)

Everything above describes `--scenario moto` (the default) — "Atlas Moto
Assemblage SARL," a motorcycle *manufacturer*. `--scenario retail` is a
structurally different business: a small Tunisian home-appliance
*retailer* that buys finished goods and resells them, with no assembly of
its own. Run it with:

```bash
go run ./cmd/seed-demo \
  --scenario retail --country Tunisia --currency TND \
  --org-name "Établissement Ben Salah Électroménager" \
  --months 18 --seed 20260101
```

`scenario.go`'s `resolveScenario` is the single place that says what a
scenario does and doesn't run — see its `scenario` struct for the full
list of booleans `seeder.go`'s `Run()` reads instead of calling every
generator unconditionally.

- **Catalog** (`catalog_retail.go`) — ~127 physical products across 20
  appliance categories (refrigerators, washing machines, air conditioners,
  TVs, small kitchen appliances, …), each with `category: ""` (never
  `"finished"`) since this business buys and sells the *same* good —
  `""` is the only value that passes both `sales.go`'s `sellableProducts`
  and `purchasing.go`'s `stockProducts` filters. A small (~6 entry)
  services list (delivery, installation, warranty, repair visit) rides
  alongside them on the same Cash Book sale.
- **Sales — Cash Book only.** No direct B2B invoices, no order→delivery
  channel — every sale goes through `POST /api/cash-sales`
  (`cash_book_sales.go`'s `createCashBookSale`), the same atomic
  client+invoice+payment endpoint the Cash Book screen itself calls, at a
  day-of-week-weighted daily volume (busier Thu-Sat, quieter Sunday — a
  retail counter, unlike the B2B channel, is open and busiest on
  weekends).
- **Customers grow organically, not via a batch pre-create.** A target
  count is picked once (`400-450`, `seeder.go`'s `targetClientCount`); each
  sale either creates a new walk-in customer inline (`CreateCashSaleRequest.NewClient`)
  while under that cap, or picks an existing one — landing on *exactly*
  the target by construction rather than tuning a flat probability to
  drift into range. No `setupClients` batch-create runs for this scenario.
- **Loan sales ("vente à tempérament")** — big-ticket items (above 600
  TND) are commonly sold on a deposit or a zero-deposit informal
  installment plan; small items are usually paid in full
  (`decideAmountReceived`). The remaining balance is collected later via
  `sales.go`'s `schedulePayment` (reused unmodified, just pointed at the
  register account instead of Bank — an installment is paid back in cash
  at the counter) with its own tiered fate distribution
  (`scheduleLoanRepayment`), in the same spirit as the B2B channel's own
  paid/slow-pay/bad-debt split.
- **The cash register account is wired automatically.** Nothing does this
  for a real organization either (see CLAUDE.md's cash register account
  note) — `masterdata.go`'s `setupCashRegisterAccount` resolves the
  chart's `1010` ("Cash") leaf account and sets
  `defaultCashRegisterAccountId`, exactly the one manual step a real admin
  does in Organization settings.
- **Cash withdrawals** (`cash_movements_retail.go`) — a weekly deposit of
  the till's excess above a 1,500 TND float to the organization's real
  Bank account, plus an occasional (~monthly) small undocumented
  petty-cash expense — both via `POST /api/cash-movements`, so the Daily
  Cash Movements report has real "out" activity, not just sales.
- **Deliberately skipped**: production/BOM/assembly (no manufacturing —
  these already no-op safely against a catalog with no `"finished"`/
  `"component"` entries, but this scenario skips calling them at all
  rather than relying on that), and Imports/foreign vendors (domestic
  vendors only, for this first version — a real scope decision, not a
  gap: F114 imports are a realistic extension for later, since small
  Tunisian appliance retailers do commonly bring in stock from abroad).
  Domestic purchasing (`purchasing.go`'s existing PO → receipt → bill →
  pay chain) is unchanged and still restocks this scenario's catalog.

## Why HTTP, and why `db` types for the request bodies

This tool calls the real HTTP API rather than importing `db.Database` and
calling its methods directly, on purpose: it exercises auth, the CSRF
header, and the exact JSON shape the frontend itself sends, which a
direct-to-`db.Database` tool would skip entirely. The trade is speed — HTTP
plus real GL posting per call is slower than direct database writes — which
is why `--volume`/`--months` exist to dial the workload down while
iterating.

Every request body is built from the real `db.CreateXRequest` structs
(imported from `github.com/MaMissaoui/fatura-cloud/db`), not
hand-written `map[string]interface{}` literals. This matters more than it
looks: several of this app's request structs mix JSON key conventions
field-by-field (e.g. `CreateClientRequest.HouseNumber` serializes as
`house_number`, camelCase everywhere else on the same struct) — a
hand-built payload can silently send the wrong key and get ignored
server-side with no error. Importing the actual struct means `json.Marshal`
always produces the correct wire format, and a field rename on the server
becomes a compile error here instead of a silent drift.

## What this deliberately doesn't do

Scope decisions, not gaps found later — read this before assuming a
missing feature is a bug:

- **Never closes a fiscal year.** `CloseFiscalYear` is irreversible (see
  CLAUDE.md's `db/fiscal_year_closing.go` note); this tool creates open
  years/periods covering the whole simulated range and stops there.
  Trigger a close yourself from the UI whenever you specifically want to
  test that workflow.
- **No serialized products.** Every seeded product has `serialized: 0` —
  `UpdateDeliveryStatus`/`UpdateInboundDeliveryStatus` need a
  `serialNumbers` map for a serialized line, which this tool never builds.
  A natural extension: add a serialized entry to `catalog.go` and generate
  serial numbers in `purchasing.go`/`sales.go` at receive/ship time.
- **Multi-currency documents are exercised narrowly, not generally.** Every
  ordinary document (direct invoices, local-vendor purchase orders/bills,
  their payments) is still in the organization's own currency. The one
  exception: `setupForeignVendors` seeds 5-7 fixed overseas (China) vendors
  with `DefaultCurrency: "USD"`, and `createImportLinkedPurchaseOrder`
  (purchasing.go) is the only path that ever creates a PO against an Import
  — always from one of those vendors, always in USD, with a plausible
  (not real-world-accurate) exchange rate that's captured once and reused
  through the receipt, bill, and payment. Every other product/vendor/
  document combination in this tool stays single-currency.
- **No OIDC users, backups, or organization membership beyond the creating
  admin.** Out of scope for a "day-to-day trading activity" demo dataset;
  add a generator file for any of these the same way `purchasing.go`/
  `sales.go`/`imports.go` were added, following `scheduler.go`'s task-queue
  pattern for anything multi-step.
- **Stock tracking is a local estimate, not read back from the server.**
  `stock.go`'s `onHand`/`adjustOnHand` mirror what *this run* has
  received/shipped, seeded at zero — it has no way to know about stock a
  previous run or a person using the UI already added. Fine for seeding one
  fresh organization end to end; re-running against an organization this
  tool didn't create from scratch (i.e. without `--reset`) would drift.
- **`--reset` requires the same admin account that created the organization
  originally.** `DELETE /api/organizations/{id}` is org-admin gated, and
  `createOrganization` only grants membership to whoever created it (see
  CLAUDE.md's "Authorization model" note) — there's no platform-admin
  bypass. Seed and re-seed with the same `--admin-email`.

## Extending this

Adding a new kind of activity (a new document type, a different sales
pattern, serialized-product handling, ...) means writing one more generator
function and wiring it into `seeder.go`'s `Run` — either called directly
from the daily loop (like `generateSalesForDay`) or scheduled for a later
date via `s.sched.Schedule` (like every multi-step chain in `sales.go`/
`purchasing.go`). Keep new money math going through `money.go`'s
`computeTotals` rather than reimplementing rounding — every totals-validated
create endpoint (`CreateInvoice`, `CreateIncomingInvoice`) needs it to agree
exactly with `db/invoice_totals.go`'s server-side check or it 409s.
