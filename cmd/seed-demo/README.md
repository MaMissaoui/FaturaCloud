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
| `--months` | `18` | Length of the simulated history, ending at `--end-date` |
| `--end-date` | today | Last simulated day, `YYYY-MM-DD` |
| `--seed` | `20260101` | RNG seed — same seed always reproduces the same dataset |
| `--volume` | `busy` | `small` or `busy` — see `volumeProfiles` in `seeder.go` |
| `--reset` | off | Delete an existing organization named `--org-name` first, then recreate from scratch |
| `--dry-run` | off | Print the plan (org, master data, fiscal years) and stop — no documents |
| `--progress-every` | `20` | Log a progress line every N simulated days (`0` disables) |

## What gets created

- **Organization** — `Country: "Germany"` (so it gets the SKR04 chart of
  accounts and every account this tool needs, e.g. the VAT accounts tax
  rates require), EUR, 14-day payment terms.
- **Tax rates** — Standard (19%), Reduced (7%), Zero-rated (0%), each wired
  to the SKR04 chart's output/input VAT accounts (codes `3800`/`1400`) —
  without that wiring, sending an invoice 409s with "no output tax account
  configured".
- **Fiscal years/periods** — one full calendar year (12 open monthly
  periods) per year the simulated range touches. Deliberately left **open**
  forever — see "What this deliberately doesn't do" below.
- **Vendors, clients, products** — counts from the chosen `--volume`
  profile. Products are a mix of services (no stock) and physical goods
  (`stockEnabled`), see `catalog.go`.
- **Daily simulation** (`seeder.go`'s `Run`, one pass per calendar day):
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
    match-override workflow) → approve → (later) pay.

Everything multi-step is coordinated by `scheduler.go`'s day-keyed task
queue — see its doc comment for the mechanism every generator uses to say
"come back and do this later."

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
- **No multi-currency documents.** Every document is EUR, the organization's
  own currency — no `exchangeRate`/foreign-currency path is exercised.
- **No imports (China-shipment consolidation), OIDC users, backups, or
  organization membership beyond the creating admin.** Out of scope for a
  "day-to-day trading activity" demo dataset; add a generator file for any
  of these the same way `purchasing.go`/`sales.go` were added, following
  `scheduler.go`'s task-queue pattern for anything multi-step.
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
