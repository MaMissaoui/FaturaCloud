import { atom } from "jotai";

import { Logout } from "src/api";
import { currentUserAtom } from "src/atoms/auth";
import {
  organizationIdAtom,
  organizationsAtom,
  organizationsLoadedAtom,
} from "src/atoms/organization";

// logoutAtom ends the session and drops every piece of per-user state, so the
// next user to log in in this browser starts clean (issue #415). That
// includes the remembered organizationId: kept, it made the next user's first
// render fetch an organization they may not belong to (a 404 toast) before
// the index route got to re-validate it. A user logging back in lands on
// their first organization.
export const logoutAtom = atom(null, async (_get, set) => {
  try {
    await Logout();
  } catch (error) {
    // The cookie may already be gone (expired session) — still clear the
    // client state so the login page isn't followed by the old user's data.
    console.error("Logout failed:", error);
  }
  set(currentUserAtom, null);
  set(organizationsAtom, []);
  set(organizationsLoadedAtom, false);
  set(organizationIdAtom, null);
});
