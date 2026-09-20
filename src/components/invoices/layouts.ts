import InvoicePDF from "src/components/invoices/pdf";
import InvoicePDFTunisia from "src/components/invoices/pdf-tunisia";

// The invoice PDF layout registry. organizations.invoiceLayout (nullable)
// stores a key here; null/"" and any key this registry doesn't recognize
// (e.g. an older frontend build looking up a layout added since) both fall
// back to "default" — a layout choice must never hard-fail invoice
// rendering the way a missing GL account default is allowed to. Adding a
// new layout means adding a component here; nothing else in the app needs
// to know the registry exists.
//
// The user-facing option list for the Organizations drawer's "Invoice PDF
// layout" select deliberately does NOT live here any more (audit F131):
// building it in this module forced the /organizations route to pull in this
// file's static imports of pdf.tsx / pdf-tunisia.tsx — the entire
// @react-pdf/renderer engine — just to render three labels. It now lives in
// src/components/organizations/organization-edit-drawer.tsx, the only
// remaining consumer. This module is otherwise orphaned since the 2026-09-08
// PDF unification made invoiceLayout inert for invoice output (see the root
// CLAUDE.md note); it is kept, along with pdf.tsx/pdf-tunisia.tsx, rather than
// deleted, which is a separate values-laden decision.
export const invoicePDFLayouts = {
  default: InvoicePDF,
  tunisia: InvoicePDFTunisia,
} as const;

export type InvoicePDFLayoutKey = keyof typeof invoicePDFLayouts;

export const getInvoicePDFLayout = (layoutId?: string | null) =>
  invoicePDFLayouts[(layoutId as InvoicePDFLayoutKey) || "default"] ?? invoicePDFLayouts.default;

// "custom" (issue #115) means "export through the org's uploaded Excel
// template via the server", not a React component — it's deliberately never
// added to invoicePDFLayouts above. Callers must check this BEFORE calling
// getInvoicePDFLayout, which would otherwise silently fall back to the
// "default" component for an unrecognized key and render the wrong document
// next to a print button that downloads something else entirely.
export const isCustomTemplateLayout = (layoutId?: string | null) => layoutId === "custom";
