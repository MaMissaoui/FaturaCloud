import { atom } from "jotai";
import { message } from "antd";
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
  GetOrders,
  GetOrder,
  GetOrderLineItems,
  CreateOrder,
  UpdateOrder,
  UpdateOrderStatus,
  DeleteOrder,
} from "src/api";
import { centsToUnits, unitsToCents } from "src/utils/currency";
import { organizationIdAtom } from "./organization";

// Orders list
export const ordersAtom = atom<any[]>([]);
export const setOrdersAtom = atom(null, async (get, set) => {
  const organizationId = get(organizationIdAtom);
  try {
    const response = await GetOrders(organizationId!);
    set(ordersAtom, response);
  } catch (error) {
    console.error("Failed to fetch orders:", error);
    message.error(t`Failed to fetch orders`);
    set(ordersAtom, []);
  }
});

// Next suggested order number (derived from the loaded list)
export const nextOrderNumberAtom = atom((get) => {
  const orders = get(ordersAtom);
  if (orders.length === 0) return "ORD-001";
  const max = orders.reduce((acc: number, o: any) => {
    const m = String(o.orderNumber ?? "").match(/(\d+)$/);
    const n = m ? parseInt(m[1], 10) : 0;
    return n > acc ? n : acc;
  }, 0);
  return `ORD-${String(max + 1).padStart(3, "0")}`;
});

// Single order (read+write)
export const orderIdAtom = atom<string | null>(null);
export const orderAtom = atom(
  async (get) => {
    const orderId = get(orderIdAtom);
    if (!orderId) return null;
    try {
      const [order, lineItems] = await Promise.all([
        GetOrder(orderId),
        GetOrderLineItems(orderId),
      ]);
      if (!order) return null;
      return {
        ...order,
        orderDate: dayjs(order.orderDate),
        deliveryDate: order.deliveryDate ? dayjs(order.deliveryDate) : null,
        lineItems: (lineItems || []).map((item: any) => ({
          ...item,
          unitPrice: centsToUnits(item.unitPrice),
        })),
      };
    } catch (error) {
      console.error("Failed to fetch order:", error);
      message.error(t`Failed to fetch order`);
      return null;
    }
  },
  async (get, set, newValues: any) => {
    const orderId = get(orderIdAtom);
    const order = omit(newValues, "lineItems");
    const lineItems = newValues.lineItems || [];

    const toTimestamp = (v: any) => (v?.valueOf ? v.valueOf() : v);

    try {
      if (!orderId) {
        const data = {
          ...order,
          id: nanoid(),
          organizationId: get(organizationIdAtom),
          status: order.status || "draft",
          orderDate: toTimestamp(order.orderDate),
          deliveryDate: order.deliveryDate ? toTimestamp(order.deliveryDate) : null,
          lineItems: lineItems.map((item: any) => ({
            ...omit(item, ["id"]),
            unitPrice: unitsToCents(item.unitPrice ?? 0),
          })),
        };
        const created = await CreateOrder(data);
        set(orderIdAtom, created.id);
        message.success(t`Order created`);
        const orders: any = get(ordersAtom);
        set(ordersAtom, [created, ...orders]);
      } else {
        const data = {
          ...order,
          orderDate: toTimestamp(order.orderDate),
          deliveryDate: order.deliveryDate ? toTimestamp(order.deliveryDate) : null,
          lineItems: lineItems.map((item: any) => ({
            ...omit(item, ["id"]),
            unitPrice: unitsToCents(item.unitPrice ?? 0),
          })),
        };
        const updated = await UpdateOrder(orderId, data);
        message.success(t`Order saved`);
        const orders: any = get(ordersAtom);
        const merged: any = keyBy([...orders, updated], "id");
        set(ordersAtom, orderBy(map(merged), "orderDate", "desc"));
      }
    } catch (error) {
      console.error("Order operation failed:", error);
      message.error(orderId ? t`Order update failed` : t`Order creation failed`);
    }
  },
);

export const updateOrderStatusAtom = atom(
  null,
  async (get, set, { orderId, status }: { orderId: string; status: string }) => {
    try {
      const updated = await UpdateOrderStatus(orderId, status);
      message.success(t`Order status updated`);
      const orders: any = get(ordersAtom);
      const merged: any = keyBy([...orders, updated], "id");
      set(ordersAtom, orderBy(map(merged), "orderDate", "desc"));
    } catch (error) {
      console.error("Failed to update order status:", error);
      message.error(t`Failed to update order status`);
    }
  },
);

export const deleteOrderAtom = atom(null, async (get, set, orderId: string) => {
  try {
    const success = await DeleteOrder(orderId);
    if (success) {
      const orders: any = reject(get(ordersAtom), (o: any) => isEqual(o.id, orderId));
      set(ordersAtom, orders);
      message.success(t`Order deleted`);
    } else {
      message.error(t`Order deletion failed`);
    }
  } catch (error) {
    console.error("Failed to delete order:", error);
    message.error(t`Order deletion failed`);
  }
});
