# FaturaCloud UI Consistency & Density Plan

## Status

**Tier 0, Tier 1, Tier 2 (including 2.3), and Tier 3 are all done**, verified
in-browser, and committed — all six document detail pages share the
`src/components/line-items/table.tsx` shell with the borderless
cell/drag-handle polish, and now use responsive header-field layouts. Tier
3's original scope (Card-wrapping, totals unification) turned out to be based
on stale premises and was corrected down to just the responsive-span
conversion — see the Tier 3 section for the full write-up.

Tier 2 progress:

- `src/components/line-items/table.tsx` — the config-driven shell from 2.1.
  Column kinds so far: `index`, `product`, `description`, `quantity`, `unit`,
  `unitPrice`, `taxRate`, `custom`. The `quantity` kind gained an optional
  `label` and `unitPrice` an optional `name` (the form field to bind, default
  `"unitPrice"`) during the inbound-deliveries migration, since that document
  uses `unitCost` and "Qty received" instead of the defaults — both additive,
  no existing usage changed. `taxRate` was added as a proper kind during the
  incoming-invoices migration (below). `lineTotal` will be added when
  `invoices/details.tsx` is migrated, rather than speculatively built now.
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
- Tier 2.3 (borderless-cell / drag-handle visual polish) is **done**, as its
  own commit after all six documents were on the shell (see below, after the
  invoices entry).
- **Aside, not fixed**: saving an order always resets its `Delivered` column
  to 0 — `db.UpdateOrder` replaces all line items (new IDs) on every save,
  so `GetOrderDeliveredQuantities` (keyed by the old `orderLineItemId`) no
  longer matches. Pre-existing backend behavior, unrelated to this migration
  (confirmed identical on `main` before this change) — out of scope here.
- `purchase-orders/details.tsx` migrated: `#` column added, `unit` now uses
  the shared kind (added in the deliveries migration), `Received` custom
  column preserved. **No `disabled`/`isEditable` gating was added**, unlike
  the plan's Tier 2.1 suspicion — checked for real this time: unlike outbound
  deliveries, `db.UpdatePurchaseOrder` has **no server-side status check at
  all**; a `PUT` with `lineItems` is applied regardless of status, so there is
  no existing rule for a client-side `disabled` to mirror. Adding one now
  would be inventing new restrictive behavior the backend doesn't enforce,
  not extracting an existing one — out of scope for a "make six tables use
  one shell" refactor, so left exactly as unrestricted as it was on `main`.
- **Aside, not fixed — more serious than the orders `Delivered` bug**: like
  `db.UpdateOrder`, `replacePurchaseOrderLineItemsTx` deletes and reinserts
  every line item (fresh ids) on any `PUT` that includes `lineItems`, which
  the frontend always sends on save. For purchase orders this is worse than
  orders' cosmetic "Delivered resets to 0": `inbound_delivery_line_items.
purchaseOrderLineItemId` and `incoming_invoice_line_items.
purchaseOrderLineItemId` — and the 3-way match in
  `db/incoming_invoice_match.go`, which looks up `purchase_order_line_items`
  by that id — all key off the old id. Editing and saving a purchase order
  that already has linked goods receipts or incoming invoices orphans those
  references silently. Confirmed pre-existing (identical on `main`), unrelated
  to this migration, out of scope here — flagging because the blast radius is
  larger than the orders case.
- `inbound-deliveries/details.tsx` migrated: `#` column added; `unit` and
  `unitPrice` reuse the shared kinds (`unitPrice` via its new `name: "unitCost"`
  override, `quantity` via its new `label: "Qty received"` override);
  `currentStock` stays `kind: "custom"`. The existing `isEditable` (draft-only)
  gating carried through as the shell's `disabled` prop unchanged — this file
  already had it correctly wired on every input plus remove/Add, same as
  deliveries. Verified live: product selection still auto-fills
  description/unit/unitCost/currentStock on a draft receipt.
