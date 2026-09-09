import { t } from "@lingui/core/macro";

export type InboundDeliveryStatus = "draft" | "received" | "cancelled";

export const INBOUND_DELIVERY_STATUSES: InboundDeliveryStatus[] = [
  "draft",
  "received",
  "cancelled",
];

// Ant Design Tag colors per status; draft is intentionally uncolored (default).
export const inboundDeliveryStatusColor: Record<InboundDeliveryStatus, string | undefined> = {
  draft: undefined,
  received: "green",
  cancelled: "volcano",
};

// Called during render (never hoisted to module scope) so the label follows the
// active locale rather than freezing at import time.
export function inboundDeliveryStatusLabel(status: string): string {
  switch (status) {
    case "draft":
      return t`Draft`;
    case "received":
      return t`Received`;
    case "cancelled":
      return t`Cancelled`;
    default:
      return status;
  }
}

export interface InboundDeliveryTransition {
  next: InboundDeliveryStatus;
  label: string;
  type?: "primary" | "default" | "dashed";
}

// Must stay in sync with inboundDeliveryStatusTransitions in
// db/inbound_delivery.go, which enforces the same matrix server-side.
// Receiving posts "in" stock movements; cancelling a received receipt reverses
// them, and the server rejects that if the goods have already been consumed.
export function inboundDeliveryTransitions(status: string): InboundDeliveryTransition[] {
  switch (status) {
    case "draft":
      return [{ next: "received", label: t`Mark as received`, type: "primary" }];
    default:
      return [];
  }
}

// The full transition matrix for src/components/status-flow.tsx. Mirrors
// inboundDeliveryStatusTransitions in db/inbound_delivery.go exactly — a
// status absent as a key is terminal. Unlike purchase orders/orders,
// "cancelled" has deliberately NOT been given a fallback here: receiving
// posts real stock movements and a GRNI GL accrual (db/gl_posting.go), and
// reversing a cancellation would mean re-deriving those — plus re-checking
// nothing was billed against the receipt in the meantime — not just
// flipping a status column. A materially bigger, separate feature.
export const inboundDeliveryStatusTransitionMatrix: Partial<
  Record<InboundDeliveryStatus, InboundDeliveryStatus[]>
> = {
  draft: ["received", "cancelled"],
  received: ["cancelled"],
};
