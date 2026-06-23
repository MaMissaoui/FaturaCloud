import { atom } from "jotai";
import { getToken } from "src/api/client";
import type { CurrentUser } from "src/api";

export const currentUserAtom = atom<CurrentUser | null>(null);

export const isAuthenticatedAtom = atom((get) => {
  const user = get(currentUserAtom);
  return user !== null && getToken() !== null;
});

export const isAdminAtom = atom((get) => {
  const user = get(currentUserAtom);
  return user?.role === "admin";
});
