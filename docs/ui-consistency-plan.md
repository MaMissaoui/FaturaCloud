# FaturaCloud UI Consistency & Density Plan

## Status

**Tier 0 and Tier 1 are done, verified in-browser, and committed.** **Tier 2
(line-item tables) is in progress** — the shared shell exists and
`orders/details.tsx` and `deliveries/details.tsx` (migrations 1-2 of 6) are
done; the other four documents still use their own hand-rolled table. **Tier 3
(detail-page shells) is not started.**

Tier 2 progress:
- `src/components/line-items/table.tsx` — the config-driven shell from 2.1.
  Column kinds so far: `index`, `product`, `description`, `quantity`, `unit`,
  `unitPrice`, `custom`. `taxRate` and `lineTotal` kinds will be added when the
  documents that need them (invoices/incoming-invoices) are migrated, rather
  than speculatively built now.
- `orders/details.tsx` migrated: `#` column added, Qty/Unit price
  right-aligned, `Delivered` custom column preserved via `kind: "custom"`.
  Verified live: existing order loads/edits/saves correctly, product
  selection still auto-fills description + unit price, new-order flow still
  works (no `Delivered` column when `isNew`).
- `deliveries/details.tsx` migrated (no price columns, per the non-goals
  list): `#` column added; `unit` promoted from a page-local custom column to
  a first-class shell kind (also needed by purchase-orders and
  inbound-deliveries next); `availableStock` stays `kind: "custom"` since it's
  delivery-specific (reads `stockEnabled`/`availableStock`/`quantity` off the
  form to render a colored `Tag`). The `isEditable` (shipped/delivered freeze)
  gating this file already had was carried through as the shell's `disabled`
  prop — verified live that a `shipped` delivery still renders every cell
  disabled with the remove/Add affordances hidden, and that a new delivery's
  product selection still auto-fills description/unit/availableStock.
  (The Tier 2.1 write-up's suspicion that this file was missing `disabled`
  gating did not hold up — the gating was already correct before this
  migration; only the extraction to the shared shell was needed.)
- Tier 2.3 (borderless-cell / drag-handle visual polish) **deliberately
  deferred** — per the plan, it's its own reviewable commit after the shell
  exists across more than one document, not bundled into the first migration.
- **Aside, not fixed**: saving an order always resets its `Delivered` column
  to 0 — `db.UpdateOrder` replaces all line items (new IDs) on every save,
  so `GetOrderDeliveredQuantities` (keyed by the old `orderLineItemId`) no
  longer matches. Pre-existing backend behavior, unrelated to this migration
  (confirmed identical on `main` before this change) — out of scope here.
- `purchase-orders/details.tsx` is still flagged (per Tier 2.1) as missing
  `disabled` gating on its line items despite having terminal
  `received`/`cancelled` statuses — worth checking for real when that
  document is migrated next.

What actually shipped, with deviations from the original plan noted inline:
- Tier 0.1 (shared `Section`), 0.3 (Drawer `size` rename) — done as planned.
- Tier 0.2 (`PageHeader` extraction) — done, on `refactor/page-header-extraction`.
  Adopted in all 13 list/settings pages named in the original plan. `inventory`'s
  product filter (a `Select`, not a search box) goes through a new `extra` slot
  not in the original spec; `users`/`organizations` keep their per-page
  `marginBottom` via a `style` prop since their tables aren't wrapped in their
  own spacing `Row`.
- Tier 0.4 (`ScrollShadow` consistency) — done, same branch. Added to
  `tax-rates/form.tsx` and `stock/movement-form.tsx`, the two drawers Tier 1.1
  found already fit without overflow (so weren't converted to the two-column
  layout) but still lacked the scroll affordance the other three have.
- Tier 1.1/1.2 — **only clients, vendors, and products needed conversion.**
  tax-rates and stock-movement were measured at 1440×900 first and already fit
  with zero overflow — converting them would have been unrequested churn. See
  the corrected 1.1 section below.
