import { t } from "@lingui/core/macro";

export type ProductionOrderStatus = "draft" | "completed" | "cancelled";

export const PRODUCTION_ORDER_STATUSES: ProductionOrderStatus[] = [
  "draft",
  "completed",
  "cancelled",
];

// Ant Design Tag colors per status; draft is intentionally uncolored (default).
export const productionOrderStatusColor: Record<ProductionOrderStatus, string | undefined> = {
  draft: undefined,
  completed: "green",
  cancelled: "volcano",
};

// Called during render (never hoisted to module scope) so the label follows the
// active locale rather than freezing at import time.
export function productionOrderStatusLabel(status: string): string {
  switch (status) {
    case "draft":
      return t`Draft`;
    case "completed":
      return t`Completed`;
    case "cancelled":
      return t`Cancelled`;
    default:
      return status;
  }
}

export interface ProductionOrderTransition {
  next: ProductionOrderStatus;
  label: string;
  type?: "primary" | "default" | "dashed";
}

// Must stay in sync with productionOrderStatusTransitions in
// db/production_order.go. Completing consumes components and produces
// finished units; cancelling a completed order reverses both, and the
// server rejects that if the produced units have already been used/shipped.
export function productionOrderTransitions(status: string): ProductionOrderTransition[] {
  switch (status) {
    case "draft":
      return [{ next: "completed", label: t`Mark as completed`, type: "primary" }];
    default:
      return [];
  }
}

// The full transition matrix for src/components/status-flow.tsx. Mirrors
// productionOrderStatusTransitions in db/production_order.go exactly — a
// status absent as a key is terminal. "cancelled" has no fallback, matching
// inbound/outbound delivery's reasoning (reversing a cancel would mean
// re-deriving stock/GL/serial state, not just flipping a status column).
export const productionOrderStatusTransitionMatrix: Partial<
  Record<ProductionOrderStatus, ProductionOrderStatus[]>
> = {
  draft: ["completed", "cancelled"],
  completed: ["cancelled"],
};
