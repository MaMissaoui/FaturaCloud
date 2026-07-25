import { t } from "@lingui/core/macro";

export type PurchaseOrderStatus = "draft" | "confirmed" | "received" | "cancelled";

export const PURCHASE_ORDER_STATUSES: PurchaseOrderStatus[] = [
  "draft",
  "confirmed",
  "received",
  "cancelled",
];

// Ant Design Tag colors per status; draft is intentionally uncolored (default).
export const purchaseOrderStatusColor: Record<PurchaseOrderStatus, string | undefined> = {
  draft: undefined,
  confirmed: "geekblue",
  received: "green",
  cancelled: "volcano",
};

// purchaseOrderStatusLabel must be called during render (not hoisted to module
// scope) so the returned label follows the currently-active locale — a
// module-scope `t` result would freeze at import-time locale and go stale on
// language switch.
export function purchaseOrderStatusLabel(status: string): string {
  switch (status) {
    case "draft":
      return t`Draft`;
    case "confirmed":
      return t`Confirmed`;
    case "received":
      return t`Received`;
    case "cancelled":
      return t`Cancelled`;
    default:
      return status;
  }
}

export interface PurchaseOrderTransition {
  next: PurchaseOrderStatus;
  label: string;
  type?: "primary" | "default" | "dashed";
}

// PURCHASE_ORDER_STATUS_TRANSITIONS must stay in sync with
// purchaseOrderStatusTransitions in db/purchase_order.go, which enforces the
// same matrix server-side ("received" and "cancelled" are terminal). Keeping it
// next to the statuses makes that pairing visible in one place.
//
// A function rather than a constant for the same locale reason as the label
// helper above.
export function purchaseOrderTransitions(status: string): PurchaseOrderTransition[] {
  switch (status) {
    case "draft":
      return [{ next: "confirmed", label: t`Confirm order`, type: "primary" }];
    case "confirmed":
      return [{ next: "received", label: t`Mark as received`, type: "primary" }];
    default:
      return [];
  }
}
