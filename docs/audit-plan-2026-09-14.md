# FaturaCloud Audit — Production Orders, BOM Versioning, Units of Measure (2026-09-14)

Fresh audit at commit `fc2a151` (`v3.20.0`), scoped to `8c6898d..HEAD` — the three
feature tranches that landed back-to-back since `v3.19.6` and shipped together as
`v3.20.0`:

- **BOM version history** (#236, #237)
- **Production Orders**, backend + frontend (#238, #240, #241, #242)
- **Base Unit of Measure** (#239)

That is 48 files / ~6,200 insertions of new document-type, master-data and GL-posting
code, none of which has had a second pass since merge. This audit is that second pass,
plus whatever those changes newly depend on. It is not a calendar-driven rotation back
over security/ops ground — the 2026-08-13 round covered that and this tranche did not
touch it.

Baseline health is good: `go vet ./...`, `gofmt -l .`, `tsc --noEmit`, `oxlint src/`,
`go test ./...` (all packages), `pnpm test` (22 tests) and every CI and Docker run on
`main` are green. There are **zero open GitHub issues**, so this document is the backlog.

This plan continues the numbering at **F70** (the 2026-08-13 fix-review ends at F69) and
runs through **F93**. Note that `F114` / `F116` / `F151` as they appear in CLAUDE.md are
*GitHub issue* numbers — a separate namespace, not findings.

**Severity calibration:** the original sweep produced nothing above Medium. **F93 —
added after the fact, when F70's mandated precondition enumeration surfaced it — is
High**, and is the only finding here reaching the bar prior audits reserved for F48's
double-posting class: it disables a guard purpose-built to protect GL integrity rather
than merely producing a wrong number.

**Instructions for the executing model:**
- This document is an audit, not a remediation — no code was changed while producing it.
- Work phase by phase. One feature branch + PR per phase (never push to `main`
  directly). Conventional commits, no Claude attribution lines.
- After every phase: `go vet ./... && go test -race ./...` and
  `pnpm lint && pnpm build` must pass.
- Update CLAUDE.md sections whose documented behavior a task changes — F71 and F77 both
  touch documented text.
- The "Explicitly excluded" section at the end is binding: those were checked and
  dismissed with a reason. Do not re-raise them.

**Status:** 24 findings. F93 was appended after the initial 23 — Phase 1.1's precondition
enumeration is what found it, which is the argument for keeping that precondition rather
than treating it as ceremony.

Remediation in flight:

| Phase | PR | Findings | State |
|---|---|---|---|
| 1 — backend correctness | #244 | F70, F71, F72, F73, F74, F93 | CI green, open |
| 2 — frontend reliability | #245 | F84, F85, F86, F87, F88, F89 | CI green, open |
| 3 — design & consistency | #246 | F75, F76, F77, F78, F79, F90, F91 | CI green, open |
| 4 — test backfill | #247 | F80 | CI green, open |
| 5 — docs, i18n, CI, ops | #248 | F81, F82, F92 (**not** F83) | CI pending, open |

Every finding except **F83** now has a fix in an open PR. F83 (the lightweight `v3.20.0`
tag) is deliberately left undone: fixing it means force-replacing a published release tag,
which re-triggers the Docker build and republishes that version's GHCR image — an
outward-facing, hard-to-reverse action that needs an explicit decision rather than being
folded into a docs PR. The two commands, if taken:

```
git tag -f -a v3.20.0 fc2a151 -m "v3.20.0"
git push --force origin refs/tags/v3.20.0
```

**Merge order matters.** #248 must land last: it rewrites the translation catalogs, and
both #245 (which adds and translates 13 strings) and #246 (which moves line numbers the
`#:` source references point at) touch the same files. Out of order, the resolution is to
re-run `pnpm extract` on the merged result — no translations are lost, only source
references move.

Both decisions this document left open are now settled in place: **F76 → hoist** (3.2),
**F79 → reviewed, not changed** (3.5). One question raised by F93 stays open — whether
purchase-order line items should additionally be *frozen* once a receipt exists. Id reuse
stops the severing, but it does not stop a quantity moving underneath an already-posted
GRNI accrual.

---

## Severity overview

| # | Finding | Area | Severity | Phase | Status |
|---|---------|------|----------|-------|--------|
| F93 | `UpdatePurchaseOrder` severs `purchase_order_line_items` ids with no status gate — the billed-receipt cancel guard is skipped entirely, GRNI never clears, and 3-way matching silently passes | Ledger integrity | **High** | 1.1b | Open |
| F70 | `UpdateOrder` deletes + reinserts `orderLineItems` with fresh ids, permanently nulling `outbound_delivery_line_items.orderLineItemId` — the outstanding-quantity prefill then re-offers already-shipped quantity | Data integrity | Medium-High | 1.1 | Open |
| F71 | `products.unit` is derived only at product-write time — renaming a unit of measure never re-derives it, and clearing `unitOfMeasureId` is handled only in the frontend | Correctness | Medium | 1.2 | Open |
| F72 | `updateProduct` / `updateTaxRate` map `*ValidationError` to 500 "internal error" instead of 409 with the real message | Correctness / UX | Medium | 1.3 | Open |
| F73 | Production orders write real `stockMovements` for a `stockEnabled = 0` product — the only stock-moving path in the repo with no such gate | Correctness | Medium | 1.4 | Open |
| F74 | `DeleteProductionOrder`'s `DELETE` lacks `AND status = 'draft'` — a delete racing a completion destroys a completed order, orphaning its stock movements and posted entry | Concurrency | Low-Medium | 1.5 | Open |
| F84 | A BOM editor fetch failure renders as an empty recipe; saving from that state silently wipes a real BOM | Frontend / data loss | Medium | 2.1 | Open |
| F85 | A failed BOM fetch on the Production Order page is reported as "this product has no Bill of Materials", with Create permanently disabled and no retry | Frontend | Medium | 2.2 | Open |
| F86 | Serial-capture confirm is double-submittable on production orders — the second `PATCH` 409s and toasts an error on top of the success | Frontend | Medium | 2.3 | Open |
| F87 | After saving a unit of measure, two rows render as "Default" and the product prefill can pick the stale one | Frontend | Low-Medium | 2.4 | Open |
| F88 | "Manage units of measure" navigates away and silently discards an in-progress product, no confirmation | Frontend | Low-Medium | 2.5 | Open |
| F89 | Production Order detail renders a fully blank page (not even the header) while loading and on fetch failure | Frontend | Low | 2.6 | Open |
| F75 | `production_order_component_lines` joins `componentSku`/`componentUnit` instead of snapshotting them, contradicting migration `0075`'s own comment | Correctness | Low | 3.1 | Open |
| F76 | `buildProductionOrderGLLines` validates the org's Inventory accounts *only* when a rounding residual exists — an unconfigured org fails randomly, not deterministically | Accounting | Low (decision) | 3.2 | Open |
| F77 | `RestoreBillOfMaterialsVersion` records no version when the restored content equals the latest, contradicting CLAUDE.md and its own doc comment | Audit trail | Low | 3.3 | Open |
| F78 | BOM version number is allocated on `d.DB` before `Beginx()` — concurrent saves collide on the unique index and 500 | Concurrency | Low | 3.4 | Open |
| F79 | `UpdateUnitOfMeasure` / `DeleteUnitOfMeasure` can leave an org with zero default unit of measure, silently killing the new-product prefill | Correctness | Low | 3.5 | Open |
| F90 | `any` reaches the whole Production Order detail view — 13 sites, regressing issue #143's completed typed-atom pass | Type safety | Low-Medium | 3.6 | Open |
| F91 | Two different list-search implementations shipped in the same release; one re-renders every row on each keystroke | Frontend consistency | Low | 3.7 | Open |
| F80 | Test-coverage cluster: the new F48 guard has no concurrency test (regressing F69's standing requirement), `crossOrgProof` omits 8 new routes, and the serialized-cancel + GL-reversal branches are dead code in CI | Test coverage | Low | 4.1 | Open |
| F81 | CLAUDE.md drift: the entire Units-of-Measure frontend is undocumented, the Settings-dropdown list omits it, and two stated guarantees are false | Docs | Low | 5.1 | Open |
| F92 | 79 msgids are untranslated in de/fr; 15 of them now render on screens this release touches, 3 directly on the new Production Order pages | i18n | Low | 5.2 | Open |
| F82 | CI has no i18n catalog-drift check and no `gofmt` / `format:check` gate | CI | Low | 5.3 | Open |
| F83 | `v3.20.0` was pushed as a lightweight tag; every prior release tag is annotated | Ops | Low | 5.4 | Open |

---

## 1. Backend correctness

### 1.1 — F70: Editing an order permanently severs its deliveries' line links

`db/order.go:358`'s `UpdateOrder` runs `DELETE FROM orderLineItems WHERE orderId = ?`
and reinserts every line with a **fresh** `gonanoid`.
`outbound_delivery_line_items.orderLineItemId` is
`TEXT REFERENCES orderLineItems(id) ON DELETE SET NULL`
(`db/migrations/0025_add_outbound_deliveries.up.sql:19`), so that delete nulls the link
on every already-shipped delivery — irreversibly.

Three consequences, all confirmed in code:

1. `GetOrderDeliveredQuantities` (`db/order.go:154`) and `orderFullyDeliveredTx`
   (`db/order.go:205`) both key on `dli.orderLineItemId` and filter `IS NOT NULL`, so
   both return an empty map after any order edit.
2. **The real severity driver:** that map is what prefills a new delivery with only the
   *outstanding* quantity per line. Empty, it re-offers the **full** quantity — quantity
   already shipped. Shipping it reduces stock again and posts COGS again
   (`buildDeliveryCOGSGLLines`).
3. PR #232's auto-advance cascade (v3.19.4) can never fire — an edited order stays
   `shipped` forever.

This is known and documented as an aside in `docs/ui-consistency-plan.md` ("saving an
order always resets its `Delivered` column to 0"), but scoped there as cosmetic. It is
not, and #232 shipped a feature on top of it.

**Fix direction:** reuse existing line-item ids for lines the request still carries
(match on incoming `id`; insert only genuinely new lines, delete only genuinely removed
ones) instead of a blanket delete-and-reinsert.

#### Precondition — resolved

The plan required enumerating every line-item replace helper before writing code,
because the id-reuse rewrite changes line-item *identity* semantics. Done. Six helpers
delete-and-reinsert with fresh `gonanoid`s (`db/order.go:416,499`,
`db/purchase_order.go:377,463`, `db/invoice.go:452,615`, `db/delivery.go:684`,
`db/inbound_delivery.go:818`, `db/incoming_invoice.go:607`), but only **three** foreign
keys in the whole schema reference a line-item table's `id`:

| Referencing column | Target | Severed by |
|---|---|---|
| `outbound_delivery_line_items.orderLineItemId` | `orderLineItems(id)` | `UpdateOrder` — **F70** |
| `inbound_delivery_line_items.purchaseOrderLineItemId` | `purchase_order_line_items(id)` | `UpdatePurchaseOrder` — **F93** |
| `incoming_invoice_line_items.purchaseOrderLineItemId` | `purchase_order_line_items(id)` | `UpdatePurchaseOrder` — **F93** |

All three are `ON DELETE SET NULL`.

**Decision: option (b) — the fix must cover both `orderLineItems` and
`purchase_order_line_items`.** The enumeration surfaced F93 (below), which is a strictly
worse instance of the same class and cannot be left behind while F70 is fixed. The
remaining four helpers — invoices, outbound deliveries, inbound deliveries, incoming
invoices — have **no** inbound FK to their line-item ids at all, so their
delete-and-reinsert is genuinely harmless and is deliberately left alone. Record that
here so a later audit does not re-raise them.

### 1.1b — F93: Editing a purchase order silently defeats the GRNI cancel guard

Found by 1.1's enumeration, not by the original sweep. Same mechanism as F70, materially
worse consequences.

`replacePurchaseOrderLineItemsTx` (`db/purchase_order.go:375-395`) deletes every
`purchase_order_line_items` row and reinserts with fresh `gonanoid`s. `UpdatePurchaseOrder`
(`db/purchase_order.go:~340`) applies it with **no status gate whatsoever** — neither the
DB layer nor `src/routes/purchase-orders/details.tsx` prevents editing a `received`
purchase order. `purchase_order_line_items.id` is the join key for four independent
subsystems:

1. **The GRNI cancel guard is skipped, not merely weakened.**
   `db/inbound_delivery.go:645-659` iterates the receipt's lines and does
   `if line.PurchaseOrderLineItemID == nil { continue }` before calling
   `grniClearedQtyForPOLine`. Once the FK is nulled, every line hits that `continue`, so
   the guard never runs and a **billed receipt becomes cancellable** — reversing the
   receipt's Dr GRNI / Cr Inventory entry while the bill's AP obligation and cost
   recognition both still stand. That is verbatim the outcome the guard's own comment
   (`:640-644`) says it exists to prevent, and the "Finding B" scenario
   `docs/inventory-cogs-integration.md` records as fixed in Phase 7.
2. **GRNI never clears.** `grniClearedQtyForPOLine` (`db/gl_posting.go:836`) and
   `grniAccrualForPOLine` (`:785`) both key on `purchaseOrderLineItemId`. A bill posted
   after the edit capitalizes its full amount to Inventory and nets nothing against the
   accrual, leaving GRNI permanently overstated.
3. **3-way matching silently passes.** `db/incoming_invoice_match.go:110,133` key on the
   same column, so every line reports `unlinked` — informational, never blocking — and
   `PreviouslyInvoiced` counts zero, dissolving the double-billing guard.
4. **Receipt prefill re-offers received quantity.** `GetPurchaseOrderReceivedQuantities`
   (`db/inbound_delivery.go:171-182`) returns empty, so a new receipt from that order
   offers the full quantity again — double stock receipt, the purchasing-side twin of
   F70's consequence 2.

**Severity: High.** This is the one finding in this audit that reaches the bar prior
audits reserved for F48's double-posting class — it does not merely produce a wrong
number, it disables a guard purpose-built to protect GL integrity, and the resulting
GRNI imbalance is silent and not self-correcting.

**Fix direction:** the same id-reuse rewrite as F70, applied to
`replacePurchaseOrderLineItemsTx`. Additionally consider — and record the decision —
whether `UpdatePurchaseOrder` should refuse line-item edits once any receipt exists
against the order, the way `outbound_deliveries` freezes line items at `shipped`
(CLAUDE.md's `outbound_deliveries.status` note). Id reuse fixes the severing; a freeze
would additionally stop quantities moving underneath an already-posted GRNI accrual,
which id reuse alone does not address.

### 1.2 — F71: `products.unit` denormalization is write-time only

CLAUDE.md:425 and CLAUDE.md:217 both state `products.unit` is "server-derived" from
`unitOfMeasureId` so every existing reader keeps working unchanged. `resolveProductUnit`
(`db/product.go:253`) does that on `CreateProduct`/`UpdateProduct` — but nothing ever
re-derives it afterwards. Two gaps:

- **Rename.** `UpdateUnitOfMeasure` (`db/unit_of_measure.go:132`) writes only
  `units_of_measure`. The only other `UPDATE products SET unit…` in non-test Go is the
  one-time seed backfill at `db/unit_of_measure.go:220`. Rename "kg" → "kilogram" and
  every linked product keeps showing `kg` in Inventory, the Products list and
  `db/product_bom.go:125`'s `componentUnit` join, indefinitely — until someone happens
  to re-save each product through `UpdateProduct`.
- **Clear.** `resolveProductUnit` returns the caller's `fallbackUnit` untouched when
  `unitOfMeasureID` is nil/empty, so `UpdateProduct` writes `unitOfMeasureId = NULL`
  with whatever `unit` text the client sent. Commit `47e1e0c` ("prefill default unit of
  measure and clear it correctly") was **+17 lines in `src/components/products/form.tsx`
  only** — zero backend files, zero tests. Any caller that echoes the legacy `unit` back
  while nulling `unitOfMeasureId` (a direct API client, a future second form)
  reproduces exactly the ghost-value bug that commit fixed.

**Fix direction:** propagate the new name to linked products inside
`UpdateUnitOfMeasure`'s existing transaction; decide server-side — not in the form —
what `unit` becomes when `unitOfMeasureId` is cleared.

### 1.3 — F72: Validation errors surfaced as 500

`api/products.go:80` (`updateProduct`) and the equivalent in `api/tax_rates.go` call
`writeInternalError`; their `create` twins call `writeMutationError`
(`api/products.go:58`), which is what maps a `*ValidationError` to a 409 carrying the
real message (`api/helpers.go:112`).

`UpdateProduct` returns two validation errors and `UpdateTaxRate` one, all
user-reachable — including the serialized-toggle refusal ("stock is non-zero"), the
invalid-category rejection, and the new cross-org `unitOfMeasureId` rejection that
`resolveProductUnit` added. Today the user sees `500 {"error":"internal error"}`, the
frontend's `message.error(error.message)` shows that string, and the server logs it as
an internal error.

Every other update handler checked — `updateAccount`, `updateInvoice`,
`updatePaymentTerm`, `updateUnitOfMeasure`, `updateImport` — already uses
`writeMutationError`. This is drift, not a convention. Sweep the rest of `api/` for the
same shape while in there.

### 1.4 — F73: No `stockEnabled` gate on production orders

`CreateProductionOrder` (`db/production_order.go:169`) gates only on
`Category == "finished"`. Neither it nor `UpdateProductionOrderStatus` checks
`product.StockEnabled`. Every other stock-moving path in the app gates on it at the SQL
level: `getShippableStockLines` (`db/delivery.go:297`) and `getReceivableStockLines`
(`db/inbound_delivery.go:409`) both carry `AND p.stockEnabled = 1`.

A `category='finished'`, `stockEnabled=0` product therefore gets real `stockMovements`
rows (`db/production_order.go:521`), a non-zero `products.stockQuantity`, and — on a
non-exact division — an Inventory / Inventory-Adjustment GL entry, for a product the
rest of the app treats as not participating in inventory at all.

Components are only *incidentally* protected: a `stockEnabled=0` component has
`stockQuantity = 0`, so the insufficient-stock guard 409s. That is accidental, not a
guard.

### 1.5 — F74: `DeleteProductionOrder` TOCTOU

`db/production_order.go:668` reads status at `:669`, then runs
`DELETE FROM production_orders WHERE id = ?` at `:677` with **no** `AND status = 'draft'`.
`RowsAffected` is read at `:681` but only as a found/not-found signal, not as a race
guard.

A delete racing a concurrent `draft → completed` destroys a **completed** order:
`production_order_component_lines` cascades away (migration `0075`, `ON DELETE CASCADE`)
while the consume/produce `stockMovements` rows survive with a `sourceDocumentId`
pointing at nothing, plus — if a rounding residual posted — an orphaned posted
`journal_entries` row that the cancel path can never reverse.

This *matches* `DeleteInboundDelivery` (`db/inbound_delivery.go:784`, identical shape)
but *diverges* from the canonical F48 delete pattern CLAUDE.md documents and
`TestConcurrentDeleteJournalEntryDoesNotDeleteAPostedEntry` (`db/concurrency_test.go:236`)
locks in. **Fix both sites.**

Trivial, same file: the refusal message at `db/production_order.go:673` tests
`!= "draft"`, so for a cancelled order it renders as *"cannot delete a cancelled
production order — cancel it instead."* `DeleteInboundDelivery`
(`db/inbound_delivery.go:789`) avoids this by testing the specific status.

---

## 2. Frontend reliability

### 2.1 — F84: A BOM editor fetch failure silently presents as an empty recipe

`src/components/products/bom-editor-drawer.tsx:128-139` is a
`Promise.all(...).then(...)` with **no** `.catch`; `:183` and `:215` likewise. Three
consequences:

1. An unhandled promise rejection — Sentry noise.
2. The drawer shows an empty recipe, indistinguishable from "no components yet" — and a
   Save from that state **wipes a real BOM**. This is the data-loss path.
3. A failed version fetch leaves `loading={!viewedVersion}` (`:342`) spinning forever
   with Restore disabled (`:269`), escapable only by closing the drawer.

### 2.2 — F85: A fetch failure is reported to the user as missing data

`src/routes/production-orders/details.tsx:123-125` — `.catch(() => setBomLines([]))`
swallows the error with no toast. The empty result then drives the warning Alert at
`:231-243` ("define one before creating a production order") and disables Create at
`:292`. On a transient 500 or an offline blip the user is told to go fix data that is
already correct, with no retry path and a permanently disabled button.

### 2.3 — F86: Serial-capture confirm is double-submittable

`src/routes/production-orders/details.tsx:554-568` never passes the `confirming` prop
that `src/components/stock/serial-capture-modal.tsx:44,157` supports, so OK stays
enabled through the `await applyStatusChange(...)` at `:377-380`. A second click fires a
second `PATCH /production-orders/{id}/status`; `draft → completed` has already consumed
stock, so the retry 409s and toasts an error on top of the success.

`src/routes/inbound-deliveries/details.tsx:614-620` omits it too — this is sibling
parity, not a new idea. Fix both; the production path is the one that moves stock in
both directions.

### 2.4 — F87: Stale `isDefault` after a unit-of-measure save

`src/atoms/unit-of-measure.ts:59-73` merges only the created/updated row into
`unitsOfMeasureAtom`, but the server clears the previous default inside the same
transaction (`db/unit_of_measure.go:80-88` on create, `:117-127` on update).

Consequence: `src/routes/settings/units-of-measure.tsx:94` renders a ✓ on two rows until
the next fetch, and `find(unitsOfMeasure, { isDefault: 1 })` at
`src/components/products/form.tsx:152` can select the stale unit (lodash `find` returns
the first name-ASC match). Inherited verbatim from `src/atoms/payment-term.ts`, but
payment terms have no equivalent prefill consumer — this is where it bites. Mitigated
only because the product drawer refetches on open (`form.tsx:115-119`).

### 2.5 — F88: Navigating to unit-of-measure settings discards an in-progress product

`src/components/products/form.tsx`, the `popupRender` block (~`:466-474`):
`<Link to="/settings/units-of-measure">` replaces router state, and the drawer's
visibility is `get(location.state, "productModal")` — so the drawer unmounts and every
unsaved field is lost, with no confirmation. The `e.stopPropagation()` there only stops
the Select popup from closing, not the navigation.

### 2.6 — F89: Blank page while loading and on failure

`src/routes/production-orders/details.tsx:401` — `if (!isNew && !order) return null;`
covers both the loadable's `loading` state and the atom's error path
(`src/atoms/production-order.ts:66-70` toasts, then returns `null`). The user gets a
fully empty page, not even the PageHeader, with no retry.

This matches `src/routes/inbound-deliveries/details.tsx:317`, so it is an established
convention — but a bad one, and this is new surface.

---

## 3. Design and consistency

### 3.1 — F75: Incomplete component-line snapshot

`production_order_component_lines` (`db/migrations/0075_add_production_orders.up.sql:44`)
denormalizes only `componentName`. `GetProductionOrderComponentLines`
(`db/production_order.go:113`) resolves `componentSku`/`componentUnit` via
`LEFT JOIN products`, so both become NULL once the component product is deleted — while
`componentName` survives.

The immediately preceding migration solved exactly this: `bill_of_materials_version_lines`
(`0074_add_bill_of_materials_versions.up.sql:37-40`) snapshots `componentProductId` +
`componentName` + **`componentSku` + `componentUnit`** for the same `ON DELETE SET NULL`
reason, and `db/product_bom.go:273-275` populates them. Migration `0075`'s own comment
(`:41`) claims it follows "the same shape `bill_of_materials_version_lines` already
uses" — it does not, for two of the four columns.

### 3.2 — F76: Inventory-account validation is conditional on a rounding residual

`buildProductionOrderGLLines` (`db/gl_posting.go:1186`) returns `nil, nil, nil` when
`netCents == 0` **before** checking `org.DefaultInventoryAccountID` /
`DefaultInventoryAdjustmentAccountID`. Exact division is the common case — always true
for a BOM built from whole-number `quantityPerUnit`s, since order quantity cancels out
algebraically — so an organization with no inventory GL accounts configured completes
production orders fine for months, then 409s on one odd batch. Every other Phase 7
posting path fails deterministically when unconfigured.

**Decision: hoist** — implemented in Phase 3 (PR #246), with a CHANGELOG line. Three
reasons. It matches the documented Phase 7 contract that *every* posting path refuses
until these are configured — shipping a delivery already 409s unconditionally, so
production orders failing only sometimes was the inconsistency. A deterministic failure
at setup time beats a mystery 409 on one batch in a hundred. And the blast radius is
small, which is what settled it: `main.go` runs
`SeedInventoryAccountingDefaultsForAllOrganizations` on **every startup**, backfilling any
organization with any of the four columns NULL (`db/account.go:732`), so the unconfigured
state is only reachable by deliberately clearing the field and not restarting — and such
an organization is already blocked at its next shipment anyway.

### 3.3 — F77: Restore can record no version

`RestoreBillOfMaterialsVersion` (`db/product_bom.go:420`) delegates to
`ReplaceBillOfMaterials`, which at `db/product_bom.go:340-356` sets `changed = false`
when the component set, quantities and `batchSize` all match the latest version.
Restoring a version whose content equals the latest therefore returns 200 with lines,
the drawer reports "Version restored", and **no version row is created**.

That contradicts three written claims: CLAUDE.md:225 ("non-destructive — itself creates
a new version"), CLAUDE.md:379 ("so it creates a new version rather than rewriting
history"), and the function's own doc comment at `db/product_bom.go:413`.
`TestRestoreBillOfMaterialsVersion` (`db/product_bom_test.go:383`) only restores a
*differing* version, so the suite never exercises it.

Either record a version unconditionally on the restore path, or correct all three
pieces of prose.

### 3.4 — F78: BOM version number allocated outside the transaction

`db/product_bom.go:289-298` reads `MAX(versionNumber)` on `d.DB` *before* `Beginx()` at
`:363` — deliberately, for the `SetMaxOpenConns(1)` constraint documented at `:236-238`.
`withDB` takes a **read** lock (`api/router.go:80-85`, `h.dbMu.RLock()`), so two concurrent
`PUT /api/products/{id}/bom` can both compute `nextVersionNumber = N+1`, and the second
hits the unique index `bill_of_materials_versions_product_number`
(`0074…up.sql:23-24`).

Consequence: a 500 (`fmt.Errorf` at `:389` → `writeInternalError`) with the whole save
rolled back, rather than a retry or a 409. Integrity is preserved by the index; only the
UX and error class are wrong. Low probability, real.

### 3.5 — F79: An org can end up with zero default units of measure

**Decision: reviewed, left unchanged — not a defect.** (Same disposition as the
2026-08-13 audit's F63: recorded as a decision rather than a fix, so it isn't re-raised.)

`db/unit_of_measure.go:132-137` applies `isDefault = COALESCE(?, isDefault)`, so an
explicit `0` unsets the organization's only default and promotes nothing.
`DeleteUnitOfMeasure` (`db/unit_of_measure.go:164-171`) has the same hole.

But both paths are reachable only through an explicit user action:
`src/components/units-of-measure/form.tsx:130` exposes `isDefault` as a plain Checkbox
labelled "Default for new products", and deleting the row is a `Popconfirm`. Unchecking
that box, or deleting that row, means "I don't want a default any more" — the prefill
stopping is the requested outcome, not silent data loss, so this finding's original
"silently stops prefilling" framing overstates it.

The alternatives are both worse: refusing to unset or delete the last default makes a
reasonable action impossible, and auto-promoting an arbitrary sibling installs a default
the user never chose. The same reasoning applies to `taxRates` and `payment_terms`, which
share the shape.

### 3.6 — F90: `any` regression against issue #143

Root cause: `src/atoms/production-order.ts:56-71` returns an undeclared
`{ ...order, componentLines }` shape with no exported type. That forces **9** `any` sites
in `src/routes/production-orders/details.tsx` — `:106, :134, :168, :214, :322, :366,
:382, :386, :467` — leaving `currentOrder.finishedProductName` / `.componentLines` /
`.quantity` / `.status` and the `serialized === 1` comparison entirely unchecked. Four
more in `src/components/stock/serial-capture-modal.tsx:195-196` and
`src/components/units-of-measure/form.tsx:32,41`.

Issue #143's typed-atom pass (PRs #197/#198) is recorded as complete — this is new drift
against a finished initiative.

Also dead code: `:366` and `:382`'s `!(order as any).then` guards are vestigial
copy-paste from the pre-`loadable` sibling. `loadable()` already unwraps the promise, so
they can never be true.

### 3.7 — F91: Two search standards shipped in one release

`src/routes/production-orders.tsx:23,42-49` and
`src/routes/settings/units-of-measure.tsx:17,36-40` use a module-level `searchAtom` plus
an unmemoized `filter(...)`; `src/components/page-header.tsx:38-45` has no debounce.
Every keystroke writes a global atom, rebuilds `dataSource`, and re-renders every
visible row.

`src/routes/bill-of-materials.tsx:34,44-52` — new in this *same* commit range — does it
correctly with `useState` + `useMemo`. The inconsistency is the finding; severity scales
with row count.

---

## 4. Test coverage

### 4.1 — F80: Test-coverage cluster

Grouped because they share one remediation PR.

**F69 regression.** `UpdateProductionOrderStatus` implements the F48 `liveStatus`
re-check correctly (`db/production_order.go:429-437`, byte-for-byte the
`db/inbound_delivery.go:516-523` / `db/delivery.go:444-451` shape). But
`db/concurrency_test.go` has a dedicated test for **all five** other F48-guarded paths
and **none** for production orders. F69 (`docs/audit-plan-2026-08-13-fix-review.md:48`)
made exactly this a standing requirement: *"`go test -race` passing proves nothing about
them"*. A new F48 site shipped reintroducing the gap.

**`crossOrgProof` drift.** `api/cross_org_test.go`'s behavioural deny-path list gained
the three BOM-version routes (`:412-414`) but **not** the five production-order routes
or the three units-of-measure routes. Not a live vulnerability — all are
`orgMemberProtected` with correct by-id resolvers and `TestPhaseCRouteCoverage` accepts
them as self-evidently gated — but the deny path is unproven behaviourally, in exactly
the place the feature's own template (three payment-terms entries at `:401-403`) proves
it.

**Dead branches in CI.**
- The serialized production-order cancel path (`db/production_order.go:573-620`):
  `TestUpdateProductionOrderStatusCompleteThenCancelReversesEverything`
  (`db/production_order_test.go:270`) uses a non-serialized fixture, so the whole
  `finished.Serialized == 1` reversal branch has never executed.
- The GL-reversal-on-cancel branch (`:414`, `:646-650`): the two GL tests are disjoint by
  construction — `...CompletePostsRoundingResidual` (`:202`) creates a residual entry but
  never cancels; `...CompleteThenCancelReversesEverything` uses the exact-division
  fixture, so `existingEntry` is always `nil`. No test has ever reversed a production
  order's journal entry.
- `GetImportProductionOrderCount`'s branch of `DeleteImport` (`db/import.go:288`):
  `db/import_test.go:127` only links a *purchase order*.
- `NextProductionOrderNumber` (`:131-140`) and `GetProductionOrders` (`:89-100`): zero
  tests, including list org-scoping.
- `validateSerialAgainstImportRange` (`:261-288`): only the out-of-range branch is
  tested. The prefix-mismatch (`:269`), non-numeric-suffix (`:275`) and
  range-not-configured (`:262`) branches are untested.
- `draft → cancelled` (a legal transition matching no switch case, so a pure status
  flip), illegal transitions generally, `Quantity <= 0`, serialized-finished non-integer
  quantity, import-not-found, the defensive serialized-component re-check at completion
  (`:357-362`), and the `resolveMovementCost` 409 path (`:372-375`).

**No `TestCreateProductionOrderRejectsCrossOrgReferences`.** `db/org_ownership_test.go`
has one for **13** other domains; `CreateProductionOrder`'s two cross-org-referenceable
fields (`FinishedProductID` at `:166`, `ImportID` at `:181`) are both guarded and both
untested.

**UoM / BOM gaps.** `db/unit_of_measure_test.go` (7 tests) covers none of: rename (F71),
clearing a product's `unitOfMeasureId` after it was set (F71),
`SeedDefaultUnitsOfMeasureForAllOrganizations` **at all** (neither the `COUNT(*)==0`
per-org gate, the idempotency claim, nor the case-insensitive backfill at
`db/unit_of_measure.go:219-229`, despite CLAUDE.md:425 describing all three),
`UpdateUnitOfMeasure` cross-org rejection, `IsDefault == nil` defaulting, unsetting the
last default, or `DeleteUnitOfMeasure` on a missing id.
`TestCreateProductRejectsCrossOrgUnitOfMeasure` (`:188`) asserts only `err != nil`, never
the `*ValidationError` type — so it would still pass if the error class regressed to a
500, which is precisely F72's bug.

`db/product_bom_test.go` (13 tests) is strong on the version-history core but covers
none of: restoring a version equal to the current recipe (F77), restore creating version
N+1 with the *restored* `batchSize`, first-save `batchSize <= 0 → 1`
(`db/product_bom.go:308-314`), clearing a BOM with no version history, or line
reordering.

**Nine new endpoints, zero api-level tests.** Stated narrowly: 0% `api/` handler coverage
is the repo-wide norm (measured across 17 handler files) and is **not** claimed as a
regression. The narrow point is that the new endpoints inherit it, and F72's 409-vs-500
asymmetry is exactly the class of bug an api-level test would have caught.

---

## 5. Documentation, i18n, CI, ops

### 5.1 — F81: CLAUDE.md drift

- The entire Units-of-Measure **frontend** is absent from CLAUDE.md:
  `src/routes/settings/units-of-measure.tsx`, `src/components/units-of-measure/form.tsx`
  and `src/atoms/unit-of-measure.ts` have zero mentions, while the backend
  (`db/unit_of_measure.go`) is documented in detail.
- CLAUDE.md:479's Settings-dropdown list omits "Units of measure", which
  `src/layouts/base.tsx:259` actually renders.
- CLAUDE.md:379 claims `roundBOMQuantity`'s 4 decimal places exist "specifically so this
  scale-divide-scale round trip doesn't visibly drift (e.g. 1 unit at a batch of 3 must
  redisplay as exactly 3, not 2.9997)". The code stores `1/3 → 0.3333`
  (`db/product_bom.go:97-99`) and `src/components/products/bom-editor-drawer.tsx:136`
  redisplays `0.9999`. `db/product_bom_test.go:280-290` asserts that *as correct*,
  calling it "an inherent, arithmetically honest consequence of storing only 4 decimal
  places". The code is fine and non-compounding (a re-save settles back on `0.3333`,
  proven at `:291-303`); the prose is false. Worth adding the undocumented downstream
  consequence: `db/production_order.go:186-215` multiplies `quantityPerUnit` by order
  quantity, so a 3-unit run consumes 0.9999 of that component, not 1.
- CLAUDE.md:225 / :379's restore-creates-a-version claim — see F77.

### 5.2 — F92: i18n

Measured, not estimated:

- New msgids added in this commit range: **59** per locale (985 → 1043 entries).
- Untranslated **new** strings: **de 0, fr 0, en 0**. `pnpm extract` *was* run (at
  `675bf77`) and the new feature is fully translated.
- Untranslated **total**: **de 79, fr 79** (identical msgid sets), en 0 — all
  pre-existing and unchanged since before `v3.19.6`.
- **15 of those 79 now render on screens this release touches**, i.e. new English
  leakage into the de/fr UI: `Qty per unit`, `Import`, `Date is required`, `Prefix`,
  `Range start`, `Select component`, `Add component`, `Component is required`,
  `Failed to save bill of materials`, `Category`, `Component / intermediate`,
  `Unclassified`, the "filters this product out of purchasing pickers" tooltip,
  `Document Templates`, `Imports`. `Qty per unit`, `Import` and `Range start` appear
  directly on the new Production Order screens — a German or French user meets English
  text on brand-new pages.
- Catalog freshness: `#:` source references are stale (the last extract predates
  `2a15e43`, which moved the footer/Popconfirm code). No missing strings — cosmetic
  churn on the next `pnpm extract`.

### 5.3 — F82: CI gaps

`.github/workflows/ci.yml` runs `go vet`, `go test -race -timeout=28m`, `govulncheck`,
`pnpm audit --prod --audit-level high`, `pnpm lint`, `pnpm test` and `pnpm build`. It
does **not** run:

- `pnpm extract` followed by `git diff --exit-code` on `src/locales/` — catalog drift is
  this repo's own recurring failure mode (#216 found 80 missing entries, 12 of them
  missing from English too).
- `gofmt -l .` or `pnpm format:check` — both pass today, so gating them costs nothing
  and prevents the drift.

Pair this with F92: translating the 15 leaking strings fixes the present, the gate stops
it recurring.

### 5.4 — F83: Release-tag hygiene

`v3.20.0` on `origin` is a **lightweight** tag pointing directly at `fc2a151`;
`v3.19.0`–`v3.19.6`, `v3.9.0`, `v3.8.0` and `v3.2.0` are all annotated (`git cat-file -t`
returns `commit` vs `tag`). The Docker build triggered regardless, so this is cosmetic —
retag annotated, or record the convention change.

---

## Watch list — suspicions, not confirmed

Recorded so a future audit doesn't re-derive them. Do not fix blind; each needs its own
confirmation first.

- `src/components/stock/serial-capture-modal.tsx:83-91`'s reset effect depends on
  `lines`, which `details.tsx:557-564` passes as a fresh array literal every render. Any
  parent re-render while the modal is open would wipe typed serials. No trigger exists
  today (the detail route does not subscribe to `productionOrdersAtom`) — latent, not
  live.
- `src/atoms/production-order.ts:39-47` caches the next document number; creating two
  orders in one session without a reload can prefill the same `PRO-nnnn`, and migration
  `0075` has no unique index on `orderNumber`. App-wide convention
  (`src/atoms/inbound-delivery.ts:44`) — no document-number table in this repo is
  unique-indexed.
- `src/components/products/form.tsx:150-154`'s prefill effect re-runs on every
  `unitsOfMeasure` identity change; if the drawer-open fetch resolves after the user has
  already picked a unit, the org default overwrites their choice. Narrow but real.
- `src/components/products/bom-editor-drawer.tsx:124-145` has no cancellation flag
  (unlike `details.tsx:117-131`), so rapid product switching in the picker can apply a
  stale recipe.
- Sub-cent GL-vs-valuation drift: `db/production_order.go:382` rounds
  `componentsCostTotal` to whole cents before the residual is computed, but the consume
  movements record `unitCost = NULL` (`:475-484`) and `recomputeAverageCostTx` replays
  them at the *exact* running average. Likely accepted — CLAUDE.md already names this
  class a deliberate visibility property (`GetInventoryValuation`'s `Difference`) — but
  `buildProductionOrderGLLines`' own doc comment claims exact agreement.
- Legacy pre-`0074` recipes are never captured as version 1; worse, saving an empty list
  against a pre-`0074` BOM hits `db/product_bom.go:357-361` and deletes the live rows
  with **no** version recording that the recipe ever existed.
- BOM line **reordering** is not detected as a change (the no-op comparison is a map
  lookup, `db/product_bom.go:341-355`), so the live table is rewritten — changing
  `GetBillOfMaterials`' ordering — while history says "unchanged".
- `SeedDefaultUnitsOfMeasureForAllOrganizations` (`db/unit_of_measure.go:202-232`) runs
  outside any transaction; a crash after the 3rd of 13 inserts leaves `count > 0`, so the
  `:213` gate makes that org permanently under-seeded. Byte-for-byte the same shape as
  `SeedDefaultPaymentTermsForAllOrganizations` — precedent, but a weak one.
- `GetBillOfMaterialsVersions` (`db/product_bom.go:145-161`) has no org predicate
  (acknowledged in its own doc comment), safe only because of the router gate, while
  `GetBillOfMaterialsVersionDetail` does verify the parent. Defense-in-depth asymmetry.
- `db/unit_of_measure.go:139-141` dereferences `*updates.Name` in a branch guarded only
  by `isDuplicateUnitOfMeasureName(err)`. Currently unreachable; one added unique index
  away from a server panic. `UpdatePaymentTerm` has the identical shape.

---

## Explicitly excluded — do not re-raise

Each of these was checked during this audit and dismissed with a reason.

- **The 40+ `react(set-state-in-effect)` oxlint warnings.** Repo-wide and pre-existing —
  they fire in `clients.tsx`, `orders.tsx`, `payment-panel.tsx`, `login.tsx` and 25 more
  as much as in the new files. Not recent-change drift; a separate initiative if ever.
- **0% `api/` handler coverage as a general claim.** Measured: it is the norm across 17
  handler files (`accounts.go`, `vendors.go`, `imports.go`, `products.go`, … all 0%).
  Only the narrow point in F80 stands.
- **Float stock comparison without an epsilon** at `db/production_order.go:366`. Checked
  against `db/delivery.go:362`, which uses the identical bare `>`. The `1e-9` epsilon
  precedent applies only to `db/order.go:238` and `db/product.go:356`. Consistent with
  convention, not a defect.
- **English-only server error text.** Standing product decision (F45, 2026-08-09) —
  CLAUDE.md says do not re-flag.
- **`ProductionOrderComponentLine.CreatedAt *string`** against an INTEGER column —
  matches `StockMovement.CreatedAt`'s existing precedent; `database/sql` coerces
  int64 → string on the reflect path.
- **`src/utils/units.ts`** is not vestigial. `UNIT_OPTIONS` is cleanly gone (zero
  remaining importers); `unitLabel` is still live in `inventory.tsx:325`,
  `products.tsx:262,296` and `bill-of-materials.tsx:118`. Its 5-word translation table
  now covers only a subset of arbitrary per-org units, which the file's own comment
  already documents as accepted.
- **Authorization.** Verified clean. All 12 new routes are correctly gated
  (`api/router.go:315-321, 430-433, 462-464`): `orgMemberProtected` with by-id org
  resolvers (`productionOrderOrgID`, `unitOfMeasureOrgID`, `productOrgID`), or bare
  `protected` plus an inline `h.requireOrgMember` for the two POST routes carrying
  `organizationId` in the body — both registered in `createRouteOrgChecks`, which
  AST-verifies the call is present. `TestPhaseCRouteCoverage` is live.
  `POST .../bom/versions/{versionId}/restore` is doubly protected: `productOrgID` on
  `{id}` plus `GetBillOfMaterialsVersionDetail`'s parent check
  (`db/product_bom.go:180-182`). The only gap is *test* coverage (F80), not access
  control.
- **Migration `0075` schema shape.** FK `ON DELETE` clauses, indexes, the status CHECK
  constraint and the `.down.sql` were all checked against `0030_add_purchase_orders` and
  are consistent. The absence of `UNIQUE (organizationId, orderNumber)` is not drift —
  no document-number table in this repo has one.
- **`db/reset.go` coverage.** `production_orders` (correctly ordered before `imports`),
  `units_of_measure` and `bill_of_materials_versions` are all listed, and the two child
  tables without an `organizationId` column are correctly omitted.
  `TestResetOrganizationDataCoversEveryOrganizationScopedTable` is live and introspects
  `sqlite_master`.
- **Transaction boundaries and the `SetMaxOpenConns(1)` constraint** in
  `db/production_order.go` and `db/product_bom.go`. Every `d.DB` read precedes
  `Beginx()`; the transaction bodies use only `tx.*` helpers. Checked line by line.
- **F61/F68 empty-`req.ID` nanoid guards** are present on both new create paths
  (`db/production_order.go:155-157`, `db/unit_of_measure.go:60-62`).
- **F56/F67 atom rethrow convention** is satisfied: `createProductionOrderAtom` rethrows
  (`production-order.ts:91`), the status/delete atoms return booleans that callers check
  (the `incoming-invoice.ts` variant CLAUDE.md explicitly blesses), and
  `unitOfMeasureAtom` rethrows with the drawer staying open on failure.
- **Accessibility, `key` props, the `{0 && <jsx>}` footgun, money handling and org date
  formatting** in the new frontend code — all checked, all clean. Every new `<Button>`
  has a text child; `useDateFormatter` / `useDatePickerFormat` are used on both the
  Production Orders list and detail.

---

## Remediation phases

**Phase 1 — backend correctness (1.1–1.5).** F70 and F93 are one fix applied to two
helpers and belong in a single commit — the enumeration in 1.1 established that leaving
either behind leaves the class half-fixed. Do F93's optional receipt-freeze question as
a separate, explicitly recorded decision; id reuse alone does not cover it.

**Phase 2 — frontend reliability (2.1–2.6).** The error/loading-state cluster. F84's
"save wipes a real BOM" is the one with data-loss potential.

**Phase 3 — design and consistency (3.1–3.7).** F76 needs its decision recorded in this
document before any code moves.

**Phase 4 — test backfill (4.1).** Start with the F48 concurrency test and the eight
missing `crossOrgProof` entries — the two that regressed an explicit standing
requirement.

**Phase 5 — docs, i18n, CI, ops (5.1–5.4).** F92 and F82 belong in the same PR: the
translations fix the present, the CI gate stops it recurring.

---

## Verification

Beyond the mandatory per-phase gates (`go vet ./... && go test -race ./...`,
`pnpm lint && pnpm build`):

- **F70** — DB-layer test: create an order, ship a partial delivery, `UpdateOrder`, then
  assert `GetOrderDeliveredQuantities` still returns the shipped quantity and
  `orderFullyDeliveredTx` still reasons correctly. Then in the running app: create a
  delivery from the edited order and confirm the prefilled quantity is the *outstanding*
  one, not the full one.
- **F93** — DB-layer test, and it must assert the guard, not just the link: create a
  purchase order, receive it, bill it, `UpdatePurchaseOrder`, then attempt to cancel the
  receipt and assert it is **refused** with "has already been billed against this
  receipt". A test that only checks `purchaseOrderLineItemId` survived would pass against
  a fix that restored the id but broke the guard some other way. Also assert
  `grniClearedQtyForPOLine` still returns the billed quantity and that the 3-way match
  still reports the line `matched` rather than `unlinked`.
- **F71** — rename a unit of measure through Settings and confirm the Products list,
  Inventory and the BOM drawer's component unit all show the new name without re-saving
  any product.
- **F72** — `curl` a `PUT /api/products/{id}` that toggles `serialized` on a product
  with stock; expect 409 plus the real message, not 500.
- **F73, F74, F76, F77, F78, F79** — DB-layer tests. F76 additionally needs a CHANGELOG
  line if the hoist option is chosen.
- **F84–F89** — drive the real app (`go run .` + `pnpm dev`) through the Chrome MCP
  tools. This repo's convention is that UI fixes are verified live, not just by passing
  tests. For F84 and F85 specifically, *force* the failure (stop the backend mid
  drawer-open) rather than only testing the happy path.
- **F92** — switch the UI to de and then fr and walk the Production Order and Bill of
  Materials screens looking for English.
