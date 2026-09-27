# Stock upload from an Excel template — plan

Status: proposal, not started (2026-09-27). Decided: counted quantity (not a delta); movements dated "now"; unit cost optional. Motivated by onboarding ELECTRO MISSAOUI: 107 products
created through the Products Excel import, each with a physical count that currently has to be typed
in one movement at a time on the Inventory screen.

## Goal

On the Inventory screen, **Download Excel** gives a template pre-filled with every stock-enabled
product and its current quantity. The user fills in a **Counted Quantity**. **Upload Excel** then
brings each product to that count by posting the difference as a stock-count movement. The rows go
through the same `CreateStockMovement` path as a manual movement, so stock, average cost and the GL
behave exactly as they do today.

## Key decisions (proposed)

1. **The file sets the quantity; it doesn't add to it.** A row says "we have N". The server
   computes `delta = N − current stockQuantity` and posts a `count_addition` (delta > 0) or a
   `count_subtraction` (delta < 0) movement. Delta 0 posts nothing.
   - Uploading the same file twice is harmless: the second upload finds every delta at 0.
     This matters because the mass-data engine continues past failed rows, so the recovery
     from a partly failed upload is simply "fix the red rows and upload the whole file again".
   - It also matches how a physical count works, and those two movement types already exist
     (`db/stock.go:119`, `src/components/stock/movement-form.tsx`).
   - A "delta" mode (add N) is deliberately left out. It is not idempotent, so a re-upload
     would double the stock.
2. **Finding the product:** by the `Product ID` column when it has a value (the downloaded template
   always fills it). Otherwise by `SKU`, then by exact `Product Name` within the organization. If
   the name matches more than one product, that row fails with an error. A file typed from scratch
   (like the ELECTRO MISSAOUI list, which has no SKUs) still works.
3. **Unit Cost column (optional):** used as the movement's `unitCost` on additions. It feeds
   `recomputeAverageCostTx` and the GL value, falling back to `products.unitCost` like
   `buildStockAdjustmentGLLines` already does. See open question B for rows that have no cost at all.
4. **Rows the upload rejects:**
   - products that aren't stock-enabled;
   - negative or non-numeric counts;
   - **serialized products** (`CreateStockMovement` already rejects count movements for them, so
     the error just needs to be clear: "use a manual Stock In with serial numbers").
     A "Serial Numbers" column could come in a later phase.
5. **Every movement gets a reference** such as `Stock upload 2026-09-27` plus the row's Note, so an
   upload can be found in the movement list and in the GL entries' descriptions.

## Template (sheet `Data`, first sheet is the one read)

| Product ID | Product Name | SKU | Current Quantity | Counted Quantity | Unit Cost | Note |
|---|---|---|---|---|---|---|

- `Current Quantity` is filled on export and **ignored** on import. It's there for reference and
  to make the file readable.
- Import reads `Counted Quantity`. **A blank count skips the row.** It does not zero the stock:
  a forgotten row must never wipe stock, the same reasoning as mass data's "no bulk delete".
  A literal `0` does set the stock to zero.

## Implementation

### Backend
- `db/mass_data_stock.go`: a `stockMassDataSpec` implementing `massDataSpec`:
  - `ExportRows`: products with `stockEnabled = 1`, in the list order.
  - `NewImportContext`: maps of ID/SKU/name → product, and name → count so duplicate names can
    be detected.
  - `ImportRow`: resolve the product, validate, compute the delta, call
    `d.CreateStockMovement(...)` with type `count_addition`/`count_subtraction` and a
    signed `Quantity`.
- `db/mass_data.go`: the engine only knows `created`/`updated`. Add an `unchanged` action plus a
  count to `MassDataImportResult`. A stock row reports `adjusted` (counted as Updated) or
  `unchanged`. This is additive and existing specs are unaffected.
  - The current quantity is read when the row is processed, not once at context build, so two rows
    for the same product (by ID and by name) don't both apply the full delta.
- `api/mass_data.go`: `exportStock`/`importStock` handlers.
- `api/router.go`: `orgMemberProtected("GET", "/api/organizations/{orgId}/stock-movements/export", …)`
  and `orgMemberProtected("POST", "/api/organizations/{orgId}/stock-movements/import", …)`.
  This is the same gate as a manual movement (`createStockMovement` requires org membership), and
  the `stock-movements` path prefix makes the section guard (`api/sections.go`) treat it as
  Inventory automatically. Update the route-coverage tests (`api/cross_org_test.go`,
  `api/sections_test.go`) as the build requires.

### Frontend
- `src/api/index.ts`: add `"stock-movements"` to `MassDataResource`, and `unchanged` to the result
  type.
- `src/routes/inventory.tsx`: render `MassDataExcelActions` (resource `stock-movements`,
  filename `stock-count`). Refresh the movement list and products on import.
- `mass-data-excel-actions.tsx`: show "unchanged" in the result modal's summary.
- `pnpm extract` last, before committing.

### Tests (`db/mass_data_stock_test.go`)
- Setting a higher count posts one `count_addition` with the right delta, and a lower count
  posts one `count_subtraction`.
