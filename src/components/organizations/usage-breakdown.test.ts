import { describe, expect, it } from "vitest";
import type { OrganizationUsageCount } from "src/api";
import { usageBreakdown } from "src/components/organizations/usage-breakdown";

// Every count set to 1, so each field shows up exactly once when its group
// is selected.
const allCounts = (): OrganizationUsageCount => ({
  clients: 1,
  vendors: 1,
  invoices: 1,
  products: 1,
  orders: 1,
  deliveries: 1,
  taxRates: 1,
  purchaseOrders: 1,
  inboundDeliveries: 1,
  incomingInvoices: 1,
  stockMovements: 1,
  productionOrders: 1,
  imports: 1,
  cashMovements: 1,
  productSerialNumbers: 1,
  reconciliationGroups: 1,
  fiscalYears: 1,
  fiscalPeriods: 1,
  paymentTerms: 1,
  unitsOfMeasure: 1,
  documentTemplates: 1,
  documentTemplateSettings: 1,
  documentNumberSettings: 1,
  billOfMaterials: 1,
  billOfMaterialsVersions: 1,
  accounts: 1,
  journals: 1,
  journalEntries: 1,
  payments: 1,
});

const labels = (rows: [number, string][]) => rows.map(([, label]) => label);

// Pins the reset half of audit F125: the reset confirmation used to list
// only 11 of the counts GetOrganizationUsageCount returns, omitting
// payments, journal entries, cash movements, accounts, BOMs and more.
describe("usageBreakdown", () => {
  it("lists every count when deleting the organization", () => {
    expect(usageBreakdown(allCounts(), ["master", "transactional"])).toHaveLength(29);
  });

  it("splits counts the same way db/reset.go's table lists do", () => {
    const transactional = labels(usageBreakdown(allCounts(), ["transactional"]));
    const master = labels(usageBreakdown(allCounts(), ["master"]));
    expect(transactional).toHaveLength(14);
    expect(master).toHaveLength(15);
    expect(transactional).toEqual(
      expect.arrayContaining([
        "payment(s)",
        "journal entr(y/ies)",
        "cash movement(s)",
        "production order(s)",
        "import(s)",
        "serial number(s)",
        "reconciliation group(s)",
      ]),
    );
    expect(master).toEqual(
      expect.arrayContaining(["account(s)", "fiscal year(s)", "bill(s) of materials"]),
    );
  });

  it("drops zero counts", () => {
    const counts = { ...allCounts(), payments: 0 };
    expect(labels(usageBreakdown(counts, ["transactional"]))).not.toContain("payment(s)");
  });
});
