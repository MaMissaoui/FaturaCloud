import { atom } from "jotai";
import { message } from "src/utils/message";
import { nanoid } from "nanoid";
import { t } from "@lingui/core/macro";
import orderBy from "lodash/orderBy";
import keyBy from "lodash/keyBy";
import map from "lodash/map";
import reject from "lodash/reject";
import isEqual from "lodash/isEqual";
import { GetPaymentTerms, CreatePaymentTerm, UpdatePaymentTerm, DeletePaymentTerm } from "src/api";

import { organizationIdAtom } from "./organization";

// Payment terms list
export const paymentTermsAtom = atom<any[]>([]);
export const setPaymentTermsAtom = atom(null, async (get, set) => {
  const organizationId = get(organizationIdAtom);
  try {
    const response = await GetPaymentTerms(organizationId!);
    set(paymentTermsAtom, response);
  } catch (error) {
    console.error("Failed to fetch payment terms:", error);
    message.error(t`Failed to fetch payment terms`);
    set(paymentTermsAtom, []);
  }
});

// Single payment term (read+write), for the Settings drawer form. There's no
// GET /payment-terms/{id} endpoint — the list is always small enough that
// the drawer just looks the record up from the already-loaded list.
export const paymentTermIdAtom = atom<string | null>(null);
export const paymentTermAtom = atom(
  (get) => {
    const id = get(paymentTermIdAtom);
    if (!id) return null;
    return get(paymentTermsAtom).find((pt: any) => pt.id === id) ?? null;
  },
  async (get, set, newValues: any) => {
    const id = get(paymentTermIdAtom);
    try {
      if (!id) {
        const data = {
          ...newValues,
          id: nanoid(),
          organizationId: get(organizationIdAtom),
          isDefault: newValues.isDefault ? 1 : 0,
        };
        const created = await CreatePaymentTerm(data);
        set(paymentTermIdAtom, created.id);
        message.success(t`Payment term created`);
        const terms: any = get(paymentTermsAtom);
        set(paymentTermsAtom, orderBy([...terms, created], "name", "asc"));
      } else {
        const data = { ...newValues, isDefault: newValues.isDefault ? 1 : 0 };
        const updated = await UpdatePaymentTerm(id, data);
        message.success(t`Payment term updated`);
        const terms: any = get(paymentTermsAtom);
        const merged: any = keyBy([...terms, updated], "id");
        set(paymentTermsAtom, orderBy(map(merged), "name", "asc"));
      }
    } catch (error) {
      console.error("Payment term operation failed:", error);
      const fallback = id ? t`Payment term update failed` : t`Payment term creation failed`;
      message.error(error instanceof Error ? error.message : fallback);
      throw error;
    }
  },
);

export const deletePaymentTermAtom = atom(null, async (get, set, id: string) => {
  try {
    const success = await DeletePaymentTerm(id);
    if (success) {
      const terms = reject(get(paymentTermsAtom), (pt: any) => isEqual(pt.id, id));
      set(paymentTermsAtom, terms);
      message.success(t`Payment term deleted`);
    } else {
      message.error(t`Payment term deletion failed`);
    }
    return success;
  } catch (error) {
    console.error("Failed to delete payment term:", error);
    message.error(error instanceof Error ? error.message : t`Payment term deletion failed`);
    return false;
  }
});
