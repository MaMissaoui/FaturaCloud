import { atom } from "jotai";
import type { Dayjs } from "dayjs";
import type { Order, OrderLineItem } from "src/types/models";
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
export const ordersAtom = atom<Order[]>([]);
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
  const max = orders.reduce((acc, o) => {
    const m = String(o.orderNumber ?? "").match(/(\d+)$/);
    const n = m ? parseInt(m[1], 10) : 0;
    return n > acc ? n : acc;
  }, 0);
  return `ORD-${String(max + 1).padStart(3, "0")}`;
});

// The order form edits dates as dayjs objects (converted to unix-ms on
// save) and line item unitPrice in display currency units (converted to
// cents on save) — not the wire shape either field is stored as.
type OrderLineItemFormValues = Omit<Partial<OrderLineItem>, "unitPrice"> & {
  unitPrice?: number;
};
type OrderFormValues = Omit<Partial<Order>, "orderDate" | "deliveryDate" | "exchangeRateDate"> & {
  orderDate?: Dayjs | number;
  deliveryDate?: Dayjs | number | null;
  exchangeRateDate?: Dayjs | number | null;
  lineItems?: OrderLineItemFormValues[];
};

// Single order (read+write)
export const orderIdAtom = atom<string | null>(null);
export const orderAtom = atom(
  async (get) => {
    const orderId = get(orderIdAtom);
    if (!orderId) return null;
    try {
      const [order, lineItems] = await Promise.all([GetOrder(orderId), GetOrderLineItems(orderId)]);
      if (!order) return null;
      return {
        ...order,
        orderDate: dayjs(order.orderDate),
        deliveryDate: order.deliveryDate ? dayjs(order.deliveryDate) : null,
        exchangeRateDate: order.exchangeRateDate ? dayjs(order.exchangeRateDate) : null,
        lineItems: (lineItems || []).map((item) => ({
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
  async (get, set, newValues: OrderFormValues) => {
    const orderId = get(orderIdAtom);
    const order = omit(newValues, "lineItems");
    const lineItems = newValues.lineItems || [];

    const toTimestamp = (v: Dayjs | number | null | undefined) =>
      v && typeof v === "object" && "valueOf" in v ? v.valueOf() : v;

    try {
      if (!orderId) {
        const data = {
          ...order,
          id: nanoid(),
          organizationId: get(organizationIdAtom)!,
          status: order.status || "draft",
          orderDate: toTimestamp(order.orderDate),
          deliveryDate: order.deliveryDate ? toTimestamp(order.deliveryDate) : null,
          exchangeRateDate: order.exchangeRateDate ? toTimestamp(order.exchangeRateDate) : null,
          lineItems: lineItems.map((item) => ({
            ...omit(item, ["id"]),
            unitPrice: unitsToCents(item.unitPrice ?? 0),
          })),
        };
        const created = await CreateOrder(data);
        set(orderIdAtom, created.id);
        message.success(t`Order created`);
        const orders = get(ordersAtom);
        set(ordersAtom, [created, ...orders]);
      } else {
        const data = {
          ...order,
          orderDate: toTimestamp(order.orderDate),
          deliveryDate: order.deliveryDate ? toTimestamp(order.deliveryDate) : null,
          exchangeRateDate: order.exchangeRateDate ? toTimestamp(order.exchangeRateDate) : null,
          lineItems: lineItems.map((item) => ({
            ...omit(item, ["id"]),
            unitPrice: unitsToCents(item.unitPrice ?? 0),
          })),
        };
        const updated = await UpdateOrder(orderId, data);
        message.success(t`Order saved`);
        const orders = get(ordersAtom);
        const merged = keyBy([...orders, updated], "id");
        set(ordersAtom, orderBy(map(merged), "orderDate", "desc"));
      }
    } catch (error) {
      console.error("Order operation failed:", error);
      const fallback = orderId ? t`Order update failed` : t`Order creation failed`;
      message.error(error instanceof Error ? error.message : fallback);
      // Rethrow (F56/F67 convention, also applied to purchase-order.ts —
      // see CLAUDE.md) so details.tsx's handleSubmit can tell a failed save
      // apart from a successful one and skip clearing its isDirty flag.
      throw error;
    }
  },
);

export const updateOrderStatusAtom = atom(
  null,
  async (get, set, { orderId, status }: { orderId: string; status: string }) => {
    try {
      const updated = await UpdateOrderStatus(orderId, status);
      message.success(t`Order status updated`);
      const orders = get(ordersAtom);
      const merged = keyBy([...orders, updated], "id");
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
      const orders = reject(get(ordersAtom), (o) => isEqual(o.id, orderId));
      set(ordersAtom, orders);
      message.success(t`Order deleted`);
    } else {
      message.error(t`Order deletion failed`);
    }
    return success;
  } catch (error) {
    console.error("Failed to delete order:", error);
    message.error(error instanceof Error ? error.message : t`Order deletion failed`);
    return false;
  }
});
