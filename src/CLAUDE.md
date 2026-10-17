# src/ — Claude notes

Loaded when working under `src/`. Project-wide invariants are in the root `CLAUDE.md`; the server rules behind these pages are in `db/CLAUDE.md`. These are the current rules; history is in git.

## State
- Jotai atoms, one file per domain in `src/atoms/`, importing from `src/api`. Storage atoms hold preferences (`localeAtom`, `siderAtom`, `cashBookLayoutAtom`); entity atoms come as `xAtom`/`setXAtom` pairs.
- **Never use module-level atoms for local state inside a Modal or Drawer**: the mask gets orphaned and freezes the UI. Use `useState`. A list page's filter atom (e.g. `searchAtom`) is fine.
- **Writable atoms rethrow after toasting a failed save**, so the drawer or page's `handleSubmit` keeps the form open and its dirty flag set (`incomingInvoiceAtom` returns a boolean instead). A new writable atom does the same.
- `organizationAtom` returns `null` without fetching while `currentUserAtom` is null; it would otherwise cache a logged-out 401. Component tests that pick an organization must also set `currentUserAtom` (see `src/routes/invoices/details.test.tsx`). `myOrgRoleAtom` is keyed on the user too.
- `logoutAtom` (`src/atoms/session.ts`) is the only way to sign out. It clears the user, the organizations and the remembered organization, but only if the same user is still current. Callers navigate to `/login` first.
- `isPlatformAdminAtom` gates only Backup, Users and Countries. Org-scoped admin UI uses `isOrgAdminAtom`, and GL Export uses `isOrgAdminOrAccountingAtom`.

## Data loading
`oxlint`'s `set-state-in-effect` rule is enforced at zero warnings, so a page never calls `setState` synchronously in a `useEffect`:
- List pages load with `useLoadOnPath(path, load, deps)` (`src/hooks/useLoadOnPath.ts`), which returns `loading`.
- Reports, per-record counts and lookups use `useFetch(key, fetcher, empty)` (`src/hooks/useFetch.ts`). `key` is a list of primitives; `null` loads nothing and forgets the previous result. It returns `{data, loading, failed, reload}` and drops superseded responses. `empty` must be a stable constant.
- State tied to the record on screen resets **during render** when its key changes (`if (shownFor !== x) { setShownFor(x); reset(); }`). `form.setFieldsValue` stays in effects.
- An effect sets state only inside promise callbacks, never in an `async` function whose `catch` could run synchronously.

## Forms and inputs
- **Every number field is `NumberInput`** (`src/components/number-input.tsx`), never `InputNumber` or `Input` with `parseFloat`. It takes `decimalSeparator` from `DecimalSeparatorContext` (`src/app.tsx`, via `inputDecimalSeparator`), so "12,5" isn't stored as 125. app.tsx reads the organization through `unwrap(..., prev => prev)` so the separator doesn't flip during a refresh. Use `suffix`/`prefix` for units, never a `formatter`/`parser` pair, which bypasses the separator. Money inputs in a 2-decimal currency take `precision={2}`.
- **Clearing a field:** antd's `allowClear` sets `undefined`, which `JSON.stringify` drops. Send `""` explicitly when the server's three-state update must clear (organization `default*AccountId`, product `familyId`, time zone, document language). `documentLayout` has no clear: `normalizeDocumentLayout` always shows `default` or `tunisia`.
- `0 && <jsx>` renders "0"; coerce feature flags with `!!` (as on the invoice page's stamp/withholding gates).
- Wire booleans stored as 0/1 (`isDefault`, `fiscalStampEnabled`, …) are converted to `Checkbox` booleans at the atom or submit boundary. `isTest` is a plain boolean.
- A collapsed panel whose fields must load and save is `forceRender`, and a validation failure opens the panel holding the field (clients form: `PANEL_OF_FIELD` + `handleFinishFailed`).

## Navigation and roles
- Sidebar: Dashboard and Cash Book on top, then groups. Sales (Invoices, Outbound Deliveries, Orders), Purchasing (Imports, Purchase Orders, Goods Receipts, Incoming Invoices), Inventory (Inventory, Production Orders), Master Data (Clients, Vendors, Products, Bill of Materials, Organizations), Accounting (GL screens and reports, plus Daily Cash Movements), Reporting (document-derived analytics, deliberately separate from Accounting). There is no Payments page; payments are recorded in each detail page's `PaymentPanel`.
- Settings is the header gear (`Dropdown`), not a sidebar group.
- `src/layouts/base.tsx` opens the active group (`defaultOpenKeys`) only while the sider is expanded, since collapsed groups render as stuck popups; the `Menu` is keyed on the collapsed state.
- **Role views:** `src/layouts/role-menu.ts`'s `ROLE_MENU` filters the sidebar and redirects typed URLs. `roleHomePath` is each role's landing page. `admin`/`power_user` see everything; `general` loses Accounting, Imports, Bill of Materials and Production Orders. `cashbook` sees only Cash Book and Clients (`isCashbookAtom`; also bounced in `src/routes/organizations/new.tsx`). The server mirrors this in `api/sections.go`; **change both together** (`src/layouts/role-menu.test.ts`). `roleCanSeeMenuItem` gates fetching a hidden section's data (the BOM card in the product form, the Import select on POs). `dashboardWidgetsForRole` hides Dashboard widgets, and a link appears only when `isRouteAllowedForRole` allows its target.
- **Configuration screens are view-only for roles that can't write** (F170): `CONFIG_WRITE_ROLES`/`useCanWriteConfig` mirror the server. List pages drop New and Upload (`MassDataExcelActions`' `canUpload`). Drawers render `disabled` with Close and no Save/Delete/Archive; an explicit `disabled` prop on a field overrides the form's, so combine it with `!canWrite`. A new configuration screen needs the same treatment.

