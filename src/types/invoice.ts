import { t } from "@lingui/core/macro";

// Canonical invoice states — the single source of truth shared by the list
// page, the state-select dropdown, and the details page. Must match the
// server-side set in db/invoice.go.
export type InvoiceState = "draft" | "sent" | "paid" | "cancelled";

export const INVOICE_STATES: InvoiceState[] = ["draft", "sent", "paid", "cancelled"];

// Ant Design Tag colors per state; draft is intentionally uncolored (default).
export const invoiceStateColor: Record<InvoiceState, string | undefined> = {
  draft: undefined,
  sent: "geekblue",
  paid: "green",
  cancelled: "volcano",
};

// invoiceStateLabel must be called during render (not hoisted to module scope)
// so the returned label follows the currently-active locale — a module-scope
// `t` result would freeze at import-time locale and go stale on language switch.
export function invoiceStateLabel(state: string): string {
  switch (state) {
    case "draft":
      return t`Draft`;
    case "sent":
      return t`Sent`;
    case "paid":
      return t`Paid`;
    case "cancelled":
      return t`Cancelled`;
    default:
      return state;
  }
}
