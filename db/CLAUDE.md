# db/ — Claude notes

Loaded when working under `db/`. Project-wide invariants (cents, ids, dates, state sets) are in the root `CLAUDE.md`. These are the current rules; the reasoning behind older decisions is in git history.

## Basics
- SQLite through `jmoiron/sqlx`. **Every write and transaction goes through one connection, `d.DB`** (`SetMaxOpenConns(1)`). That serializes writes, which is what makes the `MAX+1` allocations race-free, and means **a `d.DB` read while a `*sqlx.Tx` is open deadlocks**: do every read before `Beginx()`, or read through `tx`.
- **Read pool** (F177): `d.ReadOnly()` is a view on a second pool of `readPoolSize` (3) connections, with the same pragmas plus `query_only(1)`. Its reads run alongside the writer and each other (WAL) and see the last commit. Only heavy, read-only GET handlers use it (see `api/CLAUDE.md`). A write through it fails, so never route a function that writes, even lazily. `Close` closes both pools, so a restore reopens it (`TestRestoreReopensTheReadPool`).
- Migrations: `db/migrations/*.up.sql`, applied on every startup by golang-migrate. Database file: `/data/sqlite.db` in Docker, `~/Library/Application Support/FaturaCloud/sqlite.db` (macOS) or `~/.config/FaturaCloud/sqlite.db` (Linux) in dev.
- Every `Create*` sets `req.ID` with `gonanoid.New()` when it's `""`.
- Money: every line-item `unitPrice`/`unitCost` insert goes through `roundCents` (`db/invoice_totals.go`, half away from zero), never an `int64(x)` cast.
- Dates: derive a day/month/year only through `orgLocation`/`organizationLocation` (`db/timezone.go`, `organizations.timezone`, IANA name checked by `validateTimezone`; unset = UTC). That covers printed dates (`formatOrgDate`), FEC/DATEV, the e-invoice (`formatMillis`), number date tokens, the Cash Book register, revenue months and `DashboardYearRange`. Day-based query parameters arrive as `calendarDayMs` and are decoded with `calendarDayStart`. Instant-range filters (fiscal periods, report ranges) compare instants and stay as they are. `time/tzdata` is embedded for the slim image.
- **Computed on read, never stored:** `products.stockQuantity`, `products.unitCost`, a serial's in-stock status, 3-way match status, the PO line freeze, amounts paid, every GL report. Don't add a stored copy that could go stale.
- **State changes re-check under the transaction.** A status/state update re-reads the current status through `tx` right after `Beginx()` (or `UPDATE … WHERE status = ?` plus a `RowsAffected` check) and aborts on a mismatch, so two concurrent requests can't both move stock or post/reverse twice.
- Server validation messages are English only, by product decision (see root `CLAUDE.md`).

## Organization columns
- **Three-state nullable fields:** in `UpdateOrganization`, omitted keeps, `""` clears, a value sets. Text columns do this with `COALESCE(?, column)`. The `default*AccountId` columns have foreign keys, so `UpdateOrganization` builds their SET clauses in Go (column names from a Go literal, never from the request). The frontend must send `""` explicitly to clear, because antd's `allowClear` sets `undefined`, which `JSON.stringify` drops.
- `PUT /api/organizations/{id}` is membership-level; organization settings are not admin-gated.
- Each non-empty `default*AccountId` must be the organization's own account (`requireSameOrg`, F193); a missing one is a 409.
- Logo: a BLOB, read and written only through `/api/organizations/{id}/logo` (`GetOrganizationLogo`/`SetOrganizationLogo`, which also decodes the legacy data-URI text format). `db.Organization` has no logo field, and reads use the explicit `organizationColumns` list so a multi-MB BLOB is never loaded; `HasLogo` (`length(logo) > 0`) tells the frontend whether to fetch it.
- `brandColor`: only values from the curated `brandColorPalette` (`normalizeBrandColor`, 409 otherwise); `""` = default.
- `isTest` (`0099`): a visible marker only; nothing on the server reads it. seed-demo sets it.
- `masterDataSummaries` (`0100`, default on): when off, every summary function returns a 409 so no summary work runs.
- Account defaults: `defaultInventoryAccountId`/`defaultGRNIAccountId`/`defaultCOGSAccountId`/`defaultInventoryAdjustmentAccountId`, `defaultStampDutyAccountId`, `defaultImportCostsPayableAccountId` and `defaultCashRegisterAccountId` (the physical till, distinct from `defaultCashAccountId`, which templates wire to Bank) are nullable; a posting that needs an unset one is a 409, never a 500. **A new `default*AccountId` column goes in both `accountReferencingOrganizationColumns` (`db/account.go`) and `ResetOrganizationData`'s NULL-clear list** (`TestGetAccountUsageCountCoversEveryReference`).
- `documentLayout` (`0089`): `tunisia` → `embeddedTunisiaTemplates`, anything else → `embeddedDefaultTemplates` (`normalizeDocumentLayout`/`embeddedTemplatesFor`). Never inferred from country after `0089`'s one-time backfill. `fiscalStampEnabled`/`withholdingTaxEnabled` are separate frontend gates, deliberately independent of the layout.
- `documentLanguage` (`0091`: `en`/`de`/`fr`): language of the amount-in-words line only; `documentLanguageFor` falls back by layout (French for Tunisian, else English), never by country.

