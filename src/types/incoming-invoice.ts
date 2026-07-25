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
