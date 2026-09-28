import { afterEach, describe, expect, it, vi } from "vitest";

import { ORGANIZATION_ID_STORAGE_KEY, get } from "./client";

// A 401 means the session is gone and the next person to sign in may be a
// different user, so the remembered organization is forgotten on the way to
// the login page (the logout counterpart is src/atoms/session.ts).
describe("request on 401", () => {
  const originalLocation = window.location;

  afterEach(() => {
    vi.unstubAllGlobals();
    Object.defineProperty(window, "location", { value: originalLocation, configurable: true });
    localStorage.clear();
  });

  const expireSessionAt = (pathname: string) => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(null, { status: 401 })));
    const location = { pathname, href: `http://localhost${pathname}` };
    Object.defineProperty(window, "location", { value: location, configurable: true });
    localStorage.setItem(ORGANIZATION_ID_STORAGE_KEY, JSON.stringify("org-1"));
    return location;
  };

  it("forgets the remembered organization and redirects to /login", async () => {
    const location = expireSessionAt("/invoices");

    await expect(get("/clients")).rejects.toThrow("Session expired");

    expect(localStorage.getItem(ORGANIZATION_ID_STORAGE_KEY)).toBeNull();
    expect(location.href).toBe("/login");
  });

  it("leaves it alone on the login page itself", async () => {
    const location = expireSessionAt("/login");

    await expect(get("/auth/me")).rejects.toThrow("Session expired");

    expect(localStorage.getItem(ORGANIZATION_ID_STORAGE_KEY)).toBe(JSON.stringify("org-1"));
    expect(location.href).toBe("http://localhost/login");
  });
});
