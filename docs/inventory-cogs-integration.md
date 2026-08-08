# Inventory / COGS GL Integration (Phase 7)

## Context

Phases 1–6 of the double-entry GL (chart of accounts, auto-posting on
invoice/bill state changes, payments, reports, statutory export, fiscal
year closing) are implemented and live in PR #76. The original plan
(`model-nifty-sunrise.md`) scoped a seventh phase — "GRNI on receipt, COGS
on shipment tied to `db/product_cost.go`" — and deliberately left it
undetailed, flagging it as the highest-risk integration point because it
has to reconcile two facts that are both true today and both intentional:

- `products.unitCost` is a **moving average, always recomputed from the
  full movement history** (`recomputeAverageCostTx`, `db/product_cost.go`)
  — never adjusted in place, the same "compute, don't cache" philosophy as
  `stockQuantity`.
- A posted GL entry is **immutable** — `allocateAndFinalizeEntryTx` never
  lets one be edited, only reversed (`db/journal_entry.go`).

This note works through that tension against the actual code (not just the
plan's one-line summary), surfaces one scope-changing finding the original
plan didn't have visibility into, and lays out the open policy decisions
Phase 7 needs before it can be turned into a normal migration-by-migration
plan like Phases 1–6 were.

## Finding: today's purchase posting already conflicts with what Phase 7 needs

`buildIncomingInvoiceGLLines` (`db/gl_posting.go`) resolves every bill
line's debit account via `resolveExpenseAccount(product, defaultAccountID)`
— `product.ExpenseAccountID` if set, else
`organizations.defaultExpenseAccountId`. There is **no branch for
`product.StockEnabled`**: a vendor bill for a stock-tracked product expenses
its full amount immediately, exactly like a bill for a one-off service.
The seeded default chart's own account name is a tell —
`{code: "5100", name: "Purchases / Cost of Goods Sold", ...}`
(`db/account.go`) — "Purchases" and "Cost of Goods Sold" are already
smashed into one account and one posting moment, because today there is no
distinction between them.

That's a reasonable simplification for a small-business tool with no
inventory GL yet. It stops being reasonable the moment Phase 7 adds
COGS-on-shipment: a stock-tracked purchase would then be expensed **twice**
— once immediately on the bill (current behavior, unchanged), and again as
COGS when it ships (new behavior) — while never being carried as an asset
in between. **Phase 7 is not purely additive.** It has to also change the
existing bill-side posting for stock-tracked lines: capitalize to
Inventory (asset) at receipt/bill time, and only recognize the expense
(COGS) at shipment time.

## The core tension, verified against code

Re-examining `recomputeAverageCostTx` and its call sites shows the
"history can retroactively change out from under a frozen COGS snapshot"
risk is narrower than it first sounds, because three things are already
true in this codebase:

1. **Replay order is insertion order, not a user-editable business date.**
   `stockMovements.createdAt` is a DB-generated insert timestamp; nothing
   lets a caller backdate a movement's position in the replay. Normal
   operation can't reorder history after the fact.
2. **A received receipt's line items — including `unitCost` — are frozen.**
   `UpdateInboundDelivery` rejects `LineItems` edits once
   `current.Status != "draft"`. The cost that fed a receipt's stock
   movements can't be changed retroactively.
3. **Cancelling a received receipt requires its full quantity still be in
   stock.** If any of it has already shipped, cancellation is refused —
   so a receipt's cost can't be pulled out from under a shipment that
   already consumed it.
4. **An outflow never moves the average.** In `recomputeAverageCostTx`'s
   replay, an outflow (shipment) adds `qty * average` to value and leaves
   `average` unchanged. So `products.unitCost` read synchronously in the
   same transaction as a shipment's `"out"` movement — before or after
   inserting it — is the same number, and it's the number
   `recomputeAverageCostTx` would derive for "the average as of just
   before this shipment."

So: **freezing `products.unitCost` into a COGS line at ship time is safe
for the path this codebase already enforces**, the same "snapshot a
value that's normally live" idiom `payment.go` already uses for a
document's own frozen `exchangeRate`. The real open risks are narrower
and more specific than "history can change":

### Open question 1 — no costed inflow yet

`products.unitCost` is nullable. A stock-enabled product can ship (a
pre-order, backfilled opening stock recorded without a cost) before any
receipt has ever established a cost. Recommend: block the shipment with a
409 — "cannot post COGS: product X has no cost basis" — the same shape as
every other missing-GL-default rejection in this codebase
(`resolveRevenueAccount`/`resolveExpenseAccount`'s existing error messages).
Posting at 0 would be silently wrong in exactly the way this codebase
otherwise refuses to be.

### Open question 2 — serialized products: average vs. specific identification

`product_serial_numbers` already tracks per-unit lineage, but
`recomputeAverageCostTx` treats every `±1` serialized movement exactly
like a fungible one — a serialized unit is costed at the blended average,
not its own actual receipt cost. Standard practice for
serialized/high-value goods is specific identification. Recommend: for a
serialized product, resolve COGS from that specific serial's own `"in"`
movement row (`stockMovements.unitCost` where `serialNumberId` matches —
the same kind of windowed lookup `GetProductSerialNumbers` already does,
not a new storage concept), not from `products.unitCost`. This is a real
second code path alongside the average-cost one, not a variant of it.

### Open question 3 — manual stock adjustments have no GL trace

`POST /api/stock-movements` (adjustment/count types) only ever touches
`stockQuantity`/`unitCost` — no GL entry. Once Inventory is a real GL
account, an adjustment that changes quantity with no matching GL line
breaks the equivalence the account is supposed to represent. Recommend an
"Inventory Adjustment" expense account (mirroring the existing FX
Gain/FX Loss two-account pattern) and a Dr/Cr Inventory line for every
adjustment/count movement, using the same cost resolution as COGS.

### Open question 4 — GRNI matching against the eventual bill

Receiving goods before the vendor bill arrives should accrue
`Dr Inventory / Cr GRNI` at receipt time, at the receipt's own
`unitCost` (already captured on `inbound_delivery_line_items.unitCost`).
When the bill later arrives, it needs to **clear** that GRNI liability,
not re-debit Inventory or Expense — otherwise the same goods get
capitalized twice. The 3-way match system
(`db/incoming_invoice_match.go`) already links a bill line back to its PO
and receipt, so the data to find "which GRNI accrual does this line
clear" already exists — but `buildIncomingInvoiceGLLines` doesn't consult
it today. A price variance between the GRNI accrual (receipt cost) and
the actual bill needs a landing spot; recommend posting the delta
straight to Inventory (adjusting the asset's cost) rather than inventing
a new purchase-price-variance account and tolerance system the plan
doesn't currently ask for — reuse 3-way matching's existing variance
concept instead of a parallel one.

### Open question 5 — long-run reconciliation

Every other computed-on-read balance in this codebase (trial balance,
AR/AP aging) is provably reconcilable against its GL source of truth by
construction. Perpetual inventory would not automatically be: the
Inventory GL account is a sum of many independently-frozen historical
postings, while `stockQuantity`/`unitCost` are always the *current*
recomputed values — nothing checks that
`Σ posted Inventory lines == Σ stockQuantity × current unitCost` per
product over time, and adjustments/serialization make exact equality
nontrivial even in principle. Recommend scoping an "Inventory Valuation"
report (mirroring the trial balance) that shows both numbers side by side
so drift is visible — but treat exact automated reconciliation as a
stretch goal, not a hard requirement, and document the gap with the same
honesty Phase 6 already uses for the missing FX revaluation.

## Recommended sequencing

1. Add `Inventory` (asset) to the seeded default chart, and split what
   `5100` currently does: keep it (renamed, if desired) as a general
   non-inventory purchases/expense account, and add a dedicated `Cost of
   Goods Sold` account that only Phase 7's shipment posting ever touches
   — `5100`'s current name already conflates the two roles. Add
   `Goods Received Not Invoiced` (liability). Wire new org defaults
   (`defaultInventoryAccountId`, `defaultGRNIAccountId`) the same way the
   existing five `default*AccountId` columns are wired — no per-product
   override, keeping with "minimal generic starter chart."
2. Change `resolveExpenseAccount`'s call site in
   `buildIncomingInvoiceGLLines`: a bill line whose product has
   `stockEnabled = 1` routes to Inventory instead of Expense. This is a
   real behavior change to existing Phase 2 code, not purely additive —
   flag it clearly in the eventual implementation plan.
3. `UpdateInboundDeliveryStatus`: on receiving, post
   `Dr Inventory / Cr GRNI` at the receipt's own `unitCost`
   (`sourceDocumentType: "inbound_delivery"`, the same idempotent
   `FindPostedEntryForSourceDocument` + post/reverse pattern
   `UpdateInvoiceState` already uses); on cancelling a received receipt,
   reverse it the same way.
4. `UpdateIncomingInvoiceState`'s existing bill posting (on `approved`)
   clears GRNI instead of hitting AP directly for a matched line —
   `Dr GRNI / Cr AP` at the bill's approved amount, landing any price
   variance on Inventory (Open question 4).
5. `UpdateDeliveryStatus`: on shipping, resolve cost (average or
   specific-serial, Open question 2), block if no cost basis exists (Open
   question 1), post `Dr COGS / Cr Inventory`; on cancelling a shipped
   delivery, reverse it.
6. `CreateStockMovement` (manual adjustments): post
   `Dr/Cr Inventory` against the new Inventory Adjustment account (Open
   question 3), using the same cost resolution.
7. Factor a single "resolve this movement's cost" helper in
   `db/gl_posting.go`, shared by steps 3/5/6, rather than three separate
   implementations of the average-vs-serial logic.

## Explicitly out of scope

Same honesty stance as Phase 6's FX-revaluation deferral — stated, not
silently dropped:

- **Exact automated GL/inventory reconciliation** (Open question 5) —
  ships as a report, not a guarantee.
- **FIFO/LIFO costing** — weighted average only, matching
  `recomputeAverageCostTx`'s existing method; changing costing method
  entirely is a separate, larger feature.
- **Landed cost / freight allocation** into unit cost.
- **Multi-warehouse/location-level Inventory sub-accounts** — one
  Inventory account organization-wide, matching every other GL default
  today.

## Verification plan

Same shape as Phases 1–6: unit tests per scenario in `db/`, then a live
Playwright walkthrough — receive a PO-linked receipt and confirm the GRNI
entry; approve the matching bill and confirm GRNI clears into AP with any
price variance landing on Inventory; ship and confirm COGS posts (both a
plain product and a serialized one); cancel a receipt and a shipment and
confirm each reverses cleanly; attempt to ship a product with no cost
basis and confirm a clean 409, not a zero-cost posting; run a manual
adjustment and confirm its Inventory line; spot-check the new Inventory
Valuation report against the trial balance.
