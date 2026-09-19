import { t } from "@lingui/core/macro";

// BT-118 VAT category code (UNTDID 5305 subset EN 16931 accepts). Must stay
// in sync with taxRateCategoryCodes in db/tax_rate.go. Shared between the tax
// rate form (src/components/tax-rates/form.tsx) and the Tax Summary report
// (src/routes/reporting/tax-summary.tsx) so the two never drift apart.
export const TAX_RATE_CATEGORY_CODES = ["S", "Z", "E", "AE", "K", "G", "O", "L", "M"] as const;

export type TaxRateCategoryCode = (typeof TAX_RATE_CATEGORY_CODES)[number];

// Categories other than standard rate normally need a BT-120 exemption
// reason so the exported XRechnung line isn't rejected as invalid.
export const CATEGORIES_REQUIRING_EXEMPTION_REASON = new Set([
  "Z",
  "E",
  "AE",
  "K",
  "G",
  "O",
  "L",
  "M",
]);

export const useTaxRateCategoryLabels = (): Record<TaxRateCategoryCode, string> => ({
  S: t`Standard rate`,
  Z: t`Zero rated goods`,
  E: t`Exempt from tax`,
  AE: t`VAT reverse charge`,
  K: t`Intra-community supply (EEA)`,
  G: t`Free export item, tax not charged`,
  O: t`Outside scope of tax`,
  L: t`Canary Islands general indirect tax`,
  M: t`Tax for production, services and importation in Ceuta and Melilla`,
});
