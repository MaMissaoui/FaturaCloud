# Dual document layouts: Default + Tunisia, all 6 document types

Date: 2026-09-23
Status: approved for planning

## Problem

Commit `2f7e95a` ("make the Tunisia 'Facture' layout the default for all
documents", 2026-09-22) replaced every one of the 6 embedded default export
templates (invoice, purchase order, order, incoming invoice, outbound
delivery, inbound delivery) with the Tunisian "Facture"-style layout. That
made the Tunisian layout the *only* layout — every organization, regardless
of country, now gets Tunisia-style PDFs by default.

The request: restore a generic/default layout (matching the reference
"INVOICE" screenshot: bold seller block top-left, document title top-right,
"Bill To" block, `Product/Description/Quantity/Unit Price/Tax Rate/Line
Total` table, `Subtotal/Tax/Total` footer) as one selectable option, and
keep the Tunisian "Facture" layout (matching the reference phone-photo
screenshot: boxed buyer/vendor block, boxed title, `Code/Désignation/
Quantité/P.U HT/TOTAL HT` table, VAT recap box, `Total Brut HTVA/Remise/
Total Net HTVA/Total TVA/D.Timbre/Total TTC` footer) as the other, for all
6 document types, with a working per-organization switch between them.

## Key finding

Image 1 (the "default" reference) is not a new design — it is what the
invoice template looked like immediately before `2f7e95a`. All 6 document
types had this same generic structure pre-commit, differing only in title
text and the labeled party block:

| Document type | Title (pre-commit) | Party block label |
|---|---|---|
| invoice | INVOICE | Bill To |
| purchase_order | PURCHASE ORDER | Vendor |
| order | ORDER CONFIRMATION | Bill To |
| delivery | DELIVERY NOTE | Deliver To |
| inbound_delivery | GOODS RECEIPT | Received By |
| incoming_invoice | INCOMING INVOICE | Billed To |

This means the "default" layout side of this work is a restoration from
git history (`2f7e95a~1`), not new visual design. The Tunisian layout
already matches image 2 (confirmed via `CLAUDE.md`'s element-by-element
description of `tunisia_layout.go`'s output) and needs no changes.

Separately, `organizations.invoiceLayout` (migration `0062`) already
exists as a per-org layout selector, but is dead: it was built for a
client-side `@react-pdf/renderer` path that the 2026-09-08 "PDF
unification" made inert. The current export path is 100% server-side
(xlsx template → LibreOffice → PDF) and never reads this column. This
column also only ever covered invoices, not the other 5 document types.

## Approach

Restore the 6 pre-commit generator files as a second, selectable embedded
layout ("default"), keep the current Tunisia generator code as the other
("tunisia"), and revive+generalize `invoiceLayout` into a live, all-6-types
`documentLayout` selector.

### 1. Two embedded templates per document type (12 total)

`db/templates/gen/*.go` currently has one builder per document type
(`build<Type>Template`), writing to `<type>_default.xlsx` — today that
output is actually the Tunisia-style content. Split into two builders per
type:

- `build<Type>DefaultTemplate()` — the pre-commit generic layout, restored
  from `2f7e95a~1:db/templates/gen/<type>.go` and adapted to compile
  against the current `styles.go` (verified: `bold`, `title`, `header`,
  `right` all still exist with those names; `addAvailableFieldsSheet` and
  `finalizeWorkbook` are unchanged). Writes `<type>_default.xlsx`. Sheet
  names match the pre-commit originals (e.g. `"Invoice"`, `"Purchase
  Order"`, `"Delivery Note"`, `"Goods Receipt"`, `"Incoming Invoice"`,
  `"Order"`).
- `build<Type>TunisiaTemplate()` — today's code, functionally unchanged,
  output renamed to `<type>_tunisia.xlsx`.

`db/templates/gen/main.go` calls all 12 builders.

`db/templates_embed.go` gets 12 `//go:embed` vars and two lookup maps:

```go
var embeddedDefaultTemplates = map[string][]byte{ "invoice": invoiceDefaultTemplate, ... }
var embeddedTunisiaTemplates = map[string][]byte{ "invoice": invoiceTunisiaTemplate, ... }
```

`IsKnownDocumentType` stays layout-independent (checks document type
membership regardless of layout — it's the API allowlist for arbitrary
document-type path segments).

No fill-engine changes needed: `fillTemplate` (`db/xlsx_export.go`) already
tolerates a template with no `{{organization.logo}}` cell (the logo insert
is skipped when no cell matches the marker) and no `{{#taxLines}}` block
(that repeat block is `required: false`; a missing marker is silently
skipped). The restored generic templates have neither.

### 2. Revive the selector, generalized to all 6 types

Migration `0089_rename_invoice_layout_to_document_layout`:

```sql
ALTER TABLE organizations RENAME COLUMN invoiceLayout TO documentLayout;
UPDATE organizations SET documentLayout = NULL WHERE documentLayout = 'custom';
```

(Precedent for `RENAME COLUMN` on SQLite in this repo: migration `0008`.)
The `'custom'` normalization is needed because the old invoice-only
dropdown offered a `custom` option that never did anything real (see
below) — any organization that happens to have it stored should read as
"default" going forward, not as an unrecognized value.

`db/organization.go`: rename the `InvoiceLayout` field to `DocumentLayout`
in both the row struct and the update-request struct; update the
`COALESCE(?, invoiceLayout)` update-column line to
`documentLayout = COALESCE(?, documentLayout)`; update the SELECT column
list.

`resolveTemplateBytes` and `ExportDocumentTemplateBytes`
(`db/document_template.go`) both gain a `layout string` parameter and pick
from `embeddedDefaultTemplates` or `embeddedTunisiaTemplates` accordingly
(anything other than exactly `"tunisia"` — including `""`, `nil`-derived
empty string, or an unrecognized future value — resolves to `"default"`;
a layout choice must never hard-fail export, mirroring the documented
intent of the original `invoicePDFLayouts` registry). An org's uploaded
override (`document_templates` table) still wins over either embedded
layout, unchanged — layout selection only decides which *embedded*
template is used when there is no override.

The 6 `Fetch*ExportData` functions (`db/xlsx_export.go` and the 5
per-type files) each already fetch `org` before calling
`resolveTemplateBytes`; pass `org.DocumentLayout` straight through — no
extra DB query.

`api/document_templates.go`'s 6 export handlers need no change beyond
whatever the `Fetch*ExportData` signature changes require, since they
already destructure and forward whatever those functions return.

### 3. Fix the reset-to-Default bug this surfaces

Today `invoiceLayout` is inert, so this bug is invisible. Once the
selector actually drives export output, it becomes user-facing the moment
someone tries to switch an org back to Default:

- `src/components/organizations/organization-edit-drawer.tsx`: the
  `invoiceLayout` `Select` has `allowClear` and a `"Default"` placeholder.
  Clearing the field submits `null`.
- `db/organization.go`'s update SQL does
  `invoiceLayout = COALESCE(?, invoiceLayout)` — passing `null` keeps the
  *existing* stored value, not `NULL`/`"default"`.

Net effect: an org already set to `"tunisia"` can never be cleared back to
Default via this control.

Fix, as part of this same change (not deferred): remove `allowClear` from
the field (rename to `documentLayout`, label "Document layout"); drop the
`"custom"` option from `invoicePDFLayoutOptions()` (renaming/repurposing
that function) since it's redundant with the real, auto-detected
`document_templates` override — leaving a selectable-but-meaningless
option after reviving this setting would be worse than removing it now.
Two real options remain: `"default"` and `"tunisia"`. On load, coerce
`null`/`""`/unrecognized to `"default"` so the field always shows an
explicit, submittable value.

`src/types/models.ts`, `src/atoms/*` (if referenced), and any other
`invoiceLayout` reference get renamed to `documentLayout` for consistency
(see the grep list in the Files section below).

### 4. Testing

- Keep the existing 6 `TestEmbeddedDefault<Type>TemplatePlaceholdersAllResolve`
  tests pointed at `<type>DefaultTemplate` — same semantics as today
  (guard the generic/default template), just now actually generic content
  again instead of Tunisia content.
- Add 6 new `TestEmbeddedTunisia<Type>TemplatePlaceholdersAllResolve` tests
  calling the existing shared `assertEmbeddedTemplatePlaceholdersResolve`
  helper (`db/xlsx_export_template_guard_test.go`, unchanged) against
  `<type>TunisiaTemplate`.
- `TestTunisiaLayoutFillsEndToEnd` (`db/xlsx_export_tunisia_test.go`)
  needs no change — it already exercises the Tunisia code path, which is
  functionally unchanged.
- Add a `resolveTemplateBytes`/`ExportDocumentTemplateBytes` unit test
  covering: no override + `documentLayout=""/"default"/unrecognized` →
  default bytes; no override + `documentLayout="tunisia"` → tunisia bytes;
  override present → override bytes regardless of layout.
- `go test ./...` and `pnpm lint` / `pnpm type-check` must pass.
- Manual visual verification (placeholder-resolution tests don't catch a
  wrong layout): run `go run ./db/templates/gen`, move the 12 outputs over
  `db/templates/`, fill one invoice through each layout, convert with
  `soffice`/LibreOffice, and compare side-by-side against the two
  reference screenshots. Do the same spot-check for at least one non-invoice
  type per layout (e.g. purchase order) since the shared frame/generic
  builder differs slightly per type (title, party label, columns).

## Out of scope

- Per-organization "custom uploaded template" override mechanism — already
  independent of layout choice, unaffected.
- The `orientation` setting — already a separate per-org-per-document-type
  override, independent of layout (verified: the generic pre-commit
  builders never touched `SetPageLayout`; a document's `applyFitToPageWidth`
  handling already deals with a wide table on a portrait page).
- The Tunisian layout's visual content — unchanged, already matches the
  reference screenshot.
- Per-document-type layout selection (e.g. Tunisia for invoices, Default
  for purchase orders) — explicitly decided against; one org-wide toggle
  drives all 6 types.
- A known, pre-existing cosmetic gap in the restored default layout: the
  Description column truncates long text (visible in the reference
  screenshot as "…6kg (Premiun"). This is faithful to the pre-commit
  template being restored verbatim; not fixed here.

## Files touched (expected)

Backend:
- `db/templates/gen/main.go` — call 12 builders
- `db/templates/gen/{invoice,purchase_order,order,delivery,inbound_delivery,incoming_invoice}.go` — split into default (restored) + tunisia (renamed) builders
- `db/templates_embed.go` — 12 embeds, two maps
- `db/document_template.go` — `resolveTemplateBytes`, `ExportDocumentTemplateBytes` gain `layout` param
- `db/xlsx_export.go` + `db/xlsx_export_{order,purchase_order,delivery,inbound_delivery,incoming_invoice}.go` — thread `org.DocumentLayout` into `resolveTemplateBytes`/`ExportDocumentTemplateBytes` calls
- `db/organization.go` — `InvoiceLayout` → `DocumentLayout` field rename, SQL update
- `db/migrations/0089_rename_invoice_layout_to_document_layout.{up,down}.sql`
- New/updated test files per section 4 above

Frontend:
- `src/components/organizations/organization-edit-drawer.tsx` — field rename, drop `allowClear`, drop `"custom"` option, default coercion
- `src/types/models.ts` — `invoiceLayout` → `documentLayout`
- Any other `invoiceLayout` reference (`src/routes/invoices/details.tsx`'s now-stale comment, `src/routes/invoices/details.test.tsx` fixture, `src/components/invoices/layouts.ts`'s comment) — update references/comments; the dead `pdf.tsx`/`pdf-tunisia.tsx`/`layouts.ts` React-PDF components themselves are explicitly out of scope (separate, values-laden deletion decision per existing code comment)

Docs:
- Root `CLAUDE.md` — update the "embedded defaults... are the Tunisian layout" note to describe the dual-layout selector instead
