import { t } from "@lingui/core/macro";

// Invoice as returned by the API (storage shape): monetary values in integer
// cents, dates as Unix-millisecond numbers. The atoms convert these to display
// units / Dayjs; see InvoiceDisplay for the detail-page shape.
export interface Invoice {
  id: string;
  organizationId: string;
  number: string;
  state: string;
  clientId: string;
  date: number;
  dueDate: number | null;
  currency: string;
  customerNotes: string | null;
  overdueCharge: number | null;
  total: number;
  taxTotal: number;
  subTotal: number;
  createdAt: string | null;
  clientName: string | null;
}

export interface InvoiceLineItem {
  id: string;
  invoiceId: string;
  description: string | null;
  quantity: number;
  unitPrice: number;
  taxRate: string | null;
  position: number;
  createdAt: string | null;
}

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
