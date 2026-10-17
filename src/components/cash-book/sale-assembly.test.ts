import Decimal from "decimal.js";
import { describe, expect, it } from "vitest";
import { assembleSale, netCentsFor, type SaleLineInput, type SaleTaxRate } from "./sale-assembly";
import { unitsToCents } from "src/utils/currency";

// serverTotals mirrors db/invoice_totals.go's validateInvoiceTotals for the
// payload the Cash Book sends: per tax-rate group, the exact sum of
// quantity × net unit price (cents), the tax on it rounded once, half up; the
// total is their sum. A sale whose totals differ from this is refused (409).
function serverTotals(
  lines: Array<{ quantity: number; unitPrice: number; taxRate: string | null }>,
  taxRates: SaleTaxRate[],
) {
  const groups = new Map<string, Decimal>();
  for (const line of lines) {
    const key = line.taxRate ?? "";
    groups.set(
      key,
      (groups.get(key) ?? new Decimal(0)).plus(new Decimal(line.quantity).times(line.unitPrice)),
    );
  }
  let sub = new Decimal(0);
  let tax = new Decimal(0);
  for (const [key, subtotal] of groups) {
    const pct = taxRates.find((r) => r.id === key)?.percentage ?? 0;
    sub = sub.plus(subtotal);
    tax = tax.plus(subtotal.times(pct).div(100).toDecimalPlaces(0, Decimal.ROUND_HALF_UP));
  }
  return {
    subTotal: sub.toDecimalPlaces(0, Decimal.ROUND_HALF_UP).toNumber(),
    taxTotal: tax.toNumber(),
    total: sub.plus(tax).toDecimalPlaces(0, Decimal.ROUND_HALF_UP).toNumber(),
  };
}

const rates: SaleTaxRate[] = [
  { id: "vat19", percentage: 19, isDefault: 1 },
  { id: "vat7", percentage: 7 },
  { id: "zero", percentage: 0 },
];

const cases: Array<{
  name: string;
  lines: SaleLineInput[];
  want: { subTotal: number; taxTotal: number; total: number };
}> = [
  {
    // 3.99 / 1.19 = 3.3529… → 3.35 net; 3.35 × 19% = 0.6365 → 0.64.
    name: "a half-cent VAT rounds up once",
    lines: [{ quantity: 1, unitPrice: 3.99, taxRate: "vat19" }],
    want: { subTotal: 335, taxTotal: 64, total: 399 },
  },
  {
    // 10.00 / 1.19 → 8.40 net; ×3 = 25.20, tax 4.788 → 4.79: 29.99, not 30.00 —
    // the net is rounded once per unit, as the server stores it.
    name: "quantity amplifies the rounded net",
    lines: [{ quantity: 3, unitPrice: 10, taxRate: "vat19" }],
    want: { subTotal: 2520, taxTotal: 479, total: 2999 },
  },
  {
    name: "two rates are taxed per group",
    lines: [
      { quantity: 1, unitPrice: 119, taxRate: "vat19" },
      { quantity: 2, unitPrice: 10.7, taxRate: "vat7" },
      { quantity: 1, unitPrice: 59.5, taxRate: "vat19" },
    ],
    want: { subTotal: 17000, taxTotal: 2990, total: 19990 },
  },
  {
    name: "a free line adds nothing",
    lines: [
      { quantity: 1, unitPrice: 23.8, taxRate: "vat19" },
      { quantity: 1, unitPrice: 0, taxRate: "vat19", description: "gift" },
    ],
    want: { subTotal: 2000, taxTotal: 380, total: 2380 },
  },
  {
    name: "a line with no rate uses the default",
    lines: [{ quantity: 2, unitPrice: 11.9 }],
    want: { subTotal: 2000, taxTotal: 380, total: 2380 },
  },
  {
    name: "a zero rate is untaxed",
    lines: [{ quantity: 4, unitPrice: 2.5, taxRate: "zero" }],
    want: { subTotal: 1000, taxTotal: 0, total: 1000 },
  },
  {
    // 1.5 × 6.72 (7.99 / 1.19 → 6.714… → 6.71): 10.065 → per-group tax on the
    // exact subtotal, 1.91235 → 1.91.
    name: "a fractional quantity",
    lines: [{ quantity: 1.5, unitPrice: 7.99, taxRate: "vat19" }],
    want: { subTotal: 1007, taxTotal: 191, total: 1198 },
  },
];

describe("assembleSale (F189)", () => {
  for (const tc of cases) {
    it(tc.name, () => {
      const sale = assembleSale(tc.lines, rates);
      const got = {
        subTotal: unitsToCents(sale.subTotal),
        taxTotal: unitsToCents(sale.taxTotal),
        total: unitsToCents(sale.total),
      };
      // Agrees with the server's own computation over the payload it sends…
      expect(got).toEqual(serverTotals(sale.lines, rates));
      // …and with the hand-checked figures.
      expect(got).toEqual(tc.want);
      expect(got.total).toBe(got.subTotal + got.taxTotal);
    });
  }

  it("sends the effective rate and the rounded net price on each line", () => {
    const sale = assembleSale([{ quantity: 2, unitPrice: 11.9, productId: "p1" }], rates);
    expect(sale.lines).toEqual([
      { description: null, quantity: 2, unitPrice: 1000, taxRate: "vat19", productId: "p1" },
    ]);
  });

  it("converts gross to net once, to the cent", () => {
    expect(netCentsFor({ unitPrice: 3.99, taxRate: "vat19" }, rates)).toBe(335);
    expect(netCentsFor({ unitPrice: "12,5" }, rates)).toBe(0); // the form always holds numbers
    expect(netCentsFor({ unitPrice: 12.5, taxRate: "unknown" }, rates)).toBe(1250);
  });
});