## Membership
- `organization_users(organizationId, userId, role)`, unique per pair, cascades on both FKs. Roles: `admin`, `power_user`, `general`, `sales`, `purchasing`, `accounting`, `cashbook` (`0081`/`0086`). `users.isPlatformAdmin` is separate and grants no organization access.
- `GetOrganizationRole` is the primitive every org-scoped check uses. `AddOrganizationUser` upserts (a re-add changes the role).
- **Last-admin guard:** add/update/remove refuse (`ErrLastOrgAdmin`, 409) to leave an organization without an active admin; the check and write share one transaction (`isLastOrgAdminTx`). `GetOrganizationsWhereSoleAdmin` is checked before deleting or deactivating a user, since the membership cascade would silently lock the organization out with no recovery.

## Reset and usage tripwires
`ResetOrganizationData` (`db/reset.go`) clears `masterDataTables` and `transactionalDataTables` in FK-safe order (no-cascade references such as `payments` before `journal_entries`, `imports` after `purchase_orders`). `organization_users` and `audit_events` are in `deliberatelyUnresetTables`. Schema-reading tests fail when a table or column is missing from these lists, so a new table or reference must be added to:
- `productReferencingTables` (`TestProductUsageCountCoversEveryReference`);
- `vendorReferencingTables` (`TestVendorDocumentCountCoversEveryReference`);
- `taxRateReferencingTables` (`TestTaxRateUsageCountCoversEveryReference`, matches `taxRate` and `taxRateId`);
- `clientReferencingTables`;
- the reset lists (`TestResetOrganizationDataCoversEveryOrganizationScopedTable`);
- `masterDataTables`/`organizationUsageCountTables` for new master data.

