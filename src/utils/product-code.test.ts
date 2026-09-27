import { describe, expect, it } from "vitest";

import { normalizeWords, proposeProductCode } from "src/utils/product-code";

const catalog = [
  { name: "Machine à laver HGE 9kg", sku: "MAL-003" },
  { name: "Machine à laver Samsung 18kg", sku: "MAL-001" },
  { name: "Réfrigérateur Condor 360L blanc", sku: "REF-007" },
  { name: 'TV 55" TCL', sku: "TV-010" },
  { name: "Installation", sku: "SVC-INSTALL" },
];

describe("normalizeWords", () => {
  it("drops accents and punctuation and uppercases", () => {
    expect(normalizeWords("Réfrigérateur combiné, 370L")).toEqual([
      "REFRIGERATEUR",
      "COMBINE",
      "370L",
    ]);
  });
});

describe("proposeProductCode", () => {
  it("continues the series of the product with the most leading words in common", () => {
    expect(proposeProductCode("Machine à laver Condor 8kg", catalog)).toBe("MAL-004");
    expect(proposeProductCode("réfrigérateur Biolux 420L", catalog)).toBe("REF-008");
  });

  it("treats a short first word like TV as a series", () => {
    expect(proposeProductCode("TV 43 Hisense", catalog)).toBe("TV-011");
  });

  it("starts a new series from the first word when nothing matches", () => {
    expect(proposeProductCode("Congélateur Hisense", catalog)).toBe("CON-001");
    expect(proposeProductCode("Sèche-cheveux Galaxy", [])).toBe("SEC-001");
  });

  it("never collides with an existing code of that prefix", () => {
    const withCon = [...catalog, { name: "Something else", sku: "CON-001" }];
    expect(proposeProductCode("Congélateur Hisense", withCon)).toBe("CON-002");
  });

  it("keeps a wider series' own padding", () => {
    expect(proposeProductCode("Pompe", [{ name: "Pompe A", sku: "POM-0099" }])).toBe("POM-0100");
  });

  it("ignores codes that aren't PREFIX-NNN and falls back for an empty name", () => {
    expect(proposeProductCode("Installation murale", catalog)).toBe("INS-001");
    expect(proposeProductCode("", catalog)).toBe("PRD-001");
  });
});
