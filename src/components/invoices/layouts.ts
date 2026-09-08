import { t } from "@lingui/core/macro";

import InvoicePDF from "src/components/invoices/pdf";
import InvoicePDFTunisia from "src/components/invoices/pdf-tunisia";

// The invoice PDF layout registry. organizations.invoiceLayout (nullable)
// stores a key here; null/"" and any key this registry doesn't recognize
// (e.g. an older frontend build looking up a layout added since) both fall
// back to "default" — a layout choice must never hard-fail invoice
// rendering the way a missing GL account default is allowed to. Adding a
// new layout means adding both a component here and an entry in
// invoicePDFLayoutOptions below; nothing else in the app needs to know the
// registry exists.
export const invoicePDFLayouts = {
  default: InvoicePDF,
  tunisia: InvoicePDFTunisia,
} as const;

export type InvoicePDFLayoutKey = keyof typeof invoicePDFLayouts;

export const getInvoicePDFLayout = (layoutId?: string | null) =>
  invoicePDFLayouts[(layoutId as InvoicePDFLayoutKey) || "default"] ?? invoicePDFLayouts.default;

// Labels are a function (not a module-scope const) so they re-evaluate
// against the active locale — same reasoning as invoiceStateLabel in
// src/types/invoice.ts.
export const invoicePDFLayoutOptions = (): { value: InvoicePDFLayoutKey; label: string }[] => [
  { value: "default", label: t`Default` },
  { value: "tunisia", label: t`Tunisia` },
];