## Products
- `type`: `product` | `service` (default). `category` (`0065`): NULL | `finished` | `component`, forced NULL unless `type = product`. NULL is eligible everywhere; sales pickers hide components and purchasing pickers hide finished goods (filter the picker list, never the products atom: saved lines resolve names from the full list).
- `sku` is unique per organization (`UNIQUE(organizationId, sku)`); required-ness is enforced in the API and form, not by `NOT NULL`.
- `unitOfMeasureId` (`0076`, `SET NULL`): `products.unit` is **derived** from it at write time (`resolveProductUnit`). A nil id leaves `unit` as sent, unless the product had one, in which case clearing the link clears the copy. `UpdateUnitOfMeasure` re-derives `unit` for linked products on a rename. `UpdateProduct` is a full replace, so the form round-trips `unit` in a hidden field.
- `familyId` (`0095`, `SET NULL`): three-state on update (omitted keeps, `""` clears; `nullableFKClassification`), ownership checked (`checkProductFamilyOwnership`). Family names are unique per organization ignoring case (`0096` NOCASE index plus `checkProductFamilyNameFree`'s Unicode fold). Separate from `category` on purpose.
- **A product with history can't be deleted** (F167): `DeleteProduct` counts `productReferencingTables` in its transaction and 409s. It is archived instead: `isArchived` (`0105`), `*bool` on update, where nil keeps (the drawer save and the Excel import must never unarchive). `GetProducts`' `Archived` is `exclude`/`only`/all. Chip counts and `getLowStock` leave archived products out; the stock value includes them. `GetProductUsage` serves `/usage-count`.
- `serialized`: only with `stockEnabled`. It can't be toggled either way while `stockQuantity` ≠ 0 (tolerance `1e-9`). `product_serial_numbers` rows are never deleted; serials are unique per product. A serialized product's movements are one row per unit (`±1`, with `serialNumberId`). Manual movements for it are `in`/`out` with `serialNumbers` only. Receiving or shipping a serialized line requires a `serialNumbers` map whose count matches each line exactly.

## Stock and cost
- `stockMovements.quantity` is a signed delta and `products.stockQuantity` = `SUM(quantity)`, recomputed in the same transaction (`insertStockMovementRowTx`/`recomputeStockQuantityTx`). Never update it directly.
- `stockMovements.sourceDocumentId` (the document row id, never the free-text `reference`) is how a cancellation finds exactly the movements it must reverse. A manual movement's batch id is its own `req.ID`, which `POST /api/stock-movements` always leaves to the server (F196).
- `products.unitCost` is a weighted average replayed from movements with `math/big` (`recomputeAverageCostTx`, `db/product_cost.go`, ordered `createdAt, rowid`). Costed inflows move it; uncosted inflows and all outflows don't. Cancelling a receipt doesn't restore the old average. A product with no costed inflow keeps its typed cost.
- **Inventory valuation** (`0093`, `db/inventory_valuation.go`): `perpetual` (default; `normalizeInventoryValuation`) or `quantity_only`. Every inventory posting path checks `isQuantityOnlyInventory` before `resolveMovementCost`. Under `quantity_only`, GRNI, COGS, adjustment and production entries post nothing, components need no cost (the output gets a nil `unitCost`, never 0), and `resolvePurchaseAccount` expenses stock bill lines. `checkInventoryValuationSwitch` compares normalized values:
  - either direction is refused once anything is posted to Inventory;
  - perpetual → quantity-only is otherwise allowed;
  - quantity-only → perpetual is refused once any stock movement exists.

  Known edge: under `quantity_only`, a manual payment debiting `defaultImportCostsPayableAccountId` drives it negative (nothing credits it); post it to an expense.
- Manual movement (`CreateStockMovement`) posts a GL trace (`buildStockAdjustmentGLLines`: an uncosted inflow adds nothing, an uncosted outflow 409s). A GL-posted movement can't be deleted; record an offsetting adjustment.

## Invoices
- **Numbers** (F169, `db/invoice_number.go`; tokens via `generateDocumentNumber`, the Go port of `src/utils/invoice.ts`, so keep the two in step): a blank number gets the next free one inside `CreateInvoice`'s transaction (`nextInvoiceNumberTx`, also used by `CreateCashSale`). An explicit number is an override, refused (`refuseTakenInvoiceNumber`, 409) if taken. `UpdateInvoice` checks only a changed number. The partial unique index `invoices_org_number_unique` (`0106`, skipping blank numbers and `origin = 'opening'`) backs this; `isDuplicateInvoiceNumber` maps its error to the same 409. Invoice numbering stays on `organizations.invoice_number_format`/`_counter`, never `document_number_settings` (see Document numbering below).
- `state`: `draft` | `sent` | `paid` | `cancelled` (`invoiceStates`), no transition matrix (a bounced payment can go paid → sent); changed only via `PATCH …/state`. Frontend twin: `src/types/invoice.ts`.
- **Totals** are re-checked on every create/update by `validateInvoiceTotals` (`math/big`, per-tax-group rounding), 409 on mismatch. A partial update is filled from stored values so neither line items nor totals can bypass it; a header-only edit skips it. Formula: `subTotal − discountAmount + taxTotal + fiscalStampAmount`.
- **Discount** (`0088`, `discountAmount`): flat, pre-tax, spread over tax groups by subtotal share. The identical allocation lives in `validateInvoiceTotals`, `buildInvoiceGLLines`, `buildTaxBreakdownRows`, `invoiceNetFactor`, `allocateDiscountCents` (e-invoice) and the frontend's `allocateDiscount`; change all of them together. A discount above the subtotal is a 409.
- **Tunisia** (`0062`/`0063`): `fiscalStampAmount` is added to `total` and posts its own line via `resolveStampDutyAccount` (409 without an account). `withholdingTaxRate`/`withholdingTaxAmount` are informational only, never checked or posted. TND millimes can't be stored exactly (2-decimal cents). MF reuses `vatin`.
- When payments cover a sent invoice or approved bill it becomes `paid`, and a void that reopens a balance moves it back, but only if payments had covered it (`db/payment_state.go`, owner decision 2026-10). Other transitions stay manual.

## Sales documents
- `orders.status`: `draft` → `confirmed` → `shipped` → `delivered`, plus `cancelled`, which can return to confirmed/shipped/delivered (no side effects). `orderStatusTransitions` (Go) and `src/types/order.ts` must agree; `PATCH …/status` only.
- `outbound_deliveries.status`: `draft` | `shipped` | `delivered` | `cancelled` (terminal). Shipping checks stock on every stock-enabled line, posts COGS before touching stock (`buildDeliveryCOGSGLLines`; any line with no cost basis 409s the whole transition), and writes `out` movements. Cancelling a shipped delivery reverses both. A shipped or delivered delivery can't be deleted.
- **Shipped deliveries freeze their lines** (`deliveryLineItemsUnchangedTx`, `db/delivery_freeze.go`). It compares the payload with what's stored (the UI always sends the full array), positionally. Header edits still save.
- `outbound_delivery_line_items`: no price columns. `productId` is set directly or derived from `orderLineItemId` (`replaceDeliveryLineItemsTx`); `getShippableStockLines` decides stock lines by it.
- `outbound_deliveries.clientId` (`0064`, `SET NULL`) is the standalone delivery's client. `OwnClientID` is the raw column; `ClientID` (`effectiveClientId`) is the order's client, else its own.
- `GetOrderDeliveredQuantities` drives the "outstanding only" delivery prefill.

## Purchasing
- `purchase_orders.status`: `draft` | `confirmed` | `received` (terminal) | `cancelled` (can return to confirmed/received). Never auto-advanced from receipts. `CHECK` plus `purchaseOrderStatusTransitions`.
- **PO lines freeze once a receipt against the order is `received`** (`db/purchase_order_freeze.go`, computed, no column). Both link shapes count (`inbound_deliveries.purchaseOrderId` and a line's `purchaseOrderLineItemId`). It rejects a payload that differs from what's stored, not one that merely carries lines. A partially received order can't adjust outstanding lines: cancel the receipt, edit, re-receive.
- **Line ids are stable for orders and POs** (`db/line_item_reconcile.go`), because other tables reference them (`orderLineItemId`, `purchaseOrderLineItemId`, all `SET NULL`). The atoms echo `id`. `replaceOrderLineItemsTx`/`replacePurchaseOrderLineItemsTx` update in place by id, delete missing ones, insert id-less ones, and ignore an id that isn't the document's own. Match by id, never position. The other four line tables have no inbound FK and keep delete-and-reinsert.
- `purchase_orders.vendorId` and `purchase_orders.importId` have **no `ON DELETE`** on purpose; `DeleteVendor`/`DeleteImport` guard instead (`ErrVendorInUse`, `ErrImportInUse`).
- `inbound_deliveries.status`: `draft` | `received` | `cancelled` (terminal). Receiving adds `in` movements carrying `unitCost` (no stock check) and posts GRNI (`buildReceiptGRNILines`, uncosted lines skipped). Cancelling is refused if any linked PO line was billed (`grniClearedQtyForPOLine` > 0), then refused if the stock has already left, then reverses both. A received receipt can't be deleted.
- `inbound_delivery_line_items.unitCost` feeds the average cost; a PO-linked line with neither product nor cost takes both from the order line.
- `incoming_invoices.state`: `draft` | `approved` | `paid` | `cancelled`, no matrix; `PATCH /state` only, which is where matching is enforced. Lines reuse `CreateInvoiceLineItemRequest` and `validateInvoiceTotals`.
- **3-way matching** (`db/incoming_invoice_match.go`, computed on read): `matched` | `unlinked` | `quantity_variance` | `over_received` | `price_variance`. `PreviouslyInvoiced` counts other approved/paid bills only (the same definition as GRNI clearing). Tolerances come from `organizations.match_price_tolerance_percent`/`match_quantity_tolerance_percent`, compared as `math/big` values. Variances don't block a save; they block → `approved` (and → `paid` from non-approved) unless `matchOverride = 1` with a required `matchOverrideReason`. `unlinked` never blocks. The GL clears GRNI independently of the report.

## Tax rates
- Line items store the rate id, never the percentage. So `UpdateTaxRate` refuses (409, F168) a real percentage change while any of `taxRateDocumentLineTables` uses the rate (compared with the stored value; product defaults and manual journal lines don't count). The way forward is a new rate made the default.
- Two line tables cascade on rate delete, so `DeleteTaxRate` refuses an in-use rate (`GetTaxRateUsageCount`, `ErrTaxRateInUse`).

## General ledger
- `accounts.isGroup = 1` accounts can't be posted to (enforced by `allocateAndFinalizeEntryTx`). `UpdateAccount` refuses to change `type`/`isGroup` on a used account, to make an account with children a leaf, or a self/missing parent.
- `journal_lines`: `CHECK ((debit = 0) <> (credit = 0))`, so a group that nets to zero (0% tax, a free line) emits no row. `debit`/`credit` are always functional-currency cents; `currency`/`foreignAmount`/`exchangeRate` are a shadow. `clientId`/`vendorId` are typed FKs, at most one set.
- **`allocateAndFinalizeEntryTx` is the single posting choke point** (manual, auto, payments, closing). It checks balance, refuses a closed fiscal year, and allocates `entryNumber` as `MAX+1` over numbered entries. Posted entries are never edited or deleted, only reversed (`reverseEntryTx`, `WHERE status = 'posted'` plus `RowsAffected`). The partial unique index (`0059`, per organization since `0110`) allows one live posted entry per `(organizationId, sourceDocumentType, sourceDocumentId)`. A second entry for the same document needs its own source type (e.g. `invoice_cogs`, `invoice_opening`).
- `findPostedEntryForSourceDocumentTx` is re-checked inside the transaction by every state-change path. It takes the document's organization and finds only that organization's entries (F191); tests use the unscoped `postedEntryForSourceDocument` helper. `DeleteJournalEntry` deletes `AND status = 'draft'` only.
- **A manual entry never names a source document** (F191): `CreateJournalEntryRequest` has no source fields, because a live posted entry naming a document makes its automatic posting skip. Only automatic postings set them.
- `db/gl_posting.go`:
  - `postAutoEntryTx` creates and posts in one step.
  - Invoice and bill entries: one line per account and per tax rate, with AR/AP as the remainder so the entry balances with no plug. `UpdateInvoiceState`/`UpdateIncomingInvoiceState` post or reverse by whether the new state needs GL presence.
  - Bill lines: a stock bill line capitalizes to Inventory (`resolvePurchaseAccount`) and nets against the GRNI its receipt accrued (`grniAccrualForPOLine`/`resolveGRNIClearing`), with any variance going to Inventory.
  - Costs: `resolveMovementCost` (the average, or a serial's own receipt cost) plus `wrapCostBasisError`, which names the failing operation.
  - Landed cost (`applyLandedCost`, runs before `Beginx()`): an import's freight and customs are spread over its receipts' lines. Inventory gets the full value, GRNI the vendor value only, and Import Costs Payable the markup, all in **one** entry. A small residual is expected.
- `SeedDefaultPaymentTermsForAllOrganizations`/`SeedDefaultUnitsOfMeasureForAllOrganizations` seed an organization only when it has none, each in one transaction. The units backfill links a product only on an exact (case-insensitive) name match, never a guess.
- `db/journal.go`: system journals `VK`/`EK`/`BK`/`KA`/`OD` (`isSystem = 1`, undeletable).
- `db/fiscal_period.go`: `resolveFiscalPeriodForDate` requires an open year; its error tells "no year" apart from "closed year". Open years can't overlap; periods sit inside their year without overlapping.
- `db/fiscal_year_closing.go`: `CloseFiscalYear` (admin or accounting, **irreversible**, no reopen) posts the closing entry into `retainedEarningsAccountId` **before** flipping status, snapshots activity through `tx`, and closes `WHERE status = 'open'`. FX revaluation is not implemented (there are no period-end rates). `fiscal_years.lockDate` is unused.
- `db/payment.go`: AR/AP settle at the document's frozen rate, with a realized FX gain/loss line for the difference. The overpay guard is re-checked in the transaction (`getDocumentAmountPaidTx`). Lettrage is not implemented (`reconciliation_groups` is unused; FEC `EcritureLet`/`DateLet` stay blank). The `payments` CHECK ties direction to partner.
- `db/gl_reports.go`: trial balance, P&L, balance sheet (`CurrentEarnings` folded into equity) and AR/AP aging are computed from `journal_lines`. Aging reads `selectOutstandingInvoices`/`selectOutstandingBills`, and `agingBucketFor` counts calendar days in the organization's zone (a document is due on its due date, late the day after; F162). `ForeignTotal` is display-only. `GetInventoryValuation` compares the Inventory GL balance with Σ stock × cost: a visibility report, not a guarantee.
- `db/account.go`: `seedDefaultChartOfAccounts`/`seedDefaultJournals` run at creation and as a startup backfill. `resolveChartTemplate` picks by the free-text `country`: SKR04 (DE), PCG (FR), else generic (cited data). Accounts carry a `defaultRole` rather than hard-coded codes. The inventory accounts (`phase7BaseAdditions`) and Import Costs Payable (`importCostAdditions`) are generic codes in every template (no country has an agreed number for them), backfilled by `SeedInventoryAccountingDefaultsForAllOrganizations` (gated on any of the four being NULL) and `SeedImportCostAccountForAllOrganizations`.
- GL exports (org admin or accounting):
  - FEC (`db/export_fec.go`): sanitizes tabs/newlines; `ValidDate` = `postedAt`.
  - DATEV (`db/export_datev.go`): EXTF Buchungsstapel v13, cp1252, CRLF, and still unvalidated against a real DATEV import. Needs `datev_consultant_number`, `datev_client_number` and a `datevAccountNumber` (one length, 4–8 digits) on every posted account; all missing fields go into one `ValidationError`. The anchor line becomes the Gegenkonto. A multi-line manual entry needs `datevClearingAccountId` or is a validation problem. SKR04 codes double as DATEV numbers. Belegdatum is DDMM; Generalumkehr is not set.
- `db/sales_reports.go`: document-based analytics, deliberately not GL-reconciled. The Dashboard calls the same functions. `GetTaxSummary` keeps 0% groups and an unrated `""` bucket, applies the discount (`invoiceNetFactor`) and rounds per tax group per document.

## Cash Book
- `db/cash_sale.go`, `CreateCashSale`: one transaction resolves or creates the client and writes the invoice (number via `nextInvoiceNumberTx`, falling back to `DefaultInvoiceNumberFormat`), the GL entry and any upfront payment.
  - Reads happen before `Beginx()`.
  - It is refused in a foreign currency or with no lines.
  - The register is `defaultCashRegisterAccountId` → `defaultCashAccountId` → 409.
  - `dueDate` = `date`.
  - The sale is `paid` iff `amountReceived == total`.
- **Cash Book stock** (`0094`, `db/invoice_stock.go`): a `movesStock` invoice's stock follows its state like the GL does (`needsInvoiceStockPresence`). Movements carry `sourceDocumentId` = the invoice. COGS goes under `invoice_cogs`, through `buildCOGSGLLines`, in perpetual only. With no cost basis, `planInvoiceStockOut`'s 409 says to give the product a cost or switch to Quantities only. Owner decisions:
  - no availability check (stock may go negative);
  - serialized products and non-positive quantities are refused.

  While the stock-out stands, `DeleteInvoice` and financial edits are refused, and `DeleteStockMovement` refuses its rows. Sales from before `0094` never move stock. `postedInvoiceStockTx` counts only the invoice's own organization's movements: a manual movement's client-chosen id is its `sourceDocumentId`, so another organization's could otherwise name the sale (F196).
- `db/cash_sale_payment.go`, `CreateCashSalePayment` (`0090`): one payment per invoice line (`payment_applications.invoiceLineItemId`), capped at that line's outstanding and re-checked in the transaction. When it clears the invoice, the invoice turns `paid`. Any sent or paid invoice of the organization qualifies (F150, by decision); foreign currency is refused. Line outstanding comes from `allocateInvoiceLines` (`db/dashboard.go`): line amount = total weighted by net value (a free line weighs nothing), and invoice-level payments are spread by amount via `allocateCapped`, so paying one line never moves another (`TestAllocateInvoiceLinesSettlingALineLeavesTheOthersAlone`). AR still settles per invoice.
- `db/opening_loan.go` (`0097`): migrated paper loans are invoices with `origin = 'opening'`, carrying the original sale date, no tax and no stock.
  - An open loan posts Dr AR / Cr Retained Earnings for its outstanding, dated the cutover (`invoice_opening`). A settled one posts nothing.
  - Paid-to-date is one `origin = 'opening'` payment with no GL.
  - **Frozen:** update, delete, e-invoice, export and voiding the paid-to-date are refused; only sent ↔ paid via `updateOpeningLoanState`.
  - Excluded from analytics, Tax Summary and Dashboard revenue (`openingLoanExclusion`).
  - Collected through `findInvoiceReceivableEntry`.
  - The reference is the number and doesn't use the counter.
- `db/loan_import.go` (`0098`): the loan register import.
  - Cells are read raw. Dates are calendar days at local midnight in the organization's zone; the cutover arrives via `calendarDayStart`.
  - Customers match by CIN (all-digit ignores leading zeros), then name **and** phone, then a unique name; a phone alone never matches.
  - `DryRunLoanImport` reports without writing; `ImportLoanRegister` writes the batch in one transaction only with no errors.
  - `UndoLoanImportBatch` is refused once a real payment touched the batch. It reverses rather than deletes entries, and deletes created customers only when unreferenced.
  - The 20,000-row cap (`checkImportRowCount`) applies here too.
- `db/cash_movement.go` (`0080`): money leaving the register (bank deposit or expense), one Dr counter / Cr register entry. `counterAccountType` is checked against the account. There is no balance floor; an over-withdrawal shows, it isn't refused.
- Reports:
  - `GetDailyCashMovements` buckets per line in Go (`strftime` can't follow DST). It builds one row per day, so a range over 10 years is a 409 (`maxDailyCashMovementSpanMs`, F190).
  - `GetCashMovementDetails` takes `Kind` from the payment's position in the invoice's history, not its current state. Voided payments are excluded.
  - `db/loan_register.go` is the Go twin of `src/components/cash-book/loan-register-model.ts`. `LoanStaleAfterDays` (60) must equal the screen's `STALE_AFTER_DAYS`.
  - `GetLoanStatus` lists loans. A loan cleared by one invoice-level payment is a documented gap (`LoanStatusRow`).
- The invoice number is the loan number. `Payment.InvoiceNumbers`/`Products`/`WholeInvoice` are filled by second queries in `GetPayments`. `GetPaymentInvoiceLines` (Sales and Cash Book only, F159) shares `allocateInvoiceLines`.

## Master-data summaries and Dashboard
- What a client owes is read only through `selectOutstandingInvoices`, what a vendor is owed only through `selectOutstandingBills` (`db/dashboard.go`). Never add a second "totals minus payments" calculation.
- `GetClientSummaries`/`GetVendorSummaries` totals equal the aging totals by construction. `GetProductSummaries` uses `getStockValuation` and `LowStockThreshold`. `GetProductSummary` resolves movements to documents: `sourceDocumentId` first, else a delivery reference matched in both tables when unique.
- `db/bom_summary.go`: parts cost uses `big.Rat` and is null when any component is uncosted (always null under quantity-only). Buildable must agree with production orders: the same blocked reasons in the same order, and `maxBuildable` via `roundBOMQuantity`. `TestBOMBuildableMatchesProductionOrders` is the arbiter.
- `db/dashboard_panels.go`: `DashboardOptions` come from the caller's role. Apart from these options the payload is shared by every member, and per-role hiding happens on screen (F147, by decision). `CashRegister` is set only for Cash Book roles, and `getDashboardCashRegister` returns nil rather than an error. `LoanFollowUp` (Sales and Accounting) reads `loanRegisterCustomers`. `getLowStock` uses one threshold (there are no reorder levels).

## Activity history
- `audit_events` (`0102`, `db/audit_event.go`), written by `api/audit.go`, has one row per successful change. `organizationId` has no FK, so the history outlives the organization. It is kept for two years (`AuditRetention`, `PruneAuditEvents`). Labels come from the `auditDocuments` map (table and column names only from there).
- `changes` (`0103`, `db/audit_changes.go`): `AuditSnapshot` before and after, then `AuditChangesJSON`. Skipped: ids/timestamps, blobs and `auditSecretColumn` matches. IBAN/RIB are masked; references are stored as labels; text is cut at 300 characters. **A new column on an audited table must be classified in `auditColumnReview`** (`TestAuditColumnsAreReviewed`). `auditExtras` must not shadow columns (`TestAuditExtrasDontShadowColumns`). Keep the contract equal to STBvirement's.

## Document numbering
`db/document_number.go` (`0082`) covers `order`, `purchase_order`, `delivery`, `inbound_delivery` and `production_order`, with `documentNumberDefaults`. No row means the default. `counter` is a pointer, so a format-only save keeps it. `validateDocumentNumberFormat` accepts `{number}`, `{number:N}` (1–2 digits), date tokens and `{clientCode}`. `GenerateNextDocumentNumberTx` advances inside the document's transaction (numbers never reissued); `PreviewNextDocumentNumber` doesn't consume.

**Numbers are unique per organization and type** (`0108`, the F169 rule invoices follow; `db/document_number_unique.go`):
- generation and the preview skip a number another document of the type has (`firstFreeDocumentNumber`);
- `AssignDocumentNumberTx` refuses a taken explicit number (409);
- updates refuse renaming to a taken one (`checkDocumentNumberChangeTx`);
- `isDuplicateDocumentNumber` maps the index error to the same 409, matched on `table.column` because both delivery tables use `deliveryNumber`.

The forms send a blank number and show the preview as a placeholder.

## Templates and exports
- `db/document_template.go` (`0068`): one override per organization and type. Otherwise `resolveTemplateBytes` uses the embedded template for `org.DocumentLayout`. `ExportDocumentTemplateBytes` names downloads `<type>_<layout>.xlsx`. The upload is validated by opening it. **A new document type needs its export, a Settings card in `src/routes/settings/document-templates.tsx` and both embedded templates.** Orientation lives in `document_template_settings` (`db/document_template_settings.go`), is applied at fill time and wins over the template.
- `db/xlsx_export.go`, `fillTemplate(templateBytes, scalars, blocks, logo, orientation)`: the type-agnostic engine.
  - Repeat blocks (`{{#lineItems}}`, optional `{{#taxLines}}`) are re-located after each expansion, and rows below are found by content, never a cached index.
  - `{{organization.logo}}` is an image anchor.
  - An unresolved placeholder is blanked and reported, never an error.
  - Money goes through `formatMoneyCents`, dates through `formatOrgDate`, and line totals through `lineTotalCents` (`math/big`).
  - Money is written as formatted strings (so Excel `SUM` doesn't work: a known gap).
  - Every sheet but the first is deleted (the "Available fields" sheet).
  - PDF converts these exact bytes, so a layout fix applies to both formats.
  - A client block's street line is `{{client.addressLine}}` (`clientAddressLine`): street and house number, else the Cash Book's free-text `address`, so form clients and counter customers both print one. Every built-in client layout uses it (`TestClientLayoutsPrintTheAddressLine`).
- `computeExportTotals` (`db/xlsx_export_totals.go`) computes totals for types without validated stored totals, with the same per-group rounding. Invoices and incoming invoices read stored totals. Orders have no tax column (tax total is always 0). A missing client or vendor exports blank placeholders. Delivery notes have no prices; goods receipts show line costs but no totals. The export routes are registered outside `withDB` (see `api/CLAUDE.md`), and `Fetch*ExportData` is the only database access.
- `db/templates/gen/`: two builders per type, `build<Type>DefaultTemplate` (`<type>_default.go`) and `build<Type>TunisiaTemplate` (`<type>.go`, a spec over `buildTunisiaLayout` in `tunisia_layout.go`). The default builders use `newDarkHeaderStyle` so the Tunisia workbooks stay byte-identical. Every template sets `applyFitToPageWidth`, which LibreOffice needs. Regenerate with `go run ./db/templates/gen && mv db/templates/gen/*.xlsx db/templates/`. Each template has a `TestEmbedded{Default,Tunisia}<Type>TemplatePlaceholdersAllResolve` guard. A new type adds both builders, `main()` calls and both `db/templates_embed.go` maps.
- `db/amount_in_words.go`/`_lang.go`: en/de/fr, gated by `amountInWordsEnabled`, blank when off. `frenchNumber` handles the irregular forms (`invariableBeforeMille`). TND prints millimes as 10 × cents.
- `db/pdf_convert.go`: `ConvertXLSXToPDF` runs `soffice --headless` with its own temp profile directory and a 60 s timeout. `ErrPDFConversionUnavailable` → 503. Conversions share 2 slots (`pdfConvertSlots`, F192): a request waits up to 20 s, then gets `ErrPDFConversionBusy` → 503. Handlers answer both through `api`'s `writePDFConversionError`. CI has no LibreOffice, so `TestConvertXLSXToPDFSmoke` skips and the runtime-image workflow does the real check.
- `db/xlsx_open.go`: `openXLSX` is the only way to open a workbook (F173). Limits: 64 MB unzipped, 16 MB per sheet in memory; 409 otherwise. The caller closes it. `checkImportRowCount` caps an import at `maxImportRows` (20,000).
- `db/mass_data.go` and `mass_data_<table>.go`: Excel round trips for master data.
  - Every row goes through the real `Create*`/`Update*` (column 0 is the id: blank creates). References use Name (tax rate, unit) or Code (accounts), matched exactly and case-sensitive on purpose.
  - Chart of accounts parents may be forward references within the file.
  - A failed row is reported, never aborting the file.
  - Columns are positional, so new ones go at the end. `massDataColumnAware` keeps values for columns the sheet lacks (`Update*` is a full replace).
  - There is no bulk delete.
- `db/mass_data_stock.go`: a physical count upload. It posts `counted − current` as a count movement through `CreateStockMovement`, reading stock fresh per row. A blank count skips the row; serialized products are a row error. The reference is `Stock upload <date>` in the organization's zone. Re-uploading moves nothing.

## Other files
- `db/import.go` (F114): an import groups POs of one shipment. Its currency and rate only prefill new POs. Freight and customs are functional-currency cents. `GetImportSummary` gives `landedCostRate` from non-cancelled POs (0 when there is no value). Import Costs Payable has no settlement flow: pay it with a manual journal entry.
- `db/production_order.go`:
  - `CreateProductionOrder` snapshots the BOM into `production_order_component_lines` (with name, `componentSku`, `componentUnit`). It 409s on an empty BOM, a serialized component or a finished product that isn't stock-enabled.
  - `importId` has no `ON DELETE`; `DeleteImport` also checks `GetImportProductionOrderCount`.
  - Completing checks stock and costs (`resolveMovementCost`) and values output at total ÷ quantity rounded once, with the residual to Inventory Adjustment (usually no entry). Accounts are validated even when nothing posts.
  - Produced serials must fall in the import's range (`validateSerialAgainstImportRange`).
  - `completed → cancelled` reverses after a "not shipped yet" check. `cancelled` is terminal.
  - There is no `PUT`, no export and no labor cost.
- `db/einvoice.go`: EN 16931 UBL 2.1. The profile follows the **buyer's** country: `DE` = XRechnung 3.0, else generic core. Peppol BIS is deliberately not claimed (it needs `EndpointID`s that have no column). Missing mandatory fields are all listed in one 409. The discount becomes one `AllowanceCharge` per VAT category (`allocateDiscountCents`), and category tax uses the exact net, so `PayableAmount` = total − stamp (F142, F152; `TestGenerateEInvoiceDiscountSweep`). There is no conformance validator here; validate externally. TEIF/TTN and ZUGFeRD are out of scope.
- `journal_entries(fiscalYearId)` is indexed (`0060`).