- `incoming-invoices/details.tsx` migrated: `#` column added; `taxRate`
  promoted from a page-local custom column to a first-class shell kind. This
  document has **no product column** at all in its UI (unlike every other
  purchasing document) — `productId` is only ever carried through silently
  when a line is prefilled from a purchase order, never shown or edited —
  so the migrated columns are just `index`/`description`/`quantity`/
  `unitPrice`/`taxRate`/`match`, with no `kind: "product"` entry. `match`
  stays `kind: "custom"` (per-line 3-way-match status `Tag`, keyed off
  `matchLines` state, not the form). No `disabled` gating existed before or
  after — this document has no frozen-state concept for line items, only
  the state-level block on `approved`/`paid` enforced server-side by 3-way
  match. Verified live: tax-rate select populates from `taxRates`, and
  choosing a rate plus typing a unit price correctly recomputes the
  Subtotal/Tax/Total `Descriptions` block (which reads the same
  `Form.useWatch("lineItems")` + `useMemo` as before, untouched by the
  table extraction).
- `invoices/details.tsx` migrated — the last and riskiest of the six, and the
  only one needing new shell capability beyond a config knob:
  - **The plan's "Open decision" (below) was stale and did not need
    resolving.** It assumed invoices already put Description first; reading
    the actual code showed Product was already first, matching the other
    five documents. No behavioural change was needed or made — corrected the
    section below to say so rather than deleting the history.
  - Added `reorderable?: boolean` to the shell: it now owns the generic
    dnd-kit wiring (sensors, `DndContext`/`SortableContext`, the sortable
    `<tr>` row) and reorders the form's array field directly on drop. The
    visible drag handle stays page-owned (rendered inside invoices' Product
    cell via `useSortable` against the same row key) since it's the one
    genuinely document-specific piece — mirrors 2.1's `disabled` split
    between shell-owned mechanism and page-owned rendering.
  - `quantity`/`unitPrice`/`total` are **not** three independently
    configurable shell columns here — each one's `onChange` writes the other
    two (the editable back-computing Total), so they're one coupled unit and
    stayed `kind: "custom"`, page-owned, exactly like `Delivered`/`Received`/
    `match` on the other documents. Forcing per-column `onChange` escape
    hatches into the built-in `quantity`/`unitPrice` kinds for one document's
    quirk would have made the shell's config surface worse, not better.
  - **Preserved verbatim, not "cleaned up":** the back-compute handlers read
    and write via `form.getFieldValue(["lineItems", field.key, ...])` —
    `field.key`, not `field.name`. In antd `Form.List`, those two diverge
    after a middle row is removed (key is a stable counter, name is the
    positional index), so this is a pre-existing latent bug — fixing it
    would be a behaviour change hidden inside a refactor, exactly the kind
    of thing this migration series has been careful not to do. Flagging it
    here, not fixing it.
  - `taxRate` stayed `kind: "custom"` too, to preserve the `{name} {percentage}%`
    composition the non-goals list explicitly protects (`VAT 20% 20%` is not
    a bug — see the top of this document).
  - The shell's `custom` column kind gained an optional `onCell`, used here
    to keep the Product cell's `paddingLeft: 0` (compensating for the
    absolutely-positioned drag handle sitting in its gutter). The
    `description` kind gained an optional `rows` (invoices uses 4, not the
    shared default of 1).
  - **One deliberate visual change**: the remove button was absolutely
    positioned inside the Total cell (`right: -32`) with a `paddingRight: 0`
    cell override. It now uses the shell's plain trailing remove column,
    same as the other five documents — sanctioned by Tier 2.2's "one delete
    affordance" goal.
  - Verified live end-to-end, since this document had by far the largest
    blast radius: added a second line item, confirmed the Total field
    back-computes `unitPrice` and vice versa; dragged row 2 above row 1 with
    real pointer events (`page.mouse` — Playwright's `dragTo` uses HTML5 DnD
    events, which dnd-kit's `PointerSensor` does not listen for, so it silently
    no-ops) and confirmed the row order and all per-row values moved together;
    saved, reloaded, and confirmed both the new line and the reordering
    persisted; confirmed the PDF preview renders the persisted data; confirmed
    `VAT 20% 20%` still renders unchanged.
  - **Not a regression, but worth flagging**: while diagnosing an unrelated
    save failure during verification, found that the Product column's
    `requiredForNewLineItem` rule and antd's `required` rule on `Due date`
    both use `noStyle` on their `Form.Item`, which suppresses antd's
    validation-error rendering — so a line item missing a product (or, as
    happened during testing, a seed invoice missing its due date) fails
    `form.submit()` **silently**: no error, no network request, nothing to
    tell the user why Save did nothing. Confirmed this is pre-existing
    (identical on `main`, unrelated to the table extraction) — out of scope
    here, but a real UX footgun worth a follow-up.
