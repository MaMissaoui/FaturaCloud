import { atom } from "jotai";
import { message } from "src/utils/message";
import { nanoid } from "nanoid";
import { t } from "@lingui/core/macro";
import { CreateStockMovement, DeleteStockMovement } from "src/api";
import { organizationIdAtom } from "./organization";
import { productsAtom } from "./product";

// There's no shared stockMovementsAtom (unlike productsAtom) — the Inventory
// page is the only consumer of the movements list, and it now fetches its
// own paginated/sorted page directly rather than through a full-list atom.
// These two write-atoms stay because other pages (Inventory's summary grid,
// every line-item picker) read productsAtom.stockQuantity and expect it to
// reflect a movement immediately, without waiting for a refetch.

export const createStockMovementAtom = atom(null, async (get, set, req: any) => {
  try {
    const movement = await CreateStockMovement({
      ...req,
      id: nanoid(),
      organizationId: get(organizationIdAtom),
    });

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

export const deleteStockMovementAtom = atom(
  null,
  async (get, set, movement: { id: string; productId: string; quantity: number }) => {
    try {
      const success = await DeleteStockMovement(movement.id);
      if (success) {
        // Reverse the movement's effect on the product
        const products: any[] = get(productsAtom);
        set(
          productsAtom,
          products.map((p: any) =>
            p.id === movement.productId
              ? { ...p, stockQuantity: p.stockQuantity - movement.quantity }
              : p,
          ),
        );

        message.success(t`Stock movement deleted`);
      } else {
        message.error(t`Failed to delete stock movement`);
      }
      return success;
    } catch (error) {
      console.error("Failed to delete stock movement:", error);
      message.error(t`Failed to delete stock movement`);
      return false;
    }
  },
);