## Layout rules (`src/styles/base.scss`, `src/layouts/base.tsx`)
Don't re-solve these per page:
- `.ant-table-content { overflow-x: auto }`: tables scroll inside their own box. A table needs `scroll={{ x }}` only for `fixed` columns.
- Column `width`/`ellipsis` is ignored under `scroll.x: "max-content"`, so truncate inside the cell with a tooltip.
- `.card-head-wrap`: a Card's `extra` filters wrap under the title on a phone.
- `.responsive-footer`: the sticky document footer wraps below 768px. Footers render via `createPortal` into `#footer`, which is itself sticky.
- The inner `Layout` and `Content` carry `minWidth: 0`, so a wide table can't push the page past the viewport.
- Phone header: `wrap={false}` and a narrower org select.
- **Never hide an antd column's cells with CSS** (the measure row keeps its width). Render the column conditionally, e.g. from 1440px via `useWideScreen`.
- Pin the column that matters (Owes, Outstanding/Collect, Stock, Buildable) `fixed="right"` so it stays visible at 1280px. A pinned cell paints its own background (`.master-data-selected-row`).

## Document detail pages
- **Shared shell** (F185): `src/hooks/useDocumentForm.ts` (dirty flag via `markDirty` on `onValuesChange`, `markSaved` after a save, Ctrl/Cmd+S, the leave-page warning) and `src/components/document-export-buttons.tsx` (PDF/Excel, disabled while dirty because the server exports saved rows). A new document page uses both.
- Every export goes to the server template (`Export*Document`); there is no client-side PDF and no live preview. `src/components/invoices/layouts.ts`, `pdf.tsx` and `pdf-tunisia.tsx` are orphaned but kept, which is why `@react-pdf/renderer` stays. Deleting them is an open owner decision. `src/types/document-layout.ts` holds the layout options so the Organizations drawer doesn't import React-PDF.
- Invoice page: a new invoice's number is a placeholder preview, and the server assigns it (F169). Duplicate sends a blank number. Discount uses `allocateDiscount` (`src/utils/currency.ts`, the twin of `validateInvoiceTotals`). Stamp and withholding fields show only when their organization flags are on. A migrated loan (`origin === "opening"`) opens read-only.
- Status sets and transitions live in `src/types/<doc>.ts` and must match the Go maps. `src/components/status-flow.tsx` shows the full matrix next to the status tag (not on invoices or incoming invoices, which have none).
- Order and PO line items echo their `id` on save; the server keeps line ids stable.
- `src/components/payments/payment-panel.tsx` takes amounts already in cents (`unitsToCents`).