- **Tier 2.3 (visual polish)**, done after all six documents were on the
  shell, as its own commit:
  - **Header background and row-hover highlight were already there** —
    verified via computed styles that antd's `Table` ships both by default
    (`headerBg`/`rowHoverBg` tokens, theme-aware, toggled by a
    `.ant-table-cell-row-hover` class antd adds itself on
    mouseenter/mouseleave). The plan's ask here needed no new code, just
    confirmation it wasn't missing.
  - **Borderless-until-hover/focus cell inputs**: added
    `src/components/line-items/table.module.scss`, scoped to the table via a
    `className` on antd's `<Table>` (so it can't leak onto ordinary inputs
    elsewhere on the same page). The rule suppresses border/background on
    `.ant-input`/`.ant-input-number`/`.ant-select` only while **not**
    hovered/focused and **not** in an error/warning status — status always
    keeps its border, and hover/focus styling is left to antd's own
    (already theme-aware) CSS rather than reimplemented, so nothing here
    hardcodes a light- or dark-mode color. Verified live in both themes,
    including the one case that would have broken silently: a disabled
    (frozen) delivery's line items stay borderless and legible with no hover
    state to fall back on.
  - **Drag handle merged into the `#` column**: added an `IndexCell`
    component to the shell, used only when `reorderable` is set. It shows the
    row number at rest and swaps to a `HolderOutlined` grip on row hover,
    reusing the same `.ant-table-cell-row-hover` class the header/row
    highlight already relies on rather than adding a second hover mechanism.
    This replaced invoices' old `DragHandleCell` (an absolutely-positioned
    `MoreOutlined` glyph sitting outside the table border, in the Product
    cell's left gutter) — deleted along with the `onCell` padding hack it
    needed. Since invoices was the only document that hadn't gotten the `#`
    column in its own migration (an oversight from Tier 2.2, caught here),
    it now has one, combined with the handle in the same cell everywhere
    else gets a plain number.
  - Verified live: hovering a single cell reveals just that cell's border;
    focusing a field (e.g. via click) shows its border/focus ring with
    everything else still borderless; row hover shows the row highlight and
    swaps the `#` cell to the grip on all six documents (reorderable or not —
    the other five just never show the grip since `reorderable` is unset);
    dragged a row via the new handle position with real pointer events and
    confirmed the reorder still applies; both light and dark theme
    screenshotted directly, no hardcoded colors bleeding through either way.
  - **Two gaps found in review, both fixed before this shipped:**
    - The `:not(.ant-input-status-error)` exclusion looked untested — cleared
      a required Description on an invoice, saved, and confirmed the
      `ant-input-status-error` class does reach the child even through
      `noStyle` (antd propagates status regardless), and its red border
      renders unaffected by the borderless rule. The comment claiming this
      was accurate, not just asserted.
    - The drag handle's visibility was keyed only to
      `.ant-table-cell-row-hover` (mouseenter-driven), so a keyboard user
      tabbing onto the handle — a real activator, since `useSortable`'s
      `attributes` put `tabIndex`/`role` on it for the `KeyboardSensor` —
      would land on something fully transparent. Added a `:focus-visible`
      rule to `.indexHandle` (and a `:has()` rule to hide `.indexNumber` in
      that state) so keyboard focus reveals the handle the same way mouse
      hover does. Verified via real `Tab` key presses (not a scripted
      `.focus()`, which doesn't reliably set `:focus-visible`) that the
      handle reaches opacity 1 and the number drops to 0 when tabbed onto.

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
  seed data happens to contain a rate _named_ "VAT 20%". A rate named "Standard"
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

| Form           | Fields | Overflow before           | Converted? |
| -------------- | ------ | ------------------------- | ---------- |
| Clients        | 15     | ~2.5 screens              | ✅ yes     |
| Vendors        | 9      | cut off (Address hidden)  | ✅ yes     |
| Products       | 7      | cut off (tax rate hidden) | ✅ yes     |
| Tax rates      | 6      | **0px — already fit**     | ❌ skipped |
| Stock movement | 6      | **0px — already fit**     | ❌ skipped |

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

| Form    | Always expanded  | Collapsed by default |
| ------- | ---------------- | -------------------- |
| Clients | Contact, Address | E-invoicing          |

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
`locale={{ emptyText: t\`No line items\` }}`, `rowKey`on`index`. That block is
duplicated six times.

Locations: `invoices/details.tsx:753`, `orders/details.tsx:308`,
`deliveries/details.tsx:304`, `purchase-orders/details.tsx:351`,
`inbound-deliveries/details.tsx:307`, `incoming-invoices/details.tsx:345`.

### How they diverge (all of this is unintentional)

|                | invoices        | orders    | deliveries | purchase-orders | inbound-deliveries | incoming-invoices |
| -------------- | --------------- | --------- | ---------- | --------------- | ------------------ | ----------------- |
| First column   | **Description** | Product   | Product    | Product         | Product            | Description       |
| Unit column    | ✗               | ✗         | ✓          | ✓               | ✓                  | ✗                 |
| Line total     | ✓               | ✗         | ✗          | ✗               | ✗                  | ✗                 |
| Drag reorder   | ✓               | ✗         | ✗          | ✗               | ✗                  | ✗                 |
| Qty label      | `Qty.`          | `Qty`     | `Qty`      | `Qty`           | `Qty`              | `Qty`             |
| `marginTop: 8` | ✗               | ✓         | ✗          | ✓               | ✓                  | ✓                 |
| Titles use     | `` t`…` ``      | `<Trans>` | `<Trans>`  | `<Trans>`       | `<Trans>`          | `<Trans>`         |

Plus: invoice's delete button is a gray text button positioned _outside_ the
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
_not_ by swapping to a separate read-only view. Reference implementation:
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

### 2.3 Make the table _render_ better (not just consistently) — DONE

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

**What shipped** (see the Status section above for full detail): header
background and row-hover highlight turned out to already be there — antd's
`Table` ships both by default — so no code was needed for that part. The
borderless-until-hover/focus treatment and the `#`/drag-handle merge were
built as described. `size="small"` was **not** tried — `size="middle"` reads
fine once cells are borderless, and swapping it was never required by the
plan, just suggested as worth exploring; left alone to keep this commit
scoped to what the plan actually asked for.

Do this as its own commit _after_ the shell exists, so the visual change is
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

## Tier 3 — Detail-page shells — CORRECTED SCOPE, done

**Both premises behind this tier's original write-up were checked against the
actual code and found stale**, the same way the Tier 2 "Open decision" was —
correcting the record here rather than silently building on a wrong
assumption:

- **"`invoices/details.tsx` wraps its content in `<Card>` sections"** — false.
  Grepped for `Card` in all six detail pages: zero matches, anywhere. The
  white rounded box every page sits in is `<Content>` in `src/layouts/
base.tsx` (`padding: 24`, `borderRadiusLG`), applied identically to every
  route already — there is no per-page `<Card>` pattern to extract from
  invoices, so "wrap the five to match invoices" had nothing to copy.
  Introducing `<Card>` sections now would be a *new* visual pattern with no
  existing reference anywhere in the app, not a consistency fix — out of
  scope for the same reason Tier 2 refused to invent `disabled` gating for
  purchase orders that the backend didn't enforce.
- **"Totals render three different ways: custom Row/Col in invoices,
  `<Descriptions>` in orders/PO/incoming, nothing in deliveries/inbound"** —
  half true. Invoices *also* uses `<Descriptions>` (`details.tsx:875`), not a
  custom Row/Col. Diffing the four documents that have a totals block:
  orders and purchase-orders are **byte-identical** in structure and styling
  (`<Row justify="end"><Col><Descriptions column={1} styles={{content:
  {textAlign:"right", minWidth:100, fontSize:14}, label:{textAlign:"right",
  fontWeight:500, fontSize:14}}}>`, one `Subtotal` item — correctly, since
  neither `orders` nor `outbound_delivery` line items reference `taxRates` at
  all). Incoming-invoices matches that same structure/styling almost exactly
  (`minWidth:120` instead of `100`, plus `Tax`/`Total` items — because it
  genuinely has tax data orders/PO don't). Invoices' version differs
  cosmetically (`fontSize:15`, hardcoded `color:"rgba(0,0,0,0.88)"` — a
  latent dark-mode bug, flagged not fixed, out of scope here — and a bold
  `<strong>` Total) and sits in a shared row with the Customer note field via
  `Col span={12} offset={4}` instead of its own `justify="end"` row.
  **Unifying these four would mean either inventing totals for deliveries/
  inbound that never had one (a real behavior change — deliveries must never
  show prices per the non-goals list) or relocating a field on invoices, the
  busiest screen, to match a `minWidth`/`fontSize` difference nobody would
  ever notice.** Neither is worth doing. Dropped from scope entirely.

**What was actually true and worth fixing**: all **six** documents —
including invoices, which the original write-up incorrectly treated as the
reference — use bare `<Row gutter={16|24}><Col span={N}>` for their header
field rows, with hardcoded spans that never break to fewer columns on a
narrow viewport (unlike the Tier 1 master-data drawers, which already use
`xs={24} md={12}`). That's the one genuine, fixable inconsistency here, so
Tier 3 became: **convert every header-field `Row`/`Col` group in all six
detail pages to responsive breakpoints** (`xs={24}` stacks to one column,
`md={12}` gives two-up on tablet widths, the original `span` value is
restored at `xl` so desktop is pixel-identical to today), leaving line-item
tables, totals blocks, and footer bars untouched. Verified at 1440×900
(desktop, unchanged) **and** at ~900×800 and ~768×900 with the sider both
expanded and collapsed — 1440×900 alone would have exercised only the `xl`
breakpoint and shown nothing different from today.

One trap specific to this page family: `invoices/details.tsx` has two
`offset`-based columns (`Col span={4} offset={12}`, `Col span={12}
offset={4}`). A bare `offset` prop applies at *every* breakpoint, so writing
`xl={4}` while leaving `offset={12}` unscoped would push that column halfway
across the screen once the layout stacks to `xs`. Both were converted to the
object form (`xl={{ span: 4, offset: 12 }}`, no unscoped `offset`) instead.

The Organizations-list item mentioned in the original write-up (name-as-link,
`⋮` overflow menu) is **list-page** behavior, not detail-page shell layout —
out of scope for this tier, and left alone rather than folded in as
unrequested extra churn.

---

## Tier 4 — Scale & reporting (separate initiative, not scoped in detail yet)

Two items raised after Tier 0 shipped. Recorded here for planning; neither has
an implementation plan yet, and **both break this doc's own non-goal of
"frontend-only, no Go/API/DB changes"** — they're a distinct initiative from
the Tier 0-3 UI-consistency pass, not a continuation of it.

### 4.1 Inventory / product-list UI at scale

Confirmed today: `GetProducts`/`GetStockMovements` (`src/atoms/product.ts`,
`src/atoms/stock.ts`) fetch the _entire_ table in one request with no
`limit`/`offset`/`search` params anywhere in `api/products.go`; `/inventory`
and `/products` paginate client-side only (`pagination={{ pageSize: 50 }}` at
`src/routes/inventory.tsx:114`, `defaultPageSize: 25` in `products.tsx`). Fine
at tens/hundreds of rows; at thousands this means a multi-MB payload on every
load, full in-memory filter/sort, and no server-side search. Needs: paginated

- filtered Go endpoints, matching server-side `Table` pagination/sorting and
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
  remove and Add buttons hidden — this is a behaviour _fix_, not a preservation:
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

## Open decision — resolved by the status quo, no design call needed

This section originally asked whether the invoice line-item table should move
Description out of the first column, on the premise that "every other document
puts Product first; invoices puts Description first." That premise was wrong:
reading `invoices/details.tsx` at migration time (Tier 2, 6/6) showed Product
was already the first column, same as the other five documents. There was no
5-of-6-vs-6-of-6 tradeoff to make and nothing was changed — leaving this note
here instead of deleting it, since it's a good example of why a plan's stated
assumptions need re-verifying against the actual code right before acting on
them, not trusted at face value once Tier 2 migrations actually got underway.

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
