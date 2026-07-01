import { atom } from "jotai";
import { message } from "antd";
import { nanoid } from "nanoid";
import { t } from "@lingui/core/macro";
import isEqual from "lodash/isEqual";
import orderBy from "lodash/orderBy";
import keyBy from "lodash/keyBy";
import map from "lodash/map";
import reject from "lodash/reject";
import {
  GetProducts,
  GetProduct,
  CreateProduct,
  UpdateProduct,
  DeleteProduct,
} from "src/api";

import { organizationIdAtom } from "./organization";

export const productsAtom = atom<any[]>([]);
productsAtom.debugLabel = "productsAtom";

export const setProductsAtom = atom(null, async (get, set) => {
  const organizationId = get(organizationIdAtom);
  try {
    const response = await GetProducts(organizationId!);
    set(productsAtom, response);
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
      return null;
    }
  },
  async (get, set, newValues: any) => {
    const productId = get(productIdAtom);
    try {
      if (!productId) {
        const created = await CreateProduct({
          ...newValues,
          id: nanoid(),
          organizationId: get(organizationIdAtom),
        });
        message.success(t`Product created`);
        set(productsAtom, orderBy([...get(productsAtom), created], "name", "asc"));
      } else {
        const updated = await UpdateProduct(productId, newValues);
        message.success(t`Product updated`);
        const merged: any = keyBy([...get(productsAtom), updated], "id");
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
    }
  },
);

export const deleteProductAtom = atom(null, async (get, set, productId: string) => {
  try {
    const success = await DeleteProduct(productId);
    if (success) {
      set(productsAtom, reject(get(productsAtom), (p: any) => isEqual(p.id, productId)));
      message.success(t`Product deleted`);
    } else {
      message.error(t`Product deletion failed`);
    }
  } catch (error) {
    console.error("Failed to delete product:", error);
    message.error(t`Product deletion failed`);
  }
});
