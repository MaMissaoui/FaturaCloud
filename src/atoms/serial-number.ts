import { atom } from "jotai";
import { message } from "src/utils/message";
import { t } from "@lingui/core/macro";
import { GetProductSerialNumbers } from "src/api";
import type { SerialNumber } from "src/types/models";

// Cached per product rather than one flat list — every consumer (movement
// form, serial-capture modal) only ever needs one product's serials at a
// time, and several lines on the same document commonly share a product.
export const productSerialNumbersAtom = atom<Record<string, SerialNumber[]>>({});
productSerialNumbersAtom.debugLabel = "productSerialNumbersAtom";

export const loadProductSerialNumbersAtom = atom(null, async (get, set, productId: string) => {
  try {
    const rows = await GetProductSerialNumbers(productId);
    set(productSerialNumbersAtom, { ...get(productSerialNumbersAtom), [productId]: rows });
    return rows;
  } catch (error) {
    console.error("Failed to fetch serial numbers:", error);
    message.error(t`Failed to fetch serial numbers`);
    return [];
  }
});
loadProductSerialNumbersAtom.debugLabel = "loadProductSerialNumbersAtom";
