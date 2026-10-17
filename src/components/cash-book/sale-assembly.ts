import {
  addDecimal,
  calculateTax,
  centsToUnits,
  multiplyDecimal,
  netFromGross,
  unitsToCents,
} from "src/utils/currency";

// The Cash Book's sale assembly, as pure functions (audit F189): counter
// prices are entered GROSS (tax-inclusive) — the opposite of every other
// document's unitPrice, which is net with tax added on top — and the payload
// must agree with db/invoice_totals.go's validateInvoiceTotals to the cent,
// or the counter gets a 409. sale-assembly.test.ts holds the rounding cases.

export type SaleTaxRate = { id: string; percentage: number; isDefault?: number | boolean | null };

// A sale line as the form holds it: quantity and gross price in units.
export type SaleLineInput = {
  quantity?: unknown;
  unitPrice?: unknown;
  taxRate?: string | null;
  productId?: string | null;
  description?: string | null;
};

export type SaleTaxGroup = { taxRate: SaleTaxRate | undefined; subtotal: number; tax: number };

const toNumber = (value: unknown) => {
  const n = Number(value);
  return Number.isFinite(n) ? n : 0;
};

export const defaultTaxRateIdOf = (taxRates: SaleTaxRate[]) =>
  taxRates.find((r) => r.isDefault === 1 || r.isDefault === true)?.id;

// The tax rate a line uses: its own (from a picked product or an explicit
// choice) or, when it has none, the organization's default — so a line added
// before tax rates loaded, or typed by hand, is never silently tax-free.
export const effectiveTaxRateId = (item: SaleLineInput | undefined, defaultTaxRateId?: string) =>
  item?.taxRate || defaultTaxRateId;

// netCentsFor is the single source of truth for the net price behind a
// gross-priced line: it rounds to cents once, and every downstream number
// (the totals and the payload) is derived from that same integer rather
// than from the unrounded gross/(1+rate) value — the two can disagree by a
// cent once quantity amplifies the sub-cent gap.
export const netCentsFor = (
  item: SaleLineInput | undefined,
  taxRates: SaleTaxRate[],
  defaultTaxRateId?: string,
) => {
  const rate = taxRates.find((r) => r.id === effectiveTaxRateId(item, defaultTaxRateId));
  return unitsToCents(netFromGross(toNumber(item?.unitPrice), rate?.percentage ?? 0));
};

// assembleSale groups the lines by tax rate and totals them the way the
// server does: each group's net subtotal summed exactly, its tax rounded
// once (half up), the total their sum. Amounts are in units; the lines are
// the payload's, with net unit prices in cents.
export function assembleSale(lineItems: SaleLineInput[] | undefined, taxRates: SaleTaxRate[]) {
  const defaultTaxRateId = defaultTaxRateIdOf(taxRates);
  const groups = new Map<string, SaleTaxGroup>();
  const lines = (lineItems ?? []).map((item) => {
    const rateId = effectiveTaxRateId(item, defaultTaxRateId);
    const unitPrice = netCentsFor(item, taxRates, defaultTaxRateId);
    const quantity = toNumber(item?.quantity);
    const key = rateId || "";
    const group = groups.get(key) ?? {
      taxRate: taxRates.find((r) => r.id === rateId),
      subtotal: 0,
      tax: 0,
    };
    group.subtotal = addDecimal(group.subtotal, multiplyDecimal(quantity, centsToUnits(unitPrice)));
    groups.set(key, group);
    return {
      description: item?.description || null,
      quantity: item?.quantity as number,
      unitPrice,
      taxRate: rateId || null,
      productId: item?.productId || null,
    };
  });
  const taxGroups = [...groups.values()];
  for (const g of taxGroups) {
    g.tax = g.taxRate?.percentage ? calculateTax(g.subtotal, g.taxRate.percentage) : 0;
  }
  const subTotal = taxGroups.reduce((n, g) => addDecimal(n, g.subtotal), 0);
  const taxTotal = taxGroups.reduce((n, g) => addDecimal(n, g.tax), 0);
  const total = addDecimal(subTotal, taxTotal);
  return { lines, taxGroups, subTotal, taxTotal, total };
}
