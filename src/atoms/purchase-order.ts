import { atom } from "jotai";
import type { Dayjs } from "dayjs";
import type { PurchaseOrder, PurchaseOrderLineItem } from "src/types/models";
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
export const purchaseOrdersAtom = atom<PurchaseOrder[]>([]);
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

// The purchase order form edits dates as dayjs objects and line item
// unitPrice in display currency units — not the wire shape either field is
// stored as.
type PurchaseOrderLineItemFormValues = Omit<Partial<PurchaseOrderLineItem>, "unitPrice"> & {
  unitPrice?: number;
};
type PurchaseOrderFormValues = Omit<
  Partial<PurchaseOrder>,
  "orderDate" | "expectedDate" | "exchangeRateDate"
> & {
  orderDate?: Dayjs | number;
  expectedDate?: Dayjs | number | null;
  exchangeRateDate?: Dayjs | number | null;
  lineItems?: PurchaseOrderLineItemFormValues[];
};

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
        lineItems: (lineItems || []).map((item) => ({
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
  async (get, set, newValues: PurchaseOrderFormValues) => {
    const orderId = get(purchaseOrderIdAtom);
    const order = omit(newValues, "lineItems");
    const lineItems = newValues.lineItems || [];

    const toTimestamp = (v: Dayjs | number | null | undefined) =>
      v && typeof v === "object" && "valueOf" in v ? v.valueOf() : v;
    const toPayloadLineItems = (items: PurchaseOrderLineItemFormValues[]) =>
      items.map((item) => ({
        ...omit(item, ["id"]),
        unitPrice: unitsToCents(item.unitPrice ?? 0),
      }));

    try {
      if (!orderId) {
        const data = {
          ...order,
          id: nanoid(),
          organizationId: get(organizationIdAtom)!,
          status: order.status || "draft",
          orderDate: toTimestamp(order.orderDate),
          expectedDate: order.expectedDate ? toTimestamp(order.expectedDate) : null,
          exchangeRateDate: order.exchangeRateDate ? toTimestamp(order.exchangeRateDate) : null,
          lineItems: toPayloadLineItems(lineItems),
        };
        const created = await CreatePurchaseOrder(data);
        set(purchaseOrderIdAtom, created.id);
        message.success(t`Purchase order created`);
        const orders = get(purchaseOrdersAtom);
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
        const orders = get(purchaseOrdersAtom);
        const merged = keyBy([...orders, updated], "id");
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
      // Rethrow (F56/F67 convention — see CLAUDE.md's src/atoms/product.ts
      // entry) so the details page's handleSubmit can tell a failed save
      // apart from a successful one and skip clearing its isDirty flag —
      // otherwise a failed save would silently re-enable the Excel/PDF
      // export buttons over stale server-persisted data.
      throw error;
    }
  },
);

export const updatePurchaseOrderStatusAtom = atom(
  null,
  async (get, set, { orderId, status }: { orderId: string; status: string }) => {
    try {
      const updated = await UpdatePurchaseOrderStatus(orderId, status);
      message.success(t`Purchase order status updated`);
      const orders = get(purchaseOrdersAtom);
      const merged = keyBy([...orders, updated], "id");
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

// Links (or unlinks, passing importId: null) an existing purchase order to
// an import from the Imports drawer, without going through the full PO edit
// form. UpdatePurchaseOrder's SQL sets currency/exchangeRateDate/
// deliveryAddress/notes/importId unconditionally (not COALESCE'd) — sending
// only {importId} would silently null out the order's other fields, so this
// resends its current values from the already-loaded list record.
//
// exchangeRate is deliberately the one field NOT resent: db/purchase_order.go
// reads it back as a TEXT column (`*string`, exact decimal), but the PUT
// endpoint's UpdatePurchaseOrderRequest expects `*float64` — forwarding the
// string as-is fails strict JSON decoding with a 400 on every foreign-currency
// order. Omitting it (currency unchanged, no new rate) makes
// resolveExchangeRateForSave fall through to "keep the currently stored rate"
// and preserves the exact stored decimal instead of round-tripping it through
// float64.
export const setPurchaseOrderImportAtom = atom(
  null,
  async (get, set, { orderId, importId }: { orderId: string; importId: string | null }) => {
    const orders = get(purchaseOrdersAtom);
    const po = orders.find((o) => o.id === orderId);
    if (!po) return false;
    try {
      const updated = await UpdatePurchaseOrder(orderId, {
        vendorId: po.vendorId,
        orderNumber: po.orderNumber,
        orderDate: po.orderDate,
        expectedDate: po.expectedDate,
        currency: po.currency,
        exchangeRateDate: po.exchangeRateDate,
        deliveryAddress: po.deliveryAddress,
        notes: po.notes,
        importId,
      });
      const merged = keyBy([...get(purchaseOrdersAtom), updated], "id");
      set(purchaseOrdersAtom, orderBy(map(merged), "orderDate", "desc"));
      message.success(importId ? t`Purchase order linked` : t`Purchase order unlinked`);
      return true;
    } catch (error) {
      console.error("Failed to update purchase order's import link:", error);
      message.error(error instanceof Error ? error.message : t`Failed to update purchase order`);
      return false;
    }
  },
);

export const deletePurchaseOrderAtom = atom(null, async (get, set, orderId: string) => {
  try {
    const success = await DeletePurchaseOrder(orderId);
    if (success) {
      const orders = reject(get(purchaseOrdersAtom), (o) => isEqual(o.id, orderId));
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
