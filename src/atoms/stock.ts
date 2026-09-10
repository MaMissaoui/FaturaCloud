import { atom } from "jotai";
import type { StockMovement } from "src/types/models";
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

type CreateStockMovementRequest = Partial<StockMovement> & { serialNumbers?: string[] };

export const createStockMovementAtom = atom(
  null,
  async (get, set, req: CreateStockMovementRequest) => {
    try {
      const { movements, product } = await CreateStockMovement({
        ...req,
        id: nanoid(),
        organizationId: get(organizationIdAtom)!,
      });

      // Replace the product wholesale from the server's own refreshed state,
      // rather than patching stockQuantity by a single movement's quantity —
      // a serialized request can post many movement rows at once (one per
      // unit), so there's no single delta to apply locally.
      const products = get(productsAtom);
      set(
        productsAtom,
        products.map((p) => (p.id === product.id ? product : p)),
      );

      message.success(t`Stock movement recorded`);
      return movements;
    } catch (error) {
      console.error("Failed to create stock movement:", error);
      message.error(error instanceof Error ? error.message : t`Failed to record stock movement`);
      return null;
    }
  },
);

export const deleteStockMovementAtom = atom(
  null,
  async (get, set, movement: { id: string; productId: string; quantity: number }) => {
    try {
      const success = await DeleteStockMovement(movement.id);
      if (success) {
        // Reverse the movement's effect on the product
        const products = get(productsAtom);
        set(
          productsAtom,
          products.map((p) =>
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
