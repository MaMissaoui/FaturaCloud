import { t } from "@lingui/core/macro";
import type { OrganizationUsageCount } from "src/api";

type UsageKey = keyof OrganizationUsageCount;
export type UsageGroup = "master" | "transactional";

// Which reset each count belongs to — mirrors db/reset.go's masterDataTables
// and transactionalDataTables. Typed as a full Record so adding a field to
// OrganizationUsageCount without classifying it here fails the type-check,
// instead of silently understating the delete/reset warning (audit F125).
const USAGE_GROUPS: Record<UsageKey, UsageGroup> = {
  clients: "master",
  vendors: "master",
  invoices: "transactional",
  orders: "transactional",
  deliveries: "transactional",
  products: "master",
  taxRates: "master",
  purchaseOrders: "transactional",
  inboundDeliveries: "transactional",
  incomingInvoices: "transactional",
  productionOrders: "transactional",
  imports: "transactional",
  stockMovements: "transactional",
  cashMovements: "transactional",
  productSerialNumbers: "transactional",
  payments: "transactional",
  journalEntries: "transactional",
  reconciliationGroups: "transactional",
  accounts: "master",
  journals: "master",
  fiscalYears: "master",
  fiscalPeriods: "master",
  paymentTerms: "master",
  unitsOfMeasure: "master",
  documentTemplates: "master",
  documentTemplateSettings: "master",
  documentNumberSettings: "master",
  billOfMaterials: "master",
  billOfMaterialsVersions: "master",
};

// A function rather than a constant so each label is translated in the
// active locale at render time. Key order is the display order.
const usageLabels = (): Record<UsageKey, string> => ({
  clients: t`client(s)`,
  vendors: t`vendor(s)`,
  invoices: t`invoice(s)`,
  orders: t`order(s)`,
  deliveries: t`delivery(ies)`,
  products: t`product(s)`,
  taxRates: t`tax rate(s)`,
  purchaseOrders: t`purchase order(s)`,
  inboundDeliveries: t`goods receipt(s)`,
  incomingInvoices: t`incoming invoice(s)`,
  productionOrders: t`production order(s)`,
  imports: t`import(s)`,
  stockMovements: t`stock movement(s)`,
  cashMovements: t`cash movement(s)`,
  productSerialNumbers: t`serial number(s)`,
  payments: t`payment(s)`,
  journalEntries: t`journal entr(y/ies)`,
  reconciliationGroups: t`reconciliation group(s)`,
  accounts: t`account(s)`,
  journals: t`journal(s)`,
  fiscalYears: t`fiscal year(s)`,
  fiscalPeriods: t`fiscal period(s)`,
  paymentTerms: t`payment term(s)`,
  unitsOfMeasure: t`unit(s) of measure`,
  documentTemplates: t`document template(s)`,
  documentTemplateSettings: t`template setting(s)`,
  documentNumberSettings: t`numbering setting(s)`,
  billOfMaterials: t`bill(s) of materials`,
  billOfMaterialsVersions: t`BOM version(s)`,
});

// The non-zero counts in `groups`, as [count, label] pairs for a
// confirmation dialog. Deleting an organization removes both groups; a reset
// removes transactional data, plus master data when that box is checked.
export const usageBreakdown = (
  counts: OrganizationUsageCount,
  groups: readonly UsageGroup[],
): [number, string][] =>
  Object.entries(usageLabels())
    .filter(([key]) => groups.includes(USAGE_GROUPS[key as UsageKey]))
    .map(([key, label]): [number, string] => [counts[key as UsageKey], label])
    .filter(([n]) => n > 0);