- Tier 1.3 — done for `settings/invoice.tsx`. `settings/backup.tsx` measured at
  zero overflow (it's a backup list, not a dense form) — only its heading level
  was fixed for consistency, layout left alone.

## Context

Three UI problems have accumulated as the app grew from invoicing into a full
purchasing/inventory suite:

1. **Line-item entry tables** were copy-pasted into six document detail pages
   and then drifted apart. They share ~150 lines of identical scaffolding but
   disagree on column order, labels, alignment, and affordances.
2. **Master-data and settings forms** are single-column drawers fixed at 480px.
   The recent e-invoicing work (PR #29) added 7 fields to clients and
   organizations, pushing these forms to ~2.5 screens of vertical scrolling.
3. **Screen shells are hand-rolled per page.** Eleven list pages each re-declare
   the same header/search/table block; settings pages use a different heading
   level and a width cap the rest of the app doesn't have.

Goal: one visual system, denser entry screens that fit on a laptop viewport, and
a single shared line-item table — without breaking the domain rules that make
certain documents deliberately different.

**Verified environment**: antd 6.5.1, React 19, Vite 8. All findings below were
confirmed by reading the code and by screenshotting the running app at
1440×900, not inferred.

---

## Non-goals and do-not-touch list

Read this section before changing anything. Several things look like bugs and
are not — "fixing" them will cause regressions.

- **`VAT 20% 20%` in the tax dropdown is NOT a bug.** The rendering is
  `{rate.name} {rate.percentage}%` (`src/routes/invoices/details.tsx:943`). The
  seed data happens to contain a rate *named* "VAT 20%". A rate named "Standard"
  renders correctly as "Standard 20%". Leave the composition alone.
- **The invoice `Total` column is an editable input, not a display field.**
  `src/routes/invoices/details.tsx:963-1010` — typing in it back-computes
  `quantity` or `unitPrice`. Do not convert it to formatted currency text and do
  not add a `formatter` that would interfere with typing.
- **Purchase-order totals are hidden when `subTotal === 0`** — guarded at
  `src/routes/purchase-orders/details.tsx:507`. Correct behaviour, not a missing
  totals block.
- **Outbound delivery notes must never gain price columns.** Per CLAUDE.md,
  `outbound_delivery_line_items` has no price columns by design.
- **`inbound_delivery_line_items.unitCost` must stay** — it feeds
  `stockMovements.unitCost` and the product average cost. It is the deliberate
  divergence from outbound deliveries.
- **Per-document computed columns stay**: PO `received`, order `delivered`,
  incoming-invoice `match`, delivery `availableStock`, inbound `currentStock`.
- Do not change any Go code, API, or DB schema. This is frontend-only.
- Do not touch the `loadable()` Suspense pattern in the detail pages.

---

## Tier 0 — Shared primitives (do first; low risk, unblocks the rest)

### 0.1 Extract the duplicated `Section` component
`Section` is defined **byte-identically in five files**:
`src/components/clients/form.tsx:15`, `vendors/form.tsx:17`,
`tax-rates/form.tsx:48`, `products/form.tsx:26`, `stock/movement-form.tsx:13`.

Create `src/components/form-section.tsx` exporting that exact component
unchanged, then import it in all five and delete the local copies. No visual
change should result — verify by screenshot diff.

### 0.2 Extract a shared `PageHeader`
Eleven list pages hand-roll the same block: `Title level={3}` + icon on the
left, `<Space>` with `<Input.Search>` + primary action button on the right.
See `src/routes/clients.tsx:56-70` for the canonical version.

Create `src/components/page-header.tsx`:

```tsx
interface PageHeaderProps {
  icon?: React.ReactNode;
  title: React.ReactNode;
  search?: { value: string; onChange: (v: string) => void; placeholder?: string };
  actions?: React.ReactNode;
}
```

Adopt in: `clients`, `vendors`, `products`, `invoices/index`, `orders`,
`deliveries`, `purchase-orders`, `inbound-deliveries`, `incoming-invoices`,
`inventory`, `settings/tax-rates`, `settings/users`, `organizations/index`.

Keep the existing `Title level={3}` size — it is the majority convention.

### 0.3 Fix the antd 6 Drawer deprecation
Console shows: `Warning: [antd: Drawer] 'width' is deprecated. Please use 'size'
instead.`

In antd 6.5.1 `size` accepts arbitrary numbers, not just presets — confirmed in
`node_modules/antd/es/drawer/Drawer.d.ts:17`:
`size?: sizeType | number | string`. So this is a plain rename.

Change `width={480}` → `size={480}` in all five drawer forms (widths are changed
in Tier 1; do the rename first so the two changes stay reviewable).

### 0.4 Apply `ScrollShadow` consistently
`src/components/scroll-shadow.tsx` is used in `clients`, `vendors`, `products`
forms but missing from `tax-rates/form.tsx` and `stock/movement-form.tsx`. Add
it to those two for consistent scroll affordance.

---

## Tier 1 — Make entry screens fit without scrolling

### The reference pattern (already in the codebase, already verified)
`src/routes/organizations/index.tsx:342-570` is the target pattern and it
renders well — I screenshotted it at 1440×900:

- `<Drawer size={640}>`
- `<Card title={...}>` per logical section
- `<Row gutter={[16, 0]}>` + `<Col xs={24} md={12}>` two-column grid

At 640px the two columns are ~275px each: no label wrapping, no cramping, ~9
fields visible versus 6 in the 480px single-column clients drawer. **Use this,
not a new invention.**

Note `<Col xs={24} md={12}>` breakpoints track the **viewport**, not the drawer,
so `md` matches inside a 640px drawer on a desktop viewport. That is why the
grid works; don't switch to container queries.

### 1.1 Convert the master-data drawers that actually overflow — DONE
**Correction to the original plan**: it listed five drawers
(`clients`, `vendors`, `products`, `tax-rates`, `stock/movement-form`). Before
converting any of them, each was opened at 1440×900 and measured via
`el.scrollHeight - el.clientHeight` on `.ant-drawer-body`. Only three actually
overflowed:

| Form | Fields | Overflow before | Converted? |
|---|---|---|---|
| Clients | 15 | ~2.5 screens | ✅ yes |
| Vendors | 9 | cut off (Address hidden) | ✅ yes |
| Products | 7 | cut off (tax rate hidden) | ✅ yes |
| Tax rates | 6 | **0px — already fit** | ❌ skipped |
| Stock movement | 6 | **0px — already fit** | ❌ skipped |

Applied to the three that needed it:
- `size={480}` → `size={640}`
- Wrap each `Section` group in `<Card title={...} style={{ marginBottom: 16 }}>`
  (this **replaces** `Section`, it doesn't sit alongside it — `Section` is only
  still imported by tax-rates and stock-movement, the two that were skipped)
- Put fields in `<Row gutter={[16, 0]}>` / `<Col xs={24} md={12}>`
- **Full width (`<Col xs={24}>`) for**: Address, Notes, Description, any
  `<Input.TextArea>`, and the `E-mails` tag-mode `<Select>`
- Half width for everything short: Code, Phone, VAT, postal code, city, country,
  currency, percentages, dates
- Vendors: after the two-column pass there was still 153px of overflow. Fixed
  by folding the single-field "Address" card into the "Contact" card (one
  fewer card boundary) rather than reaching for `Collapse` on a form this
  small — down to 6px, not worth chasing further.
- Products: the conditional "Track inventory" switch (only shown when
  `type === "product"`) originally pushed its own full-width row, costing 54px.
  Fixed by widening its `Form.Item noStyle shouldUpdate` wrapper into a third
  `md={8}` column alongside Type/SKU instead of its own row — same logic,
  tighter layout.

### 1.2 Collapse secondary sections — DONE (clients only)
Only clients needed this after the Tier 1.1 pass (vendors and products fit with
the two-column grid alone). Applied:

| Form | Always expanded | Collapsed by default |
|---|---|---|
| Clients | Contact, Address | E-invoicing |

Organizations already had this pattern pre-existing and untouched (it's the
reference implementation, not something this plan changed).

**Critical — validation footgun.** A collapsed antd `Collapse` panel does not
mount its children, so `Form.Item` rules inside it never register and `submit()`
fails silently with no visible error. Mitigate both ways:

1. Set `forceRender: true` on **every** collapsed panel. Confirmed available in
   antd 6.5.1 at `node_modules/antd/es/collapse/Collapse.d.ts:61`.
2. In each form's submit handler, catch the validation rejection and expand the
   panel containing the first errored field, e.g.:

```tsx
onFinishFailed={({ errorFields }) => {
  const first = errorFields[0]?.name[0] as string;
  const panel = PANEL_OF_FIELD[first];
  if (panel) setActiveKeys((k) => [...new Set([...k, panel])]);
}}
```

Do not ship collapsible sections without both.

### 1.3 Settings pages — DONE for invoice, skipped (layout) for backup
`src/routes/settings/invoice.tsx` and `settings/backup.tsx` both used
`<div style={{ maxWidth: 720 }}>` + `Title level={4}`, while every other page
uses `level={3}` and no cap.

**invoice.tsx** overflowed badly (610px, before any fix) and was converted:
- `Title level={4}` → `level={3}`, `maxWidth: 720` → `1100`
- Paired the four cards into two outer rows — (Defaults, Numbering) and (Logo,
  Vendor invoice matching) — each `<Row gutter={[16,0]}><Col xs={24} xl={12}>`.
  **Correction to the original plan**: it specified `xxl={12}` to dodge a
  nested-breakpoint cramping risk (halving only above 1600px). In practice
  `xl={12}` at 1440px was tested directly and did not cramp, because the
  Numbering card's nested `<Col md={14}>`/`md={10}` row was flattened to
  `xs={24}` (stacked) as part of the same change — this removes the
  nested-breakpoint risk entirely rather than just deferring it to a wider
  screen, so there's no `xxl`-only edge case left to worry about.
  Same flattening applied to the Vendor-invoice-matching card's two fields.
- Even after the two-column split, 181px of overflow remained. Closed with:
  `Card size="small"` (antd's built-in compact variant, tighter than manually
  fighting `bodyStyle`) on all four cards, `TextArea rows={3}` → `rows={2}` on
  Notes, `Title` bottom margin 20→12, Save button margin 40→8, and the closing
  `<Divider />` before Save tightened to `margin: "8px 0 16px"`. Final overflow:
  8px — same negligible tolerance as the vendors drawer, no visible scrollbar.
- Verified interactive behaviour survives: "Available variables" still expands
  (and correctly pushes the second row down, since Row height follows its
  tallest column — that's normal grid behaviour, not a bug) and Save still
  submits without new console errors.

**backup.tsx** was measured first: 0px overflow at 1440×900 (it's a backup
list + action buttons, not a dense form — expected to scroll once the backup
list grows, which is legitimate, not the cramped-form problem this tier
targets). Left the layout alone; only changed `Title level={4}` → `level={3}`
for consistency with the rest of the app.

---

## Tier 2 — Unify the line-item tables

### What is actually shared today
All six pages use the identical scaffolding — `<Form.List>` wrapping an antd
`<Table>` with `<Table.Column>` children, `dataSource={fields.map((field, index)
=> ({ ...field, index }))}`, `pagination={false}`, `size="middle"`,
`locale={{ emptyText: t\`No line items\` }}`, `rowKey` on `index`. That block is
duplicated six times.

Locations: `invoices/details.tsx:753`, `orders/details.tsx:308`,
`deliveries/details.tsx:304`, `purchase-orders/details.tsx:351`,
`inbound-deliveries/details.tsx:307`, `incoming-invoices/details.tsx:345`.

### How they diverge (all of this is unintentional)
| | invoices | orders | deliveries | purchase-orders | inbound-deliveries | incoming-invoices |
|---|---|---|---|---|---|---|
| First column | **Description** | Product | Product | Product | Product | Description |
| Unit column | ✗ | ✗ | ✓ | ✓ | ✓ | ✗ |
| Line total | ✓ | ✗ | ✗ | ✗ | ✗ | ✗ |
| Drag reorder | ✓ | ✗ | ✗ | ✗ | ✗ | ✗ |
| Qty label | `Qty.` | `Qty` | `Qty` | `Qty` | `Qty` | `Qty` |
| `marginTop: 8` | ✗ | ✓ | ✗ | ✓ | ✓ | ✓ |
| Titles use | `` t`…` `` | `<Trans>` | `<Trans>` | `<Trans>` | `<Trans>` | `<Trans>` |

Plus: invoice's delete button is a gray text button positioned *outside* the
table border (`position: absolute; right: -32`), while every other page uses a
red inline `<Button danger>`. Numeric cells are left-aligned everywhere.

### 2.1 Build a config-driven shell
Create `src/components/line-items/table.tsx`. It must be **column-descriptor
driven**, not a fixed table with a long prop list. The shell owns: `Form.List`,
the `Table` scaffolding, DnD wiring, add/remove, row striping, empty state.

```tsx
export type LineItemColumn =
  | { kind: "index" }
  | { kind: "product"; width?: number; onSelect?: (...) => void }
  | { kind: "description" }
  | { kind: "quantity" }
  | { kind: "unit" }
  | { kind: "unitPrice"; label?: React.ReactNode }
  | { kind: "taxRate"; taxRates: any[] }
  | { kind: "lineTotal" }
  | { kind: "custom"; key: string; title: React.ReactNode; width?: number;
      render: (field: any) => React.ReactNode };

interface LineItemsTableProps {
  name?: string;              // Form.List name, default "lineItems"
  columns: LineItemColumn[];
  reorderable?: boolean;      // DnD, default false
  disabled?: boolean;         // frozen lines (shipped deliveries)
  addLabel?: React.ReactNode;
}
```

Per-document computed columns (`received`, `delivered`, `match`,
`availableStock`, `currentStock`) go through `kind: "custom"` — they stay owned
by their page and are **not** absorbed into the shell.

**The `disabled` contract is verified.** Freezing is done **in place** — each
input gets `disabled={!isEditable}`, and the remove/add buttons are hidden —
*not* by swapping to a separate read-only view. Reference implementation:
`src/routes/inbound-deliveries/details.tsx:202` (`const isEditable = isNew ||
currentStatus === "draft"`) applied at `:327`, `:364`, `:382`, `:393`, `:408`,
with `{isEditable && …}` guards at `:429` and `:447`. So a single `disabled`
boolean on the shell maps cleanly; no `readOnly` render mode is needed. The
shell must, when `disabled`: pass `disabled` to every cell input, hide the
remove column, and hide the Add button.

**Bug found while verifying this — fix it during the deliveries migration.**
`src/routes/deliveries/details.tsx` has `currentStatus` available (`:230`) and
uses it to gate footer buttons (`:446`, `:470`) but **never gates the line-item
inputs, the remove button, or the Add button**. Per CLAUDE.md the server freezes
outbound delivery line items once `shipped`/`delivered` and `PUT` accepts
header-only edits — so today the UI lets a user edit lines that the server will
reject. Wire `disabled={!isEditable}` there using the inbound-deliveries rule.
Check `purchase-orders/details.tsx` for the same gap (it has terminal
`received`/`cancelled` statuses and no `disabled` gating either).

### 2.2 Standardise while extracting
- **Column order**: `#` → Product → Description → Qty → Unit → Price → Tax →
  Total → (computed) → remove.
  ⚠️ **Decision required before starting — see "Open decision" below.** Whether
  invoices moves Description after Product is not settled. Do not decide this
  silently inside a refactor commit.
- Add the `#` row-number column everywhere (currently nowhere has it).
- **Right-align all numeric cells** (Qty, Unit price, Tax, Total). Currently all
  left-aligned, which makes columns of figures hard to scan.
- One delete affordance: inline trailing icon button, same style on all six.
  Drop the invoice's `right: -32` absolute positioning.
- Consistent labels: `Qty` (not `Qty.`); keep `Unit cost` for purchasing docs
  and `Price` for sales docs — that distinction is meaningful.
- All titles via `<Trans>`; drop the `` t`…` `` variant in invoices.
- Enable `reorderable` on invoices only at first (preserves current behaviour);
  extending DnD to other docs is a separate, optional follow-up.

### 2.3 Make the table *render* better (not just consistently)

Consistency work alone won't address the visual complaint. From the invoice
screenshot at 1440×900, two things make the table read poorly:

- **Every cell is a fully-bordered `<Input>`/`<InputNumber>`**, so the table
  looks like a grid of stacked boxes rather than a table with editable cells.
  Fix: use antd's borderless input variant inside line-item cells and bring the
  border back on hover/focus only — `<Input variant="borderless">` plus a CSS
  rule on the cell (`.line-items-table td:hover .ant-input { … }`). Keep the
  focus ring; only the resting state loses its box.
- **The drag handle is a near-invisible dotted glyph sitting outside the table
  border.** Fix: move it into a proper leading handle cell (combine it with the
  new `#` column — number at rest, grip icon on row hover), with
  `cursor: grab` and a visible hover state.

Also: give the table a subtle header background and row-hover highlight so rows
track horizontally, and keep row height compact (`size="small"` is worth trying
against the current `size="middle"` once cells are borderless — borderless cells
need less vertical padding).

Do this as its own commit *after* the shell exists, so the visual change is
reviewable independently of the mechanical extraction.

### 2.4 Migration order (one PR each, easiest first)
1. `orders/details.tsx` — simplest, no reorder, no tax
2. `deliveries/details.tsx` — **no price columns, ever**
3. `purchase-orders/details.tsx`
4. `inbound-deliveries/details.tsx` — keep `unitCost`
5. `incoming-invoices/details.tsx` — keep the `match` column
6. `invoices/details.tsx` — **last**, it has DnD + the editable back-computing
   Total and is by far the riskiest (1251-line file)

---

## Tier 3 — Detail-page shells (optional; do last, or defer)

`invoices/details.tsx` wraps its content in `<Card>` sections. The other five
use bare `<Row gutter={24}>` with hardcoded, non-responsive `span={N}` values
that differ per page (8/6/5/4). Totals render three different ways: a custom
Row/Col in invoices, `<Descriptions>` in orders/PO/incoming, nothing in
deliveries/inbound.

If picked up: wrap the five in `<Card>` to match invoices, replace fixed `span`
with responsive `xs={24} md={12} xl={6}`, and standardise on the
`<Descriptions>` totals block. This is the largest and least urgent change —
treat it as explicitly optional.

Also minor, from the Organizations list screenshot: org names are plain text
while every other list makes the name a link, and rows carry a text `Delete`
button instead of the `⋮` overflow menu used on invoices. Align it with the
others when convenient.

---

## Tier 4 — Scale & reporting (separate initiative, not scoped in detail yet)

Two items raised after Tier 0 shipped. Recorded here for planning; neither has
an implementation plan yet, and **both break this doc's own non-goal of
"frontend-only, no Go/API/DB changes"** — they're a distinct initiative from
the Tier 0-3 UI-consistency pass, not a continuation of it.

### 4.1 Inventory / product-list UI at scale
Confirmed today: `GetProducts`/`GetStockMovements` (`src/atoms/product.ts`,
`src/atoms/stock.ts`) fetch the *entire* table in one request with no
`limit`/`offset`/`search` params anywhere in `api/products.go`; `/inventory`
and `/products` paginate client-side only (`pagination={{ pageSize: 50 }}` at
`src/routes/inventory.tsx:114`, `defaultPageSize: 25` in `products.tsx`). Fine
at tens/hundreds of rows; at thousands this means a multi-MB payload on every
load, full in-memory filter/sort, and no server-side search. Needs: paginated
+ filtered Go endpoints, matching server-side `Table` pagination/sorting and
debounced search on the frontend, and likely a DB index on
`products(organizationId, name/sku)`. Also worth checking whether the
line-item product `Select`s (which assume the full product list fits in an
in-browser `showSearch` dropdown) need to move to a search-as-you-type API
call once this is in the thousands.

### 4.2 Dashboard(s) and reports
Confirmed today: no dashboard or reporting route/component exists anywhere in
`src/routes/` or `src/layouts/`. This is a new feature area, not a refactor —
needs a decision on what to report (revenue over time, outstanding invoices,
stock valuation, top clients/products, sales vs. purchasing) before any
implementation, new aggregate Go endpoints (`GROUP BY`/`SUM` queries, not just
returning raw entity lists for the frontend to crunch), and a new sidebar
entry/section.

---

## Verification

Run after each tier, not just at the end.

```bash
pnpm lint          # tsc --noEmit + oxlint
pnpm build         # must succeed
pnpm format:check
go build ./... && go vet ./...   # should be untouched, confirm no drift
```

**Visual acceptance criteria** — start `go run .` + `pnpm dev`, then at a
**1440×900** viewport screenshot each of:

- `/clients` → open a client. Pass = all Contact + Address fields visible with
  **no scrollbar**; E-invoicing collapsed.
- `/vendors`, `/products`, `/settings/tax-rates` → same criterion.
- `/organizations` → open an org. Pass = Details card fully visible without
  scrolling; the other four sections collapsed.
- `/settings/invoice` → pass = whole form fits one screen, no vertical scroll.
- Each of the six document detail pages → line-item table renders, columns in
  the standard order, numerics right-aligned.

**Functional checks that guard the risky parts:**
- On a collapsed-section form, clear a required field inside the collapsed
  panel and submit. The panel must expand and show the error — it must not fail
  silently. This is the single most likely regression.
- On an invoice, drag a line item to reorder — DnD must still work after the
  table extraction.
- On an invoice, type into the line `Total` field — it must still back-compute
  `quantity`/`unitPrice`.
- Mark a delivery `shipped`. Line items must now be **disabled**, with the
  remove and Add buttons hidden — this is a behaviour *fix*, not a preservation:
  they are editable today (see the bug noted in Tier 2.1). Confirm header-only
  edits (tracking number, notes) still save.
- Confirm delivery-note and goods-receipt PDFs are unchanged (Tier 2 touches
  the edit tables, not the PDF templates).

**i18n:** new strings (Collapse panel headers, `#` column, any PageHeader
titles) need extraction and manual translation:
```bash
pnpm extract   # missing-count baseline is 1 (a benign empty-header msgid)
```
Then hand-translate the new msgids in `src/locales/de.po` and `fr.po`. English
is the source locale and needs no translation.

## Open decision (resolve before Tier 2)

**Does the invoice line-item table move Description out of the first column?**

Every other document puts Product first; invoices puts Description first. It is
the only genuinely behavioural change in this plan, and it lands on the app's
most-used screen — users have muscle memory for typing a description into the
leftmost cell. Two defensible answers:

- **(a) Standardise all six** on Product-first. Full consistency; costs muscle
  memory on the busiest screen.
- **(b) Standardise the other five to each other, leave invoices
  description-first.** 5-of-6 consistency at zero behavioural cost. Justifiable
  on its merits too: on invoices the product link is optional (the column
  placeholder is literally "Optional") whereas on purchasing documents the
  product drives cost and stock, so leading with Description on invoices matches
  how the screen is actually used.

Default to **(b)** unless told otherwise — it is the lower-risk reading of "as
far as possible".

## Workflow

Per the project's established workflow: **branch + PR, never commit directly to
`main`.** Each tier should be its own branch and PR, squash-merged. Tier 2's
six document migrations should be separate commits within one PR (or separate
PRs) so a regression can be bisected to a single document.

## Suggested commit split

Per CLAUDE.md conventional commits, one concern per commit:
- `refactor: extract shared Section and PageHeader components`
- `fix: replace deprecated Drawer width prop with size`
- `feat: convert master data drawers to two-column card layout`
- `feat: collapse secondary form sections to fit without scrolling`
- `refactor: extract shared line items table` (then one commit per document)