- Re-uploading the same file gives all rows `unchanged` and no new movements.
- A blank count is skipped; `0` zeroes the stock.
- Lookup by ID, by SKU and by name. A duplicate name is a row error.
- Serialized products, non-stock products and negative counts are row errors, and the other rows
  still apply.
- GL: an addition with a unit cost posts Dr Inventory / Cr Inventory Adjustment. A shortage uses
  the average cost. A shortage with no cost basis gives the existing 409-style validation error on
  that row.
- An export → import round-trip with no edits changes nothing.

### Verification
Run it locally against an org seeded like ELECTRO MISSAOUI. Download the template, fill counts,
upload, then check Inventory quantities, the movement list and the inventory valuation report.

### Docs
Add the new spec to the `db/CLAUDE.md` mass-data notes and the Inventory entry in `src/CLAUDE.md`.

## Part 2 — running inventory without unit costs

Decided 2026-09-27: for small businesses, unit cost is **optional**. Stock and sales have to work
for products that never had a cost. Today they don't:

| Path | No cost today |
|---|---|
| Stock upload / manual count, addition | OK — no GL entry, no cost basis |
| Manual count, shortage (`buildStockAdjustmentGLLines`) | **409** "no cost basis" |
| Shipping a delivery (`buildDeliveryCOGSGLLines`) | **409** — the delivery can't ship |
| Production order consumption (`db/production_order.go:410`) | **409** |
| Bill for a stock product (`resolvePurchaseAccount`) | posts to Inventory (needs the account) |

A second gap was found while checking this: **Cash Book sales (and invoices) never move stock.**
`db/cash_sale.go` stores `productId` on the invoice lines but posts no stock movement. Only an
outbound delivery reduces stock. At a counter shop the Inventory screen would never go down.

### Option 1 (**chosen**): org setting "Inventory valuation"
`organizations.inventoryValuation` = `perpetual` (today's behaviour, the default) |
`quantity_only`. In `quantity_only` mode:
- stock movements track **quantities only**: no COGS, no GRNI, no adjustment or production GL
  entries, and no cost ever required;
- bills for stock-enabled products post to the **expense** account (`resolveExpenseAccount`), as
  they did before Phase 7;
- unit cost stays an optional informational field. Average cost is still computed when costs exist;
- the inventory valuation report shows quantities, with values only where a cost is known.

This is the "periodic inventory" method many small businesses use: purchases are expensed, and the
accountant values closing stock at year end. The books stay internally consistent, because there is
never a half-posted COGS.
Switching mode: allowed only while the org has no posted inventory-type GL entries, or with an
explicit warning, since a perpetual → quantity-only switch would leave an Inventory balance on the
books that nothing clears.

### Option 2 (rejected): lenient perpetual
Keep perpetual, but when a line has no cost, skip its GL posting (with a warning) instead of
returning 409. It's less work, but COGS and Inventory end up silently partial, which
`docs/inventory-cogs-integration.md` explicitly rejected.

### Cash Book stock-out (**included**)
`POST /api/cash-sales` would post an `out` movement per stock-enabled line inside its existing
transaction, so a counter sale reduces stock. In perpetual mode it would also post COGS. Serialized
products would need a serial picker on the counter screen, or be excluded at first. Deleting or
cancelling the sale must reverse the movement.

## Delivery — 3 PRs (decided 2026-09-27)

1. **Stock upload** (Part 1). Works in today's perpetual mode. Uncosted additions post no GL.
2. **Quantity-only inventory valuation** (Part 2, Option 1): migration + org setting (Organizations
   drawer), gates in `buildDeliveryCOGSGLLines`, `buildStockAdjustmentGLLines`,
   `buildReceiptGRNILines`, production-order GL and `resolvePurchaseAccount`; mode-switch guard;
   valuation report copes with missing costs.
3. **Cash Book stock-out**: stock movements inside `POST /api/cash-sales`, reversed on
   cancel/delete; COGS only in perpetual mode; serialized lines excluded or picked.

Suggested order is 2 → 1 → 3 for ELECTRO MISSAOUI (they have no costs, so quantity-only mode should
be on before stock is loaded and sold), but 1 is independent and can go first.

## Open questions

A. (Moot in quantity-only mode; perpetual keeps Inventory Adjustment unless asked.) **Which account the offsetting entry posts to for opening stock.** Additions post
   Dr Inventory / Cr **Inventory Adjustment**, as every manual count does today. For a first stock
   load, the accountant may want the credit on an opening-balance/equity account instead.
   Option: an optional "offset account code" chosen at upload time. Keep the default unless asked.
B. ~~Additions with no cost~~ — superseded by Part 2. **Additions with no cost.** If neither the row nor the product has a unit cost, no GL entry is
   posted and the stock has no cost basis. Later shipments of that product then fail with the
   existing "no cost basis" 409. Proposal: allow the row but report it as a warning
   ("added without cost"), rather than rejecting it, so an onboarding count isn't blocked by
   missing prices.
C. ~~Timing~~ — decided: dated "now". Movements are dated "now". A count as of a past date (e.g. year opening) would need
   a date column, and `stockMovements` has no user-settable date today. Defer unless needed.
