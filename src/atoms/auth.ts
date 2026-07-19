import { atom } from "jotai";
import type { CurrentUser } from "src/api";

export const currentUserAtom = atom<CurrentUser | null>(null);

// The session lives in an httpOnly cookie JavaScript can't read, so being
// authenticated is simply "we successfully loaded the current user" (GetMe
// succeeded, which required a valid cookie). A 401 anywhere redirects to login.
export const isAuthenticatedAtom = atom((get) => {
  return get(currentUserAtom) !== null;
});

export const isAdminAtom = atom((get) => {
  const user = get(currentUserAtom);
  return user?.role === "admin";
});
