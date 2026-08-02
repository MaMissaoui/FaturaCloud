import { t } from "@lingui/core/macro";

// Canonical outbound delivery statuses — must match the CHECK constraint on
// outbound_deliveries.status (db/migrations/0025_add_outbound_deliveries.up.sql).
export type DeliveryStatus = "draft" | "shipped" | "delivered" | "cancelled";

export const DELIVERY_STATUSES: DeliveryStatus[] = [
  "draft",
  "shipped",
  "delivered",
  "cancelled",
];

// Ant Design Tag colors per status; draft is intentionally uncolored (default).
export const deliveryStatusColor: Record<DeliveryStatus, string | undefined> = {
  draft: undefined,
  shipped: "orange",
  delivered: "green",
  cancelled: "red",
};

// Called during render (never hoisted to module scope) so the label follows
// the active locale rather than freezing at import time — see
// src/types/invoice.ts's invoiceStateLabel for the same reasoning.
export function deliveryStatusLabel(status: string): string {
  switch (status) {
    case "draft":
      return t`Draft`;
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

export interface DeliveryTransition {
  next: DeliveryStatus;
  label: string;
  type?: "primary" | "default" | "dashed";
}

// Must stay in sync with deliveryStatusTransitions in db/delivery.go, which
// enforces the same matrix server-side ("delivered" and "cancelled" are
// terminal). A function rather than a constant for the same locale reason as
// the label helper above.
export function deliveryTransitions(status: string): DeliveryTransition[] {
  switch (status) {
    case "draft":
      return [{ next: "shipped", label: t`Mark as shipped`, type: "primary" }];
    case "shipped":
      return [{ next: "delivered", label: t`Mark as delivered`, type: "primary" }];
    default:
      return [];
  }
}
