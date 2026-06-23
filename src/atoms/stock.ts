import { atom } from "jotai";
import { message } from "antd";
import { nanoid } from "nanoid";
import { t } from "@lingui/core/macro";
import { GetStockMovements, CreateStockMovement, DeleteStockMovement } from "src/api";
import { organizationIdAtom } from "./organization";
import { productsAtom } from "./product";

export const stockMovementsAtom = atom<any[]>([]);
stockMovementsAtom.debugLabel = "stockMovementsAtom";

export const setStockMovementsAtom = atom(null, async (get, set) => {
  const organizationId = get(organizationIdAtom);
  try {
    const response = await GetStockMovements(organizationId!);
    set(stockMovementsAtom, response);
  } catch (error) {
    console.error("Failed to fetch stock movements:", error);
    message.error(t`Failed to fetch stock movements`);
    set(stockMovementsAtom, []);
  }
});
setStockMovementsAtom.debugLabel = "setStockMovementsAtom";

export const createStockMovementAtom = atom(null, async (get, set, req: any) => {
  try {
    const movement = await CreateStockMovement({
      ...req,
      id: nanoid(),
      organizationId: get(organizationIdAtom),
    });
    set(stockMovementsAtom, [movement, ...get(stockMovementsAtom)]);

    // Update the product's stockQuantity in the products list
    const products: any[] = get(productsAtom);
    set(
      productsAtom,
      products.map((p: any) =>
        p.id === movement.productId
          ? { ...p, stockQuantity: p.stockQuantity + movement.quantity }
          : p,
      ),
    );

    message.success(t`Stock movement recorded`);
    return movement;
  } catch (error) {
    console.error("Failed to create stock movement:", error);
    message.error(t`Failed to record stock movement`);
    return null;
  }
});

export const deleteStockMovementAtom = atom(null, async (get, set, movementId: string) => {
  try {
    const movement = get(stockMovementsAtom).find((m: any) => m.id === movementId);
    const success = await DeleteStockMovement(movementId);
    if (success) {
      set(
        stockMovementsAtom,
        get(stockMovementsAtom).filter((m: any) => m.id !== movementId),
      );

      // Reverse the movement's effect on the product
      if (movement) {
        const products: any[] = get(productsAtom);
        set(
          productsAtom,
          products.map((p: any) =>
            p.id === movement.productId
              ? { ...p, stockQuantity: p.stockQuantity - movement.quantity }
              : p,
          ),
        );
      }

      message.success(t`Stock movement deleted`);
    } else {
      message.error(t`Failed to delete stock movement`);
    }
  } catch (error) {
    console.error("Failed to delete stock movement:", error);
    message.error(t`Failed to delete stock movement`);
  }
});
