import { t } from "@lingui/core/macro";
import type { Product } from "src/types/models";

// A product as a recipe-picker option. An archived product stays in the list
// so a saved recipe line keeps its label, but it can't be picked again
// (audit F167).
export const productOption = (p: Product) => {
  const label = p.sku ? `${p.name} (${p.sku})` : p.name;
  return {
    value: p.id,
    label: p.isArchived ? t`${label} — archived` : label,
    disabled: !!p.isArchived,
  };
};
