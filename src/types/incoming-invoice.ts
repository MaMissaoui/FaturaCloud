import { i18n } from "@lingui/core";
import { t } from "@lingui/core/macro";

export type IncomingInvoiceState = "draft" | "approved" | "paid" | "cancelled";

export const INCOMING_INVOICE_STATES: IncomingInvoiceState[] = [
  "draft",
  "approved",
  "paid",
  "cancelled",
];

// Ant Design Tag colors per state; draft is intentionally uncolored (default).
export const incomingInvoiceStateColor: Record<IncomingInvoiceState, string | undefined> = {
  draft: undefined,
  approved: "geekblue",
  paid: "green",
  cancelled: "volcano",
};

// Called during render so the label follows the active locale. Like sales
// invoices there is no transition matrix — states move freely, since a bounced
// payment can legitimately send paid back to approved.
export function incomingInvoiceStateLabel(state: string): string {
  switch (state) {
    case "draft":
      return t`Draft`;
    case "approved":
      return t`Approved`;
    case "paid":
      return t`Paid`;
    case "cancelled":
      return t`Cancelled`;
    default:
      return state;
  }
}

// 3-way match outcomes, mirroring the constants in db/incoming_invoice_match.go.
export type MatchStatus =
  | "matched"
  | "unlinked"
  | "quantity_variance"
  | "over_received"
  | "price_variance";

export const matchStatusColor: Record<string, string | undefined> = {
  matched: "success",
  unlinked: undefined,
  quantity_variance: "warning",
  over_received: "error",
  price_variance: "warning",
};

export function matchStatusLabel(status: string): string {
  switch (status) {
    case "matched":
      return t`Matched`;
    case "unlinked":
      return t`Not linked`;
    case "quantity_variance":
      return t`Over ordered`;
    case "over_received":
      return t`Over received`;
    case "price_variance":
      return t`Price variance`;
    default:
      return status;
  }
}

// The server also returns a `message`, but it is English — like every other
// server-side validation string in this app. The match panel renders it inline
// rather than in a toast, so build the explanation here from the structured
// numbers instead, which keeps it in the user's language.
export function matchStatusDetail(line: MatchLine): string {
  // Quantities are formatted with the active UI locale (comma vs. period)
  // rather than a hardcoded "." via toFixed. This module has no access to the
  // selected organization's country, so it falls back to the viewer's own
  // locale — the same fallback src/utils/currencies.tsx uses for a country
  // outside its curated table. Whole values stay clean; fractions cap at 2
  // decimals, matching the old toFixed(2).
  const qty = (n: number | null) =>
    new Intl.NumberFormat(i18n.locale, {
      minimumFractionDigits: 0,
      maximumFractionDigits: 2,
    }).format(n ?? 0);
  switch (line.status) {
    case "over_received":
      return t`Billing ${qty(line.invoicedQuantity)} but only ${qty(line.receivedQuantity)} received (${qty(line.previouslyInvoicedQuantity)} already invoiced).`;
    case "quantity_variance":
      return t`Billing ${qty(line.invoicedQuantity)} but only ${qty(line.orderedQuantity)} ordered.`;
    case "price_variance":
      return t`Billed at a different unit price than ordered.`;
    case "unlinked":
      return t`Not linked to a purchase order line.`;
    default:
      return "";
  }
}

export interface MatchLine {
  lineItemId: string;
  purchaseOrderLineItemId: string | null;
  description: string;
  orderedQuantity: number | null;
  receivedQuantity: number | null;
  previouslyInvoicedQuantity: number;
  invoicedQuantity: number;
  orderedUnitPrice: number | null;
  invoicedUnitPrice: number;
  status: string;
  message: string;
}

// `unlinked` is informational — a free-text line simply has nothing to match
// against, and the server never blocks approval on it.
export function hasBlockingVariance(lines: MatchLine[]): boolean {
  return lines.some((line) => line.status !== "matched" && line.status !== "unlinked");
}
