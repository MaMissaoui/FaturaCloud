import { atom } from "jotai";
import { message } from "src/utils/message";
import { nanoid } from "nanoid";
import { t } from "@lingui/core/macro";
import dayjs from "dayjs";
import isEqual from "lodash/isEqual";
import omit from "lodash/omit";
import orderBy from "lodash/orderBy";
import keyBy from "lodash/keyBy";
import map from "lodash/map";
import reject from "lodash/reject";

import {
  GetPurchaseOrders,
  GetNextPurchaseOrderNumber,
  GetPurchaseOrder,
  GetPurchaseOrderLineItems,
  CreatePurchaseOrder,
  UpdatePurchaseOrder,
  UpdatePurchaseOrderStatus,
  DeletePurchaseOrder,
} from "src/api";
import { centsToUnits, unitsToCents } from "src/utils/currency";
import { organizationIdAtom } from "./organization";

// Purchase orders list
export const purchaseOrdersAtom = atom<any[]>([]);
purchaseOrdersAtom.debugLabel = "purchaseOrdersAtom";

export const setPurchaseOrdersAtom = atom(null, async (get, set) => {
  const organizationId = get(organizationIdAtom);
  try {
    const response = await GetPurchaseOrders(organizationId!);
    set(purchaseOrdersAtom, response);
  } catch (error) {
    console.error("Failed to fetch purchase orders:", error);
    message.error(t`Failed to fetch purchase orders`);
    set(purchaseOrdersAtom, []);
  }
});

// Next suggested number comes from the server (MAX-based), not a client-side
// scan of the loaded list — the latter is racy and reissues numbers whenever
// the list is stale or an order was deleted.
export const nextPurchaseOrderNumberAtom = atom(async (get) => {
  const organizationId = get(organizationIdAtom);
  if (!organizationId) return "PO-0001";
  try {
    return await GetNextPurchaseOrderNumber(organizationId);
  } catch {
    return "PO-0001";
  }
});

// Single purchase order (read+write)
export const purchaseOrderIdAtom = atom<string | null>(null);

export const purchaseOrderAtom = atom(
  async (get) => {
    const orderId = get(purchaseOrderIdAtom);
    if (!orderId) return null;
    try {
      const [order, lineItems] = await Promise.all([
        GetPurchaseOrder(orderId),
        GetPurchaseOrderLineItems(orderId),
      ]);
      if (!order) return null;
      return {
        ...order,
        orderDate: dayjs(order.orderDate),
        expectedDate: order.expectedDate ? dayjs(order.expectedDate) : null,
        exchangeRateDate: order.exchangeRateDate ? dayjs(order.exchangeRateDate) : null,
        lineItems: (lineItems || []).map((item: any) => ({
          ...item,
          unitPrice: centsToUnits(item.unitPrice),
        })),
      };
    } catch (error) {
      console.error("Failed to fetch purchase order:", error);
      message.error(t`Failed to fetch purchase order`);
      return null;
    }
  },
  async (get, set, newValues: any) => {
    const orderId = get(purchaseOrderIdAtom);
    const order = omit(newValues, "lineItems");
    const lineItems = newValues.lineItems || [];

    const toTimestamp = (v: any) => (v?.valueOf ? v.valueOf() : v);
    const toPayloadLineItems = (items: any[]) =>
      items.map((item: any) => ({
        ...omit(item, ["id"]),
        unitPrice: unitsToCents(item.unitPrice ?? 0),
      }));

    try {
      if (!orderId) {
        const data = {
          ...order,
          id: nanoid(),
          organizationId: get(organizationIdAtom),
          status: order.status || "draft",
          orderDate: toTimestamp(order.orderDate),
          expectedDate: order.expectedDate ? toTimestamp(order.expectedDate) : null,
          exchangeRateDate: order.exchangeRateDate ? toTimestamp(order.exchangeRateDate) : null,
          lineItems: toPayloadLineItems(lineItems),
        };
        const created = await CreatePurchaseOrder(data);
        set(purchaseOrderIdAtom, created.id);
        message.success(t`Purchase order created`);
        const orders: any = get(purchaseOrdersAtom);
        set(purchaseOrdersAtom, [created, ...orders]);
      } else {
        const data = {
          ...order,
          orderDate: toTimestamp(order.orderDate),
          expectedDate: order.expectedDate ? toTimestamp(order.expectedDate) : null,
          exchangeRateDate: order.exchangeRateDate ? toTimestamp(order.exchangeRateDate) : null,
          lineItems: toPayloadLineItems(lineItems),
        };
        const updated = await UpdatePurchaseOrder(orderId, data);
        message.success(t`Purchase order saved`);
        const orders: any = get(purchaseOrdersAtom);
        const merged: any = keyBy([...orders, updated], "id");
        set(purchaseOrdersAtom, orderBy(map(merged), "orderDate", "desc"));
      }
    } catch (error) {
      // Server-side rejections (invalid status, totals) arrive as a 409 whose
      // message is written for the user.
      console.error("Purchase order operation failed:", error);
      const fallback = orderId
        ? t`Purchase order update failed`
        : t`Purchase order creation failed`;
      message.error(error instanceof Error ? error.message : fallback);
    }
  },
);

export const updatePurchaseOrderStatusAtom = atom(
  null,
  async (get, set, { orderId, status }: { orderId: string; status: string }) => {
    try {
      const updated = await UpdatePurchaseOrderStatus(orderId, status);
      message.success(t`Purchase order status updated`);
      const orders: any = get(purchaseOrdersAtom);
      const merged: any = keyBy([...orders, updated], "id");
      set(purchaseOrdersAtom, orderBy(map(merged), "orderDate", "desc"));
      return true;
    } catch (error) {
      console.error("Failed to update purchase order status:", error);
      message.error(
        error instanceof Error ? error.message : t`Failed to update purchase order status`,
      );
      return false;
    }
  },
);

export const deletePurchaseOrderAtom = atom(null, async (get, set, orderId: string) => {
  try {
    const success = await DeletePurchaseOrder(orderId);
    if (success) {
      const orders: any = reject(get(purchaseOrdersAtom), (o: any) => isEqual(o.id, orderId));
      set(purchaseOrdersAtom, orders);
      message.success(t`Purchase order deleted`);
    } else {
      message.error(t`Purchase order deletion failed`);
    }
    return success;
  } catch (error) {
    console.error("Failed to delete purchase order:", error);
    message.error(error instanceof Error ? error.message : t`Purchase order deletion failed`);
    return false;
  }
});
