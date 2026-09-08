import { atom } from "jotai";
import type { CurrentUser } from "src/api";

export const currentUserAtom = atom<CurrentUser | null>(null);

// The session lives in an httpOnly cookie JavaScript can't read, so being
// authenticated is simply "we successfully loaded the current user" (GetMe
// succeeded, which required a valid cookie). A 401 anywhere redirects to login.
export const isAuthenticatedAtom = atom((get) => {
  return get(currentUserAtom) !== null;
});

// isPlatformAdminAtom gates the handful of genuinely global actions (user
// management, backups, DB restore, countries) that have no natural per-org
// owner. It replaces isAdminAtom, which read the legacy global users.role
// flag — organization-scoped admin actions (org delete/reset, fiscal-year
// close, GL exports) now go through isOrgAdminAtom instead, since a platform
// admin isn't automatically an admin of every organization.
export const isPlatformAdminAtom = atom((get) => {
  const user = get(currentUserAtom);
  return !!user?.isPlatformAdmin;
});
