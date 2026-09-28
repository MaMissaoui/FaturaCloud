import { describe, expect, it, vi } from "vitest";
import { createStore } from "jotai";

vi.mock("src/api", () => ({ Logout: vi.fn() }));

import { Logout, type CurrentUser } from "src/api";
import { currentUserAtom } from "src/atoms/auth";
import {
  organizationIdAtom,
  organizationsAtom,
  organizationsLoadedAtom,
} from "src/atoms/organization";
import type { Organization } from "src/types/models";
import { logoutAtom } from "./session";

const user = (id: string): CurrentUser => ({
  id,
  email: `${id}@example.test`,
  displayName: id,
  role: "user",
  isPlatformAdmin: 0,
  isActive: 1,
  authProvider: "local",
});

const signedIn = (id: string) => {
  const store = createStore();
  store.set(currentUserAtom, user(id));
  store.set(organizationsAtom, [{ id: "org-1" } as Organization]);
  store.set(organizationsLoadedAtom, true);
  store.set(organizationIdAtom, "org-1");
  return store;
};

// Resolves the mocked Logout() only when the test says so, to hold the
// request "in flight".
const heldLogout = () => {
  let finish = () => {};
  vi.mocked(Logout).mockReturnValue(
    new Promise((resolve) => {
      finish = () => resolve({ message: "ok" });
    }),
  );
  return () => finish();
};

describe("logoutAtom", () => {
  it("clears the signed-out user's state once the server call returns", async () => {
    const store = signedIn("alice");
    const finish = heldLogout();

    const done = store.set(logoutAtom);
    finish();
    await done;

    expect(store.get(currentUserAtom)).toBeNull();
    expect(store.get(organizationsAtom)).toEqual([]);
    expect(store.get(organizationsLoadedAtom)).toBe(false);
    expect(store.get(organizationIdAtom)).toBeNull();
  });

  it("clears the state even when the server call fails", async () => {
    const store = signedIn("alice");
    vi.mocked(Logout).mockRejectedValue(new Error("unauthorized"));
    vi.spyOn(console, "error").mockImplementation(() => {});

    await store.set(logoutAtom);

    expect(store.get(currentUserAtom)).toBeNull();
    expect(store.get(organizationIdAtom)).toBeNull();
  });

  it("leaves a user who signed in while the logout was in flight alone", async () => {
    const store = signedIn("alice");
    const finish = heldLogout();

    const done = store.set(logoutAtom);
    // bob signs in before alice's logout request returns
    store.set(currentUserAtom, user("bob"));
    store.set(organizationsAtom, [{ id: "org-2" } as Organization]);
    store.set(organizationsLoadedAtom, true);
    store.set(organizationIdAtom, "org-2");
    finish();
    await done;

    expect(store.get(currentUserAtom)?.id).toBe("bob");
    expect(store.get(organizationsLoadedAtom)).toBe(true);
    expect(store.get(organizationIdAtom)).toBe("org-2");
  });
});
