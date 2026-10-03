import { describe, expect, it } from "vitest";

import { percentageFromForm } from "./tax-rate";

describe("percentageFromForm", () => {
  it("keeps 0, a real rate", () => {
    expect(percentageFromForm(0)).toBe(0);
    expect(percentageFromForm("0")).toBe(0);
  });

  it("reads numbers and numeric text", () => {
    expect(percentageFromForm(19.5)).toBe(19.5);
    expect(percentageFromForm("7")).toBe(7);
  });

  it("leaves an empty field out", () => {
    expect(percentageFromForm(undefined)).toBeUndefined();
    expect(percentageFromForm(null)).toBeUndefined();
    expect(percentageFromForm("")).toBeUndefined();
  });
});
