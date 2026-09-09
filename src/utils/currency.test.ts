import { describe, expect, it } from "vitest";
import {
  addDecimal,
  calculateTax,
  centsToUnits,
  divideDecimal,
  formatCents,
  multiplyDecimal,
  subtractDecimal,
  unitsToCents,
} from "src/utils/currency";

describe("centsToUnits / unitsToCents", () => {
  it("round-trips a whole-cent amount", () => {
    expect(centsToUnits(333)).toBe(3.33);
    expect(unitsToCents(3.33)).toBe(333);
  });

  it("handles zero and negative amounts", () => {
    expect(centsToUnits(0)).toBe(0);
    expect(centsToUnits(-150)).toBe(-1.5);
    expect(unitsToCents(-1.5)).toBe(-150);
  });
});

describe("multiplyDecimal / divideDecimal / addDecimal / subtractDecimal", () => {
  // The whole reason these wrap decimal.js instead of native `*`/`/`/`+`/`-`
  // is to avoid float64 artifacts like 0.1 + 0.2 !== 0.3 — pin that directly.
  it("avoids float64 rounding artifacts", () => {
    expect(addDecimal(0.1, 0.2)).toBe(0.3);
    expect(subtractDecimal(0.3, 0.1)).toBe(0.2);
    expect(multiplyDecimal(1.1, 3)).toBe(3.3);
    expect(divideDecimal(1, 3).toFixed(4)).toBe("0.3333");
  });
});

describe("calculateTax", () => {
  // Pinned to the exact case CLAUDE.md and db/db_test.go's
  // TestCreateInvoiceAcceptsRoundingBoundary cross-check against: a 3.33
  // unit price at 19.5% tax. True tax is 0.64935, which rounds up to 0.65 —
  // the Go side (db/invoice_totals.go's ratToCents) must land on exactly
  // this value too, or the server would start rejecting invoices the
  // frontend just computed. Verified end to end in that Go test: subtotal
  // 333 cents, tax 65 cents, total 398 cents for a single 3.33 line.
  it("matches the Go-side rounding boundary (3.33 @ 19.5%)", () => {
    const tax = calculateTax(3.33, 19.5);
    expect(tax).toBe(0.65);
    expect(unitsToCents(tax)).toBe(65);
  });

  it("always rounds to 2 decimal places regardless of the input's precision", () => {
    expect(calculateTax(10, 7.5)).toBe(0.75);
    expect(calculateTax(1, 33.333)).toBe(0.33);
  });

  it("returns 0 for a 0% rate", () => {
    expect(calculateTax(100, 0)).toBe(0);
  });
});

describe("formatCents", () => {
  // Only the fallback guard is asserted, not exact formatted output —
  // Intl.NumberFormat's output depends on the Node build's ICU data and
  // Vitest's default locale, neither of which is a stable contract here.
  it("falls back instead of throwing on an invalid currency code", () => {
    expect(() => formatCents(1000, "", "en-US")).not.toThrow();
    expect(() => formatCents(1000, "NOT_A_CURRENCY", "en-US")).not.toThrow();
  });

  it("formats a valid currency without throwing", () => {
    expect(() => formatCents(1000, "USD", "en-US")).not.toThrow();
  });
});
