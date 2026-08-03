import { t } from "@lingui/core/macro";

export const UNIT_OPTIONS = [
  "hour",
  "day",
  "week",
  "month",
  "piece",
  "kg",
  "g",
  "lb",
  "oz",
  "l",
  "ml",
  "m",
  "km",
];

// unitLabel must be called during render (not hoisted to module scope) so the
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
