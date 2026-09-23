import InvoicePDF from "src/components/invoices/pdf";
import InvoicePDFTunisia from "src/components/invoices/pdf-tunisia";

// The legacy client-side invoice PDF layout registry (React-PDF components).
// Orphaned: every exported document now renders server-side from an Excel
// template, and the per-organization layout choice lives on as
// organizations.documentLayout (src/types/document-layout.ts), which picks
// between the server's embedded "default" and "tunisia" template sets for all
// six document types. Nothing imports this module any more; it is kept, along
// with pdf.tsx/pdf-tunisia.tsx, rather than deleted, which is a separate
// values-laden decision (losing the branded React-PDF look and SEPA QR code).
//
// null/"" and any unrecognized key fall back to "default" — a layout choice
// must never hard-fail rendering.
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
