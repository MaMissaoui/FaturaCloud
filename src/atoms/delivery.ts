import { atom } from "jotai";
import { message } from "antd";
import { nanoid } from "nanoid";
import { t } from "@lingui/core/macro";
import dayjs from "dayjs";
import isEqual from "lodash/isEqual";
import omit from "lodash/omit";
import reject from "lodash/reject";

import {
  GetDeliveries,
  GetNextDeliveryNumber,
  GetDelivery,
  GetDeliveryLineItems,
  CreateDelivery,
  UpdateDelivery,
  UpdateDeliveryStatus,
  DeleteDelivery,
} from "src/api";
import { organizationIdAtom } from "./organization";

export const deliveriesAtom = atom<any[]>([]);
export const nextDeliveryNumberAtom = atom(async (get) => {
  const organizationId = get(organizationIdAtom);
  if (!organizationId) return "DEL-0001";
  try {
    return await GetNextDeliveryNumber(organizationId);
  } catch {
    return "DEL-0001";
  }
});

export const setDeliveriesAtom = atom(null, async (get, set) => {
  const organizationId = get(organizationIdAtom);
  try {
    const response = await GetDeliveries(organizationId!);
    set(deliveriesAtom, response);
  } catch (error) {
    console.error("Failed to fetch deliveries:", error);
    message.error(t`Failed to fetch deliveries`);
    set(deliveriesAtom, []);
  }
});

export const deliveryIdAtom = atom<string | null>(null);
export const deliveryAtom = atom(
  async (get) => {
    const deliveryId = get(deliveryIdAtom);
    if (!deliveryId) return null;
    try {
      const [delivery, lineItems] = await Promise.all([
        GetDelivery(deliveryId),
        GetDeliveryLineItems(deliveryId),
      ]);
      if (!delivery) return null;
      return {
        ...delivery,
        deliveryDate: dayjs(delivery.deliveryDate),
        lineItems: lineItems || [],
      };
    } catch (error) {
      console.error("Failed to fetch delivery:", error);
      return null;
    }
  },
  async (get, set, newValues: any) => {
    const deliveryId = get(deliveryIdAtom);
    const delivery = omit(newValues, "lineItems");
    const lineItems = newValues.lineItems || [];
    const toTimestamp = (v: any) => (v?.valueOf ? v.valueOf() : v);

    try {
      if (!deliveryId) {
        const data = {
          ...delivery,
          id: nanoid(),
          organizationId: get(organizationIdAtom),
          deliveryDate: toTimestamp(delivery.deliveryDate),
          lineItems,
        };
        const created = await CreateDelivery(data);
        set(deliveryIdAtom, created.id);
        message.success(t`Delivery created`);
        const list: any = get(deliveriesAtom);
        set(deliveriesAtom, [created, ...list]);
      } else {
        const data = {
          ...delivery,
          deliveryDate: toTimestamp(delivery.deliveryDate),
          lineItems,
        };
        await UpdateDelivery(deliveryId, data);
        message.success(t`Delivery saved`);
      }
    } catch (error) {
      console.error("Delivery operation failed:", error);
      message.error(deliveryId ? t`Delivery update failed` : t`Delivery creation failed`);
    }
  },
);

export const updateDeliveryStatusAtom = atom(
  null,
  async (_get, _set, { deliveryId, status }: { deliveryId: string; status: string }) => {
    try {
      await UpdateDeliveryStatus(deliveryId, status);
      message.success(t`Delivery status updated`);
    } catch (error) {
      console.error("Failed to update delivery status:", error);
      message.error(t`Failed to update delivery status`);
    }
  },
);

export const deleteDeliveryAtom = atom(null, async (get, set, deliveryId: string) => {
  try {
    const success = await DeleteDelivery(deliveryId);
    if (success) {
      const list: any = reject(get(deliveriesAtom), (d: any) => isEqual(d.id, deliveryId));
      set(deliveriesAtom, list);
      message.success(t`Delivery deleted`);
    } else {
      message.error(t`Delivery deletion failed`);
    }
  } catch (error) {
    console.error("Failed to delete delivery:", error);
    message.error(t`Delivery deletion failed`);
  }
});
