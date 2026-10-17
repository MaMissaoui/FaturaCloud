import { atom } from "jotai";
import type { Product } from "src/types/models";
import { message } from "src/utils/message";
import { nanoid } from "nanoid";
import { t } from "@lingui/core/macro";
import isEqual from "lodash/isEqual";
import orderBy from "lodash/orderBy";
import keyBy from "lodash/keyBy";
import map from "lodash/map";
import reject from "lodash/reject";
import { GetProducts, GetProduct, CreateProduct, UpdateProduct, DeleteProduct } from "src/api";

import { organizationIdAtom } from "./organization";

export const productsAtom = atom<Product[]>([]);
productsAtom.debugLabel = "productsAtom";

export const setProductsAtom = atom(null, async (get, set) => {
  const organizationId = get(organizationIdAtom);
  try {
    const response = await GetProducts(organizationId!);
    set(productsAtom, response.data);
  } catch (error) {
    console.error("Failed to fetch products:", error);
    message.error(t`Failed to fetch products`);
    set(productsAtom, []);
  }
});
setProductsAtom.debugLabel = "setProductsAtom";

export const productIdAtom = atom<string | null>(null);
productIdAtom.debugLabel = "productIdAtom";

export const productAtom = atom(
  async (get) => {
    const productId = get(productIdAtom);
    if (!productId) return null;
    try {
      return await GetProduct(productId);
    } catch (error) {
      console.error("Failed to fetch product:", error);
      message.error(t`Failed to fetch product`);
      return null;
    }
  },
  async (get, set, newValues: Partial<Product>) => {
    const productId = get(productIdAtom);
    try {
      if (!productId) {
        const created = await CreateProduct({
          ...newValues,
          id: nanoid(),
          organizationId: get(organizationIdAtom)!,
        });
        message.success(t`Product created`);
        set(productsAtom, orderBy([...get(productsAtom), created], "name", "asc"));
      } else {
        const updated = await UpdateProduct(productId, newValues);
        message.success(t`Product updated`);
        const merged = keyBy([...get(productsAtom), updated], "id");
        set(productsAtom, orderBy(map(merged), "name", "asc"));
      }
    } catch (error) {
      console.error("Product operation failed:", error);
      message.error(
        error instanceof Error
          ? error.message
          : productId
            ? t`Product update failed`
            : t`Product creation failed`,
      );
      throw error;
    }
  },
);

// archiveProductAtom archives (or restores) a product: the way to retire one
// that has history and so can't be deleted (audit F167). It resends the
// stored product with the flag flipped, since the update is a full replace,
// rather than whatever the drawer's form holds, so unsaved edits aren't
// saved along the way. Returns whether it worked.
export const archiveProductAtom = atom(
  null,
  async (get, set, { productId, archived }: { productId: string; archived: boolean }) => {
    const stored = get(productsAtom).find((p) => p.id === productId);
    if (!stored) return false;
    try {
      const updated = await UpdateProduct(productId, { ...stored, isArchived: archived });
      const merged = keyBy([...get(productsAtom), updated], "id");
      set(productsAtom, orderBy(map(merged), "name", "asc"));
      message.success(archived ? t`Product archived` : t`Product restored`);
      return true;
    } catch (error) {
      console.error("Failed to archive product:", error);
      message.error(
        error instanceof Error
          ? error.message
          : archived
            ? t`Product could not be archived`
            : t`Product could not be restored`,
      );
      return false;
    }
  },
);
archiveProductAtom.debugLabel = "archiveProductAtom";

export const deleteProductAtom = atom(null, async (get, set, productId: string) => {
  try {
    const success = await DeleteProduct(productId);
    if (success) {
      set(
        productsAtom,
        reject(get(productsAtom), (p) => isEqual(p.id, productId)),
      );
      message.success(t`Product deleted`);
    } else {
      message.error(t`Product deletion failed`);
    }
    return success;
  } catch (error) {
    console.error("Failed to delete product:", error);
    message.error(error instanceof Error ? error.message : t`Product deletion failed`);
    return false;
  }
});
