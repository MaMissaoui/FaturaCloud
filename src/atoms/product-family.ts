import { atom } from "jotai";
import type { ProductFamily } from "src/types/models";
import { message } from "src/utils/message";
import { nanoid } from "nanoid";
import { t } from "@lingui/core/macro";
import orderBy from "lodash/orderBy";
import keyBy from "lodash/keyBy";
import map from "lodash/map";
import reject from "lodash/reject";
import isEqual from "lodash/isEqual";
import {
  GetProductFamilies,
  CreateProductFamily,
  UpdateProductFamily,
  DeleteProductFamily,
} from "src/api";

import { organizationIdAtom } from "./organization";

// Product families list
export const productFamiliesAtom = atom<ProductFamily[]>([]);
export const setProductFamiliesAtom = atom(null, async (get, set) => {
  const organizationId = get(organizationIdAtom);
  try {
    const response = await GetProductFamilies(organizationId!);
    set(productFamiliesAtom, response);
  } catch (error) {
    console.error("Failed to fetch product families:", error);
    message.error(t`Failed to fetch product families`);
    set(productFamiliesAtom, []);
  }
});

// Single product family (read+write), for the Settings drawer form. There's
// no GET /product-families/{id} endpoint — the list is always small enough
// that the drawer just looks the record up from the already-loaded list.
export const productFamilyIdAtom = atom<string | null>(null);

export const productFamilyAtom = atom(
  (get) => {
    const id = get(productFamilyIdAtom);
    if (!id) return null;
    return get(productFamiliesAtom).find((f) => f.id === id) ?? null;
  },
  async (get, set, newValues: Partial<ProductFamily>) => {
    const id = get(productFamilyIdAtom);
    try {
      if (!id) {
        const data = {
          ...newValues,
          id: nanoid(),
          organizationId: get(organizationIdAtom)!,
        };
        const created = await CreateProductFamily(data);
        set(productFamilyIdAtom, created.id);
        message.success(t`Product family created`);
        const families = get(productFamiliesAtom);
        set(productFamiliesAtom, orderBy([...families, created], "name", "asc"));
      } else {
        const updated = await UpdateProductFamily(id, newValues);
        message.success(t`Product family updated`);
        const families = get(productFamiliesAtom);
        const merged = keyBy([...families, updated], "id");
        set(productFamiliesAtom, orderBy(map(merged), "name", "asc"));
      }
    } catch (error) {
      console.error("Product family operation failed:", error);
      const fallback = id ? t`Product family update failed` : t`Product family creation failed`;
      message.error(error instanceof Error ? error.message : fallback);
      throw error;
    }
  },
);

export const deleteProductFamilyAtom = atom(null, async (get, set, id: string) => {
  try {
    const success = await DeleteProductFamily(id);
    if (success) {
      const families = reject(get(productFamiliesAtom), (f) => isEqual(f.id, id));
      set(productFamiliesAtom, families);
      message.success(t`Product family deleted`);
    } else {
      message.error(t`Product family deletion failed`);
    }
    return success;
  } catch (error) {
    console.error("Failed to delete product family:", error);
    message.error(error instanceof Error ? error.message : t`Product family deletion failed`);
    return false;
  }
});
