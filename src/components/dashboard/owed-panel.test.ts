import { describe, expect, it } from "vitest";
import { bucketColumns } from "./owed-panel";

describe("bucketColumns (F184)", () => {
  it("keeps every card on one row when they fit", () => {
    expect(bucketColumns(5, 1000)).toBe(5);
    expect(bucketColumns(3, 500)).toBe(3);
  });
  it("spreads them evenly instead of leaving one alone", () => {
    expect(bucketColumns(5, 744)).toBe(3); // fits 4 → 3 + 2, not 4 + 1
    expect(bucketColumns(5, 480)).toBe(3); // fits 3 → 3 + 2
    expect(bucketColumns(4, 480)).toBe(2); // fits 3 → 2 + 2, not 3 + 1
    expect(bucketColumns(5, 320)).toBe(2); // fits 2 → 2 + 2 + 1 at best
  });
  it("never goes below one column", () => {
    expect(bucketColumns(5, 50)).toBe(1);
    expect(bucketColumns(0, 800)).toBe(1);
  });
});
