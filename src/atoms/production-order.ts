import { atom } from "jotai";
import type { ProductionOrder, ProductionOrderComponentLine } from "src/types/models";
import { message } from "src/utils/message";
import { nanoid } from "nanoid";
import { t } from "@lingui/core/macro";
import isEqual from "lodash/isEqual";
import orderBy from "lodash/orderBy";
import keyBy from "lodash/keyBy";
import map from "lodash/map";
import reject from "lodash/reject";

import {
  GetProductionOrders,
  GetNextProductionOrderNumber,
  GetProductionOrder,
  GetProductionOrderComponentLines,
  CreateProductionOrder,
  UpdateProductionOrderStatus,
  DeleteProductionOrder,
} from "src/api";
import { organizationIdAtom } from "./organization";

export const productionOrdersAtom = atom<ProductionOrder[]>([]);
productionOrdersAtom.debugLabel = "productionOrdersAtom";

export const setProductionOrdersAtom = atom(null, async (get, set) => {
  const organizationId = get(organizationIdAtom);
  try {
    const response = await GetProductionOrders(organizationId!);
    set(productionOrdersAtom, response);
  } catch (error) {
    console.error("Failed to fetch production orders:", error);
    message.error(t`Failed to fetch production orders`);
    set(productionOrdersAtom, []);
  }
});

// Server-side MAX-based numbering, like inbound/outbound deliveries.
export const nextProductionOrderNumberAtom = atom(async (get) => {
  const organizationId = get(organizationIdAtom);
  if (!organizationId) return "PRO-0001";
  try {
    return await GetNextProductionOrderNumber(organizationId);
  } catch {
    return "PRO-0001";
  }
});

export const productionOrderIdAtom = atom<string | null>(null);

// The detail page's view model: the order plus its snapshotted component
// lines. Exported and named rather than left as an inline object literal so
// the page doesn't have to fall back to `any` to read it (audit 2026-09-14
// F90) — issue #143's typed-atom pass is the standing convention here.
export type ProductionOrderWithLines = ProductionOrder & {
  componentLines: ProductionOrderComponentLine[];
};

// Read-only — there is no PUT /production-orders/{id} (see
// db/production_order.go's comment: every document owns its line items,
// snapshotted from the BOM at creation, so there is nothing left to edit on
// a draft order besides its status). Creation goes through
// createProductionOrderAtom below instead.
export const productionOrderAtom = atom<Promise<ProductionOrderWithLines | null>>(async (get) => {
  const orderId = get(productionOrderIdAtom);
  if (!orderId) return null;
  try {
    const [order, componentLines] = await Promise.all([
      GetProductionOrder(orderId),
      GetProductionOrderComponentLines(orderId),
    ]);
    if (!order) return null;
    return { ...order, componentLines };
  } catch (error) {
    console.error("Failed to fetch production order:", error);
    message.error(t`Failed to fetch production order`);
    return null;
  }
});

export const createProductionOrderAtom = atom(
  null,
  async (get, set, newValues: Partial<ProductionOrder>) => {
    try {
      const data = {
        ...newValues,
        id: nanoid(),
        organizationId: get(organizationIdAtom)!,
      };
      const created = await CreateProductionOrder(data);
      set(productionOrderIdAtom, created.id);
      message.success(t`Production order created`);
      const list = get(productionOrdersAtom);
      set(productionOrdersAtom, [created, ...list]);
      return created;
    } catch (error) {
      console.error("Production order creation failed:", error);
      message.error(error instanceof Error ? error.message : t`Production order creation failed`);
      throw error;
    }
  },
);

export const updateProductionOrderStatusAtom = atom(
  null,
  async (
    get,
    set,
    {
      orderId,
      status,
      serialNumbers,
    }: { orderId: string; status: string; serialNumbers?: string[] },
  ) => {
    try {
      const updated = await UpdateProductionOrderStatus(orderId, status, serialNumbers);
      message.success(t`Production order status updated`);
      const list = get(productionOrdersAtom);
      const merged = keyBy([...list, updated], "id");
      set(productionOrdersAtom, orderBy(map(merged), "date", "desc"));
      return true;
    } catch (error) {
      // Completing without a full cost basis, or cancelling once produced
      // units have shipped, comes back as a 409 naming the product/quantity.
      console.error("Failed to update production order status:", error);
      message.error(
        error instanceof Error ? error.message : t`Failed to update production order status`,
      );
      return false;
    }
  },
);

export const deleteProductionOrderAtom = atom(null, async (get, set, orderId: string) => {
  try {
    const success = await DeleteProductionOrder(orderId);
    if (success) {
      const list = reject(get(productionOrdersAtom), (o) => isEqual(o.id, orderId));
      set(productionOrdersAtom, list);
      message.success(t`Production order deleted`);
    } else {
      message.error(t`Production order deletion failed`);
    }
    return success;
  } catch (error) {
    console.error("Failed to delete production order:", error);
    message.error(error instanceof Error ? error.message : t`Production order deletion failed`);
    return false;
  }
});
