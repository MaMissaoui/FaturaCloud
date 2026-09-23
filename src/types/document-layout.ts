import { t } from "@lingui/core/macro";

// organizations.documentLayout: which embedded Excel/PDF template set every
// document type's export uses when the organization hasn't uploaded its own
// override in Settings ▸ Document templates. The server's source of truth is
// db/templates_embed.go (DocumentLayoutDefault/DocumentLayoutTunisia). There
// is deliberately no "custom" choice — an uploaded override is detected on its
// own and wins over either layout.
export type DocumentLayout = "default" | "tunisia";

// Mirrors the server's normalizeDocumentLayout: anything but "tunisia" —
// null, "", or an old "custom" value — is the default layout, so the
// Organizations drawer's Select always shows an explicit, submittable value
// rather than an empty placeholder (a cleared Select would submit nothing and
// the server's COALESCE update would keep the old layout).
export const normalizeDocumentLayout = (layout?: string | null): DocumentLayout =>
  layout === "tunisia" ? "tunisia" : "default";

// Labels are a function (not a module-scope const) so they re-evaluate
// against the active locale — same reasoning as invoiceStateLabel in
// src/types/invoice.ts. Kept out of src/components/invoices/layouts.ts, whose
// static React-PDF imports would drag @react-pdf/renderer into the
// /organizations route chunk (audit F131).
export const documentLayoutOptions = (): { value: DocumentLayout; label: string }[] => [
  { value: "default", label: t`Default` },
  { value: "tunisia", label: t`Tunisia` },
];
