import { t } from "@lingui/core/macro";

// unitLabel formats a product's legacy free-text unit column for display
// (Inventory, Products list, BOM component display) — the product form
// itself now sources its "Base unit of measure" options from the
// units_of_measure list (src/atoms/unit-of-measure.ts) instead of the fixed
// set this used to offer, but the org's seeded defaults reuse the same
// names, so this still translates the common cases correctly.
//
// Must be called during render (not hoisted to module scope) so the
// returned label follows the currently-active locale — a module-scope `t`
// result would freeze at import-time locale and go stale on language switch.
// Metric/imperial symbols (kg, g, lb, oz, l, ml, m, km) are identical across
// en/de/fr, so only the spelled-out words need translation; anything else
// (a legacy or custom-typed unit) falls through to the stored value as-is.
export function unitLabel(unit: string | null | undefined): string {
  switch (unit) {
    case "hour":
      return t`hour`;
    case "day":
      return t`day`;
    case "week":
      return t`week`;
    case "month":
      return t`month`;
    case "piece":
      return t`piece`;
    default:
      return unit ?? "";
  }
}
