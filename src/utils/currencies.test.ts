import { describe, expect, it } from "vitest";
import { formatMoneyUnits, formatOrgCents } from "src/utils/currencies";

// These pin the frontend half of the F121 ("Decimal places" means a true
// minimum, matching db/format_money.go's formatMoneyCents) and F123
// (grouping glyphs agree with the server's separator table) findings.
describe("formatMoneyUnits", () => {
  it("treats minimumFractionDigits as a minimum, not an exact count (F121)", () => {
    // Intl resolves maximumFractionDigits to max(min, currency default) and
    // strips trailing zeros above the minimum — the exact resolution the
    // server's formatter now mirrors.
    expect(formatMoneyUnits(12.5, "USD", "en-US", 0)).toBe("$12.5");
    expect(formatMoneyUnits(12, "USD", "en-US", 0)).toBe("$12");
    expect(formatMoneyUnits(12, "USD", "en-US", 1)).toBe("$12.0");
    expect(formatMoneyUnits(12.5, "USD", "en-US", 3)).toBe("$12.500");
    expect(formatMoneyUnits(12.5, "USD", "en-US", undefined)).toBe("$12.50");
  });

  it("normalizes NBSP and narrow-space grouping to a plain space (F123)", () => {
    // pt-PT/pl-PL/ru-RU group with U+00A0; fr-FR/fr-BE/fr-TN use U+202F.
    // Both must collapse to the same ASCII space the server emits.
    for (const locale of ["pt-PT", "ru-RU", "fr-FR"]) {
      const out = formatMoneyUnits(1234567.89, "EUR", locale);
      expect(out).not.toMatch(/[\u00A0\u202F\u2009]/);
      expect(out).toContain("1 234 567,89");
    }
  });
});

describe("formatOrgCents", () => {
  it("passes the org's decimal-places setting through as a minimum (F121)", () => {
    expect(formatOrgCents(1250, { currency: "USD", minimum_fraction_digits: 0 }, "en-US")).toBe(
      "$12.5",
    );
    expect(formatOrgCents(1200, { currency: "USD", minimum_fraction_digits: 0 }, "en-US")).toBe(
      "$12",
    );
    expect(formatOrgCents(1250, { currency: "USD" }, "en-US")).toBe("$12.50");
  });
});
