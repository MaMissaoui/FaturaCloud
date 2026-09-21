import { describe, expect, it } from "vitest";
import { searchClients } from "./client-search";

type C = Record<string, any>;

const c = (over: Partial<C>): C => ({ id: over.name ?? "x", name: "x", ...over });

// Same-named customers with different identifying fields — the case the
// ranking has to disambiguate at the counter.
const amiraA = c({
  id: "a",
  name: "Amira Sassi",
  phone: "+216 93 568 998",
  identity_number: "222",
});
const amiraB = c({
  id: "b",
  name: "Amira Sassi",
  phone: "+216 77 237 164",
  identity_number: "111",
});
const anis = c({ id: "c", name: "Anis Sassi", phone: "+216 32 605 458" });
const amine = c({ id: "d", name: "Amine Sassi", phone: "+216 88 739 836" });
const other = c({ id: "e", name: "Handwerk AG", code: "HW-1", phone: "+49 802 929229" });

const all = [amiraA, amiraB, anis, amine, other];

describe("searchClients", () => {
  it("returns nothing for a blank query", () => {
    expect(searchClients(all, "", new Map())).toEqual([]);
    expect(searchClients(all, "   ", new Map())).toEqual([]);
  });

  it("filters by every searchable field, including customer code and IBAN", () => {
    // All four match by name at the same rank (none is a prefix), so they come
    // back alphabetical: Amine, Amira, Amira, Anis.
    expect(searchClients(all, "sassi", new Map()).map((r) => r.id)).toEqual(["d", "a", "b", "c"]);
    expect(searchClients(all, "HW-1", new Map()).map((r) => r.id)).toEqual(["e"]);
    expect(searchClients(all, "802 929", new Map()).map((r) => r.id)).toEqual(["e"]);
    expect(searchClients(all, "nobody", new Map())).toEqual([]);
  });

  it("ranks an exact mobile number above a name that merely contains it", () => {
    const rows = searchClients(all, "+216 32 605 458", new Map());
    expect(rows[0].id).toBe("c");
  });

  it("puts the debtor first within a name tier", () => {
    const outstanding = new Map<string, number>([
      ["a", 0],
      ["b", 500_00],
    ]);
    const rows = searchClients([amiraA, amiraB], "amira", outstanding);
    expect(rows.map((r) => r.id)).toEqual(["b", "a"]);
  });

  it("ranks a name prefix above a mid-string match, and both above a generic substring", () => {
    const rows = searchClients(
      [c({ id: "mid", name: "Chez Sassi" }), c({ id: "prefix", name: "Sassi Frères" })],
      "sassi",
      new Map(),
    );
    expect(rows.map((r) => r.id)).toEqual(["prefix", "mid"]);
  });

  it("is deterministic on full ties", () => {
    const rows = searchClients(
      [c({ id: "z", name: "Same Name" }), c({ id: "a", name: "Same Name" })],
      "same name",
      new Map(),
    );
    expect(rows.map((r) => r.id)).toEqual(["a", "z"]);
  });
});
