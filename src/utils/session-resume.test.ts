import { beforeEach, describe, expect, it, vi } from "vitest";
import { rememberResume, safeResumePath, takeResume } from "./session-resume";

describe("session resume (F175)", () => {
  beforeEach(() => {
    sessionStorage.clear();
    vi.useRealTimers();
  });

  it("accepts only same-origin paths", () => {
    expect(safeResumePath("/invoices/abc?tab=1")).toBe("/invoices/abc?tab=1");
    for (const bad of [
      "//evil.example/x",
      "https://evil.example",
      "invoices",
      "/\\evil",
      "/login",
      "/login?next=/x",
      "/",
      42,
      null,
    ]) {
      expect(safeResumePath(bad)).toBeNull();
    }
  });

  it("returns the remembered page once", () => {
    rememberResume("/cash-book", "org_1");
    expect(takeResume()).toEqual({ path: "/cash-book", organizationId: "org_1" });
    expect(takeResume()).toBeNull();
  });

  it("ignores an unsafe or stale entry", () => {
    rememberResume("//evil.example", "org_1");
    expect(takeResume()).toBeNull();
    sessionStorage.setItem(
      "fc_resume",
      JSON.stringify({ path: "/x", organizationId: null, at: Date.now() - 2 * 60 * 60 * 1000 }),
    );
    expect(takeResume()).toBeNull();
  });
});
