import { t } from "@lingui/core/macro";

// Canonical order statuses — must match validOrderStatuses in db/order.go.
export type OrderStatus = "draft" | "confirmed" | "shipped" | "delivered" | "cancelled";

export const ORDER_STATUSES: OrderStatus[] = [
  "draft",
  "confirmed",
  "shipped",
  "delivered",
  "cancelled",
];

// Ant Design Tag colors per status; draft is intentionally uncolored (default).
export const orderStatusColor: Record<OrderStatus, string | undefined> = {
  draft: undefined,
  confirmed: "blue",
  shipped: "orange",
  delivered: "green",
  cancelled: "red",
};

// Called during render (never hoisted to module scope) so the label follows
// the active locale rather than freezing at import time — see
// src/types/invoice.ts's invoiceStateLabel for the same reasoning.
export function orderStatusLabel(status: string): string {
  switch (status) {
    case "draft":
      return t`Draft`;
    case "confirmed":
      return t`Confirmed`;
    case "shipped":
      return t`Shipped`;
    case "delivered":
      return t`Delivered`;
    case "cancelled":
      return t`Cancelled`;
    default:
      return status;
  }
}

export interface OrderTransition {
  next: OrderStatus;
  label: string;
  type?: "primary" | "default" | "dashed";
}

// Must stay in sync with orderStatusTransitions in db/order.go, which
// enforces the same matrix server-side ("delivered" and "cancelled" are
// terminal). A function rather than a constant for the same locale reason as
// the label helper above.
export function orderTransitions(status: string): OrderTransition[] {
  switch (status) {
    case "draft":
      return [{ next: "confirmed", label: t`Confirm order`, type: "primary" }];
    case "confirmed":
      return [{ next: "shipped", label: t`Mark as shipped`, type: "primary" }];
    case "shipped":
      return [{ next: "delivered", label: t`Mark as delivered`, type: "primary" }];
    default:
      return [];
  }
}