## Cash Book
- `src/components/cash-book/use-cash-book.ts` (`useCashBook`) holds all state and handlers. `modals.tsx`, `search-results.tsx` and `onProductSelect` are shared. Two layouts, by owner decision: `CashBookV1` (stacked, in `src/routes/cash-book.tsx`) and `cash-book-v2.tsx` (`counter.tsx` plus `loan-register.tsx`). They are switched per browser by `layout-switch.tsx` (disabled mid-sale). Dropping one means deleting its files and the switch; the hook stays.
- Prices are entered **gross**. `sale-assembly.ts` (`assembleSale`, `netCentsFor`) converts each line to net once from its effective tax rate (its own, else `defaultTaxRateId`), and every total derives from it so the payload matches `validateInvoiceTotals` (`sale-assembly.test.ts`, F189).
- A new customer is a local draft (`NewClientDraft`) sent inside `POST /api/cash-sales`, which keeps the write atomic. Name and phone are required.
- Sale type defaults to Loan (forgetting to switch leaves a visible balance). The amount only sets the default; `paid` iff the amount reaches the total.
- `ProductSelectCell` (`src/components/line-items/table.tsx`) offers no serialized product. It warns on stock going below zero or a missing cost under perpetual valuation (F158), and searches SKU or name.
- Loan collection is per invoice line via `CreateCashSalePayment` (capped at the line's outstanding); the server marks the invoice paid. The invoice number is shown as the loan number. Product details in Payment history open on click (touch-friendly) from `payment-products-cell.tsx`.
- `loan-register-model.ts` (unit-tested) groups by customer. `STALE_AFTER_DAYS` (60) must equal `db/loan_register.go`'s `LoanStaleAfterDays`. v2 calls `useCashBook({ scopeLoanStatusToClient: false })`, and its loan export sends the active tab as `register=`.
- After a sale or collection the button frees as soon as the POST answers, and panels refresh in parallel in the background (F172). Payments are fetched with `direction=inbound`, plus `clientId` in v1.
- A failed loan-status fetch shows a warning, never "no debt".

## Master data and lists
- `src/components/master-data/`:
  - `useSummariesEnabled(section)` requires summaries on **and** a role that may see the section (`roleCanSeeSummaries`); an unresolved role is not allowed.
  - Shared pieces: `headline-figure.tsx`, `filter-chips.tsx`, `list-with-panel.tsx` (380px panel, or a Drawer below `md`), `aging-bar.tsx`/`aging.ts` (shared with the Dashboard), `due-date.tsx`.
  - `use-status-chips.tsx` gives flow-document lists their status chips. Counts are taken over what the other filters leave. Labels use `<Trans context="document filter">` (`"production order filter"` for production orders).
- With summaries off, Clients, Vendors, Products and BOM are their plain lists; with summaries on, they get a headline, chips and a side panel (`*-summary-panel.tsx`). Headlines come from the server summaries, never by summing listed totals. A failed summaries load falls back to the plain list. Products stays server-paginated, so its chip counts come from `…/products/summary`. Stock value is hidden under quantity-only.
- Invoices and Incoming invoices take Unpaid/Overdue/Paid from `…/outstanding`, because state can lag balance. If that fails they fall back to state and due date. These chips aren't behind `masterDataSummaries`.
- `mass-data-excel-actions.tsx`: one component for every master-data list plus Inventory (`resource="stock-movements"`, `summarize`). In `compact` mode Upload sits outside the dropdown (a file input inside a closing menu unmounts).
- Archived products (F167): the drawer offers Delete only at usage 0, otherwise Archive, and Restore for an archived one (`archiveProductAtom` resends the stored product). Pickers drop archived products but keep saved values; recipe pickers show them disabled (`product-option.ts`); the stock movement form keeps them.
- `proposeProductCode` (`src/utils/product-code.ts`, unit-tested) proposes a new product's SKU from the closest `PREFIX-NNN` series.
- BOM screen: `bom-fields.tsx` is shared by the product form and `bom-editor-drawer.tsx`. Its rows use `wrap={false}` and a `minWidth: 0` Select. A failed recipe load disables Save. Batch size scales the display only. Quantities round to 4 decimals, the same as `roundBOMQuantity`, so a scale/divide round trip can show 0.9999 but never drifts further on re-save (a re-save is detected as a no-op for version history).
- Imports: a PO's Import select copies currency and rate onto **new** orders only (`if (!isNew) return`). List summaries come from `GetImportSummaries` (batch, no N+1).

## Settings and other pages
- `src/routes/organizations/index.tsx` owns every fetch and handler; its children in `src/components/organizations/` are presentational. The logo card uses a plain `<img>` against `/logo` (it can edit another organization), and `hasLogo` starts false. `myOrgAdminIds` drives per-row admin actions and the Members card.
- `src/routes/settings/document-templates.tsx` has one card per document type. Uploads use `beforeUpload` returning `false` plus the typed API function, so the CSRF header is sent.
- `src/routes/settings/document-numbering.tsx` has five cards with a live preview (`src/utils/document-number.ts`). Invoice numbering stays under Settings → Invoice.
- `units-of-measure.tsx` and `product-families.tsx` are list + drawer (no detail route). Only one unit can be the default; the server clears the old one, and a client-side merge must mirror that.
- `src/routes/settings/activity.tsx`: a new route resource needs a `resourceLabel` entry, and a new audited column wants a `fieldLabel` in `activity-changes.ts`.
- `src/routes/settings/loan-import.tsx`: Import trusts only a check of the same file and cutover (`checked`). The cutover is sent as `calendarDayMs`.
- `src/routes/dashboard.tsx` and `src/components/dashboard/`: only revenue and the top lists follow the period picker (in the Revenue header). A failed load shows the error and Retry, never zeros.
- Reporting pages own their `RangePicker` state, load through `useFetch` and chart with `@ant-design/plots` (no new chart dependency).
- `src/components/organizations/test-org-tag.tsx`: the "Test" tag beside the org switcher, in its dropdown (via `optionRender`; the `<Option>` children must stay the plain name, because `filterOption` searches them) and in the Organizations list.
- `src/routes/login.tsx` shows the SSO button when `GET /api/auth/oidc/enabled` is true. `src/utils/session-resume.ts` returns to the page and organization after a session ends.
- `src/utils/client-errors.ts` reports uncaught errors to `POST /api/client-errors` (installed in `src/main.tsx`).
- `src/locales/`: .po catalogs. Run `pnpm extract` after changing strings, and fill in de/fr.
