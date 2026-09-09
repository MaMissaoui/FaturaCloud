import { describe, expect, it } from "vitest";
import { generateInvoiceNumber, validateInvoiceFormat } from "src/utils/invoice";

describe("validateInvoiceFormat", () => {
  it("rejects an empty or blank format", () => {
    expect(validateInvoiceFormat("").isValid).toBe(false);
    expect(validateInvoiceFormat("   ").isValid).toBe(false);
  });

  it("accepts a format using only known variables", () => {
    const result = validateInvoiceFormat("INV-{year}-{number}");
    expect(result.isValid).toBe(true);
    expect(result.error).toBeUndefined();
  });

  it("accepts a format with no variables at all", () => {
    expect(validateInvoiceFormat("INVOICE").isValid).toBe(true);
  });

  it("rejects an unknown variable and names it in the error", () => {
    const result = validateInvoiceFormat("INV-{bogus}");
    expect(result.isValid).toBe(false);
    expect(result.error).toContain("{bogus}");
  });
});

describe("generateInvoiceNumber", () => {
  const date = new Date(2026, 8, 9); // 2026-09-09 (month is 0-indexed)

  it("substitutes number and date variables", () => {
    expect(generateInvoiceNumber("INV-{year}-{number}", 42, date)).toBe("INV-2026-42");
    expect(generateInvoiceNumber("{y}{month}{day}-{number}", 1, date)).toBe("260909-1");
  });

  it("substitutes the client code variable", () => {
    expect(generateInvoiceNumber("{clientCode}-{number}", 5, date, "ACME")).toBe("ACME-5");
  });

  it("returns an empty string for an empty format", () => {
    expect(generateInvoiceNumber("", 1, date)).toBe("");
  });

  // Current, documented behavior rather than necessarily desired behavior:
  // each variable is substituted via String.replace with a string needle,
  // which only replaces the first occurrence. A format repeating the same
  // variable leaves every later occurrence as the literal placeholder text.
  it("only replaces the first occurrence of a repeated variable", () => {
    expect(generateInvoiceNumber("{year}-{number}-{year}", 1, date)).toBe("2026-1-{year}");
  });

  // Same class of documented-not-fixed behavior: a format that only uses
  // {clientCode} with no client code supplied collapses to an empty string,
  // since the substitution is "".
  it("substitutes an empty string when clientCode is omitted", () => {
    expect(generateInvoiceNumber("{clientCode}", 1, date)).toBe("");
  });
});
