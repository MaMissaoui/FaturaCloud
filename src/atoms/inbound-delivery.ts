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
  GetInboundDeliveries,
  GetNextInboundDeliveryNumber,
  GetInboundDelivery,
  GetInboundDeliveryLineItems,
  CreateInboundDelivery,
  UpdateInboundDelivery,
  UpdateInboundDeliveryStatus,
  DeleteInboundDelivery,
} from "src/api";
import { centsToUnits, unitsToCents } from "src/utils/currency";
import { organizationIdAtom } from "./organization";

export const inboundDeliveriesAtom = atom<any[]>([]);
inboundDeliveriesAtom.debugLabel = "inboundDeliveriesAtom";

export const setInboundDeliveriesAtom = atom(null, async (get, set) => {
  const organizationId = get(organizationIdAtom);
  try {
    const response = await GetInboundDeliveries(organizationId!);
    set(inboundDeliveriesAtom, response);
  } catch (error) {
    console.error("Failed to fetch goods receipts:", error);
    message.error(t`Failed to fetch goods receipts`);
    set(inboundDeliveriesAtom, []);
  }
});

// Server-side MAX-based numbering, like outbound deliveries.
export const nextInboundDeliveryNumberAtom = atom(async (get) => {
  const organizationId = get(organizationIdAtom);
  if (!organizationId) return "GR-0001";
  try {
    return await GetNextInboundDeliveryNumber(organizationId);
  } catch {
    return "GR-0001";
  }
});

export const inboundDeliveryIdAtom = atom<string | null>(null);

export const inboundDeliveryAtom = atom(
  async (get) => {
    const deliveryId = get(inboundDeliveryIdAtom);
    if (!deliveryId) return null;
    try {
      const [delivery, lineItems] = await Promise.all([
        GetInboundDelivery(deliveryId),
        GetInboundDeliveryLineItems(deliveryId),
      ]);
      if (!delivery) return null;
      return {
        ...delivery,
        deliveryDate: dayjs(delivery.deliveryDate),
        exchangeRateDate: delivery.exchangeRateDate ? dayjs(delivery.exchangeRateDate) : null,
        lineItems: (lineItems || []).map((item: any) => ({
          ...item,
          unitCost: item.unitCost === null ? null : centsToUnits(item.unitCost),
        })),
      };
    } catch (error) {
      console.error("Failed to fetch goods receipt:", error);
      message.error(t`Failed to fetch goods receipt`);
      return null;
    }
  },
  async (get, set, newValues: any) => {
    const deliveryId = get(inboundDeliveryIdAtom);
    const delivery = omit(newValues, "lineItems");
    const lineItems = newValues.lineItems || [];

    const toTimestamp = (v: any) => (v?.valueOf ? v.valueOf() : v);
    const toPayloadLineItems = (items: any[]) =>
      items.map((item: any) => ({
        // stockEnabled/currentStock/productName are read-only joins; sending
        // them back would just be noise.
        ...omit(item, ["id", "stockEnabled", "currentStock", "productName"]),
        unitCost:
          item.unitCost === null || item.unitCost === undefined
            ? null
            : unitsToCents(item.unitCost),
      }));

    try {
      if (!deliveryId) {
        const data = {
          ...delivery,
          id: nanoid(),
          organizationId: get(organizationIdAtom),
          deliveryDate: toTimestamp(delivery.deliveryDate),
          exchangeRateDate: delivery.exchangeRateDate
            ? toTimestamp(delivery.exchangeRateDate)
            : null,
          lineItems: toPayloadLineItems(lineItems),
        };
        const created = await CreateInboundDelivery(data);
        set(inboundDeliveryIdAtom, created.id);
        message.success(t`Goods receipt created`);
        const list: any = get(inboundDeliveriesAtom);
        set(inboundDeliveriesAtom, [created, ...list]);
      } else {
        const data = {
          ...delivery,
          deliveryDate: toTimestamp(delivery.deliveryDate),
          exchangeRateDate: delivery.exchangeRateDate
            ? toTimestamp(delivery.exchangeRateDate)
            : null,
          lineItems: toPayloadLineItems(lineItems),
        };
        const updated = await UpdateInboundDelivery(deliveryId, data);
        message.success(t`Goods receipt saved`);
        const list: any = get(inboundDeliveriesAtom);
        const merged: any = keyBy([...list, updated], "id");
        set(inboundDeliveriesAtom, orderBy(map(merged), "deliveryDate", "desc"));
      }
    } catch (error) {
      console.error("Goods receipt operation failed:", error);
      const fallback = deliveryId
        ? t`Goods receipt update failed`
        : t`Goods receipt creation failed`;
      message.error(error instanceof Error ? error.message : fallback);
    }
  },
);

export const updateInboundDeliveryStatusAtom = atom(
  null,
  async (get, set, { deliveryId, status }: { deliveryId: string; status: string }) => {
    try {
      const updated = await UpdateInboundDeliveryStatus(deliveryId, status);
      message.success(t`Goods receipt status updated`);
      const list: any = get(inboundDeliveriesAtom);
      const merged: any = keyBy([...list, updated], "id");
      set(inboundDeliveriesAtom, orderBy(map(merged), "deliveryDate", "desc"));
      return true;
    } catch (error) {
      // Cancelling a received receipt whose goods are gone comes back as a 409
      // whose message names the product and quantities.
      console.error("Failed to update goods receipt status:", error);
      message.error(
        error instanceof Error ? error.message : t`Failed to update goods receipt status`,
      );
      return false;
    }
  },
);

export const deleteInboundDeliveryAtom = atom(null, async (get, set, deliveryId: string) => {
  try {
    const success = await DeleteInboundDelivery(deliveryId);
    if (success) {
      const list: any = reject(get(inboundDeliveriesAtom), (d: any) => isEqual(d.id, deliveryId));
      set(inboundDeliveriesAtom, list);
      message.success(t`Goods receipt deleted`);
    } else {
      message.error(t`Goods receipt deletion failed`);
    }
    return success;
  } catch (error) {
    console.error("Failed to delete goods receipt:", error);
    message.error(error instanceof Error ? error.message : t`Goods receipt deletion failed`);
    return false;
  }
});
