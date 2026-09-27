import { t } from "@lingui/core/macro";

import type { InventoryValuationMode } from "src/api";

// Mirrors the server's normalizeInventoryValuation (db/inventory_valuation.go):
// exactly "quantity_only" selects it, anything else — null, "", "perpetual" or
// an unrecognized value — is perpetual.
export const normalizeInventoryValuation = (value?: string | null): InventoryValuationMode =>
  value === "quantity_only" ? "quantity_only" : "perpetual";

export const inventoryValuationOptions = (): { value: InventoryValuationMode; label: string }[] => [
  { value: "perpetual", label: t`Valued (post stock value to the GL)` },
  { value: "quantity_only", label: t`Quantities only (no unit cost needed)` },
];
